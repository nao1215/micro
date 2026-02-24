package query

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	_ "modernc.org/sqlite"
	mediadb "github.com/nao1215/micro/internal/media/query/db"
	"github.com/nao1215/micro/pkg/middleware"
)

// testJWTSecret はテスト用のJWT署名鍵。
const testJWTSecret = "test-secret-key"

// setupTestQueryServer はテスト用のメディアクエリサーバーを作成する。
// インメモリSQLiteをRead Modelとして使用し、Projectorは起動しない。
func setupTestQueryServer(t *testing.T) (*Server, *sql.DB) {
	t.Helper()

	gin.SetMode(gin.TestMode)

	sqlDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("インメモリSQLiteの接続に失敗: %v", err)
	}

	if err := initSchema(sqlDB); err != nil {
		t.Fatalf("Read Modelスキーマの初期化に失敗: %v", err)
	}

	queries := mediadb.New(sqlDB)

	router := gin.New()
	s := &Server{
		router:  router,
		port:    "0",
		queries: queries,
		db:      sqlDB,
	}

	// JWTミドルウェア付きのルーティングを設定する
	api := router.Group("/api/v1")
	api.Use(middleware.JWTAuth(testJWTSecret))
	{
		media := api.Group("/media")
		{
			media.GET("", s.handleList())
			media.GET("/:id", s.handleGetByID())
			media.GET("/search", s.handleSearch())
		}
	}
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "media-query"})
	})

	t.Cleanup(func() {
		sqlDB.Close()
	})

	return s, sqlDB
}

// generateTestToken はテスト用のJWTトークンを生成する。
func generateTestToken(t *testing.T, userID, email string) string {
	t.Helper()
	token, err := middleware.GenerateJWT(testJWTSecret, userID, email)
	if err != nil {
		t.Fatalf("テスト用JWTトークンの生成に失敗: %v", err)
	}
	return token
}

// insertTestMedia はRead Modelにテスト用のメディアレコードを挿入する。
func insertTestMedia(t *testing.T, db *sql.DB, id, userID, filename, contentType string, size int64, storagePath, status string) {
	t.Helper()
	_, err := db.Exec(
		`INSERT INTO media_read_models (id, user_id, filename, content_type, size, storage_path, status, last_event_version, uploaded_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, 1, ?, datetime('now'))`,
		id, userID, filename, contentType, size, storagePath, status, time.Now().UTC(),
	)
	if err != nil {
		t.Fatalf("テスト用メディアレコードの挿入に失敗: %v", err)
	}
}

func TestHandleListMedia(t *testing.T) {
	t.Parallel()

	t.Run("正常系_メディアが存在しない場合空の一覧を返す", func(t *testing.T) {
		t.Parallel()

		s, _ := setupTestQueryServer(t)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/media", nil)
		token := generateTestToken(t, "user-123", "test@example.com")
		req.Header.Set("Authorization", "Bearer "+token)

		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("期待するステータスコード %d, 実際のステータスコード %d, body: %s", http.StatusOK, w.Code, w.Body.String())
		}

		var resp map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("レスポンスのデシリアライズに失敗: %v", err)
		}

		count, ok := resp["count"].(float64)
		if !ok {
			t.Fatal("レスポンスにcountフィールドが含まれていません")
		}
		if int(count) != 0 {
			t.Errorf("期待するcount 0, 実際のcount %d", int(count))
		}

		media, ok := resp["media"].([]any)
		if !ok {
			t.Fatal("レスポンスにmediaフィールドが含まれていません")
		}
		if len(media) != 0 {
			t.Errorf("期待するmedia長 0, 実際のmedia長 %d", len(media))
		}
	})

	t.Run("正常系_メディアが存在する場合一覧を返す", func(t *testing.T) {
		t.Parallel()

		s, db := setupTestQueryServer(t)

		// テストデータを挿入する
		insertTestMedia(t, db, "media-1", "user-123", "photo1.jpg", "image/jpeg", 1024, "/data/media/media-1/photo1.jpg", "uploaded")
		insertTestMedia(t, db, "media-2", "user-123", "photo2.png", "image/png", 2048, "/data/media/media-2/photo2.png", "processed")
		// 別ユーザーのデータ（返されないことを確認する）
		insertTestMedia(t, db, "media-3", "user-456", "other.jpg", "image/jpeg", 512, "/data/media/media-3/other.jpg", "uploaded")
		// 削除済みデータ（返されないことを確認する）
		insertTestMedia(t, db, "media-4", "user-123", "deleted.jpg", "image/jpeg", 256, "/data/media/media-4/deleted.jpg", "deleted")

		req := httptest.NewRequest(http.MethodGet, "/api/v1/media", nil)
		token := generateTestToken(t, "user-123", "test@example.com")
		req.Header.Set("Authorization", "Bearer "+token)

		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("期待するステータスコード %d, 実際のステータスコード %d, body: %s", http.StatusOK, w.Code, w.Body.String())
		}

		var resp map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("レスポンスのデシリアライズに失敗: %v", err)
		}

		count := int(resp["count"].(float64))
		if count != 2 {
			t.Errorf("期待するcount 2, 実際のcount %d", count)
		}

		media := resp["media"].([]any)
		if len(media) != 2 {
			t.Errorf("期待するmedia長 2, 実際のmedia長 %d", len(media))
		}
	})

	t.Run("異常系_認証なしの場合401を返す", func(t *testing.T) {
		t.Parallel()

		s, _ := setupTestQueryServer(t)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/media", nil)
		// Authorizationヘッダーなし

		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("期待するステータスコード %d, 実際のステータスコード %d", http.StatusUnauthorized, w.Code)
		}
	})
}

func TestHandleGetMedia(t *testing.T) {
	t.Parallel()

	t.Run("正常系_指定IDのメディア詳細を返す", func(t *testing.T) {
		t.Parallel()

		s, db := setupTestQueryServer(t)

		insertTestMedia(t, db, "media-detail-1", "user-123", "detail.jpg", "image/jpeg", 4096, "/data/media/media-detail-1/detail.jpg", "processed")

		// サムネイルパスとサイズ情報を追加する
		_, err := db.Exec(
			`UPDATE media_read_models SET thumbnail_path = ?, width = ?, height = ? WHERE id = ?`,
			"/data/media/media-detail-1/thumbnail.jpg", 800, 600, "media-detail-1",
		)
		if err != nil {
			t.Fatalf("テストデータの更新に失敗: %v", err)
		}

		req := httptest.NewRequest(http.MethodGet, "/api/v1/media/media-detail-1", nil)
		token := generateTestToken(t, "user-123", "test@example.com")
		req.Header.Set("Authorization", "Bearer "+token)

		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("期待するステータスコード %d, 実際のステータスコード %d, body: %s", http.StatusOK, w.Code, w.Body.String())
		}

		var resp mediaResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("レスポンスのデシリアライズに失敗: %v", err)
		}

		if resp.ID != "media-detail-1" {
			t.Errorf("期待するID %q, 実際のID %q", "media-detail-1", resp.ID)
		}
		if resp.Filename != "detail.jpg" {
			t.Errorf("期待するFilename %q, 実際のFilename %q", "detail.jpg", resp.Filename)
		}
		if resp.ContentType != "image/jpeg" {
			t.Errorf("期待するContentType %q, 実際のContentType %q", "image/jpeg", resp.ContentType)
		}
		if resp.Size != 4096 {
			t.Errorf("期待するSize %d, 実際のSize %d", 4096, resp.Size)
		}
		if resp.Status != "processed" {
			t.Errorf("期待するStatus %q, 実際のStatus %q", "processed", resp.Status)
		}
		if resp.ThumbnailPath == nil || *resp.ThumbnailPath != "/data/media/media-detail-1/thumbnail.jpg" {
			t.Errorf("期待するThumbnailPath %q, 実際のThumbnailPath %v", "/data/media/media-detail-1/thumbnail.jpg", resp.ThumbnailPath)
		}
		if resp.Width == nil || *resp.Width != 800 {
			t.Errorf("期待するWidth 800, 実際のWidth %v", resp.Width)
		}
		if resp.Height == nil || *resp.Height != 600 {
			t.Errorf("期待するHeight 600, 実際のHeight %v", resp.Height)
		}
	})

	t.Run("異常系_存在しないIDの場合404を返す", func(t *testing.T) {
		t.Parallel()

		s, _ := setupTestQueryServer(t)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/media/nonexistent-id", nil)
		token := generateTestToken(t, "user-123", "test@example.com")
		req.Header.Set("Authorization", "Bearer "+token)

		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Errorf("期待するステータスコード %d, 実際のステータスコード %d, body: %s", http.StatusNotFound, w.Code, w.Body.String())
		}

		var resp map[string]string
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("レスポンスのデシリアライズに失敗: %v", err)
		}
		if _, ok := resp["error"]; !ok {
			t.Error("レスポンスにerrorフィールドが含まれていません")
		}
	})
}

func TestHandleSearchMedia(t *testing.T) {
	t.Parallel()

	t.Run("正常系_ファイル名による検索が成功する", func(t *testing.T) {
		t.Parallel()

		s, db := setupTestQueryServer(t)

		insertTestMedia(t, db, "search-1", "user-123", "sunset_beach.jpg", "image/jpeg", 1024, "/data/media/search-1/sunset_beach.jpg", "uploaded")
		insertTestMedia(t, db, "search-2", "user-456", "sunset_mountain.png", "image/png", 2048, "/data/media/search-2/sunset_mountain.png", "processed")
		insertTestMedia(t, db, "search-3", "user-123", "portrait.jpg", "image/jpeg", 512, "/data/media/search-3/portrait.jpg", "uploaded")
		// 削除済みはヒットしないことを確認する
		insertTestMedia(t, db, "search-4", "user-123", "sunset_deleted.jpg", "image/jpeg", 256, "/data/media/search-4/sunset_deleted.jpg", "deleted")

		req := httptest.NewRequest(http.MethodGet, "/api/v1/media/search?q=sunset", nil)
		token := generateTestToken(t, "user-123", "test@example.com")
		req.Header.Set("Authorization", "Bearer "+token)

		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("期待するステータスコード %d, 実際のステータスコード %d, body: %s", http.StatusOK, w.Code, w.Body.String())
		}

		var resp map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("レスポンスのデシリアライズに失敗: %v", err)
		}

		count := int(resp["count"].(float64))
		if count != 2 {
			t.Errorf("期待するcount 2, 実際のcount %d", count)
		}

		query, ok := resp["query"].(string)
		if !ok || query != "sunset" {
			t.Errorf("期待するquery %q, 実際のquery %q", "sunset", query)
		}
	})

	t.Run("正常系_ヒットしない検索の場合空の結果を返す", func(t *testing.T) {
		t.Parallel()

		s, _ := setupTestQueryServer(t)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/media/search?q=nonexistent", nil)
		token := generateTestToken(t, "user-123", "test@example.com")
		req.Header.Set("Authorization", "Bearer "+token)

		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("期待するステータスコード %d, 実際のステータスコード %d, body: %s", http.StatusOK, w.Code, w.Body.String())
		}

		var resp map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("レスポンスのデシリアライズに失敗: %v", err)
		}

		count := int(resp["count"].(float64))
		if count != 0 {
			t.Errorf("期待するcount 0, 実際のcount %d", count)
		}
	})

	t.Run("異常系_検索クエリが指定されていない場合400を返す", func(t *testing.T) {
		t.Parallel()

		s, _ := setupTestQueryServer(t)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/media/search", nil)
		token := generateTestToken(t, "user-123", "test@example.com")
		req.Header.Set("Authorization", "Bearer "+token)

		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("期待するステータスコード %d, 実際のステータスコード %d, body: %s", http.StatusBadRequest, w.Code, w.Body.String())
		}

		var resp map[string]string
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("レスポンスのデシリアライズに失敗: %v", err)
		}
		if _, ok := resp["error"]; !ok {
			t.Error("レスポンスにerrorフィールドが含まれていません")
		}
	})

	t.Run("異常系_空のqパラメータの場合400を返す", func(t *testing.T) {
		t.Parallel()

		s, _ := setupTestQueryServer(t)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/media/search?q=", nil)
		token := generateTestToken(t, "user-123", "test@example.com")
		req.Header.Set("Authorization", "Bearer "+token)

		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("期待するステータスコード %d, 実際のステータスコード %d, body: %s", http.StatusBadRequest, w.Code, w.Body.String())
		}
	})
}

func TestQueryHealthCheck(t *testing.T) {
	t.Parallel()

	t.Run("正常系_ヘルスチェックが成功する", func(t *testing.T) {
		t.Parallel()

		s, _ := setupTestQueryServer(t)

		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("期待するステータスコード %d, 実際のステータスコード %d", http.StatusOK, w.Code)
		}

		var resp map[string]string
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("レスポンスのデシリアライズに失敗: %v", err)
		}
		if resp["status"] != "ok" {
			t.Errorf("期待するstatus %q, 実際のstatus %q", "ok", resp["status"])
		}
		if resp["service"] != "media-query" {
			t.Errorf("期待するservice %q, 実際のservice %q", "media-query", resp["service"])
		}
	})
}

func TestToMediaResponse(t *testing.T) {
	t.Parallel()

	t.Run("正常系_全フィールドが設定されたレコードを変換する", func(t *testing.T) {
		t.Parallel()

		now := time.Now().UTC()
		model := mediadb.MediaReadModel{
			ID:          "test-id",
			UserID:      "user-123",
			Filename:    "photo.jpg",
			ContentType: "image/jpeg",
			Size:        4096,
			StoragePath: "/data/media/test-id/photo.jpg",
			ThumbnailPath: sql.NullString{
				String: "/data/media/test-id/thumbnail.jpg",
				Valid:  true,
			},
			Width:  sql.NullInt64{Int64: 800, Valid: true},
			Height: sql.NullInt64{Int64: 600, Valid: true},
			DurationSeconds: sql.NullFloat64{
				Float64: 0,
				Valid:   false,
			},
			Status:     "processed",
			UploadedAt: now,
			UpdatedAt:  now,
		}

		resp := toMediaResponse(model)

		if resp.ID != "test-id" {
			t.Errorf("期待するID %q, 実際のID %q", "test-id", resp.ID)
		}
		if resp.ThumbnailPath == nil {
			t.Error("ThumbnailPathがnilです")
		} else if *resp.ThumbnailPath != "/data/media/test-id/thumbnail.jpg" {
			t.Errorf("期待するThumbnailPath %q, 実際のThumbnailPath %q", "/data/media/test-id/thumbnail.jpg", *resp.ThumbnailPath)
		}
		if resp.Width == nil || *resp.Width != 800 {
			t.Errorf("期待するWidth 800, 実際のWidth %v", resp.Width)
		}
		if resp.DurationSeconds != nil {
			t.Errorf("DurationSecondsはnilであるべき、実際は %v", *resp.DurationSeconds)
		}
	})

	t.Run("正常系_NullフィールドはnilとなるReading", func(t *testing.T) {
		t.Parallel()

		now := time.Now().UTC()
		model := mediadb.MediaReadModel{
			ID:              "test-id-null",
			UserID:          "user-123",
			Filename:        "photo.jpg",
			ContentType:     "image/jpeg",
			Size:            1024,
			StoragePath:     "/data/media/test-id-null/photo.jpg",
			ThumbnailPath:   sql.NullString{Valid: false},
			Width:           sql.NullInt64{Valid: false},
			Height:          sql.NullInt64{Valid: false},
			DurationSeconds: sql.NullFloat64{Valid: false},
			Status:          "uploaded",
			UploadedAt:      now,
			UpdatedAt:       now,
		}

		resp := toMediaResponse(model)

		if resp.ThumbnailPath != nil {
			t.Errorf("ThumbnailPathはnilであるべき、実際は %v", *resp.ThumbnailPath)
		}
		if resp.Width != nil {
			t.Errorf("Widthはnilであるべき、実際は %v", *resp.Width)
		}
		if resp.Height != nil {
			t.Errorf("Heightはnilであるべき、実際は %v", *resp.Height)
		}
		if resp.DurationSeconds != nil {
			t.Errorf("DurationSecondsはnilであるべき、実際は %v", *resp.DurationSeconds)
		}
	})
}

func TestToMediaResponses(t *testing.T) {
	t.Parallel()

	t.Run("正常系_空スライスを変換する", func(t *testing.T) {
		t.Parallel()

		result := toMediaResponses([]mediadb.MediaReadModel{})
		if len(result) != 0 {
			t.Errorf("期待する長さ 0, 実際の長さ %d", len(result))
		}
	})

	t.Run("正常系_複数レコードを変換する", func(t *testing.T) {
		t.Parallel()

		now := time.Now().UTC()
		models := []mediadb.MediaReadModel{
			{
				ID: "id-1", UserID: "user-1", Filename: "a.jpg", ContentType: "image/jpeg",
				Size: 100, StoragePath: "/a", Status: "uploaded", UploadedAt: now, UpdatedAt: now,
			},
			{
				ID: "id-2", UserID: "user-2", Filename: "b.png", ContentType: "image/png",
				Size: 200, StoragePath: "/b", Status: "processed", UploadedAt: now, UpdatedAt: now,
			},
		}

		result := toMediaResponses(models)
		if len(result) != 2 {
			t.Errorf("期待する長さ 2, 実際の長さ %d", len(result))
		}
		if result[0].ID != "id-1" {
			t.Errorf("期待するID %q, 実際のID %q", "id-1", result[0].ID)
		}
		if result[1].ID != "id-2" {
			t.Errorf("期待するID %q, 実際のID %q", "id-2", result[1].ID)
		}
	})
}
