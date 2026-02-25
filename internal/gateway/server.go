package gateway

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	_ "modernc.org/sqlite"
	gatewaydb "github.com/nao1215/micro/internal/gateway/db"
	"github.com/nao1215/micro/pkg/middleware"
)

// Server はAPI Gatewayサービスの HTTPサーバー。
type Server struct {
	// router はGinのHTTPルーター。
	router *gin.Engine
	// port はサーバーのリッスンポート。
	port string
	// queries はsqlcが生成したクエリ実行オブジェクト。
	queries *gatewaydb.Queries
	// db はSQLiteデータベース接続。
	db *sql.DB
	// jwtSecret はJWT署名用の秘密鍵。
	jwtSecret string
	// serviceURLs は内部サービスのURL。
	serviceURLs serviceURLConfig
}

// serviceURLConfig は内部サービスのURL設定。
type serviceURLConfig struct {
	MediaCommand string
	MediaQuery   string
	Album        string
	Notification string
	EventStore   string
}

// NewServer は新しいGatewayサーバーを生成する。
func NewServer(port string) (*Server, error) {
	sqlDB, err := sql.Open("sqlite", "/data/gateway.db?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("データベース接続に失敗: %w", err)
	}

	if err := initSchema(sqlDB); err != nil {
		return nil, fmt.Errorf("スキーマ初期化に失敗: %w", err)
	}

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		jwtSecret = "dev-secret-key"
	}

	urls := serviceURLConfig{
		MediaCommand: getEnvOr("MEDIA_COMMAND_URL", "http://localhost:8081"),
		MediaQuery:   getEnvOr("MEDIA_QUERY_URL", "http://localhost:8082"),
		Album:        getEnvOr("ALBUM_URL", "http://localhost:8083"),
		Notification: getEnvOr("NOTIFICATION_URL", "http://localhost:8086"),
		EventStore:   getEnvOr("EVENTSTORE_URL", "http://localhost:8084"),
	}

	frontendURL := getEnvOr("FRONTEND_URL", "http://localhost:3000")

	router := gin.New()
	router.Use(middleware.Recovery())
	router.Use(gin.Logger())
	router.Use(middleware.CORS([]string{frontendURL}))

	s := &Server{
		router:      router,
		port:        port,
		queries:     gatewaydb.New(sqlDB),
		db:          sqlDB,
		jwtSecret:   jwtSecret,
		serviceURLs: urls,
	}
	s.setupRoutes()

	return s, nil
}

// Run はHTTPサーバーを起動する。
func (s *Server) Run() error {
	return s.router.Run(fmt.Sprintf(":%s", s.port))
}

// setupRoutes はAPIルーティングを設定する。
func (s *Server) setupRoutes() {
	// OAuth2認証エンドポイント（認証不要）
	auth := s.router.Group("/auth")
	{
		auth.GET("/github", s.handleGitHubLogin())
		auth.GET("/github/callback", s.handleGitHubCallback())
		auth.GET("/google", s.handleGoogleLogin())
		auth.GET("/google/callback", s.handleGoogleCallback())
		// 開発用トークン発行
		auth.POST("/dev-token", s.handleDevToken())
	}

	// 認証必須のAPIエンドポイント
	api := s.router.Group("/api/v1")
	api.Use(middleware.JWTAuth(s.jwtSecret))
	{
		// ユーザー情報
		api.GET("/me", s.handleGetCurrentUser())

		// メディア（プロキシ）
		api.POST("/media", s.handleProxy(s.serviceURLs.MediaCommand, "/api/v1/media"))
		api.GET("/media", s.handleProxy(s.serviceURLs.MediaQuery, "/api/v1/media"))
		api.GET("/media/:id", s.handleProxyWithParam(s.serviceURLs.MediaQuery, "/api/v1/media/", "id"))
		api.DELETE("/media/:id", s.handleProxyWithParam(s.serviceURLs.MediaCommand, "/api/v1/media/", "id"))

		// アルバム（プロキシ）
		api.POST("/albums", s.handleProxy(s.serviceURLs.Album, "/api/v1/albums"))
		api.GET("/albums", s.handleProxy(s.serviceURLs.Album, "/api/v1/albums"))
		api.GET("/albums/:id", s.handleProxyWithParam(s.serviceURLs.Album, "/api/v1/albums/", "id"))
		api.DELETE("/albums/:id", s.handleProxyWithParam(s.serviceURLs.Album, "/api/v1/albums/", "id"))
		api.POST("/albums/:id/media", s.handleProxyAlbumMedia())
		api.DELETE("/albums/:id/media/:media_id", s.handleProxyAlbumRemoveMedia())

		// 通知
		api.GET("/notifications", s.handleProxy(s.serviceURLs.Notification, "/api/v1/notifications"))
		api.PUT("/notifications/:id/read", s.handleProxyWithParam(s.serviceURLs.Notification, "/api/v1/notifications/", "id", "/read"))

		// Saga監視
		api.GET("/sagas", s.handleProxy(getEnvOr("SAGA_URL", "http://localhost:8085"), "/api/v1/sagas"))

		// イベントログ
		api.GET("/events", s.handleProxy(s.serviceURLs.EventStore, "/api/v1/events"))
	}

	// サムネイル画像の取得（認証不要 - img要素から直接参照されるため）
	s.router.GET("/api/v1/media/:id/thumbnail", s.handleProxyWithParam(s.serviceURLs.MediaCommand, "/api/v1/media/", "id", "/thumbnail"))

	// ヘルスチェック
	s.router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "gateway"})
	})
}

// handleDevToken は開発用JWTトークンを発行するハンドラを返す。
// 本番環境では無効化すべき。
func (s *Server) handleDevToken() gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := uuid.New().String()

		// 開発用ユーザーが存在しなければ作成
		_, err := s.queries.GetUserByProvider(c.Request.Context(), gatewaydb.GetUserByProviderParams{
			Provider:       "dev",
			ProviderUserID: "dev-user",
		})
		if err == sql.ErrNoRows {
			if err := s.queries.CreateUser(c.Request.Context(), gatewaydb.CreateUserParams{
				ID:             userID,
				Provider:       "dev",
				ProviderUserID: "dev-user",
				Email:          "dev@localhost",
				DisplayName:    "開発ユーザー",
				AvatarUrl:      "",
			}); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "ユーザー作成に失敗しました"})
				log.Printf("開発ユーザー作成エラー: %v", err)
				return
			}
		} else if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "ユーザー取得に失敗しました"})
			return
		} else {
			// 既存の開発ユーザーを使用
			user, _ := s.queries.GetUserByProvider(c.Request.Context(), gatewaydb.GetUserByProviderParams{
				Provider:       "dev",
				ProviderUserID: "dev-user",
			})
			userID = user.ID
			_ = s.queries.UpdateLastLogin(c.Request.Context(), userID)
		}

		token, err := middleware.GenerateJWT(s.jwtSecret, userID, "dev@localhost")
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "トークン生成に失敗しました"})
			log.Printf("JWT生成エラー: %v", err)
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"token":   token,
			"user_id": userID,
		})
	}
}

// handleGitHubLogin はGitHub OAuth2ログインを開始するハンドラを返す。
func (s *Server) handleGitHubLogin() gin.HandlerFunc {
	return func(c *gin.Context) {
		clientID := os.Getenv("GITHUB_CLIENT_ID")
		if clientID == "" {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "GitHub OAuth2が設定されていません"})
			return
		}
		state := uuid.New().String()
		redirectURL := fmt.Sprintf("https://github.com/login/oauth/authorize?client_id=%s&state=%s&scope=user:email", clientID, state)
		c.Redirect(http.StatusTemporaryRedirect, redirectURL)
	}
}

// handleGitHubCallback はGitHub OAuth2コールバックを処理するハンドラを返す。
func (s *Server) handleGitHubCallback() gin.HandlerFunc {
	return func(c *gin.Context) {
		// TODO: GitHub OAuth2のアクセストークン交換とユーザー情報取得を実装
		c.JSON(http.StatusNotImplemented, gin.H{"error": "GitHub OAuth2コールバックは未実装です。開発用トークン（POST /auth/dev-token）を使用してください。"})
	}
}

// handleGoogleLogin はGoogle OAuth2ログインを開始するハンドラを返す。
func (s *Server) handleGoogleLogin() gin.HandlerFunc {
	return func(c *gin.Context) {
		clientID := os.Getenv("GOOGLE_CLIENT_ID")
		if clientID == "" {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Google OAuth2が設定されていません"})
			return
		}
		state := uuid.New().String()
		redirectURL := fmt.Sprintf("https://accounts.google.com/o/oauth2/v2/auth?client_id=%s&response_type=code&scope=openid%%20email%%20profile&state=%s&redirect_uri=%s/auth/google/callback",
			clientID, state, getEnvOr("FRONTEND_URL", "http://localhost:8080"))
		c.Redirect(http.StatusTemporaryRedirect, redirectURL)
	}
}

// handleGoogleCallback はGoogle OAuth2コールバックを処理するハンドラを返す。
func (s *Server) handleGoogleCallback() gin.HandlerFunc {
	return func(c *gin.Context) {
		// TODO: Google OAuth2のアクセストークン交換とユーザー情報取得を実装
		c.JSON(http.StatusNotImplemented, gin.H{"error": "Google OAuth2コールバックは未実装です。開発用トークン（POST /auth/dev-token）を使用してください。"})
	}
}

// handleGetCurrentUser は認証済みユーザーの情報を返すハンドラを返す。
func (s *Server) handleGetCurrentUser() gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := middleware.GetUserID(c)
		if userID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "ユーザーIDが取得できません"})
			return
		}

		user, err := s.queries.GetUserByID(c.Request.Context(), userID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "ユーザーが見つかりません"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"id":           user.ID,
			"email":        user.Email,
			"display_name": user.DisplayName,
			"avatar_url":   user.AvatarUrl,
			"provider":     user.Provider,
		})
	}
}

// handleProxy は指定されたサービスにリクエストをプロキシするハンドラを返す。
func (s *Server) handleProxy(baseURL, path string) gin.HandlerFunc {
	return func(c *gin.Context) {
		proxyURL := baseURL + path
		if c.Request.URL.RawQuery != "" {
			proxyURL += "?" + c.Request.URL.RawQuery
		}
		s.doProxy(c, c.Request.Method, proxyURL)
	}
}

// handleProxyWithParam はURLパラメータを含むプロキシハンドラを返す。
func (s *Server) handleProxyWithParam(baseURL, pathPrefix, paramName string, pathSuffix ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		proxyURL := baseURL + pathPrefix + c.Param(paramName)
		for _, suffix := range pathSuffix {
			proxyURL += suffix
		}
		if c.Request.URL.RawQuery != "" {
			proxyURL += "?" + c.Request.URL.RawQuery
		}
		s.doProxy(c, c.Request.Method, proxyURL)
	}
}

// handleProxyAlbumMedia はアルバムへのメディア追加をプロキシするハンドラを返す。
func (s *Server) handleProxyAlbumMedia() gin.HandlerFunc {
	return func(c *gin.Context) {
		albumID := c.Param("id")
		proxyURL := s.serviceURLs.Album + "/api/v1/albums/" + albumID + "/media"
		s.doProxy(c, http.MethodPost, proxyURL)
	}
}

// handleProxyAlbumRemoveMedia はアルバムからのメディア削除をプロキシするハンドラを返す。
func (s *Server) handleProxyAlbumRemoveMedia() gin.HandlerFunc {
	return func(c *gin.Context) {
		albumID := c.Param("id")
		mediaID := c.Param("media_id")
		proxyURL := s.serviceURLs.Album + "/api/v1/albums/" + albumID + "/media/" + mediaID
		s.doProxy(c, http.MethodDelete, proxyURL)
	}
}

// doProxy はリクエストを内部サービスにプロキシする共通処理。
// JWTトークンとユーザーIDヘッダーを転送する。
func (s *Server) doProxy(c *gin.Context, method, url string) {
	req, err := http.NewRequestWithContext(c.Request.Context(), method, url, c.Request.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "プロキシリクエストの作成に失敗しました"})
		return
	}

	// 元のリクエストヘッダーを転送
	req.Header.Set("Content-Type", c.GetHeader("Content-Type"))
	req.Header.Set("Authorization", c.GetHeader("Authorization"))
	req.Header.Set("X-User-ID", middleware.GetUserID(c))

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "内部サービスとの通信に失敗しました"})
		log.Printf("プロキシエラー: url=%s, error=%v", url, err)
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "レスポンスの読み取りに失敗しました"})
		return
	}

	// レスポンスのContent-Typeに応じてそのまま転送
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/json"
	}

	// JSONレスポンスの場合はパースして返す（Ginのフォーマットに合わせる）
	if json.Valid(body) {
		c.Data(resp.StatusCode, contentType, body)
	} else {
		c.Data(resp.StatusCode, contentType, body)
	}
}

// getEnvOr は環境変数を取得し、設定されていない場合はデフォルト値を返す。
func getEnvOr(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}
