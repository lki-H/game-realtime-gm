package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	ContextKeyRequestID = "request_id"
	RequestIDHeader     = "X-Request-ID"
)

func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := strings.TrimSpace(c.GetHeader(RequestIDHeader))
		if !validRequestID(requestID) {
			requestID = newRequestID(time.Now())
		}

		c.Set(ContextKeyRequestID, requestID)
		c.Header(RequestIDHeader, requestID)
		c.Next()
	}
}

func CurrentRequestID(c *gin.Context) string {
	value, exists := c.Get(ContextKeyRequestID)
	if !exists {
		return ""
	}
	requestID, ok := value.(string)
	if !ok {
		return ""
	}
	return requestID
}

func validRequestID(value string) bool {
	if len(value) == 0 || len(value) > 64 {
		return false
	}
	for _, character := range value {
		if character >= 'a' && character <= 'z' ||
			character >= 'A' && character <= 'Z' ||
			character >= '0' && character <= '9' ||
			character == '_' || character == '-' || character == '.' {
			continue
		}
		return false
	}
	return true
}

func newRequestID(now time.Time) string {
	randomBytes := make([]byte, 8)
	if _, err := rand.Read(randomBytes); err != nil {
		return fmt.Sprintf("req_%d", now.UnixNano())
	}
	return fmt.Sprintf(
		"req_%d_%s",
		now.UnixNano(),
		hex.EncodeToString(randomBytes),
	)
}
