package gateway

import (
	"fmt"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/nao1215/micro/pkg/middleware"
)

// Server はAPI Gatewayサービスの HTTPサーバー。
type Server struct {
	// router はGinのHTTPルーター。
	router *gin.Engine
	// port はサーバーのリッスンポート。
	port string
}

// NewServer は新しいGatewayサーバーを生成する。
func NewServer(port string) (*Server, error) {
	router := gin.New()
	router.Use(middleware.Recovery())
	router.Use(gin.Logger())

	frontendURL := os.Getenv("FRONTEND_URL")
	if frontendURL == "" {
		frontendURL = "http://localhost:3000"
	}
	router.Use(middleware.CORS([]string{frontendURL}))

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
	// OAuth2認証エンドポイント（認証不要）
	auth := s.router.Group("/auth")
	{
		auth.GET("/github", s.handleGitHubLogin())
		auth.GET("/github/callback", s.handleGitHubCallback())
		auth.GET("/google", s.handleGoogleLogin())
		auth.GET("/google/callback", s.handleGoogleCallback())
	}

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		jwtSecret = "dev-secret-key"
	}

	// 認証必須のAPIエンドポイント
	api := s.router.Group("/api/v1")
	api.Use(middleware.JWTAuth(jwtSecret))
	{
		// ユーザー情報
		api.GET("/me", s.handleGetCurrentUser())

		// メディア（プロキシ）
		api.POST("/media", s.handleProxyMediaUpload())
		api.GET("/media", s.handleProxyMediaList())
		api.GET("/media/:id", s.handleProxyMediaDetail())
		api.DELETE("/media/:id", s.handleProxyMediaDelete())

		// アルバム（プロキシ）
		api.POST("/albums", s.handleProxyAlbumCreate())
		api.GET("/albums", s.handleProxyAlbumList())
		api.GET("/albums/:id", s.handleProxyAlbumDetail())
		api.DELETE("/albums/:id", s.handleProxyAlbumDelete())
		api.POST("/albums/:id/media", s.handleProxyAlbumAddMedia())
		api.DELETE("/albums/:id/media/:media_id", s.handleProxyAlbumRemoveMedia())

		// 通知
		api.GET("/notifications", s.handleProxyNotificationList())
		api.PUT("/notifications/:id/read", s.handleProxyNotificationMarkRead())
	}

	// ヘルスチェック
	s.router.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok", "service": "gateway"})
	})
}

// 以下、各ハンドラのスタブ実装。

func (s *Server) handleGitHubLogin() gin.HandlerFunc {
	return func(c *gin.Context) { c.JSON(501, gin.H{"error": "未実装"}) }
}

func (s *Server) handleGitHubCallback() gin.HandlerFunc {
	return func(c *gin.Context) { c.JSON(501, gin.H{"error": "未実装"}) }
}

func (s *Server) handleGoogleLogin() gin.HandlerFunc {
	return func(c *gin.Context) { c.JSON(501, gin.H{"error": "未実装"}) }
}

func (s *Server) handleGoogleCallback() gin.HandlerFunc {
	return func(c *gin.Context) { c.JSON(501, gin.H{"error": "未実装"}) }
}

func (s *Server) handleGetCurrentUser() gin.HandlerFunc {
	return func(c *gin.Context) { c.JSON(501, gin.H{"error": "未実装"}) }
}

func (s *Server) handleProxyMediaUpload() gin.HandlerFunc {
	return func(c *gin.Context) { c.JSON(501, gin.H{"error": "未実装"}) }
}

func (s *Server) handleProxyMediaList() gin.HandlerFunc {
	return func(c *gin.Context) { c.JSON(501, gin.H{"error": "未実装"}) }
}

func (s *Server) handleProxyMediaDetail() gin.HandlerFunc {
	return func(c *gin.Context) { c.JSON(501, gin.H{"error": "未実装"}) }
}

func (s *Server) handleProxyMediaDelete() gin.HandlerFunc {
	return func(c *gin.Context) { c.JSON(501, gin.H{"error": "未実装"}) }
}

func (s *Server) handleProxyAlbumCreate() gin.HandlerFunc {
	return func(c *gin.Context) { c.JSON(501, gin.H{"error": "未実装"}) }
}

func (s *Server) handleProxyAlbumList() gin.HandlerFunc {
	return func(c *gin.Context) { c.JSON(501, gin.H{"error": "未実装"}) }
}

func (s *Server) handleProxyAlbumDetail() gin.HandlerFunc {
	return func(c *gin.Context) { c.JSON(501, gin.H{"error": "未実装"}) }
}

func (s *Server) handleProxyAlbumDelete() gin.HandlerFunc {
	return func(c *gin.Context) { c.JSON(501, gin.H{"error": "未実装"}) }
}

func (s *Server) handleProxyAlbumAddMedia() gin.HandlerFunc {
	return func(c *gin.Context) { c.JSON(501, gin.H{"error": "未実装"}) }
}

func (s *Server) handleProxyAlbumRemoveMedia() gin.HandlerFunc {
	return func(c *gin.Context) { c.JSON(501, gin.H{"error": "未実装"}) }
}

func (s *Server) handleProxyNotificationList() gin.HandlerFunc {
	return func(c *gin.Context) { c.JSON(501, gin.H{"error": "未実装"}) }
}

func (s *Server) handleProxyNotificationMarkRead() gin.HandlerFunc {
	return func(c *gin.Context) { c.JSON(501, gin.H{"error": "未実装"}) }
}
