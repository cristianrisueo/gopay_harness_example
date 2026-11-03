package response

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Response represents a standard API response
type Response struct {
	Success bool   `json:"success"`
	Data    any    `json:"data,omitempty"`
	Error   *Error `json:"error,omitempty"`
}

// Error represents an error in the API response
type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Success sends a successful JSON response
func Success(ctx *gin.Context, code int, data any) {
	ctx.JSON(code, Response{
		Success: true,
		Data:    data,
	})
}

// ErrorResponse sends an error JSON response
func ErrorResponse(ctx *gin.Context, code int, errCode string, message string) {
	ctx.JSON(code, Response{
		Success: false,
		Error: &Error{
			Code:    errCode,
			Message: message,
		},
	})
}

// BadRequest sends a 400 Bad Request response
func BadRequest(ctx *gin.Context, message string) {
	ErrorResponse(ctx, http.StatusBadRequest, ErrCodeBadRequest, message)
}

// Unauthorized sends a 401 Unauthorized response
func Unauthorized(ctx *gin.Context, message string) {
	ErrorResponse(ctx, http.StatusUnauthorized, ErrCodeUnauthorized, message)
}

// Forbidden sends a 403 Forbidden response
func Forbidden(ctx *gin.Context, message string) {
	ErrorResponse(ctx, http.StatusForbidden, ErrCodeForbidden, message)
}

// NotFound sends a 404 Not Found response
func NotFound(ctx *gin.Context, message string) {
	ErrorResponse(ctx, http.StatusNotFound, ErrCodeNotFound, message)
}

// Conflict sends a 409 Conflict response
func Conflict(ctx *gin.Context, message string) {
	ErrorResponse(ctx, http.StatusConflict, ErrCodeConflict, message)
}

// InternalServerError sends a 500 Internal Server Error response
func InternalServerError(ctx *gin.Context, message string) {
	ErrorResponse(ctx, http.StatusInternalServerError, ErrCodeInternalServer, message)
}

// HandleStakingError maps staking domain errors to HTTP responses
func HandleStakingError(ctx *gin.Context, err error) {
	switch err.Error() {
	case "insufficient available balance for staking":
		ErrorResponse(ctx, http.StatusBadRequest, ErrCodeInsufficientBalance, err.Error())
	case "invalid amount: must be at least 1000":
		ErrorResponse(ctx, http.StatusBadRequest, ErrCodeBadRequest, "Amount must be at least 1000")
	case "invalid currency":
		ErrorResponse(ctx, http.StatusBadRequest, ErrCodeBadRequest, "Invalid currency")
	case "staking position not found":
		ErrorResponse(ctx, http.StatusNotFound, ErrCodeNotFound, "Staking position not found")
	case "staking position is not active":
		ErrorResponse(ctx, http.StatusBadRequest, ErrCodeBadRequest, "Staking position is not active")
	case "staking position is still locked":
		ErrorResponse(ctx, http.StatusBadRequest, ErrCodeBadRequest, "Staking position is still locked. Cannot withdraw before lockup period ends")
	case "you are not authorized to access this position":
		Unauthorized(ctx, "You are not authorized to access this position")
	default:
		InternalServerError(ctx, "Failed to process staking request")
	}
}
