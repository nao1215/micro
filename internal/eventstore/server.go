package eventstore

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	_ "github.com/mattn/go-sqlite3"
	eventstoredb "github.com/nao1215/micro/internal/eventstore/db"
	"github.com/nao1215/micro/pkg/event"
	"github.com/nao1215/micro/pkg/middleware"
)

// Server はイベントストアサービスのHTTPサーバー。
type Server struct {
	// router はGinのHTTPルーター。
	router *gin.Engine
	// port はサーバーのリッスンポート。
	port string
	// queries はsqlcが生成したクエリ実行オブジェクト。
	queries *eventstoredb.Queries
	// db はSQLiteデータベース接続。
	db *sql.DB
}

// NewServer は新しいイベントストアサーバーを生成する。
// SQLiteデータベースの初期化とスキーマ作成を行う。
func NewServer(port string) (*Server, error) {
	sqlDB, err := sql.Open("sqlite3", "/data/eventstore.db?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("データベース接続に失敗: %w", err)
	}

	if err := initSchema(sqlDB); err != nil {
		return nil, fmt.Errorf("スキーマ初期化に失敗: %w", err)
	}

	router := gin.New()
	router.Use(middleware.Recovery())
	router.Use(gin.Logger())

	s := &Server{
		router:  router,
		port:    port,
		queries: eventstoredb.New(sqlDB),
		db:      sqlDB,
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
			// 全イベント取得（Read Model再構築用）
			events.GET("", s.handleGetAllEvents())
		}
	}

	// ヘルスチェック
	s.router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "eventstore"})
	})
}

// appendEventRequest はイベント追記リクエストのJSON構造。
type appendEventRequest struct {
	AggregateID   string          `json:"aggregate_id" binding:"required"`
	AggregateType string          `json:"aggregate_type" binding:"required"`
	EventType     string          `json:"event_type" binding:"required"`
	Data          json.RawMessage `json:"data" binding:"required"`
}

// eventResponse はイベントのJSONレスポンス構造。
type eventResponse struct {
	ID            string `json:"id"`
	AggregateID   string `json:"aggregate_id"`
	AggregateType string `json:"aggregate_type"`
	EventType     string `json:"event_type"`
	Data          string `json:"data"`
	Version       int64  `json:"version"`
	CreatedAt     string `json:"created_at"`
}

// handleAppendEvent はイベントの追記を処理するハンドラを返す。
// 楽観的排他制御: 現在の最新バージョン+1を新しいバージョンとして設定する。
func (s *Server) handleAppendEvent() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req appendEventRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("リクエストが不正です: %v", err)})
			return
		}

		// 楽観的排他制御: 最新バージョンを取得して+1する
		latestVersionRaw, err := s.queries.GetLatestVersion(c.Request.Context(), req.AggregateID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "バージョン取得に失敗しました"})
			log.Printf("バージョン取得エラー: %v", err)
			return
		}

		var latestVersion int64
		switch v := latestVersionRaw.(type) {
		case int64:
			latestVersion = v
		case float64:
			latestVersion = int64(v)
		default:
			latestVersion = 0
		}
		newVersion := latestVersion + 1

		// イベントを生成
		ev, err := event.New(
			req.AggregateID,
			event.AggregateType(req.AggregateType),
			event.Type(req.EventType),
			newVersion,
			req.Data,
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "イベント生成に失敗しました"})
			log.Printf("イベント生成エラー: %v", err)
			return
		}

		// Event Storeに追記（append-only）
		if err := s.queries.AppendEvent(c.Request.Context(), eventstoredb.AppendEventParams{
			ID:            ev.ID,
			AggregateID:   ev.AggregateID,
			AggregateType: string(ev.AggregateType),
			EventType:     string(ev.EventType),
			Data:          string(ev.Data),
			Version:       ev.Version,
			CreatedAt:     ev.CreatedAt,
		}); err != nil {
			c.JSON(http.StatusConflict, gin.H{"error": "イベントの追記に失敗しました（バージョン競合の可能性）"})
			log.Printf("イベント追記エラー: %v", err)
			return
		}

		c.JSON(http.StatusCreated, toEventResponse(ev.ID, ev.AggregateID, string(ev.AggregateType), string(ev.EventType), string(ev.Data), ev.Version, ev.CreatedAt))
	}
}

// handleGetEventsByAggregateID はAggregateIDによるイベント取得を処理するハンドラを返す。
func (s *Server) handleGetEventsByAggregateID() gin.HandlerFunc {
	return func(c *gin.Context) {
		aggregateID := c.Param("aggregate_id")

		rows, err := s.queries.GetEventsByAggregateID(c.Request.Context(), aggregateID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "イベント取得に失敗しました"})
			log.Printf("イベント取得エラー: %v", err)
			return
		}

		c.JSON(http.StatusOK, toEventResponses(rows))
	}
}

// handleGetEventsByType はイベントタイプによるイベント取得を処理するハンドラを返す。
func (s *Server) handleGetEventsByType() gin.HandlerFunc {
	return func(c *gin.Context) {
		eventType := c.Param("event_type")

		rows, err := s.queries.GetEventsByType(c.Request.Context(), eventType)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "イベント取得に失敗しました"})
			log.Printf("イベント取得エラー: %v", err)
			return
		}

		c.JSON(http.StatusOK, toEventResponses(rows))
	}
}

// handleGetEventsSince は日時指定によるイベント取得を処理するハンドラを返す。
func (s *Server) handleGetEventsSince() gin.HandlerFunc {
	return func(c *gin.Context) {
		sinceStr := c.Query("since")
		if sinceStr == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "sinceクエリパラメータが必要です"})
			return
		}

		since, err := time.Parse(time.RFC3339, sinceStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "since の形式が不正です（RFC3339形式: 2006-01-02T15:04:05Z）"})
			return
		}

		rows, err := s.queries.GetEventsSince(c.Request.Context(), since)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "イベント取得に失敗しました"})
			log.Printf("イベント取得エラー: %v", err)
			return
		}

		c.JSON(http.StatusOK, toEventResponses(rows))
	}
}

// handleGetLatestVersion はAggregateIDの最新バージョン取得を処理するハンドラを返す。
func (s *Server) handleGetLatestVersion() gin.HandlerFunc {
	return func(c *gin.Context) {
		aggregateID := c.Param("aggregate_id")

		latestVersionRaw, err := s.queries.GetLatestVersion(c.Request.Context(), aggregateID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "バージョン取得に失敗しました"})
			log.Printf("バージョン取得エラー: %v", err)
			return
		}

		var version int64
		switch v := latestVersionRaw.(type) {
		case int64:
			version = v
		case float64:
			version = int64(v)
		}

		c.JSON(http.StatusOK, gin.H{
			"aggregate_id":   aggregateID,
			"latest_version": version,
		})
	}
}

// handleGetAllEvents は全イベント取得を処理するハンドラを返す。
func (s *Server) handleGetAllEvents() gin.HandlerFunc {
	return func(c *gin.Context) {
		rows, err := s.queries.GetAllEvents(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "イベント取得に失敗しました"})
			log.Printf("イベント取得エラー: %v", err)
			return
		}

		c.JSON(http.StatusOK, toEventResponses(rows))
	}
}

// toEventResponse はDB行をJSONレスポンスに変換する。
func toEventResponse(id, aggregateID, aggregateType, eventType, data string, version int64, createdAt time.Time) eventResponse {
	return eventResponse{
		ID:            id,
		AggregateID:   aggregateID,
		AggregateType: aggregateType,
		EventType:     eventType,
		Data:          data,
		Version:       version,
		CreatedAt:     createdAt.Format(time.RFC3339),
	}
}

// toEventResponses はDB行のスライスをJSONレスポンスのスライスに変換する。
func toEventResponses(rows []eventstoredb.Event) []eventResponse {
	responses := make([]eventResponse, 0, len(rows))
	for _, row := range rows {
		responses = append(responses, toEventResponse(
			row.ID, row.AggregateID, row.AggregateType,
			row.EventType, row.Data, row.Version, row.CreatedAt,
		))
	}
	return responses
}
