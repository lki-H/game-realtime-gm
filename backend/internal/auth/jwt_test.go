package auth

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestParseTokenRequiresExpiration(t *testing.T) {
	const secret = "unit-test-signing-key"
	for _, subject := range []string{SubjectTypePlayer, SubjectTypeAdmin} {
		t.Run(subject, func(t *testing.T) {
			claims := Claims{SubjectType: subject, PlayerID: 1, AdminID: 1}
			for _, expiration := range []*jwt.NumericDate{nil, jwt.NewNumericDate(time.Now().Add(-time.Minute))} {
				claims.ExpiresAt = expiration
				token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
				if err != nil {
					t.Fatal(err)
				}
				if _, err := ParseToken(secret, token); err == nil {
					t.Fatal("token without a current expiration was accepted")
				}
			}
		})
	}
}

func TestParseTokenAcceptsIssuedPlayerAndAdminTokens(t *testing.T) {
	const secret = "unit-test-signing-key"
	player, err := GenerateVersionedToken(secret, 7, "player", 123)
	if err != nil {
		t.Fatal(err)
	}
	admin, err := GenerateAdminToken(secret, 9, "admin", "operator")
	if err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{player, admin} {
		if _, err := ParseToken(secret, token); err != nil {
			t.Fatalf("normally issued token rejected: %v", err)
		}
	}
}
