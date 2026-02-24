package middleware

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// JWTClaims はJWTトークンのクレーム（ペイロード）を表す。
// ユーザーID等の情報をサービス間で伝播するために使用する。
type JWTClaims struct {
	jwt.RegisteredClaims
	// UserID は認証済みユーザーの一意識別子。
	UserID string `json:"user_id"`
	// Email はユーザーのメールアドレス。
	Email string `json:"email"`
}

// headerKeyUserID はサービス間でユーザーIDを伝播するためのHTTPヘッダーキー。
const headerKeyUserID = "X-User-ID"

// GenerateJWT はユーザー情報からJWTトークンを生成する。
// gatewayサービスがOAuth2認証後に呼び出す。
func GenerateJWT(secret, userID, email string) (string, error) {
	claims := JWTClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "mediahub-gateway",
		},
		UserID: userID,
		Email:  email,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		return "", fmt.Errorf("JWTトークンの署名に失敗: %w", err)
	}
	return signed, nil
}

// JWTAuth はJWTトークンを検証するGinミドルウェアを返す。
// 検証に成功した場合、コンテキストに "user_id" と "email" を設定する。
func JWTAuth(secret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "Authorizationヘッダーが必要です",
			})
			return
		}

		tokenString, found := strings.CutPrefix(authHeader, "Bearer ")
		if !found {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "Bearer トークン形式が不正です",
			})
			return
		}

		claims := &JWTClaims{}
		token, err := jwt.ParseWithClaims(tokenString, claims, func(_ *jwt.Token) (any, error) {
			return []byte(secret), nil
		})
		if err != nil || !token.Valid {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "トークンが無効です",
			})
			return
		}

		c.Set("user_id", claims.UserID)
		c.Set("email", claims.Email)
		c.Header(headerKeyUserID, claims.UserID)
		c.Next()
	}
}

// GetUserID はGinコンテキストからユーザーIDを取得する。
// JWTAuthミドルウェアが事前に適用されている必要がある。
func GetUserID(c *gin.Context) string {
	userID, _ := c.Get("user_id")
	if id, ok := userID.(string); ok {
		return id
	}
	return ""
}
