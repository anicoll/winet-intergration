package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"

	dbpkg "github.com/anicoll/winet-integration/internal/pkg/database/db"
	"github.com/anicoll/winet-integration/pkg/hasher"
)

var (
	ErrInvalidCredentials = errors.New("invalid username or password")
	ErrInvalidToken       = errors.New("invalid or expired token")
)

type Claims struct {
	UserID   int    `json:"uid"`
	Username string `json:"sub"`
	jwt.RegisteredClaims
}

type tokenRecord struct {
	userID    int
	username  string
	expiresAt time.Time
}

type UserStore interface {
	GetUserByUsername(ctx context.Context, username string) (dbpkg.User, error)
}

type TokenStore interface {
	StoreRefreshToken(ctx context.Context, arg dbpkg.StoreRefreshTokenParams) error
	GetRefreshToken(ctx context.Context, tokenHash string) (dbpkg.RefreshToken, error)
	DeleteRefreshToken(ctx context.Context, tokenHash string) error
	DeleteExpiredRefreshTokens(ctx context.Context) error
}

type Service struct {
	secret          []byte
	accessTokenTTL  time.Duration
	refreshTokenTTL time.Duration
	tokens          sync.Map // sha256(rawToken) hex → tokenRecord (cache)
	db              UserStore
	tokenDB         TokenStore
}

func NewService(secret string, accessTTL, refreshTTL time.Duration, db UserStore, tokenDB TokenStore) *Service {
	return &Service{
		secret:          []byte(secret),
		accessTokenTTL:  accessTTL,
		refreshTokenTTL: refreshTTL,
		db:              db,
		tokenDB:         tokenDB,
	}
}

// StartCleanup runs a background goroutine that periodically evicts expired
// refresh tokens from the in-memory store. Call this once after NewService.
func (s *Service) RefreshTokenTTL() time.Duration { return s.refreshTokenTTL }

func (s *Service) StartCleanup(ctx context.Context, interval time.Duration) {
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				now := time.Now()
				s.tokens.Range(func(k, v any) bool {
					if v.(tokenRecord).expiresAt.Before(now) {
						s.tokens.Delete(k)
					}
					return true
				})
				_ = s.tokenDB.DeleteExpiredRefreshTokens(ctx)
			}
		}
	}()
}

func (s *Service) Login(ctx context.Context, username, password string) (accessToken, refreshToken string, err error) {
	user, err := s.db.GetUserByUsername(ctx, username)
	if err != nil {
		return "", "", ErrInvalidCredentials
	}

	if !hasher.PasswordCorrect(password, user.PasswordHash) {
		return "", "", ErrInvalidCredentials
	}

	accessToken, err = s.issueAccessToken(user.ID, user.Username)
	if err != nil {
		return "", "", fmt.Errorf("issue access token: %w", err)
	}

	refreshToken, err = hasher.GenerateToken(32)
	if err != nil {
		return "", "", fmt.Errorf("generate refresh token: %w", err)
	}

	expiresAt := time.Now().Add(s.refreshTokenTTL)
	key := hashToken(refreshToken)

	s.tokens.Store(key, tokenRecord{
		userID:    user.ID,
		username:  user.Username,
		expiresAt: expiresAt,
	})
	_ = s.tokenDB.StoreRefreshToken(ctx, dbpkg.StoreRefreshTokenParams{
		TokenHash: key,
		UserID:    user.ID,
		Username:  user.Username,
		ExpiresAt: expiresAt,
	})

	return accessToken, refreshToken, nil
}

func (s *Service) Refresh(ctx context.Context, rawRefreshToken string) (string, error) {
	key := hashToken(rawRefreshToken)

	// Fast path: in-memory cache.
	if val, ok := s.tokens.Load(key); ok {
		rec := val.(tokenRecord)
		if time.Now().After(rec.expiresAt) {
			s.tokens.Delete(key)
			_ = s.tokenDB.DeleteRefreshToken(ctx, key)
			return "", ErrInvalidToken
		}
		return s.issueAccessToken(rec.userID, rec.username)
	}

	// Slow path: DB (e.g. after a restart).
	dbTok, err := s.tokenDB.GetRefreshToken(ctx, key)
	if err != nil {
		return "", ErrInvalidToken
	}
	if time.Now().After(dbTok.ExpiresAt) {
		_ = s.tokenDB.DeleteRefreshToken(ctx, key)
		return "", ErrInvalidToken
	}

	// Re-populate cache from DB.
	s.tokens.Store(key, tokenRecord{
		userID:    dbTok.UserID,
		username:  dbTok.Username,
		expiresAt: dbTok.ExpiresAt,
	})

	return s.issueAccessToken(dbTok.UserID, dbTok.Username)
}

func (s *Service) Logout(ctx context.Context, rawRefreshToken string) {
	key := hashToken(rawRefreshToken)
	s.tokens.Delete(key)
	_ = s.tokenDB.DeleteRefreshToken(ctx, key)
}

func (s *Service) ValidateAccessToken(tokenStr string) (*Claims, error) {
	tok, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return s.secret, nil
	})
	if err != nil || !tok.Valid {
		return nil, ErrInvalidToken
	}
	claims, ok := tok.Claims.(*Claims)
	if !ok {
		return nil, ErrInvalidToken
	}
	return claims, nil
}

func (s *Service) issueAccessToken(userID int, username string) (string, error) {
	now := time.Now()
	claims := &Claims{
		UserID:   userID,
		Username: username,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   username,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(s.accessTokenTTL)),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return tok.SignedString(s.secret)
}

func hashToken(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}
