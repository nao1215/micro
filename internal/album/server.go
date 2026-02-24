package album

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
	albumdb "github.com/nao1215/micro/internal/album/db"
	"github.com/nao1215/micro/pkg/event"
	"github.com/nao1215/micro/pkg/httpclient"
	"github.com/nao1215/micro/pkg/middleware"
)

// Server はアルバムサービスのHTTPサーバー。
type Server struct {
	// router はGinのHTTPルーター。
	router *gin.Engine
	// port はサーバーのリッスンポート。
	port string
	// queries はsqlcが生成したクエリ実行オブジェクト。
	queries *albumdb.Queries
	// db はSQLiteデータベース接続。
	db *sql.DB
	// eventClient はEvent StoreへのHTTPクライアント。
	eventClient *httpclient.Client
}

// NewServer は新しいアルバムサーバーを生成する。
// SQLiteデータベースの初期化とスキーマ作成を行う。
func NewServer(port string) (*Server, error) {
	sqlDB, err := sql.Open("sqlite3", "/data/album.db?_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=ON")
	if err != nil {
		return nil, fmt.Errorf("データベース接続に失敗: %w", err)
	}

	if err := initSchema(sqlDB); err != nil {
		return nil, fmt.Errorf("スキーマ初期化に失敗: %w", err)
	}

	eventstoreURL := os.Getenv("EVENTSTORE_URL")
	if eventstoreURL == "" {
		eventstoreURL = "http://localhost:8084"
	}

	router := gin.New()
	router.Use(middleware.Recovery())
	router.Use(gin.Logger())

	s := &Server{
		router:      router,
		port:        port,
		queries:     albumdb.New(sqlDB),
		db:          sqlDB,
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
		albums := api.Group("/albums")
		{
			// アルバム作成
			albums.POST("", s.handleCreate())
			// アルバム一覧取得
			albums.GET("", s.handleList())
			// アルバム詳細取得
			albums.GET("/:id", s.handleGetByID())
			// アルバム更新
			albums.PUT("/:id", s.handleUpdate())
			// アルバム削除
			albums.DELETE("/:id", s.handleDelete())
			// アルバムにメディアを追加
			albums.POST("/:id/media", s.handleAddMedia())
			// アルバムからメディアを削除
			albums.DELETE("/:id/media/:media_id", s.handleRemoveMedia())
			// アルバム内メディア一覧取得
			albums.GET("/:id/media", s.handleListMedia())
		}
	}

	// ヘルスチェック
	s.router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "album"})
	})
}

// createAlbumRequest はアルバム作成リクエストのJSON構造。
type createAlbumRequest struct {
	// Name はアルバム名。
	Name string `json:"name" binding:"required"`
	// Description はアルバムの説明。
	Description string `json:"description"`
}

// updateAlbumRequest はアルバム更新リクエストのJSON構造。
type updateAlbumRequest struct {
	// Name はアルバム名。
	Name string `json:"name" binding:"required"`
	// Description はアルバムの説明。
	Description string `json:"description"`
}

// addMediaRequest はメディア追加リクエストのJSON構造。
type addMediaRequest struct {
	// MediaID は追加するメディアのID。
	MediaID string `json:"media_id" binding:"required"`
}

// albumResponse はアルバムのJSONレスポンス構造。
type albumResponse struct {
	// ID はアルバムの一意識別子。
	ID string `json:"id"`
	// UserID はアルバムを作成したユーザーのID。
	UserID string `json:"user_id"`
	// Name はアルバム名。
	Name string `json:"name"`
	// Description はアルバムの説明。
	Description string `json:"description"`
	// CreatedAt は作成日時。
	CreatedAt string `json:"created_at"`
	// UpdatedAt は更新日時。
	UpdatedAt string `json:"updated_at"`
}

// mediaInAlbumResponse はアルバム内メディアのJSONレスポンス構造。
type mediaInAlbumResponse struct {
	// MediaID はメディアのID。
	MediaID string `json:"media_id"`
	// AddedAt は追加日時。
	AddedAt string `json:"added_at"`
}

// toAlbumResponse はDB行をJSONレスポンスに変換する。
func toAlbumResponse(a albumdb.Album) albumResponse {
	return albumResponse{
		ID:          a.ID,
		UserID:      a.UserID,
		Name:        a.Name,
		Description: a.Description,
		CreatedAt:   a.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:   a.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}
}

// handleCreate はアルバム作成を処理するハンドラを返す。
// 新しいアルバムを作成し、AlbumCreatedイベントをEvent Storeに送信する。
func (s *Server) handleCreate() gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := middleware.GetUserID(c)
		if userID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "ユーザーIDが取得できません"})
			return
		}

		var req createAlbumRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("リクエストが不正です: %v", err)})
			return
		}

		albumID := uuid.New().String()
		if err := s.queries.CreateAlbum(c.Request.Context(), albumdb.CreateAlbumParams{
			ID:          albumID,
			UserID:      userID,
			Name:        req.Name,
			Description: req.Description,
		}); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "アルバムの作成に失敗しました"})
			log.Printf("アルバム作成エラー: %v", err)
			return
		}

		// AlbumCreatedイベントをEvent Storeに送信する
		s.emitEvent(c, fmt.Sprintf("album-%s", albumID), event.AlbumCreatedData{
			UserID:      userID,
			Name:        req.Name,
			Description: req.Description,
		}, event.TypeAlbumCreated)

		// 作成したアルバムをDBから取得してレスポンスを返す
		created, err := s.queries.GetAlbumByID(c.Request.Context(), albumID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "作成したアルバムの取得に失敗しました"})
			log.Printf("アルバム取得エラー: %v", err)
			return
		}

		c.JSON(http.StatusCreated, toAlbumResponse(created))
	}
}

// handleList はユーザーのアルバム一覧取得を処理するハンドラを返す。
func (s *Server) handleList() gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := middleware.GetUserID(c)
		if userID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "ユーザーIDが取得できません"})
			return
		}

		albums, err := s.queries.ListAlbumsByUserID(c.Request.Context(), userID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "アルバム一覧の取得に失敗しました"})
			log.Printf("アルバム一覧取得エラー: %v", err)
			return
		}

		responses := make([]albumResponse, 0, len(albums))
		for _, a := range albums {
			responses = append(responses, toAlbumResponse(a))
		}

		c.JSON(http.StatusOK, responses)
	}
}

// handleGetByID はアルバム詳細取得を処理するハンドラを返す。
// 指定されたIDのアルバムが現在のユーザーに所属しているかを確認する。
func (s *Server) handleGetByID() gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := middleware.GetUserID(c)
		if userID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "ユーザーIDが取得できません"})
			return
		}

		albumID := c.Param("id")
		a, err := s.queries.GetAlbumByID(c.Request.Context(), albumID)
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "アルバムが見つかりません"})
			return
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "アルバムの取得に失敗しました"})
			log.Printf("アルバム取得エラー: %v", err)
			return
		}

		if a.UserID != userID {
			c.JSON(http.StatusForbidden, gin.H{"error": "このアルバムへのアクセス権がありません"})
			return
		}

		c.JSON(http.StatusOK, toAlbumResponse(a))
	}
}

// handleUpdate はアルバム更新を処理するハンドラを返す。
// 指定されたIDのアルバムの名前と説明を更新する。
func (s *Server) handleUpdate() gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := middleware.GetUserID(c)
		if userID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "ユーザーIDが取得できません"})
			return
		}

		albumID := c.Param("id")

		// アルバムの存在確認と所有者チェック
		a, err := s.queries.GetAlbumByID(c.Request.Context(), albumID)
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "アルバムが見つかりません"})
			return
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "アルバムの取得に失敗しました"})
			log.Printf("アルバム取得エラー: %v", err)
			return
		}

		if a.UserID != userID {
			c.JSON(http.StatusForbidden, gin.H{"error": "このアルバムへのアクセス権がありません"})
			return
		}

		var req updateAlbumRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("リクエストが不正です: %v", err)})
			return
		}

		if err := s.queries.UpdateAlbum(c.Request.Context(), albumdb.UpdateAlbumParams{
			Name:        req.Name,
			Description: req.Description,
			ID:          albumID,
		}); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "アルバムの更新に失敗しました"})
			log.Printf("アルバム更新エラー: %v", err)
			return
		}

		// 更新後のアルバムをDBから取得してレスポンスを返す
		updated, err := s.queries.GetAlbumByID(c.Request.Context(), albumID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "更新後のアルバムの取得に失敗しました"})
			log.Printf("アルバム取得エラー: %v", err)
			return
		}

		c.JSON(http.StatusOK, toAlbumResponse(updated))
	}
}

// handleDelete はアルバム削除を処理するハンドラを返す。
// 指定されたIDのアルバムを削除し、AlbumDeletedイベントをEvent Storeに送信する。
func (s *Server) handleDelete() gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := middleware.GetUserID(c)
		if userID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "ユーザーIDが取得できません"})
			return
		}

		albumID := c.Param("id")

		// アルバムの存在確認と所有者チェック
		a, err := s.queries.GetAlbumByID(c.Request.Context(), albumID)
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "アルバムが見つかりません"})
			return
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "アルバムの取得に失敗しました"})
			log.Printf("アルバム取得エラー: %v", err)
			return
		}

		if a.UserID != userID {
			c.JSON(http.StatusForbidden, gin.H{"error": "このアルバムへのアクセス権がありません"})
			return
		}

		if err := s.queries.DeleteAlbum(c.Request.Context(), albumID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "アルバムの削除に失敗しました"})
			log.Printf("アルバム削除エラー: %v", err)
			return
		}

		// AlbumDeletedイベントをEvent Storeに送信する
		s.emitEvent(c, fmt.Sprintf("album-%s", albumID), event.AlbumDeletedData{
			UserID: userID,
		}, event.TypeAlbumDeleted)

		c.JSON(http.StatusOK, gin.H{"message": "アルバムを削除しました"})
	}
}

// handleAddMedia はアルバムへのメディア追加を処理するハンドラを返す。
// メディアをアルバムに追加し、MediaAddedToAlbumイベントをEvent Storeに送信する。
// ユーザーにデフォルトの「All Media」アルバムが存在しない場合は自動的に作成する。
func (s *Server) handleAddMedia() gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := middleware.GetUserID(c)
		if userID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "ユーザーIDが取得できません"})
			return
		}

		albumID := c.Param("id")

		// アルバムの存在確認と所有者チェック
		a, err := s.queries.GetAlbumByID(c.Request.Context(), albumID)
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "アルバムが見つかりません"})
			return
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "アルバムの取得に失敗しました"})
			log.Printf("アルバム取得エラー: %v", err)
			return
		}

		if a.UserID != userID {
			c.JSON(http.StatusForbidden, gin.H{"error": "このアルバムへのアクセス権がありません"})
			return
		}

		var req addMediaRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("リクエストが不正です: %v", err)})
			return
		}

		// デフォルトの「All Media」アルバムが存在しない場合は作成する
		defaultAlbumID, err := s.ensureDefaultAlbum(c, userID)
		if err != nil {
			log.Printf("デフォルトアルバムの確認/作成エラー: %v", err)
			// デフォルトアルバム作成に失敗しても、指定アルバムへの追加は続行する
		}

		// 指定されたアルバムにメディアを追加する
		if err := s.queries.AddMediaToAlbum(c.Request.Context(), albumdb.AddMediaToAlbumParams{
			AlbumID: albumID,
			MediaID: req.MediaID,
		}); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "メディアのアルバムへの追加に失敗しました"})
			log.Printf("メディア追加エラー: %v", err)
			return
		}

		// MediaAddedToAlbumイベントをEvent Storeに送信する
		s.emitEvent(c, fmt.Sprintf("album-%s", albumID), event.MediaAddedToAlbumData{
			MediaID: req.MediaID,
		}, event.TypeMediaAddedToAlbum)

		// デフォルトアルバムが指定アルバムと異なる場合は、デフォルトアルバムにも追加する
		if defaultAlbumID != "" && defaultAlbumID != albumID {
			if err := s.queries.AddMediaToAlbum(c.Request.Context(), albumdb.AddMediaToAlbumParams{
				AlbumID: defaultAlbumID,
				MediaID: req.MediaID,
			}); err != nil {
				// デフォルトアルバムへの追加失敗はログに記録するが、エラーレスポンスは返さない
				log.Printf("デフォルトアルバムへのメディア追加エラー: %v", err)
			} else {
				// デフォルトアルバムへの追加もイベントを送信する
				s.emitEvent(c, fmt.Sprintf("album-%s", defaultAlbumID), event.MediaAddedToAlbumData{
					MediaID: req.MediaID,
				}, event.TypeMediaAddedToAlbum)
			}
		}

		c.JSON(http.StatusOK, gin.H{"message": "メディアをアルバムに追加しました"})
	}
}

// handleRemoveMedia はアルバムからのメディア削除を処理するハンドラを返す。
// メディアをアルバムから削除し、MediaRemovedFromAlbumイベントをEvent Storeに送信する。
func (s *Server) handleRemoveMedia() gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := middleware.GetUserID(c)
		if userID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "ユーザーIDが取得できません"})
			return
		}

		albumID := c.Param("id")
		mediaID := c.Param("media_id")

		// アルバムの存在確認と所有者チェック
		a, err := s.queries.GetAlbumByID(c.Request.Context(), albumID)
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "アルバムが見つかりません"})
			return
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "アルバムの取得に失敗しました"})
			log.Printf("アルバム取得エラー: %v", err)
			return
		}

		if a.UserID != userID {
			c.JSON(http.StatusForbidden, gin.H{"error": "このアルバムへのアクセス権がありません"})
			return
		}

		if err := s.queries.RemoveMediaFromAlbum(c.Request.Context(), albumdb.RemoveMediaFromAlbumParams{
			AlbumID: albumID,
			MediaID: mediaID,
		}); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "メディアのアルバムからの削除に失敗しました"})
			log.Printf("メディア削除エラー: %v", err)
			return
		}

		// MediaRemovedFromAlbumイベントをEvent Storeに送信する
		s.emitEvent(c, fmt.Sprintf("album-%s", albumID), event.MediaRemovedFromAlbumData{
			MediaID: mediaID,
		}, event.TypeMediaRemovedFromAlbum)

		c.JSON(http.StatusOK, gin.H{"message": "メディアをアルバムから削除しました"})
	}
}

// handleListMedia はアルバム内メディア一覧取得を処理するハンドラを返す。
func (s *Server) handleListMedia() gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := middleware.GetUserID(c)
		if userID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "ユーザーIDが取得できません"})
			return
		}

		albumID := c.Param("id")

		// アルバムの存在確認と所有者チェック
		a, err := s.queries.GetAlbumByID(c.Request.Context(), albumID)
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "アルバムが見つかりません"})
			return
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "アルバムの取得に失敗しました"})
			log.Printf("アルバム取得エラー: %v", err)
			return
		}

		if a.UserID != userID {
			c.JSON(http.StatusForbidden, gin.H{"error": "このアルバムへのアクセス権がありません"})
			return
		}

		media, err := s.queries.ListMediaInAlbum(c.Request.Context(), albumID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "メディア一覧の取得に失敗しました"})
			log.Printf("メディア一覧取得エラー: %v", err)
			return
		}

		responses := make([]mediaInAlbumResponse, 0, len(media))
		for _, m := range media {
			responses = append(responses, mediaInAlbumResponse{
				MediaID: m.MediaID,
				AddedAt: m.AddedAt.Format("2006-01-02T15:04:05Z"),
			})
		}

		c.JSON(http.StatusOK, responses)
	}
}

// ensureDefaultAlbum はユーザーのデフォルト「All Media」アルバムが存在することを確認する。
// 存在しない場合は新規作成し、AlbumCreatedイベントをEvent Storeに送信する。
// デフォルトアルバムのIDを返す。
func (s *Server) ensureDefaultAlbum(c *gin.Context, userID string) (string, error) {
	// デフォルトアルバムの存在を確認する
	defaultAlbum, err := s.queries.GetDefaultAlbumByUserID(c.Request.Context(), userID)
	if err == nil {
		return defaultAlbum.ID, nil
	}
	if err != sql.ErrNoRows {
		return "", fmt.Errorf("デフォルトアルバムの取得に失敗: %w", err)
	}

	// デフォルトアルバムが存在しないので作成する
	defaultAlbumID := uuid.New().String()
	if err := s.queries.CreateAlbum(c.Request.Context(), albumdb.CreateAlbumParams{
		ID:          defaultAlbumID,
		UserID:      userID,
		Name:        "All Media",
		Description: "すべてのメディアを含むデフォルトアルバム",
	}); err != nil {
		return "", fmt.Errorf("デフォルトアルバムの作成に失敗: %w", err)
	}

	// AlbumCreatedイベントをEvent Storeに送信する
	s.emitEvent(c, fmt.Sprintf("album-%s", defaultAlbumID), event.AlbumCreatedData{
		UserID:      userID,
		Name:        "All Media",
		Description: "すべてのメディアを含むデフォルトアルバム",
	}, event.TypeAlbumCreated)

	log.Printf("ユーザー %s のデフォルトアルバムを作成しました: %s", userID, defaultAlbumID)
	return defaultAlbumID, nil
}

// emitEvent はEvent Storeにイベントを送信する。
// 送信に失敗した場合はログに記録するが、呼び出し元にはエラーを返さない。
func (s *Server) emitEvent(c *gin.Context, aggregateID string, data any, eventType event.Type) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		log.Printf("イベントデータのシリアライズに失敗: %v", err)
		return
	}

	reqBody := map[string]any{
		"aggregate_id":   aggregateID,
		"aggregate_type": string(event.AggregateTypeAlbum),
		"event_type":     string(eventType),
		"data":           json.RawMessage(jsonData),
	}

	ctx := httpclient.WithUserID(c.Request.Context(), middleware.GetUserID(c))
	if err := s.eventClient.PostJSON(ctx, "/api/v1/events", reqBody, nil); err != nil {
		log.Printf("Event Storeへのイベント送信に失敗: %v", err)
	}
}
