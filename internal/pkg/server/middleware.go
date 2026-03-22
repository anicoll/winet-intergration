package server

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/anicoll/winet-integration/internal/pkg/auth"
)

type contextKey string

const claimsKey contextKey = "claims"

// ClaimsFromContext retrieves the JWT claims stored by AuthMiddleware.
func ClaimsFromContext(ctx context.Context) (*auth.Claims, bool) {
	c, ok := ctx.Value(claimsKey).(*auth.Claims)
	return c, ok
}

func LoggingMiddleware(allowedOrigin string) func(http.Handler) http.Handler {
	logger := zap.L()
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			logger.Info(r.RequestURI)
			if allowedOrigin != "" {
				w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			}
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// AuthMiddleware validates the Bearer token on every request except /auth/* and /health.
func AuthMiddleware(svc *auth.Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.URL.Path, "/auth/") || r.URL.Path == "/health" {
				next.ServeHTTP(w, r)
				return
			}

			header := r.Header.Get("Authorization")
			if !strings.HasPrefix(header, "Bearer ") {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}

			claims, err := svc.ValidateAccessToken(strings.TrimPrefix(header, "Bearer "))
			if err != nil {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), claimsKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func TimeoutMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		r = r.WithContext(ctx)
		done := make(chan struct{})

		go func() {
			next.ServeHTTP(w, r)
			close(done)
		}()

		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				http.Error(w, "Request timed out", http.StatusGatewayTimeout)
			}
		case <-done:
		}
	})
}
