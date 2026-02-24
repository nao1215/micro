package notification

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	_ "modernc.org/sqlite"
	notificationdb "github.com/nao1215/micro/internal/notification/db"
	"github.com/nao1215/micro/pkg/event"
	"github.com/nao1215/micro/pkg/httpclient"
	"github.com/nao1215/micro/pkg/middleware"
)

// Server は通知サービスのHTTPサーバー。
type Server struct {
	// router はGinのHTTPルーター。
	router *gin.Engine
	// port はサーバーのリッスンポート。
	port string
	// queries はsqlcが生成したクエリ実行オブジェクト。
	queries *notificationdb.Queries
	// db はSQLiteデータベース接続。
	db *sql.DB
	// eventStoreClient はEvent Storeサービスへの通信クライアント。
	eventStoreClient *httpclient.Client
}

// NewServer は新しい通知サーバーを生成する。
// SQLiteデータベースの初期化とスキーマ作成を行う。
func NewServer(port string) (*Server, error) {
	sqlDB, err := sql.Open("sqlite", "/data/notification.db?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("データベース接続に失敗: %w", err)
	}

	if err := initSchema(sqlDB); err != nil {
		return nil, fmt.Errorf("スキーマ初期化に失敗: %w", err)
	}

	eventStoreURL := os.Getenv("EVENTSTORE_URL")
	if eventStoreURL == "" {
		eventStoreURL = "http://localhost:8084"
	}

	router := gin.New()
	router.Use(middleware.Recovery())
	router.Use(gin.Logger())

	s := &Server{
		router:           router,
		port:             port,
		queries:          notificationdb.New(sqlDB),
		db:               sqlDB,
		eventStoreClient: httpclient.New(eventStoreURL),
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
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "notification"})
	})
}

// notificationResponse は通知のJSONレスポンス構造。
type notificationResponse struct {
	// ID は通知の一意識別子。
	ID string `json:"id"`
	// UserID は通知先のユーザーID。
	UserID string `json:"user_id"`
	// Title は通知のタイトル。
	Title string `json:"title"`
	// Message は通知メッセージ。
	Message string `json:"message"`
	// IsRead は通知の既読状態。
	IsRead bool `json:"is_read"`
	// CreatedAt は通知の作成日時（RFC3339形式）。
	CreatedAt string `json:"created_at"`
}

// toNotificationResponse はDB行をJSONレスポンスに変換する。
func toNotificationResponse(n notificationdb.Notification) notificationResponse {
	return notificationResponse{
		ID:        n.ID,
		UserID:    n.UserID,
		Title:     n.Title,
		Message:   n.Message,
		IsRead:    n.IsRead != 0,
		CreatedAt: n.CreatedAt.Format(time.RFC3339),
	}
}

// toNotificationResponses はDB行のスライスをJSONレスポンスのスライスに変換する。
func toNotificationResponses(notifications []notificationdb.Notification) []notificationResponse {
	responses := make([]notificationResponse, 0, len(notifications))
	for _, n := range notifications {
		responses = append(responses, toNotificationResponse(n))
	}
	return responses
}

// handleList は認証済みユーザーの通知一覧を返すハンドラ。
func (s *Server) handleList() gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := middleware.GetUserID(c)
		if userID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "ユーザーIDが取得できません"})
			return
		}

		notifications, err := s.queries.ListNotificationsByUserID(c.Request.Context(), userID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "通知一覧の取得に失敗しました"})
			log.Printf("通知一覧取得エラー: %v", err)
			return
		}

		c.JSON(http.StatusOK, toNotificationResponses(notifications))
	}
}

// handleListUnread は認証済みユーザーの未読通知一覧を返すハンドラ。
func (s *Server) handleListUnread() gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := middleware.GetUserID(c)
		if userID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "ユーザーIDが取得できません"})
			return
		}

		notifications, err := s.queries.ListUnreadNotifications(c.Request.Context(), userID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "未読通知一覧の取得に失敗しました"})
			log.Printf("未読通知一覧取得エラー: %v", err)
			return
		}

		c.JSON(http.StatusOK, toNotificationResponses(notifications))
	}
}

// handleMarkAsRead は指定された通知を既読にするハンドラ。
func (s *Server) handleMarkAsRead() gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := middleware.GetUserID(c)
		if userID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "ユーザーIDが取得できません"})
			return
		}

		notificationID := c.Param("id")
		if notificationID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "通知IDが必要です"})
			return
		}

		// 通知の存在確認と所有者チェック
		n, err := s.queries.GetNotificationByID(c.Request.Context(), notificationID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "通知が見つかりません"})
			log.Printf("通知取得エラー: %v", err)
			return
		}

		if n.UserID != userID {
			c.JSON(http.StatusForbidden, gin.H{"error": "この通知を操作する権限がありません"})
			return
		}

		if err := s.queries.MarkAsRead(c.Request.Context(), notificationID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "通知の既読処理に失敗しました"})
			log.Printf("通知既読処理エラー: %v", err)
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "通知を既読にしました"})
	}
}

// handleMarkAllAsRead は認証済みユーザーの全通知を既読にするハンドラ。
func (s *Server) handleMarkAllAsRead() gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := middleware.GetUserID(c)
		if userID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "ユーザーIDが取得できません"})
			return
		}

		if err := s.queries.MarkAllAsRead(c.Request.Context(), userID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "全通知の既読処理に失敗しました"})
			log.Printf("全通知既読処理エラー: %v", err)
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "全通知を既読にしました"})
	}
}

// sendRequest は通知送信リクエストのJSON構造。
type sendRequest struct {
	// UserID は通知先のユーザーID。
	UserID string `json:"user_id" binding:"required"`
	// Title は通知のタイトル。
	Title string `json:"title" binding:"required"`
	// Message は通知メッセージ。
	Message string `json:"message" binding:"required"`
}

// appendEventRequest はEvent Storeへのイベント追記リクエストのJSON構造。
type appendEventRequest struct {
	// AggregateID は対象エンティティの識別子。
	AggregateID string `json:"aggregate_id"`
	// AggregateType は対象エンティティの種類。
	AggregateType string `json:"aggregate_type"`
	// EventType はイベントの種類。
	EventType string `json:"event_type"`
	// Data はイベント固有のデータ（JSON形式）。
	Data json.RawMessage `json:"data"`
}

// handleSend は通知を作成しNotificationSentイベントを発行するハンドラ。
// 内部API（Sagaオーケストレーターから呼び出される）。
func (s *Server) handleSend() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req sendRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("リクエストが不正です: %v", err)})
			return
		}

		notificationID := uuid.New().String()

		// 通知をデータベースに保存
		if err := s.queries.CreateNotification(c.Request.Context(), notificationdb.CreateNotificationParams{
			ID:      notificationID,
			UserID:  req.UserID,
			Title:   req.Title,
			Message: req.Message,
		}); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "通知の作成に失敗しました"})
			log.Printf("通知作成エラー: %v", err)
			return
		}

		// NotificationSentイベントをEvent Storeに送信
		eventData := event.NotificationSentData{
			UserID:  req.UserID,
			Title:   req.Title,
			Message: req.Message,
		}

		jsonData, err := json.Marshal(eventData)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "イベントデータのシリアライズに失敗しました"})
			log.Printf("イベントデータシリアライズエラー: %v", err)
			return
		}

		aggregateID := fmt.Sprintf("notification-%s", notificationID)
		eventReq := appendEventRequest{
			AggregateID:   aggregateID,
			AggregateType: string(event.AggregateTypeUser),
			EventType:     string(event.TypeNotificationSent),
			Data:          jsonData,
		}

		var eventResp map[string]any
		if err := s.eventStoreClient.PostJSON(c.Request.Context(), "/api/v1/events", eventReq, &eventResp); err != nil {
			// イベント送信に失敗してもログに記録し、通知自体は成功として扱う
			log.Printf("NotificationSentイベントの送信に失敗: %v", err)
		}

		c.JSON(http.StatusCreated, gin.H{
			"id":      notificationID,
			"message": "通知を送信しました",
		})
	}
}
