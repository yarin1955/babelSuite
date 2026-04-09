package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

type VerifyOptions struct {
	AllowQueryToken bool
	Optional        bool
}

type sessionContextKey struct{}

func PopulateSession(jwt *JWTService, options VerifyOptions) func(http.Handler) http.Handler {
	options.Optional = true
	return withSession(jwt, options)
}

func RequireSession(jwt *JWTService, options VerifyOptions) func(http.Handler) http.Handler {
	options.Optional = false
	return withSession(jwt, options)
}

func SessionFromContext(ctx context.Context) (*Claims, bool) {
	if ctx == nil {
		return nil, false
	}
	claims, ok := ctx.Value(sessionContextKey{}).(*Claims)
	return claims, ok && claims != nil
}

func withSession(jwt *JWTService, options VerifyOptions) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if next == nil {
			next = http.NotFoundHandler()
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if claims, ok := SessionFromContext(r.Context()); ok {
				next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), sessionContextKey{}, claims)))
				return
			}

			token, present := tokenFromRequest(r, options.AllowQueryToken)
			if !present {
				if options.Optional {
					next.ServeHTTP(w, r)
					return
				}
				writeSessionError(w, http.StatusUnauthorized, "Sign in required.")
				return
			}

			claims, err := jwt.Verify(token)
			if err != nil {
				if options.Optional {
					next.ServeHTTP(w, r)
					return
				}
				writeSessionError(w, http.StatusUnauthorized, "Session expired or invalid.")
				return
			}

			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), sessionContextKey{}, claims)))
		})
	}
}

func tokenFromRequest(r *http.Request, allowQueryToken bool) (string, bool) {
	if r == nil {
		return "", false
	}

	bearer := strings.TrimSpace(r.Header.Get("Authorization"))
	if strings.HasPrefix(bearer, "Bearer ") {
		token := strings.TrimSpace(strings.TrimPrefix(bearer, "Bearer "))
		if token != "" {
			return token, true
		}
	}

	if !allowQueryToken {
		return "", false
	}

	token := strings.TrimSpace(r.URL.Query().Get("token"))
	if token == "" {
		return "", false
	}
	return token, true
}

func writeSessionError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}
