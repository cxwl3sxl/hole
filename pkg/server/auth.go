package server

import (
	"errors"
	"net/http"
	"strings"
)

var (
	ErrInvalidToken = errors.New("invalid token")
	ErrMissingAuth  = errors.New("missing authorization header")
)

// Authenticate 检查 HTTP 请求中的 Bearer Token
func Authenticate(r *http.Request, globalToken string) error {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return ErrMissingAuth
	}

	// 格式: "Bearer <token>"
	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ErrInvalidToken
	}

	token := parts[1]
	if token != globalToken {
		return ErrInvalidToken
	}

	return nil
}
