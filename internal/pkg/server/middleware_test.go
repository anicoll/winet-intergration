package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"

	"github.com/anicoll/winet-integration/internal/pkg/auth"
	dbpkg "github.com/anicoll/winet-integration/internal/pkg/database/db"
	authmocks "github.com/anicoll/winet-integration/mocks/auth"
)

const (
	middlewareTestSecret   = "middleware-test-secret-long-enough"
	middlewareTestPassword = "middleware-test-pw-123"
	middlewareTestUsername = "bob"
)

// okHandler is a simple 200 handler used to detect pass-through.
var okHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
})

func newMiddlewareAuthService(t *testing.T) *auth.Service {
	t.Helper()
	h, err := bcrypt.GenerateFromPassword([]byte(middlewareTestPassword), bcrypt.MinCost)
	require.NoError(t, err)

	us := authmocks.NewUserStore(t)
	us.EXPECT().GetUserByUsername(mock.Anything, mock.Anything).Return(dbpkg.User{
		ID:           1,
		Username:     middlewareTestUsername,
		PasswordHash: string(h),
	}, nil).Maybe()

	ts := authmocks.NewTokenStore(t)
	ts.EXPECT().StoreRefreshToken(mock.Anything, mock.Anything).Return(nil).Maybe()
	ts.EXPECT().GetRefreshToken(mock.Anything, mock.Anything).Return(dbpkg.RefreshToken{}, errors.New("not found")).Maybe()
	ts.EXPECT().DeleteRefreshToken(mock.Anything, mock.Anything).Return(nil).Maybe()
	ts.EXPECT().DeleteExpiredRefreshTokens(mock.Anything).Return(nil).Maybe()

	return auth.NewService(middlewareTestSecret, 15*time.Minute, 24*time.Hour, us, ts)
}

func validBearerToken(t *testing.T, svc *auth.Service) string {
	t.Helper()
	tok, _, err := svc.Login(context.Background(), middlewareTestUsername, middlewareTestPassword)
	require.NoError(t, err)
	return tok
}

// --- AuthMiddleware ---

func TestAuthMiddleware_ValidToken_PassesThrough(t *testing.T) {
	svc := newMiddlewareAuthService(t)
	handler := AuthMiddleware(svc)(okHandler)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/properties", nil)
	req.Header.Set("Authorization", "Bearer "+validBearerToken(t, svc))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestAuthMiddleware_ValidToken_StoresClaimsInContext(t *testing.T) {
	svc := newMiddlewareAuthService(t)

	var gotClaims *auth.Claims
	capture := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotClaims, _ = ClaimsFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})
	handler := AuthMiddleware(svc)(capture)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/properties", nil)
	req.Header.Set("Authorization", "Bearer "+validBearerToken(t, svc))
	handler.ServeHTTP(httptest.NewRecorder(), req)

	require.NotNil(t, gotClaims)
	assert.Equal(t, middlewareTestUsername, gotClaims.Username)
}

func TestAuthMiddleware_MissingHeader_Returns401(t *testing.T) {
	handler := AuthMiddleware(newMiddlewareAuthService(t))(okHandler)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/properties", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestAuthMiddleware_InvalidToken_Returns401(t *testing.T) {
	handler := AuthMiddleware(newMiddlewareAuthService(t))(okHandler)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/properties", nil)
	req.Header.Set("Authorization", "Bearer not.a.real.token")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestAuthMiddleware_AuthPathsExempt(t *testing.T) {
	handler := AuthMiddleware(newMiddlewareAuthService(t))(okHandler)

	for _, path := range []string{"/auth/login", "/auth/refresh", "/auth/logout"} {
		req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, path, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code, "path %s should be exempt", path)
	}
}

func TestAuthMiddleware_HealthExempt(t *testing.T) {
	handler := AuthMiddleware(newMiddlewareAuthService(t))(okHandler)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

// --- LoggingMiddleware ---

func TestLoggingMiddleware_SetsCORSHeaders(t *testing.T) {
	origin := "https://example.com"
	handler := LoggingMiddleware([]string{origin})(okHandler)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/properties", nil)
	req.Header.Set("Origin", origin)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, origin, rec.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "true", rec.Header().Get("Access-Control-Allow-Credentials"))
}

func TestLoggingMiddleware_OptionsReturns204(t *testing.T) {
	origin := "https://example.com"
	handler := LoggingMiddleware([]string{origin})(okHandler)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodOptions, "/properties", nil)
	req.Header.Set("Origin", origin)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNoContent, rec.Code)
}

func TestLoggingMiddleware_NoCORSWhenOriginNotAllowed(t *testing.T) {
	handler := LoggingMiddleware([]string{"https://example.com"})(okHandler)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/properties", nil)
	req.Header.Set("Origin", "https://evil.com")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Empty(t, rec.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, http.StatusOK, rec.Code)
}
