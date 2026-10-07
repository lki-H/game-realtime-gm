package middleware

import (
	"bytes"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestAccessLogDoesNotExposeQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var output bytes.Buffer
	previousWriter := log.Writer()
	previousFlags := log.Flags()
	log.SetOutput(&output)
	log.SetFlags(0)
	t.Cleanup(func() {
		log.SetOutput(previousWriter)
		log.SetFlags(previousFlags)
	})

	router := gin.New()
	router.Use(RequestID())
	router.Use(AccessLog())
	router.GET("/ws", func(ginContext *gin.Context) {
		ginContext.Status(http.StatusNoContent)
	})

	request := httptest.NewRequest(http.MethodGet, "/ws?token=secret-token", nil)
	request.Header.Set("X-Request-ID", "day34-log-001")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	logText := output.String()
	if strings.Contains(logText, "secret-token") || strings.Contains(logText, "?token=") {
		t.Fatalf("access log exposed query: %s", logText)
	}
	if !strings.Contains(logText, "request_id=day34-log-001") || !strings.Contains(logText, "path=/ws") {
		t.Fatalf("access log missing safe fields: %s", logText)
	}
}
