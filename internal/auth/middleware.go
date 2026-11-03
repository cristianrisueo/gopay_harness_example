package auth

import (
	"errors"
	"net/http"
	"strings"

	"github.com/cristianrisueo/gopay/pkg/response"
	"github.com/gin-gonic/gin"
)

// Context keys for storing authenticated user information
const (
	ContextKeyUserID   = "userID"
	ContextKeyEmail    = "email"
	ContextKeyUsername = "username"
)

// AuthMiddleware creates a Gin middleware for JWT authentication
func AuthMiddleware(authService *AuthService) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		// Extracts JWT from Authorization header
		authHeader := ctx.GetHeader("Authorization")
		if authHeader == "" {
			response.Unauthorized(ctx, "Missing authorization header")

			ctx.Abort()
			return
		}

		// Checks the Bearer token format from the auth header
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			response.Unauthorized(ctx, "Invalid authorization header format")

			ctx.Abort()
			return
		}

		// Gets the JWT part from the bearer token
		tokenString := parts[1]

		// Validates the JWT
		claims, err := authService.ValidateToken(tokenString)
		if err != nil {
			if errors.Is(err, ErrExpiredToken) {
				response.ErrorResponse(ctx, http.StatusUnauthorized, response.ErrCodeTokenExpired, "Token has expired")
			} else {
				response.ErrorResponse(ctx, http.StatusUnauthorized, response.ErrCodeInvalidToken, "Invalid token")
			}

			ctx.Abort()
			return
		}

		// Stores the user claims in the context
		ctx.Set(ContextKeyUserID, claims.UserID)
		ctx.Set(ContextKeyEmail, claims.Email)
		ctx.Set(ContextKeyUsername, claims.Username)

		ctx.Next()
	}
}
