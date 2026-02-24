package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// testSecret はテスト用のJWTシークレット。
const testSecret = "test-secret-key-for-unit-tests"

// TestGenerateJWT はGenerateJWT関数を検証する。
func TestGenerateJWT(t *testing.T) {
	t.Parallel()

	t.Run("正常にJWTトークンを生成できること", func(t *testing.T) {
		t.Parallel()

		tokenStr, err := GenerateJWT(testSecret, "user-123", "test@example.com")
		if err != nil {
			t.Fatalf("GenerateJWT()でエラーが発生: %v", err)
		}
		if tokenStr == "" {
			t.Fatal("GenerateJWT()が空文字列を返した")
		}

		// トークンをパースして検証する
		claims := &JWTClaims{}
		token, err := jwt.ParseWithClaims(tokenStr, claims, func(_ *jwt.Token) (any, error) {
			return []byte(testSecret), nil
		})
		if err != nil {
			t.Fatalf("トークンのパースに失敗: %v", err)
		}
		if !token.Valid {
			t.Fatal("トークンが無効")
		}

		if claims.UserID != "user-123" {
			t.Errorf("UserID = %q, want %q", claims.UserID, "user-123")
		}
		if claims.Email != "test@example.com" {
			t.Errorf("Email = %q, want %q", claims.Email, "test@example.com")
		}
		if claims.Issuer != "mediahub-gateway" {
			t.Errorf("Issuer = %q, want %q", claims.Issuer, "mediahub-gateway")
		}
	})

	t.Run("トークンの有効期限が24時間後であること", func(t *testing.T) {
		t.Parallel()

		before := time.Now()
		tokenStr, err := GenerateJWT(testSecret, "user-exp", "exp@example.com")
		if err != nil {
			t.Fatalf("GenerateJWT()でエラーが発生: %v", err)
		}

		claims := &JWTClaims{}
		_, err = jwt.ParseWithClaims(tokenStr, claims, func(_ *jwt.Token) (any, error) {
			return []byte(testSecret), nil
		})
		if err != nil {
			t.Fatalf("トークンのパースに失敗: %v", err)
		}

		expectedExpiry := before.Add(24 * time.Hour)
		// 有効期限が24時間後の前後1分以内であること
		if claims.ExpiresAt.Time.Before(expectedExpiry.Add(-1 * time.Minute)) {
			t.Errorf("ExpiresAt = %v, 期待する最小値: %v", claims.ExpiresAt.Time, expectedExpiry.Add(-1*time.Minute))
		}
		if claims.ExpiresAt.Time.After(expectedExpiry.Add(1 * time.Minute)) {
			t.Errorf("ExpiresAt = %v, 期待する最大値: %v", claims.ExpiresAt.Time, expectedExpiry.Add(1*time.Minute))
		}
	})

	t.Run("IssuedAtが設定されていること", func(t *testing.T) {
		t.Parallel()

		before := time.Now()
		tokenStr, err := GenerateJWT(testSecret, "user-iat", "iat@example.com")
		after := time.Now()
		if err != nil {
			t.Fatalf("GenerateJWT()でエラーが発生: %v", err)
		}

		claims := &JWTClaims{}
		_, err = jwt.ParseWithClaims(tokenStr, claims, func(_ *jwt.Token) (any, error) {
			return []byte(testSecret), nil
		})
		if err != nil {
			t.Fatalf("トークンのパースに失敗: %v", err)
		}

		if claims.IssuedAt.Time.Before(before.Add(-1 * time.Second)) {
			t.Errorf("IssuedAtが呼び出し前の時刻: %v < %v", claims.IssuedAt.Time, before)
		}
		if claims.IssuedAt.Time.After(after.Add(1 * time.Second)) {
			t.Errorf("IssuedAtが呼び出し後の時刻: %v > %v", claims.IssuedAt.Time, after)
		}
	})

	t.Run("署名アルゴリズムがHS256であること", func(t *testing.T) {
		t.Parallel()

		tokenStr, err := GenerateJWT(testSecret, "user-alg", "alg@example.com")
		if err != nil {
			t.Fatalf("GenerateJWT()でエラーが発生: %v", err)
		}

		token, _, err := new(jwt.Parser).ParseUnverified(tokenStr, &JWTClaims{})
		if err != nil {
			t.Fatalf("トークンのパースに失敗: %v", err)
		}

		if token.Method.Alg() != "HS256" {
			t.Errorf("署名アルゴリズム = %q, want %q", token.Method.Alg(), "HS256")
		}
	})

	t.Run("異なるシークレットでは検証に失敗すること", func(t *testing.T) {
		t.Parallel()

		tokenStr, err := GenerateJWT(testSecret, "user-wrong", "wrong@example.com")
		if err != nil {
			t.Fatalf("GenerateJWT()でエラーが発生: %v", err)
		}

		claims := &JWTClaims{}
		_, err = jwt.ParseWithClaims(tokenStr, claims, func(_ *jwt.Token) (any, error) {
			return []byte("wrong-secret"), nil
		})
		if err == nil {
			t.Fatal("異なるシークレットでの検証がエラーを返すべき")
		}
	})
}

// TestJWTAuth はJWTAuthミドルウェアを検証する。
func TestJWTAuth(t *testing.T) {
	t.Parallel()

	t.Run("有効なトークンでリクエストが成功すること", func(t *testing.T) {
		t.Parallel()

		tokenStr, err := GenerateJWT(testSecret, "user-ok", "ok@example.com")
		if err != nil {
			t.Fatalf("GenerateJWT()でエラーが発生: %v", err)
		}

		var capturedUserID, capturedEmail string
		router := gin.New()
		router.Use(JWTAuth(testSecret))
		router.GET("/test", func(c *gin.Context) {
			if v, ok := c.Get("user_id"); ok {
				capturedUserID, _ = v.(string)
			}
			if v, ok := c.Get("email"); ok {
				capturedEmail, _ = v.(string)
			}
			c.JSON(http.StatusOK, gin.H{"status": "ok"})
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("Authorization", "Bearer "+tokenStr)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("ステータスコード = %d, want %d", w.Code, http.StatusOK)
		}
		if capturedUserID != "user-ok" {
			t.Errorf("user_id = %q, want %q", capturedUserID, "user-ok")
		}
		if capturedEmail != "ok@example.com" {
			t.Errorf("email = %q, want %q", capturedEmail, "ok@example.com")
		}
	})

	t.Run("有効なトークンでX-User-IDヘッダーが設定されること", func(t *testing.T) {
		t.Parallel()

		tokenStr, err := GenerateJWT(testSecret, "user-header", "header@example.com")
		if err != nil {
			t.Fatalf("GenerateJWT()でエラーが発生: %v", err)
		}

		router := gin.New()
		router.Use(JWTAuth(testSecret))
		router.GET("/test", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"status": "ok"})
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("Authorization", "Bearer "+tokenStr)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("ステータスコード = %d, want %d", w.Code, http.StatusOK)
		}
		if got := w.Header().Get("X-User-ID"); got != "user-header" {
			t.Errorf("X-User-ID = %q, want %q", got, "user-header")
		}
	})

	t.Run("Authorizationヘッダーが無い場合401が返ること", func(t *testing.T) {
		t.Parallel()

		router := gin.New()
		router.Use(JWTAuth(testSecret))
		router.GET("/test", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"status": "ok"})
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("ステータスコード = %d, want %d", w.Code, http.StatusUnauthorized)
		}

		var body map[string]string
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("レスポンスボディのパースに失敗: %v", err)
		}
		if body["error"] != "Authorizationヘッダーが必要です" {
			t.Errorf("error = %q, want %q", body["error"], "Authorizationヘッダーが必要です")
		}
	})

	t.Run("Bearer接頭辞が無い場合401が返ること", func(t *testing.T) {
		t.Parallel()

		tokenStr, err := GenerateJWT(testSecret, "user-nobearer", "nobearer@example.com")
		if err != nil {
			t.Fatalf("GenerateJWT()でエラーが発生: %v", err)
		}

		router := gin.New()
		router.Use(JWTAuth(testSecret))
		router.GET("/test", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"status": "ok"})
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("Authorization", tokenStr) // "Bearer "接頭辞なし
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("ステータスコード = %d, want %d", w.Code, http.StatusUnauthorized)
		}

		var body map[string]string
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("レスポンスボディのパースに失敗: %v", err)
		}
		if body["error"] != "Bearer トークン形式が不正です" {
			t.Errorf("error = %q, want %q", body["error"], "Bearer トークン形式が不正です")
		}
	})

	t.Run("無効なトークンで401が返ること", func(t *testing.T) {
		t.Parallel()

		router := gin.New()
		router.Use(JWTAuth(testSecret))
		router.GET("/test", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"status": "ok"})
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("Authorization", "Bearer invalid-token-string")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("ステータスコード = %d, want %d", w.Code, http.StatusUnauthorized)
		}

		var body map[string]string
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("レスポンスボディのパースに失敗: %v", err)
		}
		if body["error"] != "トークンが無効です" {
			t.Errorf("error = %q, want %q", body["error"], "トークンが無効です")
		}
	})

	t.Run("異なるシークレットで署名されたトークンで401が返ること", func(t *testing.T) {
		t.Parallel()

		tokenStr, err := GenerateJWT("different-secret", "user-diff", "diff@example.com")
		if err != nil {
			t.Fatalf("GenerateJWT()でエラーが発生: %v", err)
		}

		router := gin.New()
		router.Use(JWTAuth(testSecret))
		router.GET("/test", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"status": "ok"})
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("Authorization", "Bearer "+tokenStr)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("ステータスコード = %d, want %d", w.Code, http.StatusUnauthorized)
		}
	})

	t.Run("期限切れトークンで401が返ること", func(t *testing.T) {
		t.Parallel()

		// 期限切れのクレームを手動で生成する
		claims := JWTClaims{
			RegisteredClaims: jwt.RegisteredClaims{
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(-1 * time.Hour)),
				IssuedAt:  jwt.NewNumericDate(time.Now().Add(-25 * time.Hour)),
				Issuer:    "mediahub-gateway",
			},
			UserID: "user-expired",
			Email:  "expired@example.com",
		}

		token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		tokenStr, err := token.SignedString([]byte(testSecret))
		if err != nil {
			t.Fatalf("トークンの署名に失敗: %v", err)
		}

		router := gin.New()
		router.Use(JWTAuth(testSecret))
		router.GET("/test", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"status": "ok"})
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("Authorization", "Bearer "+tokenStr)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("ステータスコード = %d, want %d", w.Code, http.StatusUnauthorized)
		}
	})
}

// TestGetUserID はGetUserID関数を検証する。
func TestGetUserID(t *testing.T) {
	t.Parallel()

	t.Run("コンテキストにuser_idが設定されている場合に取得できること", func(t *testing.T) {
		t.Parallel()

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Set("user_id", "user-get-id")

		got := GetUserID(c)
		if got != "user-get-id" {
			t.Errorf("GetUserID() = %q, want %q", got, "user-get-id")
		}
	})

	t.Run("コンテキストにuser_idが設定されていない場合に空文字列が返ること", func(t *testing.T) {
		t.Parallel()

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		got := GetUserID(c)
		if got != "" {
			t.Errorf("GetUserID() = %q, want empty string", got)
		}
	})

	t.Run("user_idが文字列以外の型の場合に空文字列が返ること", func(t *testing.T) {
		t.Parallel()

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Set("user_id", 12345)

		got := GetUserID(c)
		if got != "" {
			t.Errorf("GetUserID() = %q, want empty string", got)
		}
	})

	t.Run("JWTAuthミドルウェア経由でGetUserIDが正しく動作すること", func(t *testing.T) {
		t.Parallel()

		tokenStr, err := GenerateJWT(testSecret, "user-e2e", "e2e@example.com")
		if err != nil {
			t.Fatalf("GenerateJWT()でエラーが発生: %v", err)
		}

		var gotUserID string
		router := gin.New()
		router.Use(JWTAuth(testSecret))
		router.GET("/test", func(c *gin.Context) {
			gotUserID = GetUserID(c)
			c.JSON(http.StatusOK, gin.H{"user_id": gotUserID})
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("Authorization", "Bearer "+tokenStr)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("ステータスコード = %d, want %d", w.Code, http.StatusOK)
		}
		if gotUserID != "user-e2e" {
			t.Errorf("GetUserID() = %q, want %q", gotUserID, "user-e2e")
		}
	})
}
