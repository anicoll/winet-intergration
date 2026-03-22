package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"

	dbpkg "github.com/anicoll/winet-integration/internal/pkg/database/db"
)

const (
	testSecret   = "test-secret-that-is-long-enough-32c"
	testPassword = "correct-password-123"
	testUsername = "alice"
	testUserID   = 42
)

// mockUserStore implements UserStore for tests.
type mockUserStore struct {
	user dbpkg.User
	err  error
}

func (m *mockUserStore) GetUserByUsername(_ context.Context, _ string) (dbpkg.User, error) {
	return m.user, m.err
}

// hashForTest uses minimum bcrypt cost so tests run fast.
func hashForTest(t *testing.T, pw string) string {
	t.Helper()
	h, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.MinCost)
	require.NoError(t, err)
	return string(h)
}

func newTestService(t *testing.T, store UserStore) *Service {
	t.Helper()
	return NewService(testSecret, 15*time.Minute, 24*time.Hour, store)
}

func validUserStore(t *testing.T) *mockUserStore {
	return &mockUserStore{
		user: dbpkg.User{
			ID:           testUserID,
			Username:     testUsername,
			PasswordHash: hashForTest(t, testPassword),
		},
	}
}

// --- Login ---

func TestLogin_Success(t *testing.T) {
	svc := newTestService(t, validUserStore(t))

	accessToken, refreshToken, err := svc.Login(context.Background(), testUsername, testPassword)

	require.NoError(t, err)
	assert.NotEmpty(t, accessToken)
	assert.NotEmpty(t, refreshToken)
}

func TestLogin_UnknownUser(t *testing.T) {
	svc := newTestService(t, &mockUserStore{err: errors.New("not found")})

	_, _, err := svc.Login(context.Background(), "nobody", testPassword)

	assert.ErrorIs(t, err, ErrInvalidCredentials)
}

func TestLogin_WrongPassword(t *testing.T) {
	svc := newTestService(t, validUserStore(t))

	_, _, err := svc.Login(context.Background(), testUsername, "wrong-password-123")

	assert.ErrorIs(t, err, ErrInvalidCredentials)
}

func TestLogin_StoresRefreshToken(t *testing.T) {
	svc := newTestService(t, validUserStore(t))

	_, refreshToken, err := svc.Login(context.Background(), testUsername, testPassword)
	require.NoError(t, err)

	// The stored token should be findable — a Refresh call proves it.
	newAccess, err := svc.Refresh(refreshToken)
	require.NoError(t, err)
	assert.NotEmpty(t, newAccess)
}

// --- Refresh ---

func TestRefresh_Success(t *testing.T) {
	svc := newTestService(t, validUserStore(t))
	_, refreshToken, err := svc.Login(context.Background(), testUsername, testPassword)
	require.NoError(t, err)

	newAccess, err := svc.Refresh(refreshToken)

	require.NoError(t, err)
	assert.NotEmpty(t, newAccess)
}

func TestRefresh_UnknownToken(t *testing.T) {
	svc := newTestService(t, validUserStore(t))

	_, err := svc.Refresh("this-token-was-never-issued")

	assert.ErrorIs(t, err, ErrInvalidToken)
}

func TestRefresh_ExpiredToken(t *testing.T) {
	// Create a service with a zero refresh TTL so the token is already expired.
	svc := NewService(testSecret, 15*time.Minute, -time.Second, validUserStore(t))
	_, refreshToken, err := svc.Login(context.Background(), testUsername, testPassword)
	require.NoError(t, err)

	_, err = svc.Refresh(refreshToken)

	assert.ErrorIs(t, err, ErrInvalidToken)
}

func TestRefresh_AccessTokenContainsCorrectClaims(t *testing.T) {
	svc := newTestService(t, validUserStore(t))
	_, refreshToken, err := svc.Login(context.Background(), testUsername, testPassword)
	require.NoError(t, err)

	newAccess, err := svc.Refresh(refreshToken)
	require.NoError(t, err)

	claims, err := svc.ValidateAccessToken(newAccess)
	require.NoError(t, err)
	assert.Equal(t, testUsername, claims.Username)
	assert.Equal(t, testUserID, claims.UserID)
}

// --- Logout ---

func TestLogout_RevokesToken(t *testing.T) {
	svc := newTestService(t, validUserStore(t))
	_, refreshToken, err := svc.Login(context.Background(), testUsername, testPassword)
	require.NoError(t, err)

	svc.Logout(refreshToken)

	_, err = svc.Refresh(refreshToken)
	assert.ErrorIs(t, err, ErrInvalidToken)
}

func TestLogout_UnknownTokenIsNoop(t *testing.T) {
	svc := newTestService(t, validUserStore(t))
	// Should not panic.
	assert.NotPanics(t, func() { svc.Logout("never-issued") })
}

// --- ValidateAccessToken ---

func TestValidateAccessToken_Valid(t *testing.T) {
	svc := newTestService(t, validUserStore(t))
	accessToken, _, err := svc.Login(context.Background(), testUsername, testPassword)
	require.NoError(t, err)

	claims, err := svc.ValidateAccessToken(accessToken)

	require.NoError(t, err)
	assert.Equal(t, testUsername, claims.Username)
	assert.Equal(t, testUserID, claims.UserID)
}

func TestValidateAccessToken_Expired(t *testing.T) {
	// Zero access TTL means the token expires immediately.
	svc := NewService(testSecret, -time.Second, 24*time.Hour, validUserStore(t))
	accessToken, _, err := svc.Login(context.Background(), testUsername, testPassword)
	require.NoError(t, err)

	_, err = svc.ValidateAccessToken(accessToken)

	assert.ErrorIs(t, err, ErrInvalidToken)
}

func TestValidateAccessToken_WrongSecret(t *testing.T) {
	svcA := newTestService(t, validUserStore(t))
	svcB := NewService("a-completely-different-secret-xyz", 15*time.Minute, 24*time.Hour, validUserStore(t))

	tokenFromA, _, err := svcA.Login(context.Background(), testUsername, testPassword)
	require.NoError(t, err)

	_, err = svcB.ValidateAccessToken(tokenFromA)

	assert.ErrorIs(t, err, ErrInvalidToken)
}

func TestValidateAccessToken_Garbage(t *testing.T) {
	svc := newTestService(t, validUserStore(t))

	_, err := svc.ValidateAccessToken("not.a.jwt")

	assert.ErrorIs(t, err, ErrInvalidToken)
}

func TestValidateAccessToken_Empty(t *testing.T) {
	svc := newTestService(t, validUserStore(t))

	_, err := svc.ValidateAccessToken("")

	assert.ErrorIs(t, err, ErrInvalidToken)
}
