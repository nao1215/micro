package eventstore

import (
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/nao1215/micro/pkg/middleware"
)

// Server はイベントストアサービスのHTTPサーバー。
type Server struct {
	// router はGinのHTTPルーター。
	router *gin.Engine
	// port はサーバーのリッスンポート。
	port string
}

// NewServer は新しいイベントストアサーバーを生成する。
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
	api := s.router.Group("/api/v1")
	{
		events := api.Group("/events")
		{
			// イベントの追記
			events.POST("", s.handleAppendEvent())
			// AggregateIDによるイベント取得
			events.GET("/aggregate/:aggregate_id", s.handleGetEventsByAggregateID())
			// イベントタイプによるイベント取得
			events.GET("/type/:event_type", s.handleGetEventsByType())
			// 日時指定によるイベント取得（クエリパラメータ: since）
			events.GET("/since", s.handleGetEventsSince())
			// AggregateIDの最新バージョン取得
			events.GET("/aggregate/:aggregate_id/version", s.handleGetLatestVersion())
		}
	}

	// ヘルスチェック
	s.router.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok", "service": "eventstore"})
	})
}

// handleAppendEvent はイベントの追記を処理するハンドラを返す。
func (s *Server) handleAppendEvent() gin.HandlerFunc {
	return func(c *gin.Context) {
		// TODO: 実装
		c.JSON(501, gin.H{"error": "未実装"})
	}
}

// handleGetEventsByAggregateID はAggregateIDによるイベント取得を処理するハンドラを返す。
func (s *Server) handleGetEventsByAggregateID() gin.HandlerFunc {
	return func(c *gin.Context) {
		// TODO: 実装
		c.JSON(501, gin.H{"error": "未実装"})
	}
}

// handleGetEventsByType はイベントタイプによるイベント取得を処理するハンドラを返す。
func (s *Server) handleGetEventsByType() gin.HandlerFunc {
	return func(c *gin.Context) {
		// TODO: 実装
		c.JSON(501, gin.H{"error": "未実装"})
	}
}

// handleGetEventsSince は日時指定によるイベント取得を処理するハンドラを返す。
func (s *Server) handleGetEventsSince() gin.HandlerFunc {
	return func(c *gin.Context) {
		// TODO: 実装
		c.JSON(501, gin.H{"error": "未実装"})
	}
}

// handleGetLatestVersion はAggregateIDの最新バージョン取得を処理するハンドラを返す。
func (s *Server) handleGetLatestVersion() gin.HandlerFunc {
	return func(c *gin.Context) {
		// TODO: 実装
		c.JSON(501, gin.H{"error": "未実装"})
	}
}
