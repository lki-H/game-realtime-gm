package auth

import (
	"fmt"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	SubjectTypePlayer = "player"
	SubjectTypeAdmin  = "admin"
)

type Claims struct {
	SubjectType    string `json:"subject_type"`
	PlayerID       int64  `json:"player_id,omitempty"`
	AdminID        int64  `json:"admin_id,omitempty"`
	Username       string `json:"username"`
	Role           string `json:"role,omitempty"`
	SessionVersion int64  `json:"session_version,omitempty"`
	jwt.RegisteredClaims
}

func GenerateToken(secret string, playerID int64, username string) (string, error) {
	return GenerateVersionedToken(secret, playerID, username, 0)
}

func GenerateVersionedToken(secret string, playerID int64, username string, version int64) (string, error) {
	claims := Claims{
		SubjectType:    SubjectTypePlayer,
		PlayerID:       playerID,
		Username:       username,
		SessionVersion: version,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   strconv.FormatInt(playerID, 10),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

func GenerateAdminToken(secret string, adminID int64, username string, role string) (string, error) {
	claims := Claims{
		SubjectType: SubjectTypeAdmin,
		AdminID:     adminID,
		Username:    username,
		Role:        role,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   strconv.FormatInt(adminID, 10),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

func ParseToken(secret string, tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(secret), nil
	})
	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}

	return claims, nil
}
