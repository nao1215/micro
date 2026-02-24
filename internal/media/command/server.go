package command

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	// image/png と image/gif はデコード用に副作用インポートする。
	_ "image/gif"
	_ "image/png"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/nao1215/micro/pkg/event"
	"github.com/nao1215/micro/pkg/httpclient"
	"github.com/nao1215/micro/pkg/middleware"
)

// maxUploadSize はアップロード可能なファイルの最大サイズ（50MB）。
// テスト時に差し替え可能にするためvarとして宣言する。
var maxUploadSize int64 = 50 << 20

// thumbnailSize はサムネイル画像の幅・高さ（ピクセル）。
const thumbnailSize = 200

// Server はメディアコマンドサービスのHTTPサーバー。
type Server struct {
	// router はGinのHTTPルーター。
	router *gin.Engine
	// port はサーバーのリッスンポート。
	port string
	// eventClient はEvent StoreへのHTTPクライアント。
	eventClient *httpclient.Client
}

// NewServer は新しいメディアコマンドサーバーを生成する。
// ファイル保存ディレクトリの初期化も行う。
func NewServer(port string) (*Server, error) {
	if err := initStorage(); err != nil {
		return nil, fmt.Errorf("ストレージ初期化に失敗: %w", err)
	}

	eventstoreURL := os.Getenv("EVENTSTORE_URL")
	if eventstoreURL == "" {
		eventstoreURL = "http://localhost:8084"
	}

	router := gin.New()
	router.Use(middleware.Recovery())
	router.Use(gin.Logger())

	// マルチパートフォームの最大メモリを設定する。
	router.MaxMultipartMemory = maxUploadSize

	s := &Server{
		router:      router,
		port:        port,
		eventClient: httpclient.New(eventstoreURL),
	}
	s.setupRoutes()

	return s, nil
}

// Run はHTTPサーバーを起動する。
func (s *Server) Run() error {
	return s.router.Run(fmt.Sprintf(":%s", s.port))
}

// setupRoutes はAPIルーティングを設定する。
func (s *Server) setupRoutes() {
	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		jwtSecret = "dev-secret-key"
	}

	api := s.router.Group("/api/v1")
	api.Use(middleware.JWTAuth(jwtSecret))
	{
		media := api.Group("/media")
		{
			// メディアのアップロード（マルチパートフォーム）
			media.POST("", s.handleUpload())
			// メディアの削除
			media.DELETE("/:id", s.handleDelete())
			// サムネイル生成（内部API - Sagaから呼び出される）
			media.POST("/:id/process", s.handleProcess())
			// 補償アクション: アップロード済みメディアの無効化（内部API）
			media.POST("/:id/compensate", s.handleCompensate())
		}
	}

	// ヘルスチェック
	s.router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "media-command"})
	})
}

// appendEventRequest はEvent Storeへのイベント追記リクエスト。
type appendEventRequest struct {
	// AggregateID は対象エンティティの識別子。
	AggregateID string `json:"aggregate_id"`
	// AggregateType は対象エンティティの種類。
	AggregateType string `json:"aggregate_type"`
	// EventType はイベントの種類。
	EventType string `json:"event_type"`
	// Data はイベント固有のデータ（JSON形式）。
	Data json.RawMessage `json:"data"`
}

// emitEvent はEvent Storeにイベントを送信する。
// dataにはイベント固有のデータ構造体を渡す。JSON形式にシリアライズしてから送信する。
func (s *Server) emitEvent(c *gin.Context, aggregateID string, eventType event.Type, data any) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("イベントデータのシリアライズに失敗: %w", err)
	}

	req := appendEventRequest{
		AggregateID:   aggregateID,
		AggregateType: string(event.AggregateTypeMedia),
		EventType:     string(eventType),
		Data:          jsonData,
	}

	var resp map[string]any
	if err := s.eventClient.PostJSON(c.Request.Context(), "/api/v1/events", req, &resp); err != nil {
		return fmt.Errorf("Event Storeへのイベント送信に失敗: %w", err)
	}
	return nil
}

// uploadResponse はアップロード成功時のレスポンス。
type uploadResponse struct {
	// ID はアップロードされたメディアのID（UUID）。
	ID string `json:"id"`
	// Filename は元のファイル名。
	Filename string `json:"filename"`
	// ContentType はファイルのMIMEタイプ。
	ContentType string `json:"content_type"`
	// Size はファイルサイズ（バイト）。
	Size int64 `json:"size"`
	// StoragePath はファイルの保存パス。
	StoragePath string `json:"storage_path"`
}

// handleUpload はメディアファイルのアップロードを処理するハンドラを返す。
// マルチパートフォームからファイルを受け取り、ディスクに保存し、
// MediaUploadedイベントをEvent Storeに発行する。
func (s *Server) handleUpload() gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := middleware.GetUserID(c)
		if userID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "ユーザーIDが取得できません"})
			return
		}

		// マルチパートフォームからファイルを取得する。
		file, header, err := c.Request.FormFile("file")
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("ファイルの取得に失敗しました: %v", err)})
			return
		}
		defer file.Close()

		// ファイルサイズのバリデーション。
		if header.Size > maxUploadSize {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("ファイルサイズが上限を超えています（最大%dMB）", maxUploadSize/(1<<20))})
			return
		}

		// Content-Typeのバリデーション（image/* または video/* のみ許可）。
		contentType := header.Header.Get("Content-Type")
		if !isAllowedContentType(contentType) {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("許可されていないContent-Typeです: %s（image/*またはvideo/*のみ）", contentType)})
			return
		}

		// 保存先ディレクトリを作成する。
		mediaID := uuid.New().String()
		mediaDir := filepath.Join(mediaBaseDir, mediaID)
		if err := os.MkdirAll(mediaDir, 0o755); err != nil {
			log.Printf("メディアディレクトリの作成に失敗: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "ファイル保存先の作成に失敗しました"})
			return
		}

		// ファイルをディスクに保存する。
		filename := filepath.Base(header.Filename)
		storagePath := filepath.Join(mediaDir, filename)
		dst, err := os.Create(storagePath)
		if err != nil {
			log.Printf("ファイルの作成に失敗: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "ファイルの保存に失敗しました"})
			return
		}
		defer dst.Close()

		written, err := io.Copy(dst, file)
		if err != nil {
			log.Printf("ファイルの書き込みに失敗: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "ファイルの書き込みに失敗しました"})
			return
		}

		// MediaUploadedイベントをEvent Storeに発行する。
		aggregateID := fmt.Sprintf("media-%s", mediaID)
		eventData := event.MediaUploadedData{
			UserID:      userID,
			Filename:    filename,
			ContentType: contentType,
			Size:        written,
			StoragePath: storagePath,
		}

		if err := s.emitEvent(c, aggregateID, event.TypeMediaUploaded, eventData); err != nil {
			log.Printf("MediaUploadedイベントの送信に失敗: %v", err)
			// ファイルは保存済みだがイベント送信に失敗した場合、ファイルをクリーンアップする。
			if removeErr := os.RemoveAll(mediaDir); removeErr != nil {
				log.Printf("クリーンアップ失敗: %v", removeErr)
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "イベントの送信に失敗しました"})
			return
		}

		c.JSON(http.StatusCreated, uploadResponse{
			ID:          mediaID,
			Filename:    filename,
			ContentType: contentType,
			Size:        written,
			StoragePath: storagePath,
		})
	}
}

// handleDelete はメディアの削除を処理するハンドラを返す。
// MediaDeletedイベントをEvent Storeに発行する。
// 実際のファイル削除は行わず、イベントとして削除を記録する（論理削除）。
func (s *Server) handleDelete() gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := middleware.GetUserID(c)
		if userID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "ユーザーIDが取得できません"})
			return
		}

		mediaID := c.Param("id")
		if mediaID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "メディアIDが指定されていません"})
			return
		}

		// MediaDeletedイベントをEvent Storeに発行する。
		aggregateID := fmt.Sprintf("media-%s", mediaID)
		eventData := event.MediaDeletedData{
			UserID: userID,
		}

		if err := s.emitEvent(c, aggregateID, event.TypeMediaDeleted, eventData); err != nil {
			log.Printf("MediaDeletedイベントの送信に失敗: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "イベントの送信に失敗しました"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message":  "メディアを削除しました",
			"media_id": mediaID,
		})
	}
}

// processRequest はサムネイル生成リクエストのJSON構造。
type processRequest struct {
	// StoragePath は処理対象のメディアファイルの保存パス。
	StoragePath string `json:"storage_path" binding:"required"`
}

// handleProcess はサムネイル生成を処理するハンドラを返す。
// 画像ファイルの場合は200x200のサムネイルを生成し、
// MediaProcessedイベントまたはMediaProcessingFailedイベントをEvent Storeに発行する。
func (s *Server) handleProcess() gin.HandlerFunc {
	return func(c *gin.Context) {
		mediaID := c.Param("id")
		if mediaID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "メディアIDが指定されていません"})
			return
		}

		var req processRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("リクエストが不正です: %v", err)})
			return
		}

		aggregateID := fmt.Sprintf("media-%s", mediaID)

		// 元ファイルを開く。
		srcFile, err := os.Open(req.StoragePath)
		if err != nil {
			reason := fmt.Sprintf("元ファイルのオープンに失敗: %v", err)
			log.Printf("サムネイル生成エラー: %s", reason)
			s.emitProcessingFailed(c, aggregateID, reason)
			c.JSON(http.StatusInternalServerError, gin.H{"error": reason})
			return
		}
		defer srcFile.Close()

		// 画像をデコードする。
		srcImg, _, err := image.Decode(srcFile)
		if err != nil {
			reason := fmt.Sprintf("画像のデコードに失敗: %v", err)
			log.Printf("サムネイル生成エラー: %s", reason)
			s.emitProcessingFailed(c, aggregateID, reason)
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": reason})
			return
		}

		// 元画像のサイズを取得する。
		bounds := srcImg.Bounds()
		srcWidth := bounds.Dx()
		srcHeight := bounds.Dy()

		// 200x200のサムネイル画像を最近傍補間法でリサイズして生成する。
		thumbnailImg := resizeNearestNeighbor(srcImg, thumbnailSize, thumbnailSize)

		// サムネイルをJPEG形式で保存する。
		thumbnailDir := filepath.Dir(req.StoragePath)
		thumbnailPath := filepath.Join(thumbnailDir, "thumbnail.jpg")

		thumbFile, err := os.Create(thumbnailPath)
		if err != nil {
			reason := fmt.Sprintf("サムネイルファイルの作成に失敗: %v", err)
			log.Printf("サムネイル生成エラー: %s", reason)
			s.emitProcessingFailed(c, aggregateID, reason)
			c.JSON(http.StatusInternalServerError, gin.H{"error": reason})
			return
		}
		defer thumbFile.Close()

		if err := jpeg.Encode(thumbFile, thumbnailImg, &jpeg.Options{Quality: 85}); err != nil {
			reason := fmt.Sprintf("サムネイルのエンコードに失敗: %v", err)
			log.Printf("サムネイル生成エラー: %s", reason)
			s.emitProcessingFailed(c, aggregateID, reason)
			c.JSON(http.StatusInternalServerError, gin.H{"error": reason})
			return
		}

		// MediaProcessedイベントをEvent Storeに発行する。
		eventData := event.MediaProcessedData{
			ThumbnailPath: thumbnailPath,
			Width:         srcWidth,
			Height:        srcHeight,
		}

		if err := s.emitEvent(c, aggregateID, event.TypeMediaProcessed, eventData); err != nil {
			log.Printf("MediaProcessedイベントの送信に失敗: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "イベントの送信に失敗しました"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message":        "サムネイルを生成しました",
			"media_id":       mediaID,
			"thumbnail_path": thumbnailPath,
			"width":          srcWidth,
			"height":         srcHeight,
		})
	}
}

// emitProcessingFailed はMediaProcessingFailedイベントをEvent Storeに発行する。
func (s *Server) emitProcessingFailed(c *gin.Context, aggregateID, reason string) {
	eventData := event.MediaProcessingFailedData{
		Reason: reason,
	}
	if err := s.emitEvent(c, aggregateID, event.TypeMediaProcessingFailed, eventData); err != nil {
		log.Printf("MediaProcessingFailedイベントの送信に失敗: %v", err)
	}
}

// compensateRequest は補償アクションリクエストのJSON構造。
type compensateRequest struct {
	// Reason は補償アクションが実行された理由。
	Reason string `json:"reason"`
	// SagaID は関連するSagaのID。
	SagaID string `json:"saga_id"`
}

// handleCompensate はアップロード済みメディアの補償アクションを処理するハンドラを返す。
// Sagaのロールバック時に呼び出され、ディスクからファイルを削除し、
// MediaUploadCompensatedイベントをEvent Storeに発行する。
func (s *Server) handleCompensate() gin.HandlerFunc {
	return func(c *gin.Context) {
		mediaID := c.Param("id")
		if mediaID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "メディアIDが指定されていません"})
			return
		}

		var req compensateRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("リクエストが不正です: %v", err)})
			return
		}

		// ディスクからメディアファイルを削除する。
		mediaDir := filepath.Join(mediaBaseDir, mediaID)
		if err := os.RemoveAll(mediaDir); err != nil {
			log.Printf("メディアディレクトリの削除に失敗: %v", err)
			// ディレクトリ削除に失敗しても、イベントは発行する。
		}

		// MediaUploadCompensatedイベントをEvent Storeに発行する。
		aggregateID := fmt.Sprintf("media-%s", mediaID)
		eventData := event.MediaUploadCompensatedData{
			Reason: req.Reason,
			SagaID: req.SagaID,
		}

		if err := s.emitEvent(c, aggregateID, event.TypeMediaUploadCompensated, eventData); err != nil {
			log.Printf("MediaUploadCompensatedイベントの送信に失敗: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "イベントの送信に失敗しました"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message":  "補償アクションを実行しました",
			"media_id": mediaID,
		})
	}
}

// resizeNearestNeighbor は最近傍補間法で画像をリサイズする。
// Go標準ライブラリのみを使用し、外部依存を排除する。
// アスペクト比を維持しながら、指定サイズに収まるようにリサイズし、
// 余白部分は白で埋める。
func resizeNearestNeighbor(src image.Image, width, height int) *image.RGBA {
	srcBounds := src.Bounds()
	srcW := srcBounds.Dx()
	srcH := srcBounds.Dy()

	// アスペクト比を維持したスケーリング係数を算出する。
	scaleX := float64(width) / float64(srcW)
	scaleY := float64(height) / float64(srcH)
	scale := math.Min(scaleX, scaleY)

	// リサイズ後の実際のサイズを算出する。
	newW := int(float64(srcW) * scale)
	newH := int(float64(srcH) * scale)

	// 出力画像を白背景で初期化する。
	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(dst, dst.Bounds(), &image.Uniform{color.White}, image.Point{}, draw.Src)

	// 中央に配置するためのオフセットを算出する。
	offsetX := (width - newW) / 2
	offsetY := (height - newH) / 2

	// 最近傍補間法でリサイズする。
	for y := 0; y < newH; y++ {
		srcY := srcBounds.Min.Y + int(float64(y)/scale)
		if srcY >= srcBounds.Max.Y {
			srcY = srcBounds.Max.Y - 1
		}
		for x := 0; x < newW; x++ {
			srcX := srcBounds.Min.X + int(float64(x)/scale)
			if srcX >= srcBounds.Max.X {
				srcX = srcBounds.Max.X - 1
			}
			dst.Set(offsetX+x, offsetY+y, src.At(srcX, srcY))
		}
	}

	return dst
}

// isAllowedContentType は許可されたContent-Typeかどうかを判定する。
// image/* または video/* のみ許可する。
func isAllowedContentType(contentType string) bool {
	ct := strings.ToLower(contentType)
	return strings.HasPrefix(ct, "image/") || strings.HasPrefix(ct, "video/")
}
