package middleware

import (
	"log"
	"time"

	"github.com/gin-gonic/gin"
)

func AccessLog() gin.HandlerFunc {
	return func(ginContext *gin.Context) {
		startedAt := time.Now()
		path := ginContext.Request.URL.Path

		ginContext.Next()

		log.Printf(
			"http access: request_id=%s status=%d method=%s path=%s latency=%s client_ip=%s",
			CurrentRequestID(ginContext),
			ginContext.Writer.Status(),
			ginContext.Request.Method,
			path,
			time.Since(startedAt).Round(time.Microsecond),
			ginContext.ClientIP(),
		)
	}
}
