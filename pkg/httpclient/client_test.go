package httpclient

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// testRequest はテストサーバーが受け取ったリクエスト情報を保持する構造体。
type testRequest struct {
	// Method はHTTPメソッド。
	Method string
	// Path はリクエストパス。
	Path string
	// Body はリクエストボディ。
	Body []byte
	// Headers はリクエストヘッダー。
	Headers http.Header
}

// testPayload はテスト用のリクエスト/レスポンスペイロード。
type testPayload struct {
	// Name はテスト用の名前フィールド。
	Name string `json:"name"`
	// Value はテスト用の値フィールド。
	Value int `json:"value"`
}

// TestNew はNew関数でクライアントが正しく生成されることを検証する。
func TestNew(t *testing.T) {
	t.Parallel()

	t.Run("クライアントが正常に生成されること", func(t *testing.T) {
		t.Parallel()

		client := New("http://localhost:8080")
		if client == nil {
			t.Fatal("New()がnilを返した")
		}
		if client.baseURL != "http://localhost:8080" {
			t.Errorf("baseURL = %q, want %q", client.baseURL, "http://localhost:8080")
		}
		if client.httpClient == nil {
			t.Fatal("httpClientがnil")
		}
	})

	t.Run("タイムアウトが30秒に設定されていること", func(t *testing.T) {
		t.Parallel()

		client := New("http://localhost:8080")
		if client.httpClient.Timeout.Seconds() != 30 {
			t.Errorf("Timeout = %v, want 30s", client.httpClient.Timeout)
		}
	})
}

// TestPostJSON はPostJSON関数を検証する。
func TestPostJSON(t *testing.T) {
	t.Parallel()

	t.Run("正常にPOSTリクエストを送信してレスポンスを取得できること", func(t *testing.T) {
		t.Parallel()

		var received testRequest
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			received.Method = r.Method
			received.Path = r.URL.Path
			received.Body, _ = io.ReadAll(r.Body)
			received.Headers = r.Header

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(testPayload{Name: "response", Value: 200})
		}))
		defer ts.Close()

		client := New(ts.URL)
		body := testPayload{Name: "request", Value: 100}
		var result testPayload

		err := client.PostJSON(context.Background(), "/api/events", body, &result)
		if err != nil {
			t.Fatalf("PostJSON()でエラーが発生: %v", err)
		}

		// リクエストの検証
		if received.Method != http.MethodPost {
			t.Errorf("Method = %q, want %q", received.Method, http.MethodPost)
		}
		if received.Path != "/api/events" {
			t.Errorf("Path = %q, want %q", received.Path, "/api/events")
		}

		// リクエストボディの検証
		var sentBody testPayload
		if err := json.Unmarshal(received.Body, &sentBody); err != nil {
			t.Fatalf("リクエストボディのパースに失敗: %v", err)
		}
		if sentBody.Name != "request" {
			t.Errorf("sent Name = %q, want %q", sentBody.Name, "request")
		}
		if sentBody.Value != 100 {
			t.Errorf("sent Value = %d, want %d", sentBody.Value, 100)
		}

		// Content-Typeヘッダーの検証
		if got := received.Headers.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q, want %q", got, "application/json")
		}

		// レスポンスの検証
		if result.Name != "response" {
			t.Errorf("result.Name = %q, want %q", result.Name, "response")
		}
		if result.Value != 200 {
			t.Errorf("result.Value = %d, want %d", result.Value, 200)
		}
	})

	t.Run("サーバーが400エラーを返した場合にエラーが返ること", func(t *testing.T) {
		t.Parallel()

		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"error":"bad request"}`))
		}))
		defer ts.Close()

		client := New(ts.URL)
		body := testPayload{Name: "bad", Value: 0}
		var result testPayload

		err := client.PostJSON(context.Background(), "/api/events", body, &result)
		if err == nil {
			t.Fatal("PostJSON()がエラーを返すべきだが、nilが返った")
		}
	})

	t.Run("サーバーが500エラーを返した場合にエラーが返ること", func(t *testing.T) {
		t.Parallel()

		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"error":"internal server error"}`))
		}))
		defer ts.Close()

		client := New(ts.URL)
		body := testPayload{Name: "error", Value: 0}
		var result testPayload

		err := client.PostJSON(context.Background(), "/api/events", body, &result)
		if err == nil {
			t.Fatal("PostJSON()がエラーを返すべきだが、nilが返った")
		}
	})

	t.Run("resultがnilの場合でもエラーにならないこと", func(t *testing.T) {
		t.Parallel()

		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"status":"created"}`))
		}))
		defer ts.Close()

		client := New(ts.URL)
		body := testPayload{Name: "no-result", Value: 1}

		err := client.PostJSON(context.Background(), "/api/events", body, nil)
		if err != nil {
			t.Fatalf("PostJSON()でエラーが発生: %v", err)
		}
	})

	t.Run("キャンセルされたコンテキストでエラーが返ること", func(t *testing.T) {
		t.Parallel()

		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(testPayload{Name: "response", Value: 1})
		}))
		defer ts.Close()

		client := New(ts.URL)
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // 即座にキャンセル

		body := testPayload{Name: "cancelled", Value: 0}
		var result testPayload

		err := client.PostJSON(ctx, "/api/events", body, &result)
		if err == nil {
			t.Fatal("PostJSON()がエラーを返すべきだが、nilが返った")
		}
	})
}

// TestGetJSON はGetJSON関数を検証する。
func TestGetJSON(t *testing.T) {
	t.Parallel()

	t.Run("正常にGETリクエストを送信してレスポンスを取得できること", func(t *testing.T) {
		t.Parallel()

		var received testRequest
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			received.Method = r.Method
			received.Path = r.URL.Path
			received.Headers = r.Header

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(testPayload{Name: "get-response", Value: 42})
		}))
		defer ts.Close()

		client := New(ts.URL)
		var result testPayload

		err := client.GetJSON(context.Background(), "/api/media/123", &result)
		if err != nil {
			t.Fatalf("GetJSON()でエラーが発生: %v", err)
		}

		// リクエストの検証
		if received.Method != http.MethodGet {
			t.Errorf("Method = %q, want %q", received.Method, http.MethodGet)
		}
		if received.Path != "/api/media/123" {
			t.Errorf("Path = %q, want %q", received.Path, "/api/media/123")
		}

		// レスポンスの検証
		if result.Name != "get-response" {
			t.Errorf("result.Name = %q, want %q", result.Name, "get-response")
		}
		if result.Value != 42 {
			t.Errorf("result.Value = %d, want %d", result.Value, 42)
		}
	})

	t.Run("GETリクエストにリクエストボディが含まれないこと", func(t *testing.T) {
		t.Parallel()

		var receivedBody []byte
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedBody, _ = io.ReadAll(r.Body)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(testPayload{Name: "ok", Value: 1})
		}))
		defer ts.Close()

		client := New(ts.URL)
		var result testPayload

		err := client.GetJSON(context.Background(), "/api/test", &result)
		if err != nil {
			t.Fatalf("GetJSON()でエラーが発生: %v", err)
		}

		if len(receivedBody) != 0 {
			t.Errorf("GETリクエストにボディが含まれている: %q", string(receivedBody))
		}
	})

	t.Run("サーバーが404を返した場合にエラーが返ること", func(t *testing.T) {
		t.Parallel()

		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"error":"not found"}`))
		}))
		defer ts.Close()

		client := New(ts.URL)
		var result testPayload

		err := client.GetJSON(context.Background(), "/api/media/nonexistent", &result)
		if err == nil {
			t.Fatal("GetJSON()がエラーを返すべきだが、nilが返った")
		}
	})

	t.Run("不正なJSONレスポンスでエラーが返ること", func(t *testing.T) {
		t.Parallel()

		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{invalid json}`))
		}))
		defer ts.Close()

		client := New(ts.URL)
		var result testPayload

		err := client.GetJSON(context.Background(), "/api/test", &result)
		if err == nil {
			t.Fatal("GetJSON()がエラーを返すべきだが、nilが返った")
		}
	})

	t.Run("接続できないサーバーに対してエラーが返ること", func(t *testing.T) {
		t.Parallel()

		// 存在しないサーバーに接続を試みる
		client := New("http://127.0.0.1:1")
		var result testPayload

		err := client.GetJSON(context.Background(), "/api/test", &result)
		if err == nil {
			t.Fatal("GetJSON()がエラーを返すべきだが、nilが返った")
		}
	})
}

// TestWithUserID はWithUserID関数を検証する。
func TestWithUserID(t *testing.T) {
	t.Parallel()

	t.Run("コンテキストにユーザーIDを設定して伝播できること", func(t *testing.T) {
		t.Parallel()

		var receivedUserID string
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedUserID = r.Header.Get("X-User-ID")
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(testPayload{Name: "ok", Value: 1})
		}))
		defer ts.Close()

		client := New(ts.URL)
		ctx := WithUserID(context.Background(), "propagated-user-id")
		var result testPayload

		err := client.GetJSON(ctx, "/api/test", &result)
		if err != nil {
			t.Fatalf("GetJSON()でエラーが発生: %v", err)
		}

		if receivedUserID != "propagated-user-id" {
			t.Errorf("X-User-ID = %q, want %q", receivedUserID, "propagated-user-id")
		}
	})

	t.Run("PostJSONでもユーザーIDが伝播されること", func(t *testing.T) {
		t.Parallel()

		var receivedUserID string
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedUserID = r.Header.Get("X-User-ID")
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(testPayload{Name: "ok", Value: 1})
		}))
		defer ts.Close()

		client := New(ts.URL)
		ctx := WithUserID(context.Background(), "post-user-id")
		body := testPayload{Name: "test", Value: 1}
		var result testPayload

		err := client.PostJSON(ctx, "/api/events", body, &result)
		if err != nil {
			t.Fatalf("PostJSON()でエラーが発生: %v", err)
		}

		if receivedUserID != "post-user-id" {
			t.Errorf("X-User-ID = %q, want %q", receivedUserID, "post-user-id")
		}
	})

	t.Run("WithUserIDが設定されていない場合X-User-IDヘッダーが空であること", func(t *testing.T) {
		t.Parallel()

		var receivedUserID string
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedUserID = r.Header.Get("X-User-ID")
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(testPayload{Name: "ok", Value: 1})
		}))
		defer ts.Close()

		client := New(ts.URL)
		var result testPayload

		err := client.GetJSON(context.Background(), "/api/test", &result)
		if err != nil {
			t.Fatalf("GetJSON()でエラーが発生: %v", err)
		}

		if receivedUserID != "" {
			t.Errorf("X-User-ID = %q, want empty string", receivedUserID)
		}
	})

	t.Run("WithUserIDで空文字列を設定した場合の動作", func(t *testing.T) {
		t.Parallel()

		var receivedUserID string
		var hasHeader bool
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedUserID = r.Header.Get("X-User-ID")
			_, hasHeader = r.Header["X-User-Id"]
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(testPayload{Name: "ok", Value: 1})
		}))
		defer ts.Close()

		client := New(ts.URL)
		ctx := WithUserID(context.Background(), "")
		var result testPayload

		err := client.GetJSON(ctx, "/api/test", &result)
		if err != nil {
			t.Fatalf("GetJSON()でエラーが発生: %v", err)
		}

		// 空文字列でもヘッダーは設定される
		if !hasHeader {
			t.Error("空文字列のユーザーIDでもX-User-IDヘッダーが設定されるべき")
		}
		if receivedUserID != "" {
			t.Errorf("X-User-ID = %q, want empty string", receivedUserID)
		}
	})
}

// TestPostJSON_SerializationError はシリアライズ不可能なボディでエラーが返ることを検証する。
func TestPostJSON_SerializationError(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(testPayload{Name: "ok", Value: 1})
	}))
	defer ts.Close()

	client := New(ts.URL)
	// json.Marshalでエラーになるチャネル型を渡す
	body := make(chan int)
	var result testPayload

	err := client.PostJSON(context.Background(), "/api/events", body, &result)
	if err == nil {
		t.Fatal("PostJSON()がエラーを返すべきだが、nilが返った")
	}
}
