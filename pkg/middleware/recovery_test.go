package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestRecovery はRecoveryミドルウェアを検証する。
func TestRecovery(t *testing.T) {
	t.Parallel()

	t.Run("パニックが発生した場合500が返ること", func(t *testing.T) {
		t.Parallel()

		router := gin.New()
		router.Use(Recovery())
		router.GET("/panic", func(_ *gin.Context) {
			panic("テスト用パニック")
		})

		req := httptest.NewRequest(http.MethodGet, "/panic", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != http.StatusInternalServerError {
			t.Errorf("ステータスコード = %d, want %d", w.Code, http.StatusInternalServerError)
		}

		var body map[string]string
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("レスポンスボディのパースに失敗: %v", err)
		}
		if body["error"] != "内部サーバーエラーが発生しました" {
			t.Errorf("error = %q, want %q", body["error"], "内部サーバーエラーが発生しました")
		}
	})

	t.Run("パニックが発生しない場合は正常にレスポンスが返ること", func(t *testing.T) {
		t.Parallel()

		router := gin.New()
		router.Use(Recovery())
		router.GET("/ok", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"status": "ok"})
		})

		req := httptest.NewRequest(http.MethodGet, "/ok", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("ステータスコード = %d, want %d", w.Code, http.StatusOK)
		}

		var body map[string]string
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("レスポンスボディのパースに失敗: %v", err)
		}
		if body["status"] != "ok" {
			t.Errorf("status = %q, want %q", body["status"], "ok")
		}
	})

	t.Run("文字列以外のパニック値でも500が返ること", func(t *testing.T) {
		t.Parallel()

		router := gin.New()
		router.Use(Recovery())
		router.GET("/panic-int", func(_ *gin.Context) {
			panic(42)
		})

		req := httptest.NewRequest(http.MethodGet, "/panic-int", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != http.StatusInternalServerError {
			t.Errorf("ステータスコード = %d, want %d", w.Code, http.StatusInternalServerError)
		}
	})

	t.Run("error型のパニック値でも500が返ること", func(t *testing.T) {
		t.Parallel()

		router := gin.New()
		router.Use(Recovery())
		router.GET("/panic-error", func(_ *gin.Context) {
			panic(http.ErrAbortHandler)
		})

		req := httptest.NewRequest(http.MethodGet, "/panic-error", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != http.StatusInternalServerError {
			t.Errorf("ステータスコード = %d, want %d", w.Code, http.StatusInternalServerError)
		}
	})

	t.Run("パニック後もサーバーが次のリクエストを処理できること", func(t *testing.T) {
		t.Parallel()

		router := gin.New()
		router.Use(Recovery())
		router.GET("/panic", func(_ *gin.Context) {
			panic("パニック発生")
		})
		router.GET("/ok", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"status": "recovered"})
		})

		// パニックが発生するリクエスト
		req1 := httptest.NewRequest(http.MethodGet, "/panic", nil)
		w1 := httptest.NewRecorder()
		router.ServeHTTP(w1, req1)

		if w1.Code != http.StatusInternalServerError {
			t.Errorf("1回目のステータスコード = %d, want %d", w1.Code, http.StatusInternalServerError)
		}

		// パニック後の正常なリクエスト
		req2 := httptest.NewRequest(http.MethodGet, "/ok", nil)
		w2 := httptest.NewRecorder()
		router.ServeHTTP(w2, req2)

		if w2.Code != http.StatusOK {
			t.Errorf("2回目のステータスコード = %d, want %d", w2.Code, http.StatusOK)
		}
	})

	t.Run("POSTリクエストでのパニックでも500が返ること", func(t *testing.T) {
		t.Parallel()

		router := gin.New()
		router.Use(Recovery())
		router.POST("/panic-post", func(_ *gin.Context) {
			panic("POSTでパニック")
		})

		req := httptest.NewRequest(http.MethodPost, "/panic-post", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != http.StatusInternalServerError {
			t.Errorf("ステータスコード = %d, want %d", w.Code, http.StatusInternalServerError)
		}
	})
}
