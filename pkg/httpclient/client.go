package httpclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client はサービス間通信用のHTTPクライアント。
// タイムアウトとリトライの設定を持つ。
type Client struct {
	// httpClient は内部で使用するHTTPクライアント。
	httpClient *http.Client
	// baseURL は接続先サービスのベースURL。
	baseURL string
}

// New は新しいサービス間通信用HTTPクライアントを生成する。
// baseURLには接続先サービスのベースURL（例: "http://eventstore:8084"）を指定する。
func New(baseURL string) *Client {
	return &Client{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		baseURL: baseURL,
	}
}

// PostJSON は指定パスにJSONボディでPOSTリクエストを送信する。
// レスポンスボディをresultにデシリアライズする。
func (c *Client) PostJSON(ctx context.Context, path string, body any, result any) error {
	return c.doJSON(ctx, http.MethodPost, path, body, result)
}

// GetJSON は指定パスにGETリクエストを送信する。
// レスポンスボディをresultにデシリアライズする。
func (c *Client) GetJSON(ctx context.Context, path string, result any) error {
	return c.doJSON(ctx, http.MethodGet, path, nil, result)
}

// doJSON はJSON形式のHTTPリクエストを実行する共通処理。
func (c *Client) doJSON(ctx context.Context, method, path string, body any, result any) error {
	var bodyReader io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("リクエストボディのシリアライズに失敗: %w", err)
		}
		bodyReader = bytes.NewReader(jsonBody)
	}

	url := c.baseURL + path
	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return fmt.Errorf("HTTPリクエストの作成に失敗: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// コンテキストからユーザーIDを伝播する
	if userID, ok := ctx.Value(contextKeyUserID).(string); ok {
		req.Header.Set("X-User-ID", userID)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("HTTPリクエストの送信に失敗: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTPエラー: status=%d, body=%s", resp.StatusCode, string(respBody))
	}

	if result != nil {
		if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
			return fmt.Errorf("レスポンスボディのデシリアライズに失敗: %w", err)
		}
	}
	return nil
}

// contextKey はコンテキストキーの型。
type contextKey string

// contextKeyUserID はコンテキストにユーザーIDを格納するためのキー。
const contextKeyUserID contextKey = "user_id"

// WithUserID はコンテキストにユーザーIDを設定する。
// サービス間通信時にユーザーIDを伝播するために使用する。
func WithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, contextKeyUserID, userID)
}
