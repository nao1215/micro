package command

import (
	"fmt"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/nao1215/micro/pkg/middleware"
)

// Server はメディアコマンドサービスのHTTPサーバー。
type Server struct {
	// router はGinのHTTPルーター。
	router *gin.Engine
	// port はサーバーのリッスンポート。
	port string
}

// NewServer は新しいメディアコマンドサーバーを生成する。
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
		c.JSON(200, gin.H{"status": "ok", "service": "media-command"})
	})
}

func (s *Server) handleUpload() gin.HandlerFunc {
	return func(c *gin.Context) { c.JSON(501, gin.H{"error": "未実装"}) }
}

func (s *Server) handleDelete() gin.HandlerFunc {
	return func(c *gin.Context) { c.JSON(501, gin.H{"error": "未実装"}) }
}

func (s *Server) handleProcess() gin.HandlerFunc {
	return func(c *gin.Context) { c.JSON(501, gin.H{"error": "未実装"}) }
}

func (s *Server) handleCompensate() gin.HandlerFunc {
	return func(c *gin.Context) { c.JSON(501, gin.H{"error": "未実装"}) }
}
