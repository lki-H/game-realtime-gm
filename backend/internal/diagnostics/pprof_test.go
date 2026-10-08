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

func TestDiagnosticAddressRequiresExplicitLoopback(t *testing.T) {
	for _, address := range []string{"127.0.0.1:6060", "[::1]:6060"} {
		if !LoopbackAddress(address) {
			t.Fatalf("loopback rejected: %s", address)
		}
	}
	for _, address := range []string{"0.0.0.0:6060", ":6060", "192.168.1.2:6060", "localhost:6060", "127.0.0.1"} {
		if LoopbackAddress(address) {
			t.Fatalf("unsafe diagnostic address accepted: %s", address)
		}
	}
}
