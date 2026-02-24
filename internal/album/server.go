package album

import (
	"fmt"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/nao1215/micro/pkg/middleware"
)

// Server はアルバムサービスのHTTPサーバー。
type Server struct {
	// router はGinのHTTPルーター。
	router *gin.Engine
	// port はサーバーのリッスンポート。
	port string
}

// NewServer は新しいアルバムサーバーを生成する。
func NewServer(port string) (*Server, error) {
	router := gin.New()
	router.Use(middleware.Recovery())
	router.Use(gin.Logger())

	s := &Server{
		router: router,
		port:   port,
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
		c.JSON(200, gin.H{"status": "ok", "service": "album"})
	})
}

func (s *Server) handleCreate() gin.HandlerFunc {
	return func(c *gin.Context) { c.JSON(501, gin.H{"error": "未実装"}) }
}

func (s *Server) handleList() gin.HandlerFunc {
	return func(c *gin.Context) { c.JSON(501, gin.H{"error": "未実装"}) }
}

func (s *Server) handleGetByID() gin.HandlerFunc {
	return func(c *gin.Context) { c.JSON(501, gin.H{"error": "未実装"}) }
}

func (s *Server) handleUpdate() gin.HandlerFunc {
	return func(c *gin.Context) { c.JSON(501, gin.H{"error": "未実装"}) }
}

func (s *Server) handleDelete() gin.HandlerFunc {
	return func(c *gin.Context) { c.JSON(501, gin.H{"error": "未実装"}) }
}

func (s *Server) handleAddMedia() gin.HandlerFunc {
	return func(c *gin.Context) { c.JSON(501, gin.H{"error": "未実装"}) }
}

func (s *Server) handleRemoveMedia() gin.HandlerFunc {
	return func(c *gin.Context) { c.JSON(501, gin.H{"error": "未実装"}) }
}

func (s *Server) handleListMedia() gin.HandlerFunc {
	return func(c *gin.Context) { c.JSON(501, gin.H{"error": "未実装"}) }
}
