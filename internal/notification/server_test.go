package notification

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	_ "modernc.org/sqlite"
	notificationdb "github.com/nao1215/micro/internal/notification/db"
	"github.com/nao1215/micro/pkg/httpclient"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// setupTestServer はテスト用の通知サーバーをインメモリSQLiteで構築する。
// Event Storeのモックサーバーも生成し、テスト終了時にクリーンアップする。
func setupTestServer(t *testing.T) (*Server, *gin.Engine) {
	t.Helper()

	sqlDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("インメモリDBの作成に失敗: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })

	if err := initSchema(sqlDB); err != nil {
		t.Fatalf("スキーマ初期化に失敗: %v", err)
	}

	// Event Storeのモックサーバーを作成する
	eventStore := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		fmt.Fprint(w, `{"id":"mock-event-id"}`)
	}))
	t.Cleanup(func() { eventStore.Close() })

	router := gin.New()
	s := &Server{
		router:           router,
		port:             "0",
		queries:          notificationdb.New(sqlDB),
		db:               sqlDB,
		eventStoreClient: httpclient.New(eventStore.URL),
	}

	// JWTミドルウェアの代わりにテスト用のユーザーID設定ミドルウェアを使用する
	api := router.Group("/api/v1")
	api.Use(func(c *gin.Context) {
		userID := c.GetHeader("X-User-ID")
		if userID != "" {
			c.Set("user_id", userID)
		}
		c.Next()
	})
	{
		notifications := api.Group("/notifications")
		{
			notifications.GET("", s.handleList())
			notifications.GET("/unread", s.handleListUnread())
			notifications.PUT("/:id/read", s.handleMarkAsRead())
			notifications.PUT("/read-all", s.handleMarkAllAsRead())
		}

		internal := api.Group("/internal")
		{
			internal.POST("/send", s.handleSend())
		}
	}
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "notification"})
	})

	return s, router
}

// createTestNotification はテスト用に通知をDBに直接挿入するヘルパー関数。
func createTestNotification(t *testing.T, s *Server, id, userID, title, message string) {
	t.Helper()
	err := s.queries.CreateNotification(
		t.Context(),
		notificationdb.CreateNotificationParams{
			ID:      id,
			UserID:  userID,
			Title:   title,
			Message: message,
		},
	)
	if err != nil {
		t.Fatalf("テスト用通知の作成に失敗: %v", err)
	}
}

// doRequest はテスト用のHTTPリクエストを実行し、レスポンスを返すヘルパー関数。
func doRequest(router *gin.Engine, method, path, userID string, body any) *httptest.ResponseRecorder {
	var reqBody *bytes.Reader
	if body != nil {
		jsonBytes, _ := json.Marshal(body)
		reqBody = bytes.NewReader(jsonBytes)
	} else {
		reqBody = bytes.NewReader(nil)
	}

	req := httptest.NewRequest(method, path, reqBody)
	req.Header.Set("Content-Type", "application/json")
	if userID != "" {
		req.Header.Set("X-User-ID", userID)
	}

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// parseJSON はレスポンスボディをmapにデコードするヘルパー関数。
func parseJSON(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var result map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("JSONのデコードに失敗: %v, body=%s", err, w.Body.String())
	}
	return result
}

// parseJSONArray はレスポンスボディをスライスにデコードするヘルパー関数。
func parseJSONArray(t *testing.T, w *httptest.ResponseRecorder) []map[string]any {
	t.Helper()
	var result []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("JSON配列のデコードに失敗: %v, body=%s", err, w.Body.String())
	}
	return result
}

// TestHealthCheck はヘルスチェックエンドポイントの正常動作を検証する。
func TestHealthCheck(t *testing.T) {
	t.Parallel()

	_, router := setupTestServer(t)

	w := doRequest(router, http.MethodGet, "/health", "", nil)

	if w.Code != http.StatusOK {
		t.Errorf("ステータスコード: got %d, want %d", w.Code, http.StatusOK)
	}

	result := parseJSON(t, w)
	if result["status"] != "ok" {
		t.Errorf("status: got %v, want ok", result["status"])
	}
	if result["service"] != "notification" {
		t.Errorf("service: got %v, want notification", result["service"])
	}
}

// TestHandleListNotifications は通知一覧取得ハンドラのテスト。
func TestHandleListNotifications(t *testing.T) {
	t.Parallel()

	t.Run("通知が存在しない場合は空配列を返す", func(t *testing.T) {
		t.Parallel()
		_, router := setupTestServer(t)

		w := doRequest(router, http.MethodGet, "/api/v1/notifications", "user-1", nil)

		if w.Code != http.StatusOK {
			t.Errorf("ステータスコード: got %d, want %d", w.Code, http.StatusOK)
		}

		result := parseJSONArray(t, w)
		if len(result) != 0 {
			t.Errorf("配列の長さ: got %d, want 0", len(result))
		}
	})

	t.Run("作成済み通知の一覧を取得できる", func(t *testing.T) {
		t.Parallel()
		s, router := setupTestServer(t)

		createTestNotification(t, s, "notif-1", "user-1", "タイトル1", "メッセージ1")
		createTestNotification(t, s, "notif-2", "user-1", "タイトル2", "メッセージ2")
		// 別ユーザーの通知は含まれないことを確認するため
		createTestNotification(t, s, "notif-3", "user-2", "他ユーザー", "他ユーザーのメッセージ")

		w := doRequest(router, http.MethodGet, "/api/v1/notifications", "user-1", nil)

		if w.Code != http.StatusOK {
			t.Errorf("ステータスコード: got %d, want %d", w.Code, http.StatusOK)
		}

		result := parseJSONArray(t, w)
		if len(result) != 2 {
			t.Errorf("配列の長さ: got %d, want 2", len(result))
		}
	})

	t.Run("通知のフィールドが正しく返される", func(t *testing.T) {
		t.Parallel()
		s, router := setupTestServer(t)

		createTestNotification(t, s, "notif-1", "user-1", "テストタイトル", "テストメッセージ")

		w := doRequest(router, http.MethodGet, "/api/v1/notifications", "user-1", nil)

		if w.Code != http.StatusOK {
			t.Errorf("ステータスコード: got %d, want %d", w.Code, http.StatusOK)
		}

		result := parseJSONArray(t, w)
		if len(result) != 1 {
			t.Fatalf("配列の長さ: got %d, want 1", len(result))
		}

		notif := result[0]
		if notif["id"] != "notif-1" {
			t.Errorf("id: got %v, want notif-1", notif["id"])
		}
		if notif["user_id"] != "user-1" {
			t.Errorf("user_id: got %v, want user-1", notif["user_id"])
		}
		if notif["title"] != "テストタイトル" {
			t.Errorf("title: got %v, want テストタイトル", notif["title"])
		}
		if notif["message"] != "テストメッセージ" {
			t.Errorf("message: got %v, want テストメッセージ", notif["message"])
		}
		if notif["is_read"] != false {
			t.Errorf("is_read: got %v, want false", notif["is_read"])
		}
	})

	t.Run("ユーザーIDが未設定の場合はUnauthorized", func(t *testing.T) {
		t.Parallel()
		_, router := setupTestServer(t)

		w := doRequest(router, http.MethodGet, "/api/v1/notifications", "", nil)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("ステータスコード: got %d, want %d", w.Code, http.StatusUnauthorized)
		}
	})
}

// TestHandleListUnread は未読通知一覧取得ハンドラのテスト。
func TestHandleListUnread(t *testing.T) {
	t.Parallel()

	t.Run("未読通知のみを返す", func(t *testing.T) {
		t.Parallel()
		s, router := setupTestServer(t)

		createTestNotification(t, s, "notif-1", "user-1", "未読1", "メッセージ1")
		createTestNotification(t, s, "notif-2", "user-1", "未読2", "メッセージ2")
		createTestNotification(t, s, "notif-3", "user-1", "既読", "メッセージ3")

		// notif-3を既読にする
		err := s.queries.MarkAsRead(t.Context(), "notif-3")
		if err != nil {
			t.Fatalf("既読処理に失敗: %v", err)
		}

		w := doRequest(router, http.MethodGet, "/api/v1/notifications/unread", "user-1", nil)

		if w.Code != http.StatusOK {
			t.Errorf("ステータスコード: got %d, want %d", w.Code, http.StatusOK)
		}

		result := parseJSONArray(t, w)
		if len(result) != 2 {
			t.Errorf("配列の長さ: got %d, want 2", len(result))
		}
	})

	t.Run("未読通知がない場合は空配列を返す", func(t *testing.T) {
		t.Parallel()
		s, router := setupTestServer(t)

		createTestNotification(t, s, "notif-1", "user-1", "既読", "メッセージ")
		err := s.queries.MarkAsRead(t.Context(), "notif-1")
		if err != nil {
			t.Fatalf("既読処理に失敗: %v", err)
		}

		w := doRequest(router, http.MethodGet, "/api/v1/notifications/unread", "user-1", nil)

		if w.Code != http.StatusOK {
			t.Errorf("ステータスコード: got %d, want %d", w.Code, http.StatusOK)
		}

		result := parseJSONArray(t, w)
		if len(result) != 0 {
			t.Errorf("配列の長さ: got %d, want 0", len(result))
		}
	})

	t.Run("ユーザーIDが未設定の場合はUnauthorized", func(t *testing.T) {
		t.Parallel()
		_, router := setupTestServer(t)

		w := doRequest(router, http.MethodGet, "/api/v1/notifications/unread", "", nil)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("ステータスコード: got %d, want %d", w.Code, http.StatusUnauthorized)
		}
	})
}

// TestHandleMarkRead は通知を既読にするハンドラのテスト。
func TestHandleMarkRead(t *testing.T) {
	t.Parallel()

	t.Run("正常に通知を既読にできる", func(t *testing.T) {
		t.Parallel()
		s, router := setupTestServer(t)

		createTestNotification(t, s, "notif-1", "user-1", "テスト", "メッセージ")

		w := doRequest(router, http.MethodPut, "/api/v1/notifications/notif-1/read", "user-1", nil)

		if w.Code != http.StatusOK {
			t.Errorf("ステータスコード: got %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
		}

		result := parseJSON(t, w)
		if result["message"] == nil {
			t.Error("messageが含まれていません")
		}

		// 既読になったことを未読一覧で確認する
		w2 := doRequest(router, http.MethodGet, "/api/v1/notifications/unread", "user-1", nil)
		unread := parseJSONArray(t, w2)
		if len(unread) != 0 {
			t.Errorf("未読通知の数: got %d, want 0", len(unread))
		}
	})

	t.Run("存在しない通知の場合はNotFound", func(t *testing.T) {
		t.Parallel()
		_, router := setupTestServer(t)

		w := doRequest(router, http.MethodPut, "/api/v1/notifications/nonexistent/read", "user-1", nil)

		if w.Code != http.StatusNotFound {
			t.Errorf("ステータスコード: got %d, want %d", w.Code, http.StatusNotFound)
		}
	})

	t.Run("他ユーザーの通知を既読にするとForbidden", func(t *testing.T) {
		t.Parallel()
		s, router := setupTestServer(t)

		createTestNotification(t, s, "notif-1", "user-1", "ユーザー1の通知", "メッセージ")

		w := doRequest(router, http.MethodPut, "/api/v1/notifications/notif-1/read", "user-2", nil)

		if w.Code != http.StatusForbidden {
			t.Errorf("ステータスコード: got %d, want %d", w.Code, http.StatusForbidden)
		}
	})

	t.Run("ユーザーIDが未設定の場合はUnauthorized", func(t *testing.T) {
		t.Parallel()
		_, router := setupTestServer(t)

		w := doRequest(router, http.MethodPut, "/api/v1/notifications/notif-1/read", "", nil)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("ステータスコード: got %d, want %d", w.Code, http.StatusUnauthorized)
		}
	})
}

// TestHandleMarkAllRead は全通知を既読にするハンドラのテスト。
func TestHandleMarkAllRead(t *testing.T) {
	t.Parallel()

	t.Run("正常に全通知を既読にできる", func(t *testing.T) {
		t.Parallel()
		s, router := setupTestServer(t)

		createTestNotification(t, s, "notif-1", "user-1", "通知1", "メッセージ1")
		createTestNotification(t, s, "notif-2", "user-1", "通知2", "メッセージ2")
		createTestNotification(t, s, "notif-3", "user-1", "通知3", "メッセージ3")

		w := doRequest(router, http.MethodPut, "/api/v1/notifications/read-all", "user-1", nil)

		if w.Code != http.StatusOK {
			t.Errorf("ステータスコード: got %d, want %d", w.Code, http.StatusOK)
		}

		result := parseJSON(t, w)
		if result["message"] == nil {
			t.Error("messageが含まれていません")
		}

		// 全て既読になったことを未読一覧で確認する
		w2 := doRequest(router, http.MethodGet, "/api/v1/notifications/unread", "user-1", nil)
		unread := parseJSONArray(t, w2)
		if len(unread) != 0 {
			t.Errorf("未読通知の数: got %d, want 0", len(unread))
		}
	})

	t.Run("他ユーザーの通知は既読にならない", func(t *testing.T) {
		t.Parallel()
		s, router := setupTestServer(t)

		createTestNotification(t, s, "notif-1", "user-1", "ユーザー1の通知", "メッセージ")
		createTestNotification(t, s, "notif-2", "user-2", "ユーザー2の通知", "メッセージ")

		// user-1の全通知を既読にする
		w := doRequest(router, http.MethodPut, "/api/v1/notifications/read-all", "user-1", nil)
		if w.Code != http.StatusOK {
			t.Errorf("ステータスコード: got %d, want %d", w.Code, http.StatusOK)
		}

		// user-2の未読通知は残っていることを確認する
		w2 := doRequest(router, http.MethodGet, "/api/v1/notifications/unread", "user-2", nil)
		unread := parseJSONArray(t, w2)
		if len(unread) != 1 {
			t.Errorf("user-2の未読通知の数: got %d, want 1", len(unread))
		}
	})

	t.Run("通知が存在しない場合でも成功する", func(t *testing.T) {
		t.Parallel()
		_, router := setupTestServer(t)

		w := doRequest(router, http.MethodPut, "/api/v1/notifications/read-all", "user-1", nil)

		if w.Code != http.StatusOK {
			t.Errorf("ステータスコード: got %d, want %d", w.Code, http.StatusOK)
		}
	})

	t.Run("ユーザーIDが未設定の場合はUnauthorized", func(t *testing.T) {
		t.Parallel()
		_, router := setupTestServer(t)

		w := doRequest(router, http.MethodPut, "/api/v1/notifications/read-all", "", nil)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("ステータスコード: got %d, want %d", w.Code, http.StatusUnauthorized)
		}
	})
}

// TestHandleSend は通知送信（内部API）ハンドラのテスト。
func TestHandleSend(t *testing.T) {
	t.Parallel()

	t.Run("正常に通知を送信できる", func(t *testing.T) {
		t.Parallel()
		_, router := setupTestServer(t)

		body := map[string]string{
			"user_id": "user-1",
			"title":   "アップロード完了",
			"message": "メディアのアップロードが完了しました",
		}
		w := doRequest(router, http.MethodPost, "/api/v1/internal/send", "system", body)

		if w.Code != http.StatusCreated {
			t.Errorf("ステータスコード: got %d, want %d, body=%s", w.Code, http.StatusCreated, w.Body.String())
		}

		result := parseJSON(t, w)
		if result["id"] == nil || result["id"] == "" {
			t.Error("idが空です")
		}
		if result["message"] == nil {
			t.Error("messageが含まれていません")
		}

		// 送信された通知が一覧に含まれることを確認する
		w2 := doRequest(router, http.MethodGet, "/api/v1/notifications", "user-1", nil)
		notifications := parseJSONArray(t, w2)
		if len(notifications) != 1 {
			t.Fatalf("通知の数: got %d, want 1", len(notifications))
		}
		if notifications[0]["title"] != "アップロード完了" {
			t.Errorf("title: got %v, want アップロード完了", notifications[0]["title"])
		}
	})

	t.Run("user_idが未指定の場合はBadRequest", func(t *testing.T) {
		t.Parallel()
		_, router := setupTestServer(t)

		body := map[string]string{
			"title":   "テスト",
			"message": "メッセージ",
		}
		w := doRequest(router, http.MethodPost, "/api/v1/internal/send", "system", body)

		if w.Code != http.StatusBadRequest {
			t.Errorf("ステータスコード: got %d, want %d", w.Code, http.StatusBadRequest)
		}
	})

	t.Run("titleが未指定の場合はBadRequest", func(t *testing.T) {
		t.Parallel()
		_, router := setupTestServer(t)

		body := map[string]string{
			"user_id": "user-1",
			"message": "メッセージ",
		}
		w := doRequest(router, http.MethodPost, "/api/v1/internal/send", "system", body)

		if w.Code != http.StatusBadRequest {
			t.Errorf("ステータスコード: got %d, want %d", w.Code, http.StatusBadRequest)
		}
	})

	t.Run("messageが未指定の場合はBadRequest", func(t *testing.T) {
		t.Parallel()
		_, router := setupTestServer(t)

		body := map[string]string{
			"user_id": "user-1",
			"title":   "テスト",
		}
		w := doRequest(router, http.MethodPost, "/api/v1/internal/send", "system", body)

		if w.Code != http.StatusBadRequest {
			t.Errorf("ステータスコード: got %d, want %d", w.Code, http.StatusBadRequest)
		}
	})

	t.Run("全フィールドが未指定の場合はBadRequest", func(t *testing.T) {
		t.Parallel()
		_, router := setupTestServer(t)

		body := map[string]string{}
		w := doRequest(router, http.MethodPost, "/api/v1/internal/send", "system", body)

		if w.Code != http.StatusBadRequest {
			t.Errorf("ステータスコード: got %d, want %d", w.Code, http.StatusBadRequest)
		}
	})

	t.Run("複数通知を送信し一覧で確認できる", func(t *testing.T) {
		t.Parallel()
		_, router := setupTestServer(t)

		for i := 0; i < 3; i++ {
			body := map[string]string{
				"user_id": "user-1",
				"title":   fmt.Sprintf("通知%d", i),
				"message": fmt.Sprintf("メッセージ%d", i),
			}
			w := doRequest(router, http.MethodPost, "/api/v1/internal/send", "system", body)
			if w.Code != http.StatusCreated {
				t.Fatalf("通知%d の送信に失敗: status=%d", i, w.Code)
			}
		}

		w := doRequest(router, http.MethodGet, "/api/v1/notifications", "user-1", nil)
		notifications := parseJSONArray(t, w)
		if len(notifications) != 3 {
			t.Errorf("通知の数: got %d, want 3", len(notifications))
		}
	})
}

// TestSendAndMarkReadFlow は通知送信から既読までの一連のフローを検証する。
func TestSendAndMarkReadFlow(t *testing.T) {
	t.Parallel()

	_, router := setupTestServer(t)

	// 通知を送信する
	sendBody := map[string]string{
		"user_id": "user-1",
		"title":   "フローテスト",
		"message": "統合テストメッセージ",
	}
	w := doRequest(router, http.MethodPost, "/api/v1/internal/send", "system", sendBody)
	if w.Code != http.StatusCreated {
		t.Fatalf("通知送信に失敗: status=%d, body=%s", w.Code, w.Body.String())
	}

	sendResult := parseJSON(t, w)
	notifID, ok := sendResult["id"].(string)
	if !ok || notifID == "" {
		t.Fatal("送信結果にidが含まれていません")
	}

	// 未読一覧に含まれることを確認する
	w2 := doRequest(router, http.MethodGet, "/api/v1/notifications/unread", "user-1", nil)
	unread := parseJSONArray(t, w2)
	if len(unread) != 1 {
		t.Fatalf("未読通知の数: got %d, want 1", len(unread))
	}

	// 既読にする
	w3 := doRequest(router, http.MethodPut, fmt.Sprintf("/api/v1/notifications/%s/read", notifID), "user-1", nil)
	if w3.Code != http.StatusOK {
		t.Errorf("既読処理のステータスコード: got %d, want %d", w3.Code, http.StatusOK)
	}

	// 未読一覧が空になったことを確認する
	w4 := doRequest(router, http.MethodGet, "/api/v1/notifications/unread", "user-1", nil)
	unreadAfter := parseJSONArray(t, w4)
	if len(unreadAfter) != 0 {
		t.Errorf("既読後の未読通知の数: got %d, want 0", len(unreadAfter))
	}

	// 全通知一覧には引き続き含まれることを確認する
	w5 := doRequest(router, http.MethodGet, "/api/v1/notifications", "user-1", nil)
	allNotifs := parseJSONArray(t, w5)
	if len(allNotifs) != 1 {
		t.Errorf("全通知の数: got %d, want 1", len(allNotifs))
	}
	if allNotifs[0]["is_read"] != true {
		t.Errorf("is_read: got %v, want true", allNotifs[0]["is_read"])
	}
}
