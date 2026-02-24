package eventstore

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	_ "github.com/mattn/go-sqlite3"
	eventstoredb "github.com/nao1215/micro/internal/eventstore/db"
)

// setupTestServer はテスト用のサーバーをインメモリSQLiteで構築するヘルパー関数。
// 各テストケースで独立したデータベースを使用するため、テスト間の干渉が発生しない。
func setupTestServer(t *testing.T) *Server {
	t.Helper()

	gin.SetMode(gin.TestMode)

	sqlDB, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("インメモリSQLiteの接続に失敗: %v", err)
	}

	if err := initSchema(sqlDB); err != nil {
		t.Fatalf("スキーマ初期化に失敗: %v", err)
	}

	t.Cleanup(func() {
		sqlDB.Close()
	})

	router := gin.New()

	s := &Server{
		router:  router,
		port:    "0",
		queries: eventstoredb.New(sqlDB),
		db:      sqlDB,
	}
	s.setupRoutes()

	return s
}

// appendTestEvent はテスト用にイベントをPOSTするヘルパー関数。
// レスポンスレコーダーを返すため、必要に応じてレスポンス内容を検証できる。
func appendTestEvent(t *testing.T, s *Server, aggregateID, aggregateType, eventType string, data map[string]interface{}) *httptest.ResponseRecorder {
	t.Helper()

	dataJSON, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("テストデータのJSON変換に失敗: %v", err)
	}

	reqBody := appendEventRequest{
		AggregateID:   aggregateID,
		AggregateType: aggregateType,
		EventType:     eventType,
		Data:          dataJSON,
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		t.Fatalf("リクエストボディのJSON変換に失敗: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/events", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	return w
}

// TestHealthCheck はヘルスチェックエンドポイントの正常動作を検証する。
func TestHealthCheck(t *testing.T) {
	t.Parallel()

	s := setupTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("ステータスコード = %d; 期待値 = %d", w.Code, http.StatusOK)
	}

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("レスポンスのJSONデコードに失敗: %v", err)
	}

	if resp["status"] != "ok" {
		t.Errorf("status = %q; 期待値 = %q", resp["status"], "ok")
	}
	if resp["service"] != "eventstore" {
		t.Errorf("service = %q; 期待値 = %q", resp["service"], "eventstore")
	}
}

// TestHandleAppendEvent はイベント追記ハンドラの各パターンを検証する。
func TestHandleAppendEvent(t *testing.T) {
	t.Parallel()

	t.Run("正常にイベントを追記できる", func(t *testing.T) {
		t.Parallel()

		s := setupTestServer(t)

		w := appendTestEvent(t, s, "agg-1", "Media", "MediaUploaded", map[string]interface{}{
			"user_id":  "user-1",
			"filename": "photo.jpg",
		})

		if w.Code != http.StatusCreated {
			t.Errorf("ステータスコード = %d; 期待値 = %d", w.Code, http.StatusCreated)
		}

		var resp eventResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("レスポンスのJSONデコードに失敗: %v", err)
		}

		if resp.AggregateID != "agg-1" {
			t.Errorf("aggregate_id = %q; 期待値 = %q", resp.AggregateID, "agg-1")
		}
		if resp.AggregateType != "Media" {
			t.Errorf("aggregate_type = %q; 期待値 = %q", resp.AggregateType, "Media")
		}
		if resp.EventType != "MediaUploaded" {
			t.Errorf("event_type = %q; 期待値 = %q", resp.EventType, "MediaUploaded")
		}
		if resp.Version != 1 {
			t.Errorf("version = %d; 期待値 = %d", resp.Version, 1)
		}
		if resp.ID == "" {
			t.Error("id が空文字列になっている")
		}
		if resp.CreatedAt == "" {
			t.Error("created_at が空文字列になっている")
		}
	})

	t.Run("バージョンが自動インクリメントされる", func(t *testing.T) {
		t.Parallel()

		s := setupTestServer(t)

		// 同じAggregateIDに対して3つのイベントを追記する
		for i := 1; i <= 3; i++ {
			w := appendTestEvent(t, s, "agg-inc", "Media", "MediaUploaded", map[string]interface{}{
				"user_id":  "user-1",
				"filename": fmt.Sprintf("photo%d.jpg", i),
			})

			if w.Code != http.StatusCreated {
				t.Fatalf("イベント%d: ステータスコード = %d; 期待値 = %d", i, w.Code, http.StatusCreated)
			}

			var resp eventResponse
			if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
				t.Fatalf("イベント%d: レスポンスのJSONデコードに失敗: %v", i, err)
			}

			if resp.Version != int64(i) {
				t.Errorf("イベント%d: version = %d; 期待値 = %d", i, resp.Version, i)
			}
		}
	})

	t.Run("異なるAggregateIDのバージョンは独立している", func(t *testing.T) {
		t.Parallel()

		s := setupTestServer(t)

		// 異なるAggregateIDにそれぞれイベントを追記する
		w1 := appendTestEvent(t, s, "agg-a", "Media", "MediaUploaded", map[string]interface{}{"user_id": "user-1"})
		w2 := appendTestEvent(t, s, "agg-b", "Album", "AlbumCreated", map[string]interface{}{"user_id": "user-2"})

		if w1.Code != http.StatusCreated || w2.Code != http.StatusCreated {
			t.Fatalf("いずれかのイベント追記に失敗: w1=%d, w2=%d", w1.Code, w2.Code)
		}

		var resp1, resp2 eventResponse
		json.Unmarshal(w1.Body.Bytes(), &resp1)
		json.Unmarshal(w2.Body.Bytes(), &resp2)

		if resp1.Version != 1 {
			t.Errorf("agg-a: version = %d; 期待値 = 1", resp1.Version)
		}
		if resp2.Version != 1 {
			t.Errorf("agg-b: version = %d; 期待値 = 1", resp2.Version)
		}
	})

	t.Run("必須フィールドが欠けている場合は400エラーを返す", func(t *testing.T) {
		t.Parallel()

		s := setupTestServer(t)

		testCases := []struct {
			name string
			body map[string]interface{}
		}{
			{
				name: "aggregate_idが欠けている",
				body: map[string]interface{}{
					"aggregate_type": "Media",
					"event_type":     "MediaUploaded",
					"data":           map[string]interface{}{"key": "value"},
				},
			},
			{
				name: "aggregate_typeが欠けている",
				body: map[string]interface{}{
					"aggregate_id": "agg-1",
					"event_type":   "MediaUploaded",
					"data":         map[string]interface{}{"key": "value"},
				},
			},
			{
				name: "event_typeが欠けている",
				body: map[string]interface{}{
					"aggregate_id":   "agg-1",
					"aggregate_type": "Media",
					"data":           map[string]interface{}{"key": "value"},
				},
			},
			{
				name: "dataが欠けている",
				body: map[string]interface{}{
					"aggregate_id":   "agg-1",
					"aggregate_type": "Media",
					"event_type":     "MediaUploaded",
				},
			},
			{
				name: "空のボディ",
				body: map[string]interface{}{},
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				body, err := json.Marshal(tc.body)
				if err != nil {
					t.Fatalf("リクエストボディのJSON変換に失敗: %v", err)
				}

				req := httptest.NewRequest(http.MethodPost, "/api/v1/events", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				w := httptest.NewRecorder()
				s.router.ServeHTTP(w, req)

				if w.Code != http.StatusBadRequest {
					t.Errorf("ステータスコード = %d; 期待値 = %d", w.Code, http.StatusBadRequest)
				}

				var resp map[string]string
				if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
					t.Fatalf("レスポンスのJSONデコードに失敗: %v", err)
				}
				if _, ok := resp["error"]; !ok {
					t.Error("レスポンスにerrorフィールドが含まれていない")
				}
			})
		}
	})

	t.Run("不正なJSONの場合は400エラーを返す", func(t *testing.T) {
		t.Parallel()

		s := setupTestServer(t)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/events", bytes.NewReader([]byte("invalid json")))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("ステータスコード = %d; 期待値 = %d", w.Code, http.StatusBadRequest)
		}
	})

	t.Run("バージョンの自動インクリメントが正しく動作する", func(t *testing.T) {
		t.Parallel()

		s := setupTestServer(t)

		// 3件連続でイベントを追記し、バージョンが1,2,3と増えることを確認
		for i := 1; i <= 3; i++ {
			w := appendTestEvent(t, s, "agg-autoversion", "Media", "MediaUploaded", map[string]interface{}{
				"user_id": fmt.Sprintf("user-%d", i),
			})
			if w.Code != http.StatusCreated {
				t.Fatalf("イベント%d追記に失敗: ステータスコード = %d", i, w.Code)
			}

			var resp eventResponse
			if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
				t.Fatalf("レスポンスのJSONデコードに失敗: %v", err)
			}
			if resp.Version != int64(i) {
				t.Errorf("イベント%dのバージョン = %d; 期待値 = %d", i, resp.Version, i)
			}
		}
	})

	t.Run("レスポンスのDataフィールドにJSONデータが含まれる", func(t *testing.T) {
		t.Parallel()

		s := setupTestServer(t)

		testData := map[string]interface{}{
			"user_id":      "user-123",
			"filename":     "test.png",
			"content_type": "image/png",
			"size":         float64(1024),
		}

		w := appendTestEvent(t, s, "agg-data", "Media", "MediaUploaded", testData)
		if w.Code != http.StatusCreated {
			t.Fatalf("ステータスコード = %d; 期待値 = %d", w.Code, http.StatusCreated)
		}

		var resp eventResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("レスポンスのJSONデコードに失敗: %v", err)
		}

		// DataフィールドのJSONをパースして元のデータと比較する
		var parsedData map[string]interface{}
		if err := json.Unmarshal([]byte(resp.Data), &parsedData); err != nil {
			t.Fatalf("Dataフィールドのパースに失敗: %v", err)
		}

		if parsedData["user_id"] != "user-123" {
			t.Errorf("data.user_id = %v; 期待値 = %v", parsedData["user_id"], "user-123")
		}
		if parsedData["filename"] != "test.png" {
			t.Errorf("data.filename = %v; 期待値 = %v", parsedData["filename"], "test.png")
		}
	})
}

// TestHandleGetEventsByAggregateID はAggregateIDによるイベント取得ハンドラを検証する。
func TestHandleGetEventsByAggregateID(t *testing.T) {
	t.Parallel()

	t.Run("AggregateIDに紐づくイベントを取得できる", func(t *testing.T) {
		t.Parallel()

		s := setupTestServer(t)

		// テストデータを投入する
		appendTestEvent(t, s, "agg-get-1", "Media", "MediaUploaded", map[string]interface{}{"user_id": "user-1"})
		appendTestEvent(t, s, "agg-get-1", "Media", "MediaProcessed", map[string]interface{}{"thumbnail_path": "/thumb.jpg"})
		appendTestEvent(t, s, "agg-get-2", "Album", "AlbumCreated", map[string]interface{}{"user_id": "user-2"})

		req := httptest.NewRequest(http.MethodGet, "/api/v1/events/aggregate/agg-get-1", nil)
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("ステータスコード = %d; 期待値 = %d", w.Code, http.StatusOK)
		}

		var resp []eventResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("レスポンスのJSONデコードに失敗: %v", err)
		}

		if len(resp) != 2 {
			t.Fatalf("イベント数 = %d; 期待値 = 2", len(resp))
		}

		// バージョン順にソートされていることを確認する
		if resp[0].Version != 1 || resp[1].Version != 2 {
			t.Errorf("バージョン順序が不正: v1=%d, v2=%d", resp[0].Version, resp[1].Version)
		}
		if resp[0].EventType != "MediaUploaded" {
			t.Errorf("1番目のevent_type = %q; 期待値 = %q", resp[0].EventType, "MediaUploaded")
		}
		if resp[1].EventType != "MediaProcessed" {
			t.Errorf("2番目のevent_type = %q; 期待値 = %q", resp[1].EventType, "MediaProcessed")
		}
	})

	t.Run("存在しないAggregateIDの場合は空配列を返す", func(t *testing.T) {
		t.Parallel()

		s := setupTestServer(t)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/events/aggregate/nonexistent", nil)
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("ステータスコード = %d; 期待値 = %d", w.Code, http.StatusOK)
		}

		var resp []eventResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("レスポンスのJSONデコードに失敗: %v", err)
		}

		if len(resp) != 0 {
			t.Errorf("イベント数 = %d; 期待値 = 0", len(resp))
		}
	})
}

// TestHandleGetEventsByType はイベントタイプによるイベント取得ハンドラを検証する。
func TestHandleGetEventsByType(t *testing.T) {
	t.Parallel()

	t.Run("イベントタイプに一致するイベントを取得できる", func(t *testing.T) {
		t.Parallel()

		s := setupTestServer(t)

		// 異なるイベントタイプのデータを投入する
		appendTestEvent(t, s, "agg-type-1", "Media", "MediaUploaded", map[string]interface{}{"user_id": "user-1"})
		appendTestEvent(t, s, "agg-type-2", "Media", "MediaUploaded", map[string]interface{}{"user_id": "user-2"})
		appendTestEvent(t, s, "agg-type-3", "Album", "AlbumCreated", map[string]interface{}{"user_id": "user-3"})

		req := httptest.NewRequest(http.MethodGet, "/api/v1/events/type/MediaUploaded", nil)
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("ステータスコード = %d; 期待値 = %d", w.Code, http.StatusOK)
		}

		var resp []eventResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("レスポンスのJSONデコードに失敗: %v", err)
		}

		if len(resp) != 2 {
			t.Fatalf("イベント数 = %d; 期待値 = 2", len(resp))
		}

		for _, r := range resp {
			if r.EventType != "MediaUploaded" {
				t.Errorf("event_type = %q; 期待値 = %q", r.EventType, "MediaUploaded")
			}
		}
	})

	t.Run("存在しないイベントタイプの場合は空配列を返す", func(t *testing.T) {
		t.Parallel()

		s := setupTestServer(t)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/events/type/NonExistentType", nil)
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("ステータスコード = %d; 期待値 = %d", w.Code, http.StatusOK)
		}

		var resp []eventResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("レスポンスのJSONデコードに失敗: %v", err)
		}

		if len(resp) != 0 {
			t.Errorf("イベント数 = %d; 期待値 = 0", len(resp))
		}
	})

	t.Run("複数のAggregateIDにまたがるイベントを正しく取得できる", func(t *testing.T) {
		t.Parallel()

		s := setupTestServer(t)

		appendTestEvent(t, s, "agg-cross-1", "Media", "MediaDeleted", map[string]interface{}{"user_id": "user-1"})
		appendTestEvent(t, s, "agg-cross-2", "Media", "MediaDeleted", map[string]interface{}{"user_id": "user-2"})
		appendTestEvent(t, s, "agg-cross-3", "Media", "MediaDeleted", map[string]interface{}{"user_id": "user-3"})

		req := httptest.NewRequest(http.MethodGet, "/api/v1/events/type/MediaDeleted", nil)
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("ステータスコード = %d; 期待値 = %d", w.Code, http.StatusOK)
		}

		var resp []eventResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("レスポンスのJSONデコードに失敗: %v", err)
		}

		if len(resp) != 3 {
			t.Errorf("イベント数 = %d; 期待値 = 3", len(resp))
		}
	})
}

// TestHandleGetEventsSince は日時指定によるイベント取得ハンドラを検証する。
func TestHandleGetEventsSince(t *testing.T) {
	t.Parallel()

	t.Run("指定日時以降のイベントを取得できる", func(t *testing.T) {
		t.Parallel()

		s := setupTestServer(t)

		// 基準時刻の前にイベントを記録する
		past := time.Now().UTC().Add(-1 * time.Hour)

		appendTestEvent(t, s, "agg-since-1", "Media", "MediaUploaded", map[string]interface{}{"user_id": "user-1"})
		appendTestEvent(t, s, "agg-since-2", "Album", "AlbumCreated", map[string]interface{}{"user_id": "user-2"})

		// 過去の時刻を指定して全イベントが取得されることを確認する
		sinceStr := past.Format(time.RFC3339)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/events/since?since="+sinceStr, nil)
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("ステータスコード = %d; 期待値 = %d", w.Code, http.StatusOK)
		}

		var resp []eventResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("レスポンスのJSONデコードに失敗: %v", err)
		}

		if len(resp) != 2 {
			t.Errorf("イベント数 = %d; 期待値 = 2", len(resp))
		}
	})

	t.Run("未来の日時を指定すると空配列を返す", func(t *testing.T) {
		t.Parallel()

		s := setupTestServer(t)

		appendTestEvent(t, s, "agg-future", "Media", "MediaUploaded", map[string]interface{}{"user_id": "user-1"})

		future := time.Now().UTC().Add(1 * time.Hour)
		sinceStr := future.Format(time.RFC3339)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/events/since?since="+sinceStr, nil)
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("ステータスコード = %d; 期待値 = %d", w.Code, http.StatusOK)
		}

		var resp []eventResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("レスポンスのJSONデコードに失敗: %v", err)
		}

		if len(resp) != 0 {
			t.Errorf("イベント数 = %d; 期待値 = 0", len(resp))
		}
	})

	t.Run("sinceクエリパラメータが欠けている場合は400エラーを返す", func(t *testing.T) {
		t.Parallel()

		s := setupTestServer(t)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/events/since", nil)
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("ステータスコード = %d; 期待値 = %d", w.Code, http.StatusBadRequest)
		}

		var resp map[string]string
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("レスポンスのJSONデコードに失敗: %v", err)
		}
		if _, ok := resp["error"]; !ok {
			t.Error("レスポンスにerrorフィールドが含まれていない")
		}
	})

	t.Run("sinceパラメータが不正な形式の場合は400エラーを返す", func(t *testing.T) {
		t.Parallel()

		s := setupTestServer(t)

		testCases := []struct {
			name  string
			since string
		}{
			{name: "不正な日付文字列", since: "not-a-date"},
			{name: "日付のみ（時刻なし）", since: "2024-01-01"},
			{name: "Unixタイムスタンプ", since: "1700000000"},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				req := httptest.NewRequest(http.MethodGet, "/api/v1/events/since?since="+tc.since, nil)
				w := httptest.NewRecorder()
				s.router.ServeHTTP(w, req)

				if w.Code != http.StatusBadRequest {
					t.Errorf("ステータスコード = %d; 期待値 = %d (since=%q)", w.Code, http.StatusBadRequest, tc.since)
				}
			})
		}
	})
}

// TestHandleGetLatestVersion はAggregateIDの最新バージョン取得ハンドラを検証する。
func TestHandleGetLatestVersion(t *testing.T) {
	t.Parallel()

	t.Run("イベントが存在するAggregateIDの最新バージョンを取得できる", func(t *testing.T) {
		t.Parallel()

		s := setupTestServer(t)

		// 3つのイベントを追記してバージョン3まで進める
		appendTestEvent(t, s, "agg-ver", "Media", "MediaUploaded", map[string]interface{}{"user_id": "user-1"})
		appendTestEvent(t, s, "agg-ver", "Media", "MediaProcessed", map[string]interface{}{"thumbnail_path": "/thumb.jpg"})
		appendTestEvent(t, s, "agg-ver", "Media", "MediaDeleted", map[string]interface{}{"user_id": "user-1"})

		req := httptest.NewRequest(http.MethodGet, "/api/v1/events/aggregate/agg-ver/version", nil)
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("ステータスコード = %d; 期待値 = %d", w.Code, http.StatusOK)
		}

		var resp map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("レスポンスのJSONデコードに失敗: %v", err)
		}

		if resp["aggregate_id"] != "agg-ver" {
			t.Errorf("aggregate_id = %v; 期待値 = %v", resp["aggregate_id"], "agg-ver")
		}

		latestVersion, ok := resp["latest_version"].(float64)
		if !ok {
			t.Fatalf("latest_version の型がfloat64ではない: %T", resp["latest_version"])
		}
		if int64(latestVersion) != 3 {
			t.Errorf("latest_version = %v; 期待値 = 3", latestVersion)
		}
	})

	t.Run("イベントが存在しないAggregateIDの場合はバージョン0を返す", func(t *testing.T) {
		t.Parallel()

		s := setupTestServer(t)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/events/aggregate/nonexistent/version", nil)
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("ステータスコード = %d; 期待値 = %d", w.Code, http.StatusOK)
		}

		var resp map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("レスポンスのJSONデコードに失敗: %v", err)
		}

		latestVersion, ok := resp["latest_version"].(float64)
		if !ok {
			t.Fatalf("latest_version の型がfloat64ではない: %T", resp["latest_version"])
		}
		if int64(latestVersion) != 0 {
			t.Errorf("latest_version = %v; 期待値 = 0", latestVersion)
		}
	})

	t.Run("イベント追記後にバージョンが正しく更新される", func(t *testing.T) {
		t.Parallel()

		s := setupTestServer(t)

		// 初期状態: バージョン0
		req := httptest.NewRequest(http.MethodGet, "/api/v1/events/aggregate/agg-ver-update/version", nil)
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)

		var resp map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &resp)

		if int64(resp["latest_version"].(float64)) != 0 {
			t.Errorf("初期バージョン = %v; 期待値 = 0", resp["latest_version"])
		}

		// イベントを1つ追記してバージョン1になることを確認する
		appendTestEvent(t, s, "agg-ver-update", "Media", "MediaUploaded", map[string]interface{}{"user_id": "user-1"})

		req = httptest.NewRequest(http.MethodGet, "/api/v1/events/aggregate/agg-ver-update/version", nil)
		w = httptest.NewRecorder()
		s.router.ServeHTTP(w, req)

		json.Unmarshal(w.Body.Bytes(), &resp)

		if int64(resp["latest_version"].(float64)) != 1 {
			t.Errorf("追記後バージョン = %v; 期待値 = 1", resp["latest_version"])
		}
	})
}

// TestHandleGetAllEvents は全イベント取得ハンドラを検証する。
func TestHandleGetAllEvents(t *testing.T) {
	t.Parallel()

	t.Run("全イベントを取得できる", func(t *testing.T) {
		t.Parallel()

		s := setupTestServer(t)

		// 異なるAggregate・Typeのイベントを複数投入する
		appendTestEvent(t, s, "agg-all-1", "Media", "MediaUploaded", map[string]interface{}{"user_id": "user-1"})
		appendTestEvent(t, s, "agg-all-2", "Album", "AlbumCreated", map[string]interface{}{"user_id": "user-2"})
		appendTestEvent(t, s, "agg-all-1", "Media", "MediaProcessed", map[string]interface{}{"thumbnail_path": "/thumb.jpg"})

		req := httptest.NewRequest(http.MethodGet, "/api/v1/events", nil)
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("ステータスコード = %d; 期待値 = %d", w.Code, http.StatusOK)
		}

		var resp []eventResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("レスポンスのJSONデコードに失敗: %v", err)
		}

		if len(resp) != 3 {
			t.Fatalf("イベント数 = %d; 期待値 = 3", len(resp))
		}
	})

	t.Run("イベントが存在しない場合は空配列を返す", func(t *testing.T) {
		t.Parallel()

		s := setupTestServer(t)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/events", nil)
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("ステータスコード = %d; 期待値 = %d", w.Code, http.StatusOK)
		}

		var resp []eventResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("レスポンスのJSONデコードに失敗: %v", err)
		}

		if len(resp) != 0 {
			t.Errorf("イベント数 = %d; 期待値 = 0", len(resp))
		}
	})

	t.Run("イベントがcreated_at昇順でソートされている", func(t *testing.T) {
		t.Parallel()

		s := setupTestServer(t)

		appendTestEvent(t, s, "agg-order-1", "Media", "MediaUploaded", map[string]interface{}{"user_id": "user-1"})
		appendTestEvent(t, s, "agg-order-2", "Album", "AlbumCreated", map[string]interface{}{"user_id": "user-2"})
		appendTestEvent(t, s, "agg-order-3", "Media", "MediaDeleted", map[string]interface{}{"user_id": "user-3"})

		req := httptest.NewRequest(http.MethodGet, "/api/v1/events", nil)
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)

		var resp []eventResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("レスポンスのJSONデコードに失敗: %v", err)
		}

		if len(resp) < 2 {
			t.Fatalf("ソート順序の検証にはイベントが2つ以上必要: %d", len(resp))
		}

		// created_at順にソートされていることを確認する
		for i := 1; i < len(resp); i++ {
			prev, err := time.Parse(time.RFC3339, resp[i-1].CreatedAt)
			if err != nil {
				t.Fatalf("created_at[%d]のパースに失敗: %v", i-1, err)
			}
			curr, err := time.Parse(time.RFC3339, resp[i].CreatedAt)
			if err != nil {
				t.Fatalf("created_at[%d]のパースに失敗: %v", i, err)
			}
			if curr.Before(prev) {
				t.Errorf("ソート順序が不正: resp[%d].created_at=%v > resp[%d].created_at=%v", i-1, prev, i, curr)
			}
		}
	})
}

// TestToEventResponse はtoEventResponse変換関数の動作を検証する。
func TestToEventResponse(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	resp := toEventResponse("id-1", "agg-1", "Media", "MediaUploaded", `{"key":"value"}`, 5, now)

	if resp.ID != "id-1" {
		t.Errorf("ID = %q; 期待値 = %q", resp.ID, "id-1")
	}
	if resp.AggregateID != "agg-1" {
		t.Errorf("AggregateID = %q; 期待値 = %q", resp.AggregateID, "agg-1")
	}
	if resp.AggregateType != "Media" {
		t.Errorf("AggregateType = %q; 期待値 = %q", resp.AggregateType, "Media")
	}
	if resp.EventType != "MediaUploaded" {
		t.Errorf("EventType = %q; 期待値 = %q", resp.EventType, "MediaUploaded")
	}
	if resp.Data != `{"key":"value"}` {
		t.Errorf("Data = %q; 期待値 = %q", resp.Data, `{"key":"value"}`)
	}
	if resp.Version != 5 {
		t.Errorf("Version = %d; 期待値 = 5", resp.Version)
	}

	expectedTime := now.Format(time.RFC3339)
	if resp.CreatedAt != expectedTime {
		t.Errorf("CreatedAt = %q; 期待値 = %q", resp.CreatedAt, expectedTime)
	}
}

// TestToEventResponses はtoEventResponsesスライス変換関数の動作を検証する。
func TestToEventResponses(t *testing.T) {
	t.Parallel()

	t.Run("複数のイベント行を正しく変換できる", func(t *testing.T) {
		t.Parallel()

		now := time.Now().UTC()
		rows := []eventstoredb.Event{
			{
				ID:            "id-1",
				AggregateID:   "agg-1",
				AggregateType: "Media",
				EventType:     "MediaUploaded",
				Data:          `{"user_id":"user-1"}`,
				Version:       1,
				CreatedAt:     now,
			},
			{
				ID:            "id-2",
				AggregateID:   "agg-1",
				AggregateType: "Media",
				EventType:     "MediaProcessed",
				Data:          `{"thumbnail_path":"/thumb.jpg"}`,
				Version:       2,
				CreatedAt:     now.Add(1 * time.Second),
			},
		}

		responses := toEventResponses(rows)

		if len(responses) != 2 {
			t.Fatalf("レスポンス数 = %d; 期待値 = 2", len(responses))
		}

		if responses[0].ID != "id-1" {
			t.Errorf("responses[0].ID = %q; 期待値 = %q", responses[0].ID, "id-1")
		}
		if responses[1].ID != "id-2" {
			t.Errorf("responses[1].ID = %q; 期待値 = %q", responses[1].ID, "id-2")
		}
		if responses[0].Version != 1 {
			t.Errorf("responses[0].Version = %d; 期待値 = 1", responses[0].Version)
		}
		if responses[1].Version != 2 {
			t.Errorf("responses[1].Version = %d; 期待値 = 2", responses[1].Version)
		}
	})

	t.Run("空のスライスを渡すと空のスライスを返す", func(t *testing.T) {
		t.Parallel()

		responses := toEventResponses([]eventstoredb.Event{})

		if len(responses) != 0 {
			t.Errorf("レスポンス数 = %d; 期待値 = 0", len(responses))
		}
		if responses == nil {
			t.Error("空スライスがnilになっている（空のスライスが期待される）")
		}
	})
}

// TestRouteNotFound は未定義ルートへのリクエストが404を返すことを検証する。
func TestRouteNotFound(t *testing.T) {
	t.Parallel()

	s := setupTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/nonexistent", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("ステータスコード = %d; 期待値 = %d", w.Code, http.StatusNotFound)
	}
}

// TestMethodNotAllowed は許可されていないHTTPメソッドへのリクエストを検証する。
func TestMethodNotAllowed(t *testing.T) {
	t.Parallel()

	s := setupTestServer(t)

	// GETエンドポイントにPOSTリクエストを送信する
	req := httptest.NewRequest(http.MethodPost, "/health", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	// Ginのデフォルト動作では404が返る
	if w.Code != http.StatusNotFound && w.Code != http.StatusMethodNotAllowed {
		t.Errorf("ステータスコード = %d; 期待値 = 404 または 405", w.Code)
	}
}

// TestAppendAndRetrieveIntegration はイベントの追記と取得を組み合わせた統合テスト。
// イベントを投入し、各種取得APIで正しくデータが返されることを一気通貫で検証する。
func TestAppendAndRetrieveIntegration(t *testing.T) {
	t.Parallel()

	s := setupTestServer(t)

	// ステップ1: 複数のイベントを投入する
	appendTestEvent(t, s, "media-001", "Media", "MediaUploaded", map[string]interface{}{
		"user_id":      "user-100",
		"filename":     "sunset.jpg",
		"content_type": "image/jpeg",
		"size":         float64(2048),
		"storage_path": "/uploads/sunset.jpg",
	})
	appendTestEvent(t, s, "media-001", "Media", "MediaProcessed", map[string]interface{}{
		"thumbnail_path":   "/thumbnails/sunset_thumb.jpg",
		"width":            1920,
		"height":           1080,
		"duration_seconds": 0,
	})
	appendTestEvent(t, s, "album-001", "Album", "AlbumCreated", map[string]interface{}{
		"user_id":     "user-100",
		"name":        "旅行の思い出",
		"description": "2024年夏の旅行写真",
	})

	// ステップ2: AggregateIDで取得
	t.Run("AggregateIDで取得した結果を検証", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/events/aggregate/media-001", nil)
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)

		var events []eventResponse
		json.Unmarshal(w.Body.Bytes(), &events)

		if len(events) != 2 {
			t.Fatalf("media-001のイベント数 = %d; 期待値 = 2", len(events))
		}
		if events[0].AggregateType != "Media" {
			t.Errorf("aggregate_type = %q; 期待値 = %q", events[0].AggregateType, "Media")
		}
	})

	// ステップ3: イベントタイプで取得
	t.Run("イベントタイプで取得した結果を検証", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/events/type/AlbumCreated", nil)
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)

		var events []eventResponse
		json.Unmarshal(w.Body.Bytes(), &events)

		if len(events) != 1 {
			t.Fatalf("AlbumCreatedイベント数 = %d; 期待値 = 1", len(events))
		}
		if events[0].AggregateID != "album-001" {
			t.Errorf("aggregate_id = %q; 期待値 = %q", events[0].AggregateID, "album-001")
		}
	})

	// ステップ4: 最新バージョンの取得
	t.Run("最新バージョンの検証", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/events/aggregate/media-001/version", nil)
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)

		var resp map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &resp)

		if int64(resp["latest_version"].(float64)) != 2 {
			t.Errorf("latest_version = %v; 期待値 = 2", resp["latest_version"])
		}
	})

	// ステップ5: 全イベント取得
	t.Run("全イベント取得の検証", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/events", nil)
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)

		var events []eventResponse
		json.Unmarshal(w.Body.Bytes(), &events)

		if len(events) != 3 {
			t.Errorf("全イベント数 = %d; 期待値 = 3", len(events))
		}
	})

	// ステップ6: 日時指定取得
	t.Run("日時指定取得の検証", func(t *testing.T) {
		past := time.Now().UTC().Add(-1 * time.Hour).Format(time.RFC3339)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/events/since?since="+past, nil)
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)

		var events []eventResponse
		json.Unmarshal(w.Body.Bytes(), &events)

		if len(events) != 3 {
			t.Errorf("日時指定取得イベント数 = %d; 期待値 = 3", len(events))
		}
	})
}

// TestInitSchema はスキーマ初期化関数の動作を検証する。
func TestInitSchema(t *testing.T) {
	t.Parallel()

	t.Run("正常にスキーマを初期化できる", func(t *testing.T) {
		t.Parallel()

		sqlDB, err := sql.Open("sqlite3", ":memory:")
		if err != nil {
			t.Fatalf("SQLite接続に失敗: %v", err)
		}
		defer sqlDB.Close()

		if err := initSchema(sqlDB); err != nil {
			t.Fatalf("スキーマ初期化に失敗: %v", err)
		}

		// テーブルが作成されたことを確認する
		var tableName string
		err = sqlDB.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='events'").Scan(&tableName)
		if err != nil {
			t.Fatalf("eventsテーブルの確認に失敗: %v", err)
		}
		if tableName != "events" {
			t.Errorf("テーブル名 = %q; 期待値 = %q", tableName, "events")
		}
	})

	t.Run("二重初期化してもエラーにならない", func(t *testing.T) {
		t.Parallel()

		sqlDB, err := sql.Open("sqlite3", ":memory:")
		if err != nil {
			t.Fatalf("SQLite接続に失敗: %v", err)
		}
		defer sqlDB.Close()

		if err := initSchema(sqlDB); err != nil {
			t.Fatalf("1回目のスキーマ初期化に失敗: %v", err)
		}
		if err := initSchema(sqlDB); err != nil {
			t.Fatalf("2回目のスキーマ初期化に失敗（冪等性の問題）: %v", err)
		}
	})
}
