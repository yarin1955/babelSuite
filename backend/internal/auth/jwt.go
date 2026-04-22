package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	TokenTTL      = 72 * time.Hour
	tokenIssuer   = "babelsuite"
	tokenAudience = "babelsuite.user.access"
	tokenKeyID    = "v1"
)

type Claims struct {
	UserID      string   `json:"userId"`
	WorkspaceID string   `json:"workspaceId"`
	IsAdmin     bool     `json:"isAdmin"`
	Groups      []string `json:"groups,omitempty"`
	Provider    string   `json:"provider,omitempty"`
	jwt.RegisteredClaims
}

type JWTService struct {
	secret    []byte
	mu        sync.RWMutex
	blocklist map[string]time.Time
}

func NewJWT(secret string) *JWTService {
	return &JWTService{
		secret:    []byte(secret),
		blocklist: make(map[string]time.Time),
	}
}

func (j *JWTService) Sign(userID, workspaceID string, isAdmin bool, groups []string, provider string) (string, time.Time, error) {
	expiresAt := time.Now().UTC().Add(TokenTTL)
	claims := Claims{
		UserID:      userID,
		WorkspaceID: workspaceID,
		IsAdmin:     isAdmin,
		Groups:      groups,
		Provider:    provider,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(time.Now().UTC()),
			Issuer:    tokenIssuer,
			Audience:  jwt.ClaimStrings{tokenAudience},
		},
	}

	t := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	t.Header["kid"] = tokenKeyID
	token, err := t.SignedString(j.secret)
	if err != nil {
		return "", time.Time{}, err
	}
	return token, expiresAt, nil
}

func (j *JWTService) Verify(tokenStr string) (*Claims, error) {
	parsed, err := jwt.ParseWithClaims(
		tokenStr,
		&Claims{},
		func(t *jwt.Token) (any, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, errors.New("unexpected signing method")
			}
			if kid, ok := t.Header["kid"].(string); ok && kid != tokenKeyID {
				return nil, errors.New("unrecognized token key")
			}
			return j.secret, nil
		},
		jwt.WithIssuedAt(),
		jwt.WithIssuer(tokenIssuer),
		jwt.WithAudience(tokenAudience),
	)
	if err != nil {
		return nil, err
	}

	claims, ok := parsed.Claims.(*Claims)
	if !ok || !parsed.Valid {
		return nil, errors.New("invalid token")
	}

	if j.isRevoked(tokenStr) {
		return nil, errors.New("token has been revoked")
	}

	return claims, nil
}

func (j *JWTService) Revoke(tokenStr string) {
	expiry := time.Now().UTC().Add(TokenTTL)
	if parsed, _, err := new(jwt.Parser).ParseUnverified(tokenStr, &Claims{}); err == nil {
		if c, ok := parsed.Claims.(*Claims); ok && c.ExpiresAt != nil {
			expiry = c.ExpiresAt.Time
		}
	}
	h := hashToken(tokenStr)

	j.mu.Lock()
	defer j.mu.Unlock()
	j.blocklist[h] = expiry
	j.pruneExpiredLocked()
}

func (j *JWTService) isRevoked(tokenStr string) bool {
	h := hashToken(tokenStr)
	j.mu.RLock()
	exp, found := j.blocklist[h]
	j.mu.RUnlock()
	if !found {
		return false
	}
	if time.Now().UTC().After(exp) {
		j.mu.Lock()
		delete(j.blocklist, h)
		j.mu.Unlock()
		return false
	}
	return true
}

func (j *JWTService) pruneExpiredLocked() {
	now := time.Now().UTC()
	for h, exp := range j.blocklist {
		if now.After(exp) {
			delete(j.blocklist, h)
		}
	}
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
