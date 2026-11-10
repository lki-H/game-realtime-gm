package auth

import (
	"errors"
	"strings"
)

func BearerToken(header string) (string, error) {
	parts := strings.Fields(header)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || strings.ContainsAny(header, "\r\n") {
		return "", errors.New("invalid authorization header")
	}
	return parts[1], nil
}

func ParseBearerToken(secret, header string) (*Claims, error) {
	token, err := BearerToken(header)
	if err != nil {
		return nil, err
	}
	return ParseToken(secret, token)
}
