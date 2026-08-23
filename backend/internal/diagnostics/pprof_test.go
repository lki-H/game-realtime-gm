package diagnostics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewPprofServerRegistersIndex(t *testing.T) {
	server := NewPprofServer("127.0.0.1:0")
	request := httptest.NewRequest(http.MethodGet, "/debug/pprof/", nil)
	response := httptest.NewRecorder()
	server.Handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", response.Code)
	}
	if !strings.Contains(response.Body.String(), "Types of profiles available") {
		t.Fatalf("unexpected pprof index: %s", response.Body.String())
	}
}
