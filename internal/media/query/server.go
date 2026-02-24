package query

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	_ "github.com/mattn/go-sqlite3"
	mediadb "github.com/nao1215/micro/internal/media/query/db"
	"github.com/nao1215/micro/pkg/middleware"
)

// Server はメディアクエリサービスのHTTPサーバー。
// CQRSのQuery側を担当し、Read Modelからの読み取りクエリを処理する。
type Server struct {
	// router はGinのHTTPルーター。
	router *gin.Engine
	// port はサーバーのリッスンポート。
	port string
	// queries はsqlcが生成したクエリ実行オブジェクト。
	queries *mediadb.Queries
	// db はSQLite Read Modelのデータベース接続。
	db *sql.DB
	// projector はEvent Storeからイベントをポーリングし、Read Modelを更新するバックグラウンドプロセス。
	projector *Projector
}

// NewServer は新しいメディアクエリサーバーを生成する。
// SQLite Read Modelの初期化、スキーマ作成、およびProjectorのバックグラウンド起動を行う。
func NewServer(port string) (*Server, error) {
	sqlDB, err := sql.Open("sqlite3", "/data/media-query.db?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("Read Modelデータベース接続に失敗: %w", err)
	}

	if err := initSchema(sqlDB); err != nil {
		return nil, fmt.Errorf("Read Modelスキーマ初期化に失敗: %w", err)
	}

	queries := mediadb.New(sqlDB)

	eventstoreURL := os.Getenv("EVENTSTORE_URL")
	if eventstoreURL == "" {
		eventstoreURL = "http://localhost:8084"
	}

	projector := NewProjector(queries, eventstoreURL)

	router := gin.New()
	router.Use(middleware.Recovery())
	router.Use(gin.Logger())

	s := &Server{
		router:    router,
		port:      port,
		queries:   queries,
		db:        sqlDB,
		projector: projector,
	}
	s.setupRoutes()

	// バックグラウンドでEvent Storeのポーリングを開始する
	projector.Start(context.Background())

	return s, nil
}

// Run はHTTPサーバーを起動する。
func (s *Server) Run() error {
	return s.router.Run(fmt.Sprintf(":%s", s.port))
}

// Shutdown はサーバーを停止する。
// Projectorの停止とデータベース接続のクローズを行う。
func (s *Server) Shutdown() {
	if s.projector != nil {
		s.projector.Stop()
	}
	if s.db != nil {
		if err := s.db.Close(); err != nil {
			log.Printf("データベースのクローズに失敗: %v", err)
		}
	}
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
			// メディア一覧取得
			media.GET("", s.handleList())
			// メディア詳細取得
			media.GET("/:id", s.handleGetByID())
			// メディア検索
			media.GET("/search", s.handleSearch())
		}

		// Read Model管理（内部API）
		internal := api.Group("/internal")
		{
			// Read Modelの完全再構築
			internal.POST("/rebuild", s.handleRebuild())
		}
	}

	// ヘルスチェック
	s.router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "media-query"})
	})
}

// mediaResponse はメディア情報のJSONレスポンス構造。
type mediaResponse struct {
	// ID はメディアの一意識別子。
	ID string `json:"id"`
	// UserID はアップロードしたユーザーのID。
	UserID string `json:"user_id"`
	// Filename は元のファイル名。
	Filename string `json:"filename"`
	// ContentType はファイルのMIMEタイプ。
	ContentType string `json:"content_type"`
	// Size はファイルサイズ（バイト）。
	Size int64 `json:"size"`
	// StoragePath はファイルの保存パス。
	StoragePath string `json:"storage_path"`
	// ThumbnailPath はサムネイル画像の保存パス。処理完了前はnull。
	ThumbnailPath *string `json:"thumbnail_path"`
	// Width は画像/動画の幅（ピクセル）。処理完了前はnull。
	Width *int64 `json:"width"`
	// Height は画像/動画の高さ（ピクセル）。処理完了前はnull。
	Height *int64 `json:"height"`
	// DurationSeconds は動画の長さ（秒）。画像の場合はnull。
	DurationSeconds *float64 `json:"duration_seconds"`
	// Status はメディアの状態（uploaded, processed, failed, deleted）。
	Status string `json:"status"`
	// UploadedAt はアップロード日時。
	UploadedAt string `json:"uploaded_at"`
	// UpdatedAt はRead Model更新日時。
	UpdatedAt string `json:"updated_at"`
}

// toMediaResponse はRead Modelのレコードを外部レスポンス形式に変換する。
func toMediaResponse(m mediadb.MediaReadModel) mediaResponse {
	resp := mediaResponse{
		ID:          m.ID,
		UserID:      m.UserID,
		Filename:    m.Filename,
		ContentType: m.ContentType,
		Size:        m.Size,
		StoragePath: m.StoragePath,
		Status:      m.Status,
		UploadedAt:  m.UploadedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:   m.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}

	if m.ThumbnailPath.Valid {
		resp.ThumbnailPath = &m.ThumbnailPath.String
	}
	if m.Width.Valid {
		resp.Width = &m.Width.Int64
	}
	if m.Height.Valid {
		resp.Height = &m.Height.Int64
	}
	if m.DurationSeconds.Valid {
		resp.DurationSeconds = &m.DurationSeconds.Float64
	}

	return resp
}

// toMediaResponses はRead Modelのレコードスライスを外部レスポンス形式のスライスに変換する。
func toMediaResponses(models []mediadb.MediaReadModel) []mediaResponse {
	responses := make([]mediaResponse, 0, len(models))
	for _, m := range models {
		responses = append(responses, toMediaResponse(m))
	}
	return responses
}

// handleList は認証済みユーザーのメディア一覧を返すハンドラ。
// X-User-IDヘッダーまたはJWTクレームからユーザーIDを取得する。
func (s *Server) handleList() gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := middleware.GetUserID(c)
		if userID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "ユーザーIDが取得できません"})
			return
		}

		models, err := s.queries.ListMediaByUserID(c.Request.Context(), userID)
		if err != nil {
			log.Printf("メディア一覧取得エラー: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "メディア一覧の取得に失敗しました"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"media": toMediaResponses(models),
			"count": len(models),
		})
	}
}

// handleGetByID は指定されたIDのメディア詳細を返すハンドラ。
// パスパラメータ :id からメディアIDを取得する。
func (s *Server) handleGetByID() gin.HandlerFunc {
	return func(c *gin.Context) {
		mediaID := c.Param("id")
		if mediaID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "メディアIDが必要です"})
			return
		}

		model, err := s.queries.GetMediaByID(c.Request.Context(), mediaID)
		if err != nil {
			if err == sql.ErrNoRows {
				c.JSON(http.StatusNotFound, gin.H{"error": "メディアが見つかりません"})
				return
			}
			log.Printf("メディア詳細取得エラー: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "メディア詳細の取得に失敗しました"})
			return
		}

		c.JSON(http.StatusOK, toMediaResponse(model))
	}
}

// handleSearch はファイル名によるメディア検索を処理するハンドラ。
// クエリパラメータ q でファイル名のパターンを指定する（部分一致検索）。
func (s *Server) handleSearch() gin.HandlerFunc {
	return func(c *gin.Context) {
		q := c.Query("q")
		if q == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "検索クエリ(q)が必要です"})
			return
		}

		// LIKE句による部分一致検索
		pattern := fmt.Sprintf("%%%s%%", q)
		models, err := s.queries.SearchMedia(c.Request.Context(), pattern)
		if err != nil {
			log.Printf("メディア検索エラー: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "メディアの検索に失敗しました"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"media": toMediaResponses(models),
			"count": len(models),
			"query": q,
		})
	}
}

// handleRebuild はRead Modelの完全再構築を実行するハンドラ。
// Event Storeの全イベントから Read Modelを再構築する。
// データの整合性回復やスキーマ変更後に使用する。
func (s *Server) handleRebuild() gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := s.projector.RebuildFromEventStore(c.Request.Context()); err != nil {
			log.Printf("Read Model再構築エラー: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Read Modelの再構築に失敗しました"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message": "Read Modelの再構築が完了しました",
		})
	}
}
