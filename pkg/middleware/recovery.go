package middleware

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
)

// Recovery はパニックからの回復を行うGinミドルウェアを返す。
// パニック発生時にスタックトレースをログに出力し、500エラーを返す。
func Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[PANIC] %s %s: %v", c.Request.Method, c.Request.URL.Path, r)
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
					"error": "内部サーバーエラーが発生しました",
				})
			}
		}()
		c.Next()
	}
}
