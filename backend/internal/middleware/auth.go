package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/example/invest-portfolio-platform/backend/internal/auth"
)

type contextKey string

const claimsContextKey contextKey = "jwt_claims"

func JWTAuth(secret string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		if header == "" || !strings.HasPrefix(header, "Bearer ") {
			writeMiddlewareError(w, http.StatusUnauthorized, "missing bearer token")
			return
		}
		claims, err := auth.ParseToken(strings.TrimPrefix(header, "Bearer "), secret)
		if err != nil {
			writeMiddlewareError(w, http.StatusUnauthorized, "invalid token")
			return
		}
		ctx := context.WithValue(r.Context(), claimsContextKey, claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func ClaimsFromContext(ctx context.Context) (*auth.Claims, bool) {
	claims, ok := ctx.Value(claimsContextKey).(*auth.Claims)
	return claims, ok
}

func writeMiddlewareError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(`{"error":"` + message + `"}`))
}
