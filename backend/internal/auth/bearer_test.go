package auth

import "testing"

func TestBearerHeaderRequiresSchemeAndSingleCredential(test *testing.T) {
	for _, header := range []string{"Bearer example.jwt.token", "bearer example.jwt.token", "Bearer\texample.jwt.token"} {
		value, err := BearerToken(header)
		if err != nil || value != "example.jwt.token" {
			test.Fatalf("valid Bearer header rejected: %v", err)
		}
	}
	for _, header := range []string{"", "example.jwt.token", "Bearerexample.jwt.token", "Basic example.jwt.token", "Bearer", "Bearer first second", "Bearer first\nsecond"} {
		if _, err := BearerToken(header); err == nil {
			test.Fatal("malformed authorization header accepted")
		}
	}
}
