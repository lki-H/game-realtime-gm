package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRegisterRejectsPasswordOverBcryptByteLimit(t *testing.T) {
	for _, scenario := range []struct {
		name     string
		password string
	}{
		{"Chinese", strings.Repeat("界", 25)},
		{"emoji", strings.Repeat("😀", 19)},
		{"mixed", "abcdef" + strings.Repeat("界", 23)},
		{"ASCII", strings.Repeat("a", 73)},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			body, err := json.Marshal(map[string]string{"username": "register-test", "password": scenario.password, "nickname": "test"})
			if err != nil {
				t.Fatal(err)
			}
			router := gin.New()
			// Invalid input must be rejected before password hashing or DB access.
			router.POST("/register", NewAuthHandler(nil, "test-only").Register)
			request := httptest.NewRequest(http.MethodPost, "/register", bytes.NewReader(body))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			var result struct {
				Code int `json:"code"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if response.Code != http.StatusBadRequest || result.Code != 40001 {
				t.Fatalf("oversized password returned status=%d code=%d", response.Code, result.Code)
			}
		})
	}
}
