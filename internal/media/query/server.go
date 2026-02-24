package query

import (
	"fmt"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/nao1215/micro/pkg/middleware"
)

// Server はメディアクエリサービスのHTTPサーバー。
type Server struct {
	// router はGinのHTTPルーター。
	router *gin.Engine
	// port はサーバーのリッスンポート。
	port string
}

// NewServer は新しいメディアクエリサーバーを生成する。
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
		c.JSON(200, gin.H{"status": "ok", "service": "media-query"})
	})
}

func (s *Server) handleList() gin.HandlerFunc {
	return func(c *gin.Context) { c.JSON(501, gin.H{"error": "未実装"}) }
}

func (s *Server) handleGetByID() gin.HandlerFunc {
	return func(c *gin.Context) { c.JSON(501, gin.H{"error": "未実装"}) }
}

func (s *Server) handleSearch() gin.HandlerFunc {
	return func(c *gin.Context) { c.JSON(501, gin.H{"error": "未実装"}) }
}

func (s *Server) handleRebuild() gin.HandlerFunc {
	return func(c *gin.Context) { c.JSON(501, gin.H{"error": "未実装"}) }
}
