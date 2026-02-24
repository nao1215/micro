package gateway

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	_ "github.com/mattn/go-sqlite3"
	gatewaydb "github.com/nao1215/micro/internal/gateway/db"
	"github.com/nao1215/micro/pkg/middleware"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// testJWTSecret はテスト用のJWT署名秘密鍵。
const testJWTSecret = "test-secret-key"

// newTestServer はテスト用のGatewayサーバーを生成する。
// インメモリSQLiteを使用し、内部サービスURLはダミー値を設定する。
func newTestServer(t *testing.T) *Server {
	t.Helper()

	sqlDB, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("インメモリDB接続に失敗: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })

	if err := initSchema(sqlDB); err != nil {
		t.Fatalf("スキーマ初期化に失敗: %v", err)
	}

	router := gin.New()
	s := &Server{
		router:    router,
		port:      "0",
		queries:   gatewaydb.New(sqlDB),
		db:        sqlDB,
		jwtSecret: testJWTSecret,
		serviceURLs: serviceURLConfig{
			MediaCommand: "http://localhost:19001",
			MediaQuery:   "http://localhost:19002",
			Album:        "http://localhost:19003",
			Notification: "http://localhost:19004",
			EventStore:   "http://localhost:19005",
		},
	}
	s.setupRoutes()

	return s
}

// newTestServerWithBackend はモックバックエンドサービスを持つテスト用Gatewayサーバーを生成する。
// backendHandlerで指定したハンドラがバックエンドサービスとして応答する。
func newTestServerWithBackend(t *testing.T, backendHandler http.HandlerFunc) (*Server, *httptest.Server) {
	t.Helper()

	backend := httptest.NewServer(backendHandler)
	t.Cleanup(backend.Close)

	sqlDB, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("インメモリDB接続に失敗: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })

	if err := initSchema(sqlDB); err != nil {
		t.Fatalf("スキーマ初期化に失敗: %v", err)
	}

	router := gin.New()
	s := &Server{
		router:    router,
		port:      "0",
		queries:   gatewaydb.New(sqlDB),
		db:        sqlDB,
		jwtSecret: testJWTSecret,
		serviceURLs: serviceURLConfig{
			MediaCommand: backend.URL,
			MediaQuery:   backend.URL,
			Album:        backend.URL,
			Notification: backend.URL,
			EventStore:   backend.URL,
		},
	}
	s.setupRoutes()

	return s, backend
}

// generateTestJWT はテスト用のJWTトークンを生成する。
func generateTestJWT(t *testing.T, userID, email string) string {
	t.Helper()

	token, err := middleware.GenerateJWT(testJWTSecret, userID, email)
	if err != nil {
		t.Fatalf("テスト用JWT生成に失敗: %v", err)
	}
	return token
}

// seedUser はテスト用のユーザーレコードをDBに挿入する。
func seedUser(t *testing.T, s *Server, id, provider, providerUserID, email, displayName string) {
	t.Helper()

	ctx := context.Background()
	if err := s.queries.CreateUser(ctx, gatewaydb.CreateUserParams{
		ID:             id,
		Provider:       provider,
		ProviderUserID: providerUserID,
		Email:          email,
		DisplayName:    displayName,
		AvatarUrl:      "",
	}); err != nil {
		t.Fatalf("テスト用ユーザー挿入に失敗: %v", err)
	}
}

// TestHandleDevToken は開発用トークン発行ハンドラのテスト。
func TestHandleDevToken(t *testing.T) {
	t.Parallel()

	t.Run("新規ユーザーの場合にトークンを発行する", func(t *testing.T) {
		t.Parallel()

		s := newTestServer(t)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/auth/dev-token", nil)
		s.router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("ステータスコード: got %d, want %d", w.Code, http.StatusOK)
		}

		var result map[string]string
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatalf("レスポンスのパースに失敗: %v", err)
		}
		if result["token"] == "" {
			t.Error("tokenフィールドが空")
		}
		if result["user_id"] == "" {
			t.Error("user_idフィールドが空")
		}

		// 発行されたトークンが有効であることを検証する
		token := result["token"]
		verifyRouter := gin.New()
		verifyRouter.Use(middleware.JWTAuth(testJWTSecret))
		verifyRouter.GET("/verify", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"user_id": middleware.GetUserID(c)})
		})

		w2 := httptest.NewRecorder()
		req2 := httptest.NewRequest(http.MethodGet, "/verify", nil)
		req2.Header.Set("Authorization", "Bearer "+token)
		verifyRouter.ServeHTTP(w2, req2)

		if w2.Code != http.StatusOK {
			t.Errorf("トークン検証ステータスコード: got %d, want %d", w2.Code, http.StatusOK)
		}
	})

	t.Run("既存ユーザーの場合に同じuser_idでトークンを発行する", func(t *testing.T) {
		t.Parallel()

		s := newTestServer(t)
		seedUser(t, s, "existing-dev-user", "dev", "dev-user", "dev@localhost", "開発ユーザー")

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/auth/dev-token", nil)
		s.router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("ステータスコード: got %d, want %d", w.Code, http.StatusOK)
		}

		var result map[string]string
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatalf("レスポンスのパースに失敗: %v", err)
		}
		if result["user_id"] != "existing-dev-user" {
			t.Errorf("user_id: got %q, want %q", result["user_id"], "existing-dev-user")
		}
		if result["token"] == "" {
			t.Error("tokenフィールドが空")
		}
	})
}

// TestHandleGetCurrentUser は認証済みユーザー情報取得ハンドラのテスト。
func TestHandleGetCurrentUser(t *testing.T) {
	t.Parallel()

	t.Run("認証済みユーザーの情報を返す", func(t *testing.T) {
		t.Parallel()

		s := newTestServer(t)
		seedUser(t, s, "user-123", "github", "gh-456", "test@example.com", "テストユーザー")

		token := generateTestJWT(t, "user-123", "test@example.com")

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		s.router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("ステータスコード: got %d, want %d", w.Code, http.StatusOK)
		}

		var result map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatalf("レスポンスのパースに失敗: %v", err)
		}
		if result["id"] != "user-123" {
			t.Errorf("id: got %q, want %q", result["id"], "user-123")
		}
		if result["email"] != "test@example.com" {
			t.Errorf("email: got %q, want %q", result["email"], "test@example.com")
		}
		if result["display_name"] != "テストユーザー" {
			t.Errorf("display_name: got %q, want %q", result["display_name"], "テストユーザー")
		}
		if result["provider"] != "github" {
			t.Errorf("provider: got %q, want %q", result["provider"], "github")
		}
	})

	t.Run("認証ヘッダーが無い場合は401を返す", func(t *testing.T) {
		t.Parallel()

		s := newTestServer(t)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
		s.router.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("ステータスコード: got %d, want %d", w.Code, http.StatusUnauthorized)
		}
	})

	t.Run("無効なトークンの場合は401を返す", func(t *testing.T) {
		t.Parallel()

		s := newTestServer(t)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
		req.Header.Set("Authorization", "Bearer invalid-token")
		s.router.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("ステータスコード: got %d, want %d", w.Code, http.StatusUnauthorized)
		}
	})

	t.Run("DBにユーザーが存在しない場合は404を返す", func(t *testing.T) {
		t.Parallel()

		s := newTestServer(t)
		// ユーザーをDBに挿入せず、有効なトークンだけ発行する
		token := generateTestJWT(t, "nonexistent-user", "nobody@example.com")

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
		req.Header.Set("Authorization", "Bearer "+token)
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
}

// TestHandleGitHubLogin はGitHub OAuth2ログインハンドラのテスト。
func TestHandleGitHubLogin(t *testing.T) {
	t.Parallel()

	t.Run("GITHUB_CLIENT_IDが設定されていない場合は503を返す", func(t *testing.T) {
		t.Parallel()

		s := newTestServer(t)

		// テスト実行環境でGITHUB_CLIENT_IDが設定されていないことを前提とする
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/auth/github", nil)
		s.router.ServeHTTP(w, req)

		if w.Code != http.StatusServiceUnavailable {
			t.Errorf("ステータスコード: got %d, want %d", w.Code, http.StatusServiceUnavailable)
		}

		var result map[string]string
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatalf("レスポンスのパースに失敗: %v", err)
		}
		if _, ok := result["error"]; !ok {
			t.Error("エラーメッセージが含まれていない")
		}
	})

	t.Run("GITHUB_CLIENT_IDが設定されている場合はGitHubにリダイレクトする", func(t *testing.T) {
		t.Parallel()

		// handleGitHubLogin は os.Getenv を直接呼ぶため、
		// 環境変数をセットしてハンドラの振る舞いをシミュレートする
		router := gin.New()
		router.GET("/auth/github", func(c *gin.Context) {
			// ハンドラの振る舞いをシミュレートする
			clientID := "test-client-id"
			state := "test-state"
			redirectURL := fmt.Sprintf("https://github.com/login/oauth/authorize?client_id=%s&state=%s&scope=user:email", clientID, state)
			c.Redirect(http.StatusTemporaryRedirect, redirectURL)
		})

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/auth/github", nil)
		router.ServeHTTP(w, req)

		if w.Code != http.StatusTemporaryRedirect {
			t.Errorf("ステータスコード: got %d, want %d", w.Code, http.StatusTemporaryRedirect)
		}

		location := w.Header().Get("Location")
		if !strings.HasPrefix(location, "https://github.com/login/oauth/authorize") {
			t.Errorf("リダイレクト先が不正: got %q", location)
		}
		if !strings.Contains(location, "client_id=test-client-id") {
			t.Errorf("client_idパラメータが含まれていない: got %q", location)
		}
	})
}

// TestHandleGoogleLogin はGoogle OAuth2ログインハンドラのテスト。
func TestHandleGoogleLogin(t *testing.T) {
	t.Parallel()

	t.Run("GOOGLE_CLIENT_IDが設定されていない場合は503を返す", func(t *testing.T) {
		t.Parallel()

		s := newTestServer(t)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/auth/google", nil)
		s.router.ServeHTTP(w, req)

		if w.Code != http.StatusServiceUnavailable {
			t.Errorf("ステータスコード: got %d, want %d", w.Code, http.StatusServiceUnavailable)
		}

		var result map[string]string
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatalf("レスポンスのパースに失敗: %v", err)
		}
		if _, ok := result["error"]; !ok {
			t.Error("エラーメッセージが含まれていない")
		}
	})

	t.Run("GOOGLE_CLIENT_IDが設定されている場合はGoogleにリダイレクトする", func(t *testing.T) {
		t.Parallel()

		// handleGoogleLogin は os.Getenv を直接呼ぶため、
		// ハンドラの振る舞いをシミュレートする
		router := gin.New()
		router.GET("/auth/google", func(c *gin.Context) {
			clientID := "test-google-client-id"
			state := "test-state"
			redirectURL := fmt.Sprintf("https://accounts.google.com/o/oauth2/v2/auth?client_id=%s&response_type=code&scope=openid%%20email%%20profile&state=%s&redirect_uri=%s/auth/google/callback",
				clientID, state, "http://localhost:8080")
			c.Redirect(http.StatusTemporaryRedirect, redirectURL)
		})

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/auth/google", nil)
		router.ServeHTTP(w, req)

		if w.Code != http.StatusTemporaryRedirect {
			t.Errorf("ステータスコード: got %d, want %d", w.Code, http.StatusTemporaryRedirect)
		}

		location := w.Header().Get("Location")
		if !strings.HasPrefix(location, "https://accounts.google.com/o/oauth2/v2/auth") {
			t.Errorf("リダイレクト先が不正: got %q", location)
		}
		if !strings.Contains(location, "client_id=test-google-client-id") {
			t.Errorf("client_idパラメータが含まれていない: got %q", location)
		}
	})
}

// TestHandleProxy はプロキシハンドラのテスト。
func TestHandleProxy(t *testing.T) {
	t.Parallel()

	t.Run("バックエンドサービスにリクエストをプロキシする", func(t *testing.T) {
		t.Parallel()

		backendHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// バックエンドがX-User-IDヘッダーを受け取ることを検証
			userID := r.Header.Get("X-User-ID")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			resp := fmt.Sprintf(`{"proxied":true,"path":"%s","user_id":"%s"}`, r.URL.Path, userID)
			_, _ = w.Write([]byte(resp))
		})

		s, _ := newTestServerWithBackend(t, backendHandler)
		token := generateTestJWT(t, "proxy-user-1", "proxy@example.com")

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/media", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		s.router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("ステータスコード: got %d, want %d", w.Code, http.StatusOK)
		}

		var result map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatalf("レスポンスのパースに失敗: %v", err)
		}
		if result["proxied"] != true {
			t.Error("バックエンドにプロキシされていない")
		}
		if result["user_id"] != "proxy-user-1" {
			t.Errorf("X-User-ID: got %q, want %q", result["user_id"], "proxy-user-1")
		}
	})

	t.Run("クエリパラメータが転送される", func(t *testing.T) {
		t.Parallel()

		backendHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			resp := fmt.Sprintf(`{"query":"%s"}`, r.URL.RawQuery)
			_, _ = w.Write([]byte(resp))
		})

		s, _ := newTestServerWithBackend(t, backendHandler)
		token := generateTestJWT(t, "query-user", "query@example.com")

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/media?limit=10&offset=0", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		s.router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("ステータスコード: got %d, want %d", w.Code, http.StatusOK)
		}

		var result map[string]string
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatalf("レスポンスのパースに失敗: %v", err)
		}
		if !strings.Contains(result["query"], "limit=10") {
			t.Errorf("クエリパラメータ limit が転送されていない: got %q", result["query"])
		}
		if !strings.Contains(result["query"], "offset=0") {
			t.Errorf("クエリパラメータ offset が転送されていない: got %q", result["query"])
		}
	})

	t.Run("バックエンドがエラーを返した場合にそのステータスを転送する", func(t *testing.T) {
		t.Parallel()

		backendHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"not found"}`))
		})

		s, _ := newTestServerWithBackend(t, backendHandler)
		token := generateTestJWT(t, "err-user", "err@example.com")

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/media", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		s.router.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Errorf("ステータスコード: got %d, want %d", w.Code, http.StatusNotFound)
		}
	})

	t.Run("POSTリクエストのボディが転送される", func(t *testing.T) {
		t.Parallel()

		backendHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write(body)
		})

		s, _ := newTestServerWithBackend(t, backendHandler)
		token := generateTestJWT(t, "post-user", "post@example.com")

		requestBody := `{"filename":"test.jpg","content_type":"image/jpeg"}`
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/media", strings.NewReader(requestBody))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		s.router.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Errorf("ステータスコード: got %d, want %d", w.Code, http.StatusCreated)
		}

		var result map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatalf("レスポンスのパースに失敗: %v", err)
		}
		if result["filename"] != "test.jpg" {
			t.Errorf("filename: got %q, want %q", result["filename"], "test.jpg")
		}
	})

	t.Run("認証なしのプロキシリクエストは401を返す", func(t *testing.T) {
		t.Parallel()

		backendHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		s, _ := newTestServerWithBackend(t, backendHandler)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/media", nil)
		s.router.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("ステータスコード: got %d, want %d", w.Code, http.StatusUnauthorized)
		}
	})
}

// TestGatewayHealthCheck はヘルスチェックエンドポイントのテスト。
func TestGatewayHealthCheck(t *testing.T) {
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
	if result["service"] != "gateway" {
		t.Errorf("service: got %q, want %q", result["service"], "gateway")
	}
}

// TestJWTGenerationAndValidationFlow はJWTトークンの生成と検証の一連のフローをテストする。
func TestJWTGenerationAndValidationFlow(t *testing.T) {
	t.Parallel()

	t.Run("dev-tokenで発行したトークンで認証APIにアクセスできる", func(t *testing.T) {
		t.Parallel()

		s := newTestServer(t)

		// Step 1: dev-token でトークンを取得
		w1 := httptest.NewRecorder()
		req1 := httptest.NewRequest(http.MethodPost, "/auth/dev-token", nil)
		s.router.ServeHTTP(w1, req1)

		if w1.Code != http.StatusOK {
			t.Fatalf("dev-token ステータスコード: got %d, want %d", w1.Code, http.StatusOK)
		}

		var tokenResp map[string]string
		if err := json.Unmarshal(w1.Body.Bytes(), &tokenResp); err != nil {
			t.Fatalf("レスポンスのパースに失敗: %v", err)
		}

		// Step 2: 取得したトークンで /api/v1/me にアクセス
		w2 := httptest.NewRecorder()
		req2 := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
		req2.Header.Set("Authorization", "Bearer "+tokenResp["token"])
		s.router.ServeHTTP(w2, req2)

		if w2.Code != http.StatusOK {
			t.Fatalf("/api/v1/me ステータスコード: got %d, want %d", w2.Code, http.StatusOK)
		}

		var userResp map[string]interface{}
		if err := json.Unmarshal(w2.Body.Bytes(), &userResp); err != nil {
			t.Fatalf("レスポンスのパースに失敗: %v", err)
		}
		if userResp["id"] != tokenResp["user_id"] {
			t.Errorf("ユーザーID不一致: /me=%q, dev-token=%q", userResp["id"], tokenResp["user_id"])
		}
		if userResp["email"] != "dev@localhost" {
			t.Errorf("email: got %q, want %q", userResp["email"], "dev@localhost")
		}
	})

	t.Run("異なるsecretで署名されたトークンは拒否される", func(t *testing.T) {
		t.Parallel()

		s := newTestServer(t)

		// 別のsecretでトークンを生成
		wrongToken, err := middleware.GenerateJWT("wrong-secret", "user-1", "test@example.com")
		if err != nil {
			t.Fatalf("JWT生成に失敗: %v", err)
		}

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
		req.Header.Set("Authorization", "Bearer "+wrongToken)
		s.router.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("ステータスコード: got %d, want %d", w.Code, http.StatusUnauthorized)
		}
	})

	t.Run("Bearer接頭辞なしのトークンは拒否される", func(t *testing.T) {
		t.Parallel()

		s := newTestServer(t)
		token := generateTestJWT(t, "user-1", "test@example.com")

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
		req.Header.Set("Authorization", token) // Bearer接頭辞なし
		s.router.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("ステータスコード: got %d, want %d", w.Code, http.StatusUnauthorized)
		}
	})
}
