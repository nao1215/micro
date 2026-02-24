package saga

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	_ "modernc.org/sqlite"
	sagadb "github.com/nao1215/micro/internal/saga/db"
	"github.com/nao1215/micro/pkg/httpclient"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// newTestServer はテスト用のSagaサーバーを生成する。
// インメモリSQLiteを使用し、オーケストレータのバックグラウンドgoroutineは起動しない。
func newTestServer(t *testing.T) *Server {
	t.Helper()

	sqlDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("インメモリDB接続に失敗: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })

	if err := initSchema(sqlDB); err != nil {
		t.Fatalf("スキーマ初期化に失敗: %v", err)
	}

	queries := sagadb.New(sqlDB)

	// ダミーのオーケストレータを生成（バックグラウンドgoroutineは起動しない）
	orch := NewOrchestrator(
		queries,
		httpclient.New("http://localhost:19001"),
		httpclient.New("http://localhost:19002"),
		httpclient.New("http://localhost:19003"),
		httpclient.New("http://localhost:19004"),
	)

	router := gin.New()
	s := &Server{
		router:       router,
		port:         "0",
		queries:      queries,
		db:           sqlDB,
		orchestrator: orch,
	}
	s.setupRoutes()

	return s
}

// seedSaga はテスト用のSagaレコードをDBに挿入する。
func seedSaga(t *testing.T, s *Server, id, sagaType, currentStep, status, payload string) {
	t.Helper()

	ctx := context.Background()
	if err := s.queries.CreateSaga(ctx, sagadb.CreateSagaParams{
		ID:          id,
		SagaType:    sagaType,
		CurrentStep: currentStep,
		Payload:     payload,
	}); err != nil {
		t.Fatalf("テスト用Saga挿入に失敗: %v", err)
	}

	// status が "started" 以外の場合は更新する
	if status != "started" {
		if err := s.queries.UpdateSagaStep(ctx, sagadb.UpdateSagaStepParams{
			CurrentStep: currentStep,
			Status:      status,
			Payload:     payload,
			ID:          id,
		}); err != nil {
			t.Fatalf("テスト用Sagaステータス更新に失敗: %v", err)
		}
	}
}

// seedSagaStep はテスト用のSagaステップレコードをDBに挿入する。
func seedSagaStep(t *testing.T, s *Server, id, sagaID, stepName, status string) {
	t.Helper()

	ctx := context.Background()
	if err := s.queries.CreateSagaStep(ctx, sagadb.CreateSagaStepParams{
		ID:       id,
		SagaID:   sagaID,
		StepName: stepName,
		Status:   status,
	}); err != nil {
		t.Fatalf("テスト用Sagaステップ挿入に失敗: %v", err)
	}
}

// TestHandleListActiveSagas はアクティブSaga一覧取得ハンドラのテスト。
func TestHandleListActiveSagas(t *testing.T) {
	t.Parallel()

	t.Run("空のSaga一覧を返す", func(t *testing.T) {
		t.Parallel()

		s := newTestServer(t)
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/sagas", nil)
		s.router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("ステータスコード: got %d, want %d", w.Code, http.StatusOK)
		}

		var result []sagaResponse
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatalf("レスポンスのパースに失敗: %v", err)
		}
		if len(result) != 0 {
			t.Errorf("Saga件数: got %d, want 0", len(result))
		}
	})

	t.Run("アクティブなSagaが存在する場合に一覧を返す", func(t *testing.T) {
		t.Parallel()

		s := newTestServer(t)
		seedSaga(t, s, "saga-001", "media_upload", "process_media", "started", `{}`)
		seedSaga(t, s, "saga-002", "media_upload", "add_to_album", "started", `{}`)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/sagas", nil)
		s.router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("ステータスコード: got %d, want %d", w.Code, http.StatusOK)
		}

		var result []sagaResponse
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatalf("レスポンスのパースに失敗: %v", err)
		}
		if len(result) != 2 {
			t.Fatalf("Saga件数: got %d, want 2", len(result))
		}

		// started_at ASCでソートされるが同時刻の場合順序は不定なのでIDの存在だけ確認
		ids := map[string]bool{}
		for _, s := range result {
			ids[s.ID] = true
			if s.SagaType != "media_upload" {
				t.Errorf("SagaType: got %q, want %q", s.SagaType, "media_upload")
			}
		}
		if !ids["saga-001"] {
			t.Error("saga-001が一覧に含まれていない")
		}
		if !ids["saga-002"] {
			t.Error("saga-002が一覧に含まれていない")
		}
	})

	t.Run("完了済みSagaは一覧に含まれない", func(t *testing.T) {
		t.Parallel()

		s := newTestServer(t)
		seedSaga(t, s, "saga-active", "media_upload", "process_media", "started", `{}`)
		seedSaga(t, s, "saga-done", "media_upload", "send_notification", "completed", `{}`)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/sagas", nil)
		s.router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("ステータスコード: got %d, want %d", w.Code, http.StatusOK)
		}

		var result []sagaResponse
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatalf("レスポンスのパースに失敗: %v", err)
		}
		if len(result) != 1 {
			t.Fatalf("Saga件数: got %d, want 1", len(result))
		}
		if result[0].ID != "saga-active" {
			t.Errorf("Saga ID: got %q, want %q", result[0].ID, "saga-active")
		}
	})
}

// TestHandleGetSagaDetail はSaga詳細取得ハンドラのテスト。
func TestHandleGetSagaDetail(t *testing.T) {
	t.Parallel()

	t.Run("ステップ付きのSaga詳細を返す", func(t *testing.T) {
		t.Parallel()

		s := newTestServer(t)
		seedSaga(t, s, "saga-100", "media_upload", "add_to_album", "in_progress", `{"media_id":"m-1"}`)
		seedSagaStep(t, s, "step-1", "saga-100", "process_media", "completed")
		seedSagaStep(t, s, "step-2", "saga-100", "add_to_album", "executing")

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/sagas/saga-100", nil)
		s.router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("ステータスコード: got %d, want %d", w.Code, http.StatusOK)
		}

		var result sagaResponse
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatalf("レスポンスのパースに失敗: %v", err)
		}

		if result.ID != "saga-100" {
			t.Errorf("Saga ID: got %q, want %q", result.ID, "saga-100")
		}
		if result.SagaType != "media_upload" {
			t.Errorf("SagaType: got %q, want %q", result.SagaType, "media_upload")
		}
		if result.CurrentStep != "add_to_album" {
			t.Errorf("CurrentStep: got %q, want %q", result.CurrentStep, "add_to_album")
		}
		if result.Status != "in_progress" {
			t.Errorf("Status: got %q, want %q", result.Status, "in_progress")
		}
		if result.Payload != `{"media_id":"m-1"}` {
			t.Errorf("Payload: got %q, want %q", result.Payload, `{"media_id":"m-1"}`)
		}
		if len(result.Steps) != 2 {
			t.Fatalf("ステップ数: got %d, want 2", len(result.Steps))
		}
		if result.Steps[0].StepName != "process_media" {
			t.Errorf("Step[0].StepName: got %q, want %q", result.Steps[0].StepName, "process_media")
		}
		if result.Steps[0].Status != "completed" {
			t.Errorf("Step[0].Status: got %q, want %q", result.Steps[0].Status, "completed")
		}
		if result.Steps[1].StepName != "add_to_album" {
			t.Errorf("Step[1].StepName: got %q, want %q", result.Steps[1].StepName, "add_to_album")
		}
		if result.Steps[1].Status != "executing" {
			t.Errorf("Step[1].Status: got %q, want %q", result.Steps[1].Status, "executing")
		}
	})

	t.Run("存在しないSaga IDの場合は404を返す", func(t *testing.T) {
		t.Parallel()

		s := newTestServer(t)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/sagas/nonexistent", nil)
		s.router.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Errorf("ステータスコード: got %d, want %d", w.Code, http.StatusNotFound)
		}

		var result map[string]string
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatalf("レスポンスのパースに失敗: %v", err)
		}
		if _, ok := result["error"]; !ok {
			t.Error("エラーメッセージが含まれていない")
		}
	})

	t.Run("ステップが無いSagaの場合は空のステップ配列を返す", func(t *testing.T) {
		t.Parallel()

		s := newTestServer(t)
		seedSaga(t, s, "saga-nostep", "media_upload", "process_media", "started", `{}`)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/sagas/saga-nostep", nil)
		s.router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("ステータスコード: got %d, want %d", w.Code, http.StatusOK)
		}

		var result sagaResponse
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatalf("レスポンスのパースに失敗: %v", err)
		}
		// Steps が omitempty のため、ステップが無い場合はnullまたは空配列のどちらかになる
		if len(result.Steps) != 0 {
			t.Errorf("ステップ数: got %d, want 0", len(result.Steps))
		}
	})
}

// TestHandleEventNotify はイベント通知ハンドラのテスト。
func TestHandleEventNotify(t *testing.T) {
	t.Parallel()

	t.Run("正常なイベント通知を受け付ける", func(t *testing.T) {
		t.Parallel()

		s := newTestServer(t)

		body := eventNotifyRequest{
			EventType:   "MediaUploaded",
			AggregateID: "media-abc",
			Data:        `{"user_id":"u-1","filename":"test.jpg"}`,
		}
		jsonBody, _ := json.Marshal(body)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/events/notify", bytes.NewReader(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		s.router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("ステータスコード: got %d, want %d", w.Code, http.StatusOK)
		}

		var result map[string]string
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatalf("レスポンスのパースに失敗: %v", err)
		}
		if result["status"] != "accepted" {
			t.Errorf("status: got %q, want %q", result["status"], "accepted")
		}
	})

	t.Run("必須フィールドが不足している場合は400を返す", func(t *testing.T) {
		t.Parallel()

		s := newTestServer(t)

		// event_type が欠けている
		body := map[string]string{
			"aggregate_id": "media-abc",
		}
		jsonBody, _ := json.Marshal(body)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/events/notify", bytes.NewReader(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		s.router.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("ステータスコード: got %d, want %d", w.Code, http.StatusBadRequest)
		}
	})

	t.Run("不正なJSONの場合は400を返す", func(t *testing.T) {
		t.Parallel()

		s := newTestServer(t)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/events/notify", bytes.NewReader([]byte("invalid json")))
		req.Header.Set("Content-Type", "application/json")
		s.router.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("ステータスコード: got %d, want %d", w.Code, http.StatusBadRequest)
		}
	})

	t.Run("Dataフィールドが空でも受け付ける", func(t *testing.T) {
		t.Parallel()

		s := newTestServer(t)

		body := eventNotifyRequest{
			EventType:   "MediaProcessed",
			AggregateID: "media-xyz",
		}
		jsonBody, _ := json.Marshal(body)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/events/notify", bytes.NewReader(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		s.router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("ステータスコード: got %d, want %d", w.Code, http.StatusOK)
		}
	})
}

// TestSagaHealthCheck はヘルスチェックエンドポイントのテスト。
func TestSagaHealthCheck(t *testing.T) {
	t.Parallel()

	s := newTestServer(t)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("ステータスコード: got %d, want %d", w.Code, http.StatusOK)
	}

	var result map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("レスポンスのパースに失敗: %v", err)
	}
	if result["status"] != "ok" {
		t.Errorf("status: got %q, want %q", result["status"], "ok")
	}
	if result["service"] != "saga" {
		t.Errorf("service: got %q, want %q", result["service"], "saga")
	}
}
