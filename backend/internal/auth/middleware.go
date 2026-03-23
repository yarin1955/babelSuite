package auth

import (
	"context"
	"net/http"
	"strings"
)

type contextKey struct{}

func Middleware(jwtSvc *JWTService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			bearer := r.Header.Get("Authorization")
			if !strings.HasPrefix(bearer, "Bearer ") {
				writeErr(w, http.StatusUnauthorized, "unauthorized")
				return
			}
			claims, err := jwtSvc.Verify(strings.TrimPrefix(bearer, "Bearer "))
			if err != nil {
				writeErr(w, http.StatusUnauthorized, "unauthorized")
				return
			}
			ctx := context.WithValue(r.Context(), contextKey{}, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func ClaimsFrom(r *http.Request) *Claims {
	c, _ := r.Context().Value(contextKey{}).(*Claims)
	return c
}
