package notification

import (
	"fmt"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/nao1215/micro/pkg/middleware"
)

// Server は通知サービスのHTTPサーバー。
type Server struct {
	// router はGinのHTTPルーター。
	router *gin.Engine
	// port はサーバーのリッスンポート。
	port string
}

// NewServer は新しい通知サーバーを生成する。
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
		notifications := api.Group("/notifications")
		{
			// 通知一覧取得
			notifications.GET("", s.handleList())
			// 未読通知一覧取得
			notifications.GET("/unread", s.handleListUnread())
			// 通知を既読にする
			notifications.PUT("/:id/read", s.handleMarkAsRead())
			// 全通知を既読にする
			notifications.PUT("/read-all", s.handleMarkAllAsRead())
		}

		// 通知送信（内部API - Sagaから呼び出される）
		internal := api.Group("/internal")
		{
			internal.POST("/send", s.handleSend())
		}
	}

	// ヘルスチェック
	s.router.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok", "service": "notification"})
	})
}

func (s *Server) handleList() gin.HandlerFunc {
	return func(c *gin.Context) { c.JSON(501, gin.H{"error": "未実装"}) }
}

func (s *Server) handleListUnread() gin.HandlerFunc {
	return func(c *gin.Context) { c.JSON(501, gin.H{"error": "未実装"}) }
}

func (s *Server) handleMarkAsRead() gin.HandlerFunc {
	return func(c *gin.Context) { c.JSON(501, gin.H{"error": "未実装"}) }
}

func (s *Server) handleMarkAllAsRead() gin.HandlerFunc {
	return func(c *gin.Context) { c.JSON(501, gin.H{"error": "未実装"}) }
}

func (s *Server) handleSend() gin.HandlerFunc {
	return func(c *gin.Context) { c.JSON(501, gin.H{"error": "未実装"}) }
}
