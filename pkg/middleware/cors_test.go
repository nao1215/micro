package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestCORS はCORSミドルウェアを検証する。
func TestCORS(t *testing.T) {
	t.Parallel()

	t.Run("許可されたオリジンからのリクエストにCORSヘッダーが設定されること", func(t *testing.T) {
		t.Parallel()

		router := gin.New()
		router.Use(CORS([]string{"http://localhost:3000", "https://example.com"}))
		router.GET("/test", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"status": "ok"})
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("Origin", "http://localhost:3000")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("ステータスコード = %d, want %d", w.Code, http.StatusOK)
		}
		if got := w.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:3000" {
			t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, "http://localhost:3000")
		}
		if got := w.Header().Get("Access-Control-Allow-Methods"); got != "GET, POST, PUT, DELETE, OPTIONS" {
			t.Errorf("Access-Control-Allow-Methods = %q, want %q", got, "GET, POST, PUT, DELETE, OPTIONS")
		}
		if got := w.Header().Get("Access-Control-Allow-Headers"); got != "Authorization, Content-Type" {
			t.Errorf("Access-Control-Allow-Headers = %q, want %q", got, "Authorization, Content-Type")
		}
		if got := w.Header().Get("Access-Control-Max-Age"); got != "86400" {
			t.Errorf("Access-Control-Max-Age = %q, want %q", got, "86400")
		}
	})

	t.Run("許可リストの2番目のオリジンでも正しくCORSヘッダーが設定されること", func(t *testing.T) {
		t.Parallel()

		router := gin.New()
		router.Use(CORS([]string{"http://localhost:3000", "https://example.com"}))
		router.GET("/test", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"status": "ok"})
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("Origin", "https://example.com")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if got := w.Header().Get("Access-Control-Allow-Origin"); got != "https://example.com" {
			t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, "https://example.com")
		}
	})

	t.Run("許可されていないオリジンからのリクエストにCORSヘッダーが設定されないこと", func(t *testing.T) {
		t.Parallel()

		router := gin.New()
		router.Use(CORS([]string{"http://localhost:3000"}))
		router.GET("/test", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"status": "ok"})
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("Origin", "https://evil.com")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("ステータスコード = %d, want %d", w.Code, http.StatusOK)
		}
		if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
			t.Errorf("Access-Control-Allow-Origin = %q, want empty string", got)
		}
	})

	t.Run("Originヘッダーが無いリクエストにCORSヘッダーが設定されないこと", func(t *testing.T) {
		t.Parallel()

		router := gin.New()
		router.Use(CORS([]string{"http://localhost:3000"}))
		router.GET("/test", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"status": "ok"})
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("ステータスコード = %d, want %d", w.Code, http.StatusOK)
		}
		if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
			t.Errorf("Access-Control-Allow-Origin = %q, want empty string", got)
		}
	})

	t.Run("OPTIONSリクエストで204が返りリクエストが中断されること", func(t *testing.T) {
		t.Parallel()

		handlerCalled := false
		router := gin.New()
		router.Use(CORS([]string{"http://localhost:3000"}))
		router.OPTIONS("/test", func(c *gin.Context) {
			handlerCalled = true
			c.JSON(http.StatusOK, gin.H{"status": "ok"})
		})

		req := httptest.NewRequest(http.MethodOptions, "/test", nil)
		req.Header.Set("Origin", "http://localhost:3000")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != http.StatusNoContent {
			t.Errorf("ステータスコード = %d, want %d", w.Code, http.StatusNoContent)
		}
		if handlerCalled {
			t.Error("OPTIONSリクエストでハンドラーが呼ばれるべきではない")
		}
	})

	t.Run("許可されていないオリジンからのOPTIONSリクエストで204が返ること", func(t *testing.T) {
		t.Parallel()

		router := gin.New()
		router.Use(CORS([]string{"http://localhost:3000"}))
		router.OPTIONS("/test", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"status": "ok"})
		})

		req := httptest.NewRequest(http.MethodOptions, "/test", nil)
		req.Header.Set("Origin", "https://evil.com")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		// OPTIONSリクエストは常にAbortWithStatus(204)で中断される
		if w.Code != http.StatusNoContent {
			t.Errorf("ステータスコード = %d, want %d", w.Code, http.StatusNoContent)
		}
		// CORSヘッダーは設定されない
		if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
			t.Errorf("Access-Control-Allow-Origin = %q, want empty string", got)
		}
	})

	t.Run("空のオリジンリストでCORSヘッダーが設定されないこと", func(t *testing.T) {
		t.Parallel()

		router := gin.New()
		router.Use(CORS([]string{}))
		router.GET("/test", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"status": "ok"})
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("Origin", "http://localhost:3000")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("ステータスコード = %d, want %d", w.Code, http.StatusOK)
		}
		if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
			t.Errorf("Access-Control-Allow-Origin = %q, want empty string", got)
		}
	})

	t.Run("GETリクエストではc.Next()が呼ばれハンドラーが実行されること", func(t *testing.T) {
		t.Parallel()

		handlerCalled := false
		router := gin.New()
		router.Use(CORS([]string{"http://localhost:3000"}))
		router.GET("/test", func(c *gin.Context) {
			handlerCalled = true
			c.JSON(http.StatusOK, gin.H{"status": "ok"})
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("Origin", "http://localhost:3000")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if !handlerCalled {
			t.Error("GETリクエストでハンドラーが呼ばれるべき")
		}
	})
}
