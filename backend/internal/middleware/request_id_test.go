package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestValidRequestID(t *testing.T) {
	valid := []string{"day33-summary-001", "req_123.abc"}
	for _, value := range valid {
		if !validRequestID(value) {
			t.Fatalf("expected %q to be valid", value)
		}
	}

	invalid := []string{"", "bad request", "bad\nrequest", strings.Repeat("a", 65)}
	for _, value := range invalid {
		if validRequestID(value) {
			t.Fatalf("expected %q to be invalid", value)
		}
	}
}

func TestRequestIDMiddlewarePreservesValidHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RequestID())
	router.GET("/", func(c *gin.Context) {
		c.String(http.StatusOK, CurrentRequestID(c))
	})

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set(RequestIDHeader, "day33-summary-001")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Header().Get(RequestIDHeader) != "day33-summary-001" {
		t.Fatalf("unexpected response request id: %s", response.Header().Get(RequestIDHeader))
	}
	if response.Body.String() != "day33-summary-001" {
		t.Fatalf("unexpected context request id: %s", response.Body.String())
	}
}

func TestRequestIDMiddlewareReplacesInvalidHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RequestID())
	router.GET("/", func(c *gin.Context) {
		c.String(http.StatusOK, CurrentRequestID(c))
	})

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set(RequestIDHeader, "bad request id")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	requestID := response.Header().Get(RequestIDHeader)
	if !validRequestID(requestID) || requestID == "bad request id" {
		t.Fatalf("invalid replacement request id: %s", requestID)
	}
}
