package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"game-realtime-gm/backend/internal/auth"
	"game-realtime-gm/backend/internal/pve"
	"github.com/gin-gonic/gin"
)

func TestV2HandlersRejectMalformedBearerBeforeDatabaseAccess(t *testing.T) {
	const secret = "unit-test-v2-bearer-signing-key"
	token, err := auth.GenerateVersionedToken(secret, 101, "audit", 1)
	if err != nil {
		t.Fatal(err)
	}
	// A malformed header must be rejected before a handler touches storage.
	transport := &V2Transport{App: &pve.App{}, Secret: secret}
	for _, scenario := range []struct {
		name    string
		handler gin.HandlerFunc
		status  int
	}{
		{"query", transport.Auth, http.StatusUnauthorized},
		{"session", V2Session(nil, secret), http.StatusForbidden},
		{"websocket header", transport.WebSocket, http.StatusUnauthorized},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			for _, header := range []string{token, "Bearer" + token, "Basic " + token, "Bearer " + token + " extra"} {
				router := gin.New()
				router.GET("/", scenario.handler)
				request := httptest.NewRequest(http.MethodGet, "/", nil)
				request.Header.Set("Authorization", header)
				response := httptest.NewRecorder()
				router.ServeHTTP(response, request)
				if response.Code != scenario.status {
					t.Fatalf("malformed authorization status=%d; want %d", response.Code, scenario.status)
				}
			}
		})
	}
}
