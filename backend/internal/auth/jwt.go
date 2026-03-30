package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const TokenTTL = 72 * time.Hour

type Claims struct {
	UserID      string `json:"userId"`
	WorkspaceID string `json:"workspaceId"`
	IsAdmin     bool   `json:"isAdmin"`
	jwt.RegisteredClaims
}

type JWTService struct {
	secret []byte
}

func NewJWT(secret string) *JWTService {
	return &JWTService{secret: []byte(secret)}
}

func (j *JWTService) Sign(userID, workspaceID string, isAdmin bool) (string, time.Time, error) {
	expiresAt := time.Now().UTC().Add(TokenTTL)
	claims := Claims{
		UserID:      userID,
		WorkspaceID: workspaceID,
		IsAdmin:     isAdmin,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(time.Now().UTC()),
		},
	}

	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(j.secret)
	if err != nil {
		return "", time.Time{}, err
	}
	return token, expiresAt, nil
}

func (j *JWTService) Verify(token string) (*Claims, error) {
	parsed, err := jwt.ParseWithClaims(token, &Claims{}, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return j.secret, nil
	})
	if err != nil {
		return nil, err
	}

	claims, ok := parsed.Claims.(*Claims)
	if !ok || !parsed.Valid {
		return nil, errors.New("invalid token")
	}
	return claims, nil
}

