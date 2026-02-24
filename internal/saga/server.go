package saga

import (
	"database/sql"
	"fmt"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	_ "github.com/mattn/go-sqlite3"
	sagadb "github.com/nao1215/micro/internal/saga/db"
	"github.com/nao1215/micro/pkg/httpclient"
	"github.com/nao1215/micro/pkg/middleware"
)

// Server はSagaオーケストレータサービスのHTTPサーバー。
type Server struct {
	// router はGinのHTTPルーター。
	router *gin.Engine
	// port はサーバーのリッスンポート。
	port string
	// queries はsqlcが生成したクエリ実行オブジェクト。
	queries *sagadb.Queries
	// db はSQLiteデータベース接続。
	db *sql.DB
	// orchestrator はSagaオーケストレータ。イベントポーリングとSaga実行を管理する。
	orchestrator *Orchestrator
}

// NewServer は新しいSagaサーバーを生成する。
func NewServer(port string) (*Server, error) {
	sqlDB, err := sql.Open("sqlite3", "/data/saga.db?_journal_mode=WAL&_busy_timeout=5000")
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
	mediaCommandURL := os.Getenv("MEDIA_COMMAND_URL")
	if mediaCommandURL == "" {
		mediaCommandURL = "http://localhost:8081"
	}
	albumURL := os.Getenv("ALBUM_URL")
	if albumURL == "" {
		albumURL = "http://localhost:8083"
	}
	notificationURL := os.Getenv("NOTIFICATION_URL")
	if notificationURL == "" {
		notificationURL = "http://localhost:8086"
	}

	queries := sagadb.New(sqlDB)

	orch := NewOrchestrator(
		queries,
		httpclient.New(eventstoreURL),
		httpclient.New(mediaCommandURL),
		httpclient.New(albumURL),
		httpclient.New(notificationURL),
	)
	go orch.Start()

	router := gin.New()
	router.Use(middleware.Recovery())
	router.Use(gin.Logger())

	s := &Server{
		router:       router,
		port:         port,
		queries:      queries,
		db:           sqlDB,
		orchestrator: orch,
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
	// Saga管理API（認証不要 - 内部ネットワーク）
	api := s.router.Group("/api/v1")
	{
		sagas := api.Group("/sagas")
		{
			// アクティブなSaga一覧取得
			sagas.GET("", s.handleListActive())
			// Saga詳細取得（ステップ履歴含む）
			sagas.GET("/:id", s.handleGetByID())
		}

		// イベント受信（イベントポーリングの代替として手動通知も受け付ける）
		events := api.Group("/events")
		{
			events.POST("/notify", s.handleEventNotify())
		}
	}

	// ヘルスチェック
	s.router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "saga"})
	})
}

// sagaResponse はSagaのJSONレスポンス構造。
type sagaResponse struct {
	ID          string             `json:"id"`
	SagaType    string             `json:"saga_type"`
	CurrentStep string             `json:"current_step"`
	Status      string             `json:"status"`
	Payload     string             `json:"payload"`
	StartedAt   string             `json:"started_at"`
	UpdatedAt   string             `json:"updated_at"`
	CompletedAt *string            `json:"completed_at,omitempty"`
	Steps       []sagaStepResponse `json:"steps,omitempty"`
}

// sagaStepResponse はSagaステップのJSONレスポンス構造。
type sagaStepResponse struct {
	ID          string  `json:"id"`
	StepName    string  `json:"step_name"`
	Status      string  `json:"status"`
	Result      string  `json:"result"`
	StartedAt   *string `json:"started_at,omitempty"`
	CompletedAt *string `json:"completed_at,omitempty"`
}

// handleListActive はアクティブなSaga一覧を返すハンドラ。
func (s *Server) handleListActive() gin.HandlerFunc {
	return func(c *gin.Context) {
		sagas, err := s.queries.ListActiveSagas(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Saga一覧の取得に失敗しました"})
			return
		}

		responses := make([]sagaResponse, 0, len(sagas))
		for _, saga := range sagas {
			resp := sagaResponse{
				ID:          saga.ID,
				SagaType:    saga.SagaType,
				CurrentStep: saga.CurrentStep,
				Status:      saga.Status,
				Payload:     saga.Payload,
				StartedAt:   saga.StartedAt.Format("2006-01-02T15:04:05Z"),
				UpdatedAt:   saga.UpdatedAt.Format("2006-01-02T15:04:05Z"),
			}
			if saga.CompletedAt.Valid {
				t := saga.CompletedAt.Time.Format("2006-01-02T15:04:05Z")
				resp.CompletedAt = &t
			}
			responses = append(responses, resp)
		}

		c.JSON(http.StatusOK, responses)
	}
}

// handleGetByID はSaga詳細（ステップ履歴含む）を返すハンドラ。
func (s *Server) handleGetByID() gin.HandlerFunc {
	return func(c *gin.Context) {
		sagaID := c.Param("id")

		saga, err := s.queries.GetSagaByID(c.Request.Context(), sagaID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Sagaが見つかりません"})
			return
		}

		steps, err := s.queries.ListSagaSteps(c.Request.Context(), sagaID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Sagaステップの取得に失敗しました"})
			return
		}

		resp := sagaResponse{
			ID:          saga.ID,
			SagaType:    saga.SagaType,
			CurrentStep: saga.CurrentStep,
			Status:      saga.Status,
			Payload:     saga.Payload,
			StartedAt:   saga.StartedAt.Format("2006-01-02T15:04:05Z"),
			UpdatedAt:   saga.UpdatedAt.Format("2006-01-02T15:04:05Z"),
		}
		if saga.CompletedAt.Valid {
			t := saga.CompletedAt.Time.Format("2006-01-02T15:04:05Z")
			resp.CompletedAt = &t
		}

		resp.Steps = make([]sagaStepResponse, 0, len(steps))
		for _, step := range steps {
			sr := sagaStepResponse{
				ID:       step.ID,
				StepName: step.StepName,
				Status:   step.Status,
				Result:   step.Result,
			}
			if step.StartedAt.Valid {
				t := step.StartedAt.Time.Format("2006-01-02T15:04:05Z")
				sr.StartedAt = &t
			}
			if step.CompletedAt.Valid {
				t := step.CompletedAt.Time.Format("2006-01-02T15:04:05Z")
				sr.CompletedAt = &t
			}
			resp.Steps = append(resp.Steps, sr)
		}

		c.JSON(http.StatusOK, resp)
	}
}

// eventNotifyRequest はイベント通知リクエストの構造。
type eventNotifyRequest struct {
	EventType   string `json:"event_type" binding:"required"`
	AggregateID string `json:"aggregate_id" binding:"required"`
	Data        string `json:"data"`
}

// handleEventNotify はイベント通知を受け取り、該当するSagaを進行させるハンドラ。
func (s *Server) handleEventNotify() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req eventNotifyRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("リクエストが不正です: %v", err)})
			return
		}

		s.orchestrator.HandleEvent(c.Request.Context(), req.EventType, req.AggregateID, req.Data)

		c.JSON(http.StatusOK, gin.H{"status": "accepted"})
	}
}
