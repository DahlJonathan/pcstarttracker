package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

// Manager issues and validates user JWTs and device tokens.
type Manager struct {
	jwtSecret []byte
	ttl       time.Duration
}

// New returns an auth Manager. secret must be a stable, non-empty value shared
// across process restarts (set via env in production).
func New(secret string, ttl time.Duration) *Manager {
	return &Manager{jwtSecret: []byte(secret), ttl: ttl}
}

// --- Password hashing -------------------------------------------------------

func HashPassword(pw string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	return string(h), err
}

func CheckPassword(hash, pw string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(pw)) == nil
}

// --- User JWTs --------------------------------------------------------------

type userClaims struct {
	jwt.RegisteredClaims
	UserID int64 `json:"uid"`
}

// IssueUserToken returns a signed JWT for the given user id.
func (m *Manager) IssueUserToken(userID int64) (string, error) {
	now := time.Now()
	claims := userClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   fmt.Sprintf("%d", userID),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(m.ttl)),
		},
		UserID: userID,
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return tok.SignedString(m.jwtSecret)
}

// ParseUserToken validates a JWT and returns the embedded user id.
func (m *Manager) ParseUserToken(tokenStr string) (int64, error) {
	claims := &userClaims{}
	_, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return m.jwtSecret, nil
	})
	if err != nil {
		return 0, err
	}
	return claims.UserID, nil
}

// --- Device tokens & opaque secrets ----------------------------------------

// RandomToken returns a URL-safe random string with n bytes of entropy.
func RandomToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// NumericCode returns a zero-padded random code with the given number of digits.
func NumericCode(digits int) (string, error) {
	max := big.NewInt(1)
	for i := 0; i < digits; i++ {
		max.Mul(max, big.NewInt(10))
	}
	n, err := rand.Int(rand.Reader, max)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%0*d", digits, n), nil
}

// HashToken returns a hex SHA-256 digest, used to store device tokens at rest
// so a DB leak does not expose usable credentials.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
