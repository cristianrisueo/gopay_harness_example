package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Custom errors
var (
	ErrInvalidToken = errors.New("invalid token")
	ErrExpiredToken = errors.New("token has expired")
)

// Claims represents the JWT claims structure for authenticated users
type Claims struct {
	UserID   string `json:"user_id"`
	Email    string `json:"email"`
	Username string `json:"username"`
	jwt.RegisteredClaims
}

// AuthService handles JWT token operations: Generation and validation
type AuthService struct {
	secret          []byte
	expirationHours int
}

// NewAuthService creates a new auth service instance
func NewAuthService(secret string, expirationHours int) *AuthService {
	return &AuthService{
		secret:          []byte(secret),
		expirationHours: expirationHours,
	}
}

// GenerateToken creates a new JWT token for the given user
func (as *AuthService) GenerateToken(userID, email, username string) (string, error) {
	// Generates JWT expiration time
	now := time.Now()
	expirationTime := now.Add(time.Duration(as.expirationHours) * time.Hour)

	// Creates the claims and token from them
	claims := &Claims{
		UserID:   userID,
		Email:    email,
		Username: username,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expirationTime),
			IssuedAt:  jwt.NewNumericDate(now),
			Issuer:    "gopay",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	// Signs the token and if everything is ok returns it
	tokenString, err := token.SignedString(as.secret)
	if err != nil {
		return "", err
	}

	return tokenString, nil
}

// ValidateToken validates and parses a JWT token string
func (as *AuthService) ValidateToken(tokenString string) (*Claims, error) {
	claims := &Claims{}

	// Verifies the token signing method and returns its claims
	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalidToken
		}

		return as.secret, nil
	})

	// Checks the errors during the process
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrExpiredToken
		}

		return nil, ErrInvalidToken
	}

	// Checks if the token is valid after the process, returns it if ok
	if !token.Valid {
		return nil, ErrInvalidToken
	}

	return claims, nil
}
