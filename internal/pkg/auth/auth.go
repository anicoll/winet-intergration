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

type Service struct {
	secret          []byte
	accessTokenTTL  time.Duration
	refreshTokenTTL time.Duration
	tokens          sync.Map // sha256(rawToken) hex → tokenRecord
	db              UserStore
}

func NewService(secret string, accessTTL, refreshTTL time.Duration, db UserStore) *Service {
	return &Service{
		secret:          []byte(secret),
		accessTokenTTL:  accessTTL,
		refreshTokenTTL: refreshTTL,
		db:              db,
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

	s.tokens.Store(hashToken(refreshToken), tokenRecord{
		userID:    user.ID,
		username:  user.Username,
		expiresAt: time.Now().Add(s.refreshTokenTTL),
	})

	return accessToken, refreshToken, nil
}

func (s *Service) Refresh(rawRefreshToken string) (string, error) {
	key := hashToken(rawRefreshToken)
	val, ok := s.tokens.Load(key)
	if !ok {
		return "", ErrInvalidToken
	}

	rec := val.(tokenRecord)
	if time.Now().After(rec.expiresAt) {
		s.tokens.Delete(key)
		return "", ErrInvalidToken
	}

	return s.issueAccessToken(rec.userID, rec.username)
}

func (s *Service) Logout(rawRefreshToken string) {
	s.tokens.Delete(hashToken(rawRefreshToken))
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
