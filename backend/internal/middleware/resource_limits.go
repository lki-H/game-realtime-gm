package middleware

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

func BodyLimit(maxBytes int64) gin.HandlerFunc {
	if maxBytes <= 0 {
		maxBytes = 1 << 20
	}
	return func(c *gin.Context) {
		if c.Request.ContentLength > maxBytes {
			c.AbortWithStatusJSON(http.StatusRequestEntityTooLarge, gin.H{"code": 41370, "message": "request body too large"})
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
		c.Next()
	}
}

func RedisRateLimit(client *redis.Client, prefix string, limit int, window time.Duration) gin.HandlerFunc {
	if limit <= 0 {
		limit = 20
	}
	if window <= 0 {
		window = time.Minute
	}
	return func(c *gin.Context) {
		if client == nil {
			c.Next()
			return
		}
		key := "ratelimit:" + prefix + ":" + strings.ReplaceAll(c.ClientIP(), ":", "_")
		count, err := TakeRateLimit(c.Request.Context(), client, key, limit, window)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"code": 50370, "message": "rate limiter unavailable"})
			return
		}
		if !count {
			c.Header("Retry-After", strconv.Itoa(int(window/time.Second)))
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"code": 42970, "message": "rate limit exceeded"})
			return
		}
		c.Next()
	}
}

var rateLimitScript = redis.NewScript(`
local count = redis.call('INCR', KEYS[1])
if count == 1 or redis.call('PTTL', KEYS[1]) < 0 then
    redis.call('PEXPIRE', KEYS[1], ARGV[1])
end
return count <= tonumber(ARGV[2]) and 1 or 0
`)

func TakeRateLimit(ctx context.Context, client *redis.Client, key string, limit int, window time.Duration) (bool, error) {
	value, err := rateLimitScript.Run(ctx, client, []string{key}, window.Milliseconds(), limit).Int64()
	return value == 1, err
}
