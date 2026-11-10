package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"game-realtime-gm/backend/internal/auth"
	"github.com/gin-gonic/gin"
)

func TestBearerMiddlewareUsesStandardScheme(t *testing.T) {
	const secret = "unit-test-bearer-signing-key"
	playerToken, err := auth.GenerateToken(secret, 7, "player")
	if err != nil {
		t.Fatal(err)
	}
	adminToken, err := auth.GenerateAdminToken(secret, 9, "admin", "gm")
	if err != nil {
		t.Fatal(err)
	}
	for _, subject := range []struct {
		name       string
		middleware gin.HandlerFunc
		token      string
		otherToken string
	}{
		{"player", Auth(secret), playerToken, adminToken},
		{"admin", AdminAuth(secret), adminToken, playerToken},
	} {
		t.Run(subject.name, func(t *testing.T) {
			for _, scenario := range []struct {
				name, header string
				status       int
			}{
				{"standard", "Bearer " + subject.token, http.StatusNoContent},
				{"lowercase", "bearer " + subject.token, http.StatusNoContent},
				{"tab separator", "Bearer\t" + subject.token, http.StatusNoContent},
				{"raw token", subject.token, http.StatusUnauthorized},
				{"missing separator", "Bearer" + subject.token, http.StatusUnauthorized},
				{"wrong scheme", "Basic " + subject.token, http.StatusUnauthorized},
				{"extra credential", "Bearer " + subject.token + " second", http.StatusUnauthorized},
				{"empty", "", http.StatusUnauthorized},
				{"other subject", "Bearer " + subject.otherToken, http.StatusForbidden},
			} {
				t.Run(scenario.name, func(t *testing.T) {
					router := gin.New()
					router.GET("/", subject.middleware, func(c *gin.Context) { c.Status(http.StatusNoContent) })
					request := httptest.NewRequest(http.MethodGet, "/", nil)
					request.Header.Set("Authorization", scenario.header)
					response := httptest.NewRecorder()
					router.ServeHTTP(response, request)
					if response.Code != scenario.status {
						t.Fatalf("status=%d; want %d", response.Code, scenario.status)
					}
				})
			}
		})
	}
}
