package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var ErrInvalidToken = errors.New("invalid token")

// Claims is the JWT payload for Agent Hub sessions.
type Claims struct {
	UserID   string `json:"uid"`
	Username string `json:"username"`
	Role     string `json:"role"`
	jwt.RegisteredClaims
}

// TokenService issues and validates JWTs.
// When ttl <= 0, tokens are issued without an expiration (forever).
type TokenService struct {
	secret []byte
	ttl    time.Duration
}

// NewTokenService constructs a TokenService.
// Pass ttl <= 0 for non-expiring tokens.
func NewTokenService(secret string, ttl time.Duration) *TokenService {
	return &TokenService{secret: []byte(secret), ttl: ttl}
}

// Issue creates a signed access token for the given user.
// expiresAt is zero when the token never expires.
func (t *TokenService) Issue(userID, username, role string) (token string, expiresAt time.Time, err error) {
	now := time.Now().UTC()
	claims := Claims{
		UserID:   userID,
		Username: username,
		Role:     role,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:  userID,
			IssuedAt: jwt.NewNumericDate(now),
			Issuer:   "agent-hub",
		},
	}
	if t.ttl > 0 {
		exp := now.Add(t.ttl)
		claims.ExpiresAt = jwt.NewNumericDate(exp)
		expiresAt = exp
	}
	// ttl <= 0: omit ExpiresAt → token never expires

	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString(t.secret)
	if err != nil {
		return "", time.Time{}, err
	}
	return signed, expiresAt, nil
}

// Parse validates a token string and returns claims.
// Tokens without exp are accepted (forever sessions).
func (t *TokenService) Parse(tokenStr string) (*Claims, error) {
	tok, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return t.secret, nil
	})
	if err != nil {
		return nil, ErrInvalidToken
	}
	claims, ok := tok.Claims.(*Claims)
	if !ok || !tok.Valid {
		return nil, ErrInvalidToken
	}
	return claims, nil
}
