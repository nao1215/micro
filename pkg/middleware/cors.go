package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// CORS は指定されたオリジンからのクロスオリジンリクエストを許可するGinミドルウェアを返す。
// フロントエンドからのAPIアクセスを許可するためにgatewayサービスで使用する。
func CORS(allowedOrigins []string) gin.HandlerFunc {
	originsSet := make(map[string]struct{}, len(allowedOrigins))
	for _, o := range allowedOrigins {
		originsSet[o] = struct{}{}
	}

	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if _, ok := originsSet[origin]; ok {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type")
			c.Header("Access-Control-Max-Age", "86400")
		}

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}
