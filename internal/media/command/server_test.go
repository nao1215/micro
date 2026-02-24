package command

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/nao1215/micro/pkg/httpclient"
	"github.com/nao1215/micro/pkg/middleware"
)

// jwtSecret はテスト用のJWT署名鍵。
const jwtSecret = "test-secret-key"

// setupTestServer はテスト用のServerインスタンスを作成する。
// Event StoreのモックURLとファイル保存先のテンポラリディレクトリを設定する。
func setupTestServer(t *testing.T, eventStoreURL string) *Server {
	t.Helper()

	gin.SetMode(gin.TestMode)

	router := gin.New()
	s := &Server{
		router:      router,
		port:        "0",
		eventClient: httpclient.New(eventStoreURL),
	}

	// JWTミドルウェア付きのルーティングを設定する
	api := router.Group("/api/v1")
	api.Use(middleware.JWTAuth(jwtSecret))
	{
		media := api.Group("/media")
		{
			media.POST("", s.handleUpload())
			media.DELETE("/:id", s.handleDelete())
			media.POST("/:id/process", s.handleProcess())
			media.POST("/:id/compensate", s.handleCompensate())
		}
	}
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "media-command"})
	})

	return s
}

// generateTestJWT はテスト用のJWTトークンを生成する。
func generateTestJWT(t *testing.T, userID, email string) string {
	t.Helper()
	token, err := middleware.GenerateJWT(jwtSecret, userID, email)
	if err != nil {
		t.Fatalf("テスト用JWTトークンの生成に失敗: %v", err)
	}
	return token
}

// createTestImage はテスト用のPNG画像を指定パスに作成する。
func createTestImage(t *testing.T, path string, width, height int) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x % 256), G: uint8(y % 256), B: 128, A: 255})
		}
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("テスト画像ディレクトリの作成に失敗: %v", err)
	}

	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("テスト画像ファイルの作成に失敗: %v", err)
	}
	defer f.Close()

	if err := png.Encode(f, img); err != nil {
		t.Fatalf("テスト画像のエンコードに失敗: %v", err)
	}
}

// createMultipartFile はマルチパートフォームデータのバッファとContent-Typeを返す。
// contentTypeが空文字列の場合は自動推定に任せる。
func createMultipartFile(t *testing.T, fieldName, fileName string, data []byte, contentType string) (*bytes.Buffer, string) {
	t.Helper()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	if contentType != "" {
		h := make(textproto.MIMEHeader)
		h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, fieldName, fileName))
		h.Set("Content-Type", contentType)
		part, err := writer.CreatePart(h)
		if err != nil {
			t.Fatalf("マルチパートパートの作成に失敗: %v", err)
		}
		if _, err := part.Write(data); err != nil {
			t.Fatalf("マルチパートデータの書き込みに失敗: %v", err)
		}
	} else {
		part, err := writer.CreateFormFile(fieldName, fileName)
		if err != nil {
			t.Fatalf("マルチパートフォームファイルの作成に失敗: %v", err)
		}
		if _, err := part.Write(data); err != nil {
			t.Fatalf("マルチパートデータの書き込みに失敗: %v", err)
		}
	}

	if err := writer.Close(); err != nil {
		t.Fatalf("マルチパートライターのクローズに失敗: %v", err)
	}
	return body, writer.FormDataContentType()
}

func TestHandleUpload(t *testing.T) {
	// mediaBaseDirを差し替えるため、並列実行はしない
	t.Run("正常系_画像ファイルのアップロードが成功する", func(t *testing.T) {
		// テスト用の一時ディレクトリをmediaBaseDirとして使用する
		tmpDir := t.TempDir()
		origBaseDir := mediaBaseDir

		// Event StoreのモックHTTPサーバーを起動する
		eventStore := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]any{"id": "event-1", "version": 1})
		}))
		defer eventStore.Close()

		s := setupTestServer(t, eventStore.URL)

		// テスト用のPNG画像データを生成する
		imgBuf := &bytes.Buffer{}
		img := image.NewRGBA(image.Rect(0, 0, 10, 10))
		for y := 0; y < 10; y++ {
			for x := 0; x < 10; x++ {
				img.Set(x, y, color.RGBA{R: 255, G: 0, B: 0, A: 255})
			}
		}
		if err := png.Encode(imgBuf, img); err != nil {
			t.Fatalf("テスト画像のエンコードに失敗: %v", err)
		}

		body, ct := createMultipartFile(t, "file", "test.png", imgBuf.Bytes(), "image/png")

		req := httptest.NewRequest(http.MethodPost, "/api/v1/media", body)
		req.Header.Set("Content-Type", ct)
		token := generateTestJWT(t, "user-123", "test@example.com")
		req.Header.Set("Authorization", "Bearer "+token)

		w := httptest.NewRecorder()

		// mediaBaseDirをテスト用に差し替える
		// NOTE: パッケージレベル変数なのでt.Cleanupで復元する
		mediaBaseDir = tmpDir
		t.Cleanup(func() { mediaBaseDir = origBaseDir })

		s.router.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Errorf("期待するステータスコード %d, 実際のステータスコード %d, body: %s", http.StatusCreated, w.Code, w.Body.String())
		}

		var resp uploadResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("レスポンスのデシリアライズに失敗: %v", err)
		}

		if resp.Filename != "test.png" {
			t.Errorf("期待するファイル名 %q, 実際のファイル名 %q", "test.png", resp.Filename)
		}
		if resp.ContentType != "image/png" {
			t.Errorf("期待するContent-Type %q, 実際のContent-Type %q", "image/png", resp.ContentType)
		}
		if resp.ID == "" {
			t.Error("レスポンスのIDが空です")
		}
		if resp.Size == 0 {
			t.Error("レスポンスのSizeが0です")
		}
	})

	t.Run("異常系_ファイルが指定されていない場合400を返す", func(t *testing.T) {
		eventStore := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer eventStore.Close()

		s := setupTestServer(t, eventStore.URL)

		// ファイルなしのリクエストを送信する
		req := httptest.NewRequest(http.MethodPost, "/api/v1/media", nil)
		req.Header.Set("Content-Type", "multipart/form-data")
		token := generateTestJWT(t, "user-123", "test@example.com")
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

	t.Run("異常系_ファイルサイズが上限を超えている場合400を返す", func(t *testing.T) {
		// テスト用にmaxUploadSizeを小さくする
		origMaxUploadSize := maxUploadSize
		maxUploadSize = 1024 // 1KB
		t.Cleanup(func() { maxUploadSize = origMaxUploadSize })

		eventStore := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer eventStore.Close()

		s := setupTestServer(t, eventStore.URL)
		s.router.MaxMultipartMemory = maxUploadSize

		// maxUploadSize(1KB)を超えるデータを作成する
		largeData := make([]byte, maxUploadSize+1)
		body, ct := createMultipartFile(t, "file", "large.png", largeData, "image/png")

		req := httptest.NewRequest(http.MethodPost, "/api/v1/media", body)
		req.Header.Set("Content-Type", ct)
		token := generateTestJWT(t, "user-123", "test@example.com")
		req.Header.Set("Authorization", "Bearer "+token)

		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("期待するステータスコード %d, 実際のステータスコード %d, body: %s", http.StatusBadRequest, w.Code, w.Body.String())
		}
	})

	t.Run("異常系_許可されていないContent-Typeの場合400を返す", func(t *testing.T) {
		tmpDir := t.TempDir()
		origBaseDir := mediaBaseDir

		eventStore := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer eventStore.Close()

		s := setupTestServer(t, eventStore.URL)

		mediaBaseDir = tmpDir
		t.Cleanup(func() { mediaBaseDir = origBaseDir })

		// テキストファイルとしてアップロードする
		body, ct := createMultipartFile(t, "file", "test.txt", []byte("hello world"), "text/plain")

		req := httptest.NewRequest(http.MethodPost, "/api/v1/media", body)
		req.Header.Set("Content-Type", ct)
		token := generateTestJWT(t, "user-123", "test@example.com")
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
		if errMsg, ok := resp["error"]; !ok {
			t.Error("レスポンスにerrorフィールドが含まれていません")
		} else if !bytes.Contains([]byte(errMsg), []byte("Content-Type")) {
			t.Errorf("エラーメッセージにContent-Typeが含まれていません: %s", errMsg)
		}
	})
}

func TestHandleDelete(t *testing.T) {
	t.Parallel()

	t.Run("正常系_メディアの削除が成功する", func(t *testing.T) {
		t.Parallel()

		eventStore := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]any{"id": "event-1", "version": 1})
		}))
		defer eventStore.Close()

		s := setupTestServer(t, eventStore.URL)

		req := httptest.NewRequest(http.MethodDelete, "/api/v1/media/test-media-id", nil)
		token := generateTestJWT(t, "user-123", "test@example.com")
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
		if resp["media_id"] != "test-media-id" {
			t.Errorf("期待するmedia_id %q, 実際のmedia_id %q", "test-media-id", resp["media_id"])
		}
	})

	t.Run("異常系_Event Storeへの送信が失敗した場合500を返す", func(t *testing.T) {
		t.Parallel()

		// Event Storeがエラーを返すモックサーバー
		eventStore := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"error":"internal error"}`))
		}))
		defer eventStore.Close()

		s := setupTestServer(t, eventStore.URL)

		req := httptest.NewRequest(http.MethodDelete, "/api/v1/media/test-media-id", nil)
		token := generateTestJWT(t, "user-123", "test@example.com")
		req.Header.Set("Authorization", "Bearer "+token)

		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)

		if w.Code != http.StatusInternalServerError {
			t.Errorf("期待するステータスコード %d, 実際のステータスコード %d, body: %s", http.StatusInternalServerError, w.Code, w.Body.String())
		}
	})
}

func TestHandleProcess(t *testing.T) {
	t.Parallel()

	t.Run("正常系_サムネイル生成が成功する", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()

		// テスト画像を作成する
		testImagePath := filepath.Join(tmpDir, "test.png")
		createTestImage(t, testImagePath, 400, 300)

		eventStore := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]any{"id": "event-1", "version": 1})
		}))
		defer eventStore.Close()

		s := setupTestServer(t, eventStore.URL)

		reqBody, _ := json.Marshal(processRequest{StoragePath: testImagePath})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/media/test-media-id/process", bytes.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		token := generateTestJWT(t, "user-123", "test@example.com")
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
		if resp["media_id"] != "test-media-id" {
			t.Errorf("期待するmedia_id %q, 実際のmedia_id %q", "test-media-id", resp["media_id"])
		}

		// サムネイルが生成されていることを確認する
		thumbnailPath := filepath.Join(tmpDir, "thumbnail.jpg")
		if _, err := os.Stat(thumbnailPath); os.IsNotExist(err) {
			t.Error("サムネイルファイルが生成されていません")
		}

		// widthとheightが返されることを確認する
		if width, ok := resp["width"].(float64); !ok || width != 400 {
			t.Errorf("期待するwidth 400, 実際のwidth %v", resp["width"])
		}
		if height, ok := resp["height"].(float64); !ok || height != 300 {
			t.Errorf("期待するheight 300, 実際のheight %v", resp["height"])
		}
	})

	t.Run("異常系_storage_pathが指定されていない場合400を返す", func(t *testing.T) {
		t.Parallel()

		eventStore := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer eventStore.Close()

		s := setupTestServer(t, eventStore.URL)

		// storage_pathなしのリクエスト
		reqBody := []byte(`{}`)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/media/test-media-id/process", bytes.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		token := generateTestJWT(t, "user-123", "test@example.com")
		req.Header.Set("Authorization", "Bearer "+token)

		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("期待するステータスコード %d, 実際のステータスコード %d, body: %s", http.StatusBadRequest, w.Code, w.Body.String())
		}
	})

	t.Run("異常系_存在しないファイルパスの場合エラーを返す", func(t *testing.T) {
		t.Parallel()

		eventStore := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]any{"id": "event-1", "version": 1})
		}))
		defer eventStore.Close()

		s := setupTestServer(t, eventStore.URL)

		reqBody, _ := json.Marshal(processRequest{StoragePath: "/nonexistent/path/image.png"})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/media/test-media-id/process", bytes.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		token := generateTestJWT(t, "user-123", "test@example.com")
		req.Header.Set("Authorization", "Bearer "+token)

		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)

		if w.Code != http.StatusInternalServerError {
			t.Errorf("期待するステータスコード %d, 実際のステータスコード %d, body: %s", http.StatusInternalServerError, w.Code, w.Body.String())
		}
	})
}

func TestHandleCompensate(t *testing.T) {
	// mediaBaseDirを差し替えるため、並列実行はしない
	t.Run("正常系_補償アクションが成功する", func(t *testing.T) {
		tmpDir := t.TempDir()
		origBaseDir := mediaBaseDir

		// 補償対象のメディアディレクトリとファイルを作成する
		mediaID := "compensate-test-id"
		mediaDir := filepath.Join(tmpDir, mediaID)
		if err := os.MkdirAll(mediaDir, 0o755); err != nil {
			t.Fatalf("テスト用メディアディレクトリの作成に失敗: %v", err)
		}
		testFile := filepath.Join(mediaDir, "test.png")
		if err := os.WriteFile(testFile, []byte("dummy"), 0o644); err != nil {
			t.Fatalf("テスト用ファイルの書き込みに失敗: %v", err)
		}

		eventStore := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]any{"id": "event-1", "version": 1})
		}))
		defer eventStore.Close()

		s := setupTestServer(t, eventStore.URL)

		mediaBaseDir = tmpDir
		t.Cleanup(func() { mediaBaseDir = origBaseDir })

		reqBody, _ := json.Marshal(compensateRequest{
			Reason: "Saga処理の失敗によるロールバック",
			SagaID: "saga-123",
		})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/media/"+mediaID+"/compensate", bytes.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		token := generateTestJWT(t, "user-123", "test@example.com")
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
		if resp["media_id"] != mediaID {
			t.Errorf("期待するmedia_id %q, 実際のmedia_id %q", mediaID, resp["media_id"])
		}

		// メディアディレクトリが削除されていることを確認する
		if _, err := os.Stat(mediaDir); !os.IsNotExist(err) {
			t.Error("メディアディレクトリが削除されていません")
		}
	})

	t.Run("正常系_存在しないメディアディレクトリでも補償アクションが成功する", func(t *testing.T) {
		tmpDir := t.TempDir()
		origBaseDir := mediaBaseDir

		eventStore := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]any{"id": "event-1", "version": 1})
		}))
		defer eventStore.Close()

		s := setupTestServer(t, eventStore.URL)

		mediaBaseDir = tmpDir
		t.Cleanup(func() { mediaBaseDir = origBaseDir })

		reqBody, _ := json.Marshal(compensateRequest{
			Reason: "テスト用補償",
			SagaID: "saga-456",
		})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/media/nonexistent-id/compensate", bytes.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		token := generateTestJWT(t, "user-123", "test@example.com")
		req.Header.Set("Authorization", "Bearer "+token)

		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("期待するステータスコード %d, 実際のステータスコード %d, body: %s", http.StatusOK, w.Code, w.Body.String())
		}
	})
}

func TestHealthCheck(t *testing.T) {
	t.Parallel()

	t.Run("正常系_ヘルスチェックが成功する", func(t *testing.T) {
		t.Parallel()

		s := setupTestServer(t, "http://localhost:9999")

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
		if resp["service"] != "media-command" {
			t.Errorf("期待するservice %q, 実際のservice %q", "media-command", resp["service"])
		}
	})
}

func TestIsAllowedContentType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		contentType string
		want        bool
	}{
		{name: "image/jpegは許可される", contentType: "image/jpeg", want: true},
		{name: "image/pngは許可される", contentType: "image/png", want: true},
		{name: "image/gifは許可される", contentType: "image/gif", want: true},
		{name: "video/mp4は許可される", contentType: "video/mp4", want: true},
		{name: "video/webmは許可される", contentType: "video/webm", want: true},
		{name: "text/plainは許可されない", contentType: "text/plain", want: false},
		{name: "application/jsonは許可されない", contentType: "application/json", want: false},
		{name: "application/pdfは許可されない", contentType: "application/pdf", want: false},
		{name: "空文字列は許可されない", contentType: "", want: false},
		{name: "大文字のImage/PNGは許可される", contentType: "Image/PNG", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := isAllowedContentType(tt.contentType)
			if got != tt.want {
				t.Errorf("isAllowedContentType(%q) = %v, want %v", tt.contentType, got, tt.want)
			}
		})
	}
}

func TestResizeNearestNeighbor(t *testing.T) {
	t.Parallel()

	t.Run("正常系_画像が指定サイズにリサイズされる", func(t *testing.T) {
		t.Parallel()

		src := image.NewRGBA(image.Rect(0, 0, 800, 600))
		for y := 0; y < 600; y++ {
			for x := 0; x < 800; x++ {
				src.Set(x, y, color.RGBA{R: 255, G: 128, B: 0, A: 255})
			}
		}

		result := resizeNearestNeighbor(src, 200, 200)

		bounds := result.Bounds()
		if bounds.Dx() != 200 || bounds.Dy() != 200 {
			t.Errorf("期待するサイズ 200x200, 実際のサイズ %dx%d", bounds.Dx(), bounds.Dy())
		}
	})

	t.Run("正常系_正方形画像のリサイズ", func(t *testing.T) {
		t.Parallel()

		src := image.NewRGBA(image.Rect(0, 0, 100, 100))
		result := resizeNearestNeighbor(src, 200, 200)

		bounds := result.Bounds()
		if bounds.Dx() != 200 || bounds.Dy() != 200 {
			t.Errorf("期待するサイズ 200x200, 実際のサイズ %dx%d", bounds.Dx(), bounds.Dy())
		}
	})
}
