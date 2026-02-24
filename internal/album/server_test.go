package album

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
	albumdb "github.com/nao1215/micro/internal/album/db"
	"github.com/nao1215/micro/pkg/httpclient"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// setupTestServer はテスト用のアルバムサーバーをインメモリSQLiteで構築する。
// Event Storeのモックサーバーも生成し、テスト終了時にクリーンアップする。
func setupTestServer(t *testing.T) (*Server, *gin.Engine) {
	t.Helper()

	sqlDB, err := sql.Open("sqlite", ":memory:?_foreign_keys=ON")
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
		router:      router,
		port:        "0",
		queries:     albumdb.New(sqlDB),
		db:          sqlDB,
		eventClient: httpclient.New(eventStore.URL),
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
		albums := api.Group("/albums")
		{
			albums.POST("", s.handleCreate())
			albums.GET("", s.handleList())
			albums.GET("/:id", s.handleGetByID())
			albums.PUT("/:id", s.handleUpdate())
			albums.DELETE("/:id", s.handleDelete())
			albums.POST("/:id/media", s.handleAddMedia())
			albums.DELETE("/:id/media/:media_id", s.handleRemoveMedia())
			albums.GET("/:id/media", s.handleListMedia())
		}
	}
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "album"})
	})

	return s, router
}

// createTestAlbum はテスト用にアルバムをDBに直接挿入するヘルパー関数。
func createTestAlbum(t *testing.T, s *Server, id, userID, name, description string) {
	t.Helper()
	err := s.queries.CreateAlbum(
		t.Context(),
		albumdb.CreateAlbumParams{
			ID:          id,
			UserID:      userID,
			Name:        name,
			Description: description,
		},
	)
	if err != nil {
		t.Fatalf("テスト用アルバムの作成に失敗: %v", err)
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
	if result["service"] != "album" {
		t.Errorf("service: got %v, want album", result["service"])
	}
}

// TestHandleCreateAlbum はアルバム作成ハンドラのテスト。
func TestHandleCreateAlbum(t *testing.T) {
	t.Parallel()

	t.Run("正常にアルバムを作成できる", func(t *testing.T) {
		t.Parallel()
		_, router := setupTestServer(t)

		body := map[string]string{
			"name":        "旅行の写真",
			"description": "2024年の旅行写真集",
		}
		w := doRequest(router, http.MethodPost, "/api/v1/albums", "user-1", body)

		if w.Code != http.StatusCreated {
			t.Errorf("ステータスコード: got %d, want %d, body=%s", w.Code, http.StatusCreated, w.Body.String())
		}

		result := parseJSON(t, w)
		if result["name"] != "旅行の写真" {
			t.Errorf("name: got %v, want 旅行の写真", result["name"])
		}
		if result["description"] != "2024年の旅行写真集" {
			t.Errorf("description: got %v, want 2024年の旅行写真集", result["description"])
		}
		if result["user_id"] != "user-1" {
			t.Errorf("user_id: got %v, want user-1", result["user_id"])
		}
		if result["id"] == nil || result["id"] == "" {
			t.Error("idが空です")
		}
	})

	t.Run("名前が未指定の場合はBadRequest", func(t *testing.T) {
		t.Parallel()
		_, router := setupTestServer(t)

		body := map[string]string{
			"description": "説明のみ",
		}
		w := doRequest(router, http.MethodPost, "/api/v1/albums", "user-1", body)

		if w.Code != http.StatusBadRequest {
			t.Errorf("ステータスコード: got %d, want %d", w.Code, http.StatusBadRequest)
		}

		result := parseJSON(t, w)
		if result["error"] == nil {
			t.Error("エラーメッセージが含まれていません")
		}
	})

	t.Run("ユーザーIDが未設定の場合はUnauthorized", func(t *testing.T) {
		t.Parallel()
		_, router := setupTestServer(t)

		body := map[string]string{"name": "テスト"}
		w := doRequest(router, http.MethodPost, "/api/v1/albums", "", body)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("ステータスコード: got %d, want %d", w.Code, http.StatusUnauthorized)
		}
	})
}

// TestHandleListAlbums はアルバム一覧取得ハンドラのテスト。
func TestHandleListAlbums(t *testing.T) {
	t.Parallel()

	t.Run("アルバムが存在しない場合は空配列を返す", func(t *testing.T) {
		t.Parallel()
		_, router := setupTestServer(t)

		w := doRequest(router, http.MethodGet, "/api/v1/albums", "user-1", nil)

		if w.Code != http.StatusOK {
			t.Errorf("ステータスコード: got %d, want %d", w.Code, http.StatusOK)
		}

		result := parseJSONArray(t, w)
		if len(result) != 0 {
			t.Errorf("配列の長さ: got %d, want 0", len(result))
		}
	})

	t.Run("作成済みアルバムの一覧を取得できる", func(t *testing.T) {
		t.Parallel()
		s, router := setupTestServer(t)

		createTestAlbum(t, s, "album-1", "user-1", "アルバム1", "説明1")
		createTestAlbum(t, s, "album-2", "user-1", "アルバム2", "説明2")
		// 別ユーザーのアルバムは含まれないことを確認するため
		createTestAlbum(t, s, "album-3", "user-2", "他ユーザー", "他ユーザーの説明")

		w := doRequest(router, http.MethodGet, "/api/v1/albums", "user-1", nil)

		if w.Code != http.StatusOK {
			t.Errorf("ステータスコード: got %d, want %d", w.Code, http.StatusOK)
		}

		result := parseJSONArray(t, w)
		if len(result) != 2 {
			t.Errorf("配列の長さ: got %d, want 2", len(result))
		}
	})

	t.Run("ユーザーIDが未設定の場合はUnauthorized", func(t *testing.T) {
		t.Parallel()
		_, router := setupTestServer(t)

		w := doRequest(router, http.MethodGet, "/api/v1/albums", "", nil)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("ステータスコード: got %d, want %d", w.Code, http.StatusUnauthorized)
		}
	})
}

// TestHandleGetAlbum はアルバム詳細取得ハンドラのテスト。
func TestHandleGetAlbum(t *testing.T) {
	t.Parallel()

	t.Run("正常にアルバムを取得できる", func(t *testing.T) {
		t.Parallel()
		s, router := setupTestServer(t)

		createTestAlbum(t, s, "album-1", "user-1", "テストアルバム", "テスト説明")

		w := doRequest(router, http.MethodGet, "/api/v1/albums/album-1", "user-1", nil)

		if w.Code != http.StatusOK {
			t.Errorf("ステータスコード: got %d, want %d", w.Code, http.StatusOK)
		}

		result := parseJSON(t, w)
		if result["id"] != "album-1" {
			t.Errorf("id: got %v, want album-1", result["id"])
		}
		if result["name"] != "テストアルバム" {
			t.Errorf("name: got %v, want テストアルバム", result["name"])
		}
		if result["description"] != "テスト説明" {
			t.Errorf("description: got %v, want テスト説明", result["description"])
		}
	})

	t.Run("存在しないアルバムの場合はNotFound", func(t *testing.T) {
		t.Parallel()
		_, router := setupTestServer(t)

		w := doRequest(router, http.MethodGet, "/api/v1/albums/nonexistent", "user-1", nil)

		if w.Code != http.StatusNotFound {
			t.Errorf("ステータスコード: got %d, want %d", w.Code, http.StatusNotFound)
		}
	})

	t.Run("他ユーザーのアルバムにアクセスするとForbidden", func(t *testing.T) {
		t.Parallel()
		s, router := setupTestServer(t)

		createTestAlbum(t, s, "album-1", "user-1", "ユーザー1のアルバム", "説明")

		w := doRequest(router, http.MethodGet, "/api/v1/albums/album-1", "user-2", nil)

		if w.Code != http.StatusForbidden {
			t.Errorf("ステータスコード: got %d, want %d", w.Code, http.StatusForbidden)
		}
	})

	t.Run("ユーザーIDが未設定の場合はUnauthorized", func(t *testing.T) {
		t.Parallel()
		_, router := setupTestServer(t)

		w := doRequest(router, http.MethodGet, "/api/v1/albums/album-1", "", nil)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("ステータスコード: got %d, want %d", w.Code, http.StatusUnauthorized)
		}
	})
}

// TestHandleDeleteAlbum はアルバム削除ハンドラのテスト。
func TestHandleDeleteAlbum(t *testing.T) {
	t.Parallel()

	t.Run("正常にアルバムを削除できる", func(t *testing.T) {
		t.Parallel()
		s, router := setupTestServer(t)

		createTestAlbum(t, s, "album-1", "user-1", "削除対象", "削除テスト")

		w := doRequest(router, http.MethodDelete, "/api/v1/albums/album-1", "user-1", nil)

		if w.Code != http.StatusOK {
			t.Errorf("ステータスコード: got %d, want %d", w.Code, http.StatusOK)
		}

		result := parseJSON(t, w)
		if result["message"] == nil {
			t.Error("messageが含まれていません")
		}

		// 削除後に取得するとNotFoundになることを確認する
		w2 := doRequest(router, http.MethodGet, "/api/v1/albums/album-1", "user-1", nil)
		if w2.Code != http.StatusNotFound {
			t.Errorf("削除後のステータスコード: got %d, want %d", w2.Code, http.StatusNotFound)
		}
	})

	t.Run("他ユーザーのアルバムを削除するとForbidden", func(t *testing.T) {
		t.Parallel()
		s, router := setupTestServer(t)

		createTestAlbum(t, s, "album-1", "user-1", "ユーザー1のアルバム", "説明")

		w := doRequest(router, http.MethodDelete, "/api/v1/albums/album-1", "user-2", nil)

		if w.Code != http.StatusForbidden {
			t.Errorf("ステータスコード: got %d, want %d", w.Code, http.StatusForbidden)
		}
	})

	t.Run("存在しないアルバムを削除するとNotFound", func(t *testing.T) {
		t.Parallel()
		_, router := setupTestServer(t)

		w := doRequest(router, http.MethodDelete, "/api/v1/albums/nonexistent", "user-1", nil)

		if w.Code != http.StatusNotFound {
			t.Errorf("ステータスコード: got %d, want %d", w.Code, http.StatusNotFound)
		}
	})

	t.Run("ユーザーIDが未設定の場合はUnauthorized", func(t *testing.T) {
		t.Parallel()
		_, router := setupTestServer(t)

		w := doRequest(router, http.MethodDelete, "/api/v1/albums/album-1", "", nil)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("ステータスコード: got %d, want %d", w.Code, http.StatusUnauthorized)
		}
	})
}

// TestHandleAddMedia はアルバムへのメディア追加ハンドラのテスト。
func TestHandleAddMedia(t *testing.T) {
	t.Parallel()

	t.Run("正常にメディアを追加できる", func(t *testing.T) {
		t.Parallel()
		s, router := setupTestServer(t)

		createTestAlbum(t, s, "album-1", "user-1", "テストアルバム", "説明")

		body := map[string]string{"media_id": "media-1"}
		w := doRequest(router, http.MethodPost, "/api/v1/albums/album-1/media", "user-1", body)

		if w.Code != http.StatusOK {
			t.Errorf("ステータスコード: got %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
		}

		result := parseJSON(t, w)
		if result["message"] == nil {
			t.Error("messageが含まれていません")
		}
	})

	t.Run("media_idが未指定の場合はBadRequest", func(t *testing.T) {
		t.Parallel()
		s, router := setupTestServer(t)

		createTestAlbum(t, s, "album-1", "user-1", "テストアルバム", "説明")

		body := map[string]string{}
		w := doRequest(router, http.MethodPost, "/api/v1/albums/album-1/media", "user-1", body)

		if w.Code != http.StatusBadRequest {
			t.Errorf("ステータスコード: got %d, want %d", w.Code, http.StatusBadRequest)
		}
	})

	t.Run("存在しないアルバムへの追加はNotFound", func(t *testing.T) {
		t.Parallel()
		_, router := setupTestServer(t)

		body := map[string]string{"media_id": "media-1"}
		w := doRequest(router, http.MethodPost, "/api/v1/albums/nonexistent/media", "user-1", body)

		if w.Code != http.StatusNotFound {
			t.Errorf("ステータスコード: got %d, want %d", w.Code, http.StatusNotFound)
		}
	})

	t.Run("他ユーザーのアルバムへの追加はForbidden", func(t *testing.T) {
		t.Parallel()
		s, router := setupTestServer(t)

		createTestAlbum(t, s, "album-1", "user-1", "ユーザー1のアルバム", "説明")

		body := map[string]string{"media_id": "media-1"}
		w := doRequest(router, http.MethodPost, "/api/v1/albums/album-1/media", "user-2", body)

		if w.Code != http.StatusForbidden {
			t.Errorf("ステータスコード: got %d, want %d", w.Code, http.StatusForbidden)
		}
	})

	t.Run("ユーザーIDが未設定の場合はUnauthorized", func(t *testing.T) {
		t.Parallel()
		_, router := setupTestServer(t)

		body := map[string]string{"media_id": "media-1"}
		w := doRequest(router, http.MethodPost, "/api/v1/albums/album-1/media", "", body)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("ステータスコード: got %d, want %d", w.Code, http.StatusUnauthorized)
		}
	})
}

// TestHandleListAlbumMedia はアルバム内メディア一覧取得ハンドラのテスト。
func TestHandleListAlbumMedia(t *testing.T) {
	t.Parallel()

	t.Run("メディアが存在しない場合は空配列を返す", func(t *testing.T) {
		t.Parallel()
		s, router := setupTestServer(t)

		createTestAlbum(t, s, "album-1", "user-1", "空のアルバム", "説明")

		w := doRequest(router, http.MethodGet, "/api/v1/albums/album-1/media", "user-1", nil)

		if w.Code != http.StatusOK {
			t.Errorf("ステータスコード: got %d, want %d", w.Code, http.StatusOK)
		}

		result := parseJSONArray(t, w)
		if len(result) != 0 {
			t.Errorf("配列の長さ: got %d, want 0", len(result))
		}
	})

	t.Run("追加済みメディアの一覧を取得できる", func(t *testing.T) {
		t.Parallel()
		s, router := setupTestServer(t)

		createTestAlbum(t, s, "album-1", "user-1", "テストアルバム", "説明")

		// メディアをDBに直接追加する
		err := s.queries.AddMediaToAlbum(t.Context(), albumdb.AddMediaToAlbumParams{
			AlbumID: "album-1",
			MediaID: "media-1",
		})
		if err != nil {
			t.Fatalf("メディア追加に失敗: %v", err)
		}
		err = s.queries.AddMediaToAlbum(t.Context(), albumdb.AddMediaToAlbumParams{
			AlbumID: "album-1",
			MediaID: "media-2",
		})
		if err != nil {
			t.Fatalf("メディア追加に失敗: %v", err)
		}

		w := doRequest(router, http.MethodGet, "/api/v1/albums/album-1/media", "user-1", nil)

		if w.Code != http.StatusOK {
			t.Errorf("ステータスコード: got %d, want %d", w.Code, http.StatusOK)
		}

		result := parseJSONArray(t, w)
		if len(result) != 2 {
			t.Errorf("配列の長さ: got %d, want 2", len(result))
		}
	})

	t.Run("他ユーザーのアルバムメディア一覧はForbidden", func(t *testing.T) {
		t.Parallel()
		s, router := setupTestServer(t)

		createTestAlbum(t, s, "album-1", "user-1", "ユーザー1のアルバム", "説明")

		w := doRequest(router, http.MethodGet, "/api/v1/albums/album-1/media", "user-2", nil)

		if w.Code != http.StatusForbidden {
			t.Errorf("ステータスコード: got %d, want %d", w.Code, http.StatusForbidden)
		}
	})

	t.Run("存在しないアルバムの場合はNotFound", func(t *testing.T) {
		t.Parallel()
		_, router := setupTestServer(t)

		w := doRequest(router, http.MethodGet, "/api/v1/albums/nonexistent/media", "user-1", nil)

		if w.Code != http.StatusNotFound {
			t.Errorf("ステータスコード: got %d, want %d", w.Code, http.StatusNotFound)
		}
	})
}

// TestHandleRemoveMedia はアルバムからのメディア削除ハンドラのテスト。
func TestHandleRemoveMedia(t *testing.T) {
	t.Parallel()

	t.Run("正常にメディアを削除できる", func(t *testing.T) {
		t.Parallel()
		s, router := setupTestServer(t)

		createTestAlbum(t, s, "album-1", "user-1", "テストアルバム", "説明")
		err := s.queries.AddMediaToAlbum(t.Context(), albumdb.AddMediaToAlbumParams{
			AlbumID: "album-1",
			MediaID: "media-1",
		})
		if err != nil {
			t.Fatalf("メディア追加に失敗: %v", err)
		}

		w := doRequest(router, http.MethodDelete, "/api/v1/albums/album-1/media/media-1", "user-1", nil)

		if w.Code != http.StatusOK {
			t.Errorf("ステータスコード: got %d, want %d", w.Code, http.StatusOK)
		}

		result := parseJSON(t, w)
		if result["message"] == nil {
			t.Error("messageが含まれていません")
		}

		// 削除後にメディア一覧を取得して空であることを確認する
		w2 := doRequest(router, http.MethodGet, "/api/v1/albums/album-1/media", "user-1", nil)
		list := parseJSONArray(t, w2)
		if len(list) != 0 {
			t.Errorf("削除後のメディア数: got %d, want 0", len(list))
		}
	})

	t.Run("他ユーザーのアルバムからの削除はForbidden", func(t *testing.T) {
		t.Parallel()
		s, router := setupTestServer(t)

		createTestAlbum(t, s, "album-1", "user-1", "ユーザー1のアルバム", "説明")

		w := doRequest(router, http.MethodDelete, "/api/v1/albums/album-1/media/media-1", "user-2", nil)

		if w.Code != http.StatusForbidden {
			t.Errorf("ステータスコード: got %d, want %d", w.Code, http.StatusForbidden)
		}
	})

	t.Run("存在しないアルバムからの削除はNotFound", func(t *testing.T) {
		t.Parallel()
		_, router := setupTestServer(t)

		w := doRequest(router, http.MethodDelete, "/api/v1/albums/nonexistent/media/media-1", "user-1", nil)

		if w.Code != http.StatusNotFound {
			t.Errorf("ステータスコード: got %d, want %d", w.Code, http.StatusNotFound)
		}
	})

	t.Run("ユーザーIDが未設定の場合はUnauthorized", func(t *testing.T) {
		t.Parallel()
		_, router := setupTestServer(t)

		w := doRequest(router, http.MethodDelete, "/api/v1/albums/album-1/media/media-1", "", nil)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("ステータスコード: got %d, want %d", w.Code, http.StatusUnauthorized)
		}
	})
}

// TestEnsureDefaultAlbum はデフォルトアルバム自動作成の動作を検証する。
func TestEnsureDefaultAlbum(t *testing.T) {
	t.Parallel()

	t.Run("デフォルトアルバムが存在しない場合は新規作成される", func(t *testing.T) {
		t.Parallel()
		s, router := setupTestServer(t)

		// ユーザーのアルバムを作成する（デフォルトではない）
		createTestAlbum(t, s, "album-1", "user-1", "普通のアルバム", "説明")

		// メディアを追加するとデフォルトアルバムが自動作成される
		body := map[string]string{"media_id": "media-1"}
		w := doRequest(router, http.MethodPost, "/api/v1/albums/album-1/media", "user-1", body)

		if w.Code != http.StatusOK {
			t.Errorf("ステータスコード: got %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
		}

		// アルバム一覧にデフォルトアルバムが含まれることを確認する
		w2 := doRequest(router, http.MethodGet, "/api/v1/albums", "user-1", nil)
		albums := parseJSONArray(t, w2)

		found := false
		for _, a := range albums {
			if a["name"] == "All Media" {
				found = true
				break
			}
		}
		if !found {
			t.Error("デフォルトアルバム 'All Media' が作成されていません")
		}
	})

	t.Run("デフォルトアルバムが既に存在する場合は再作成されない", func(t *testing.T) {
		t.Parallel()
		s, router := setupTestServer(t)

		// デフォルトアルバムを事前に作成する
		createTestAlbum(t, s, "default-album", "user-1", "All Media", "デフォルト")
		createTestAlbum(t, s, "album-1", "user-1", "普通のアルバム", "説明")

		body := map[string]string{"media_id": "media-1"}
		w := doRequest(router, http.MethodPost, "/api/v1/albums/album-1/media", "user-1", body)

		if w.Code != http.StatusOK {
			t.Errorf("ステータスコード: got %d, want %d", w.Code, http.StatusOK)
		}

		// アルバム一覧を取得して "All Media" が1つだけであることを確認する
		w2 := doRequest(router, http.MethodGet, "/api/v1/albums", "user-1", nil)
		albums := parseJSONArray(t, w2)

		count := 0
		for _, a := range albums {
			if a["name"] == "All Media" {
				count++
			}
		}
		if count != 1 {
			t.Errorf("'All Media' アルバムの数: got %d, want 1", count)
		}
	})
}

// TestHandleUpdateAlbum はアルバム更新ハンドラのテスト。
func TestHandleUpdateAlbum(t *testing.T) {
	t.Parallel()

	t.Run("正常にアルバムを更新できる", func(t *testing.T) {
		t.Parallel()
		s, router := setupTestServer(t)

		createTestAlbum(t, s, "album-1", "user-1", "元の名前", "元の説明")

		body := map[string]string{
			"name":        "新しい名前",
			"description": "新しい説明",
		}
		w := doRequest(router, http.MethodPut, "/api/v1/albums/album-1", "user-1", body)

		if w.Code != http.StatusOK {
			t.Errorf("ステータスコード: got %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
		}

		result := parseJSON(t, w)
		if result["name"] != "新しい名前" {
			t.Errorf("name: got %v, want 新しい名前", result["name"])
		}
		if result["description"] != "新しい説明" {
			t.Errorf("description: got %v, want 新しい説明", result["description"])
		}
	})

	t.Run("他ユーザーのアルバムの更新はForbidden", func(t *testing.T) {
		t.Parallel()
		s, router := setupTestServer(t)

		createTestAlbum(t, s, "album-1", "user-1", "ユーザー1のアルバム", "説明")

		body := map[string]string{"name": "乗っ取り"}
		w := doRequest(router, http.MethodPut, "/api/v1/albums/album-1", "user-2", body)

		if w.Code != http.StatusForbidden {
			t.Errorf("ステータスコード: got %d, want %d", w.Code, http.StatusForbidden)
		}
	})

	t.Run("存在しないアルバムの更新はNotFound", func(t *testing.T) {
		t.Parallel()
		_, router := setupTestServer(t)

		body := map[string]string{"name": "テスト"}
		w := doRequest(router, http.MethodPut, "/api/v1/albums/nonexistent", "user-1", body)

		if w.Code != http.StatusNotFound {
			t.Errorf("ステータスコード: got %d, want %d", w.Code, http.StatusNotFound)
		}
	})

	t.Run("名前が未指定の場合はBadRequest", func(t *testing.T) {
		t.Parallel()
		s, router := setupTestServer(t)

		createTestAlbum(t, s, "album-1", "user-1", "テスト", "説明")

		body := map[string]string{"description": "説明のみ"}
		w := doRequest(router, http.MethodPut, "/api/v1/albums/album-1", "user-1", body)

		if w.Code != http.StatusBadRequest {
			t.Errorf("ステータスコード: got %d, want %d", w.Code, http.StatusBadRequest)
		}
	})
}
