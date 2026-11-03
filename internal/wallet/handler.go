package wallet

import (
	"context"
	"errors"
	"net/http"

	"github.com/cristianrisueo/gopay/internal/auth"
	"github.com/cristianrisueo/gopay/pkg/currency"
	"github.com/cristianrisueo/gopay/pkg/idempotency"
	"github.com/cristianrisueo/gopay/pkg/response"
	"github.com/gin-gonic/gin"
)

// WalletHandler handles HTTP requests for wallet operations
type WalletHandler struct {
	walletService *WalletService
}

// NewWalletHandler creates a new wallet handler instance
func NewWalletHandler(walletService *WalletService) *WalletHandler {
	return &WalletHandler{
		walletService: walletService,
	}
}

// GetBalance retrieves the balance for all currencies in the authenticated user's wallet
func (wh *WalletHandler) GetBalance(ctx *gin.Context) {
	// Gets user ID from context (set by AuthMiddleware)
	userID, exists := ctx.Get(auth.ContextKeyUserID)
	if !exists {
		response.Unauthorized(ctx, "User not authenticated")
		return
	}

	// Gets the balance
	balance, err := wh.walletService.GetBalance(ctx.Request.Context(), userID.(string))
	if err != nil {
		if errors.Is(err, ErrWalletNotFound) {
			response.NotFound(ctx, "Wallet not found")
			return
		}

		response.InternalServerError(ctx, "Failed to get balance")
		return
	}

	// Returns success response with balance data
	response.Success(ctx, http.StatusOK, balance)
}

// TransactionFunc represents the service function that processes the deposit or withdraw once per
// idempotency key. It reports replayed = true when the key was already used and no money moved
type TransactionFunc func(ctx context.Context, userID, amountStr string, curr currency.Currency, key string) (*Wallet, bool, error)

// ProcessTransaction is a factory function that creates a closure. Receives a
// service function and returns a Gin handler that uses that function.
func (wh *WalletHandler) ProcessTransaction(serviceFunc TransactionFunc) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		// Validates the idempotency key before reading the body
		idempotencyKey, err := idempotency.ParseKey(ctx.GetHeader(idempotency.HeaderKey))
		if err != nil {
			response.BadRequest(ctx, "Missing or invalid "+idempotency.HeaderKey+" header: must be a UUID")
			return
		}

		// Validates the input data
		var req TransactionRequest

		if err := ctx.ShouldBindJSON(&req); err != nil {
			response.BadRequest(ctx, "Invalid input: "+err.Error())
			return
		}

		// Gets user ID from context (set by AuthMiddleware)
		userID, exists := ctx.Get(auth.ContextKeyUserID)
		if !exists {
			response.Unauthorized(ctx, "User not authenticated")
			return
		}

		// Processes the transaction using the provided service function
		wallet, replayed, err := serviceFunc(ctx.Request.Context(), userID.(string), req.Amount, req.Currency, idempotencyKey)
		if err != nil {
			if errors.Is(err, ErrInvalidCurrency) {
				response.BadRequest(ctx, "Invalid currency")
				return
			}

			if errors.Is(err, ErrAmountOutOfRange) {
				response.BadRequest(ctx, "Invalid amount: at most 4 decimals and at most 999999999999999.9999")
				return
			}

			if errors.Is(err, ErrInvalidAmount) {
				response.BadRequest(ctx, "Invalid amount: must be positive")
				return
			}

			if errors.Is(err, ErrIdempotencyKeyMismatch) {
				response.ErrorResponse(
					ctx,
					http.StatusUnprocessableEntity,
					response.ErrCodeIdempotencyKeyReused,
					"Idempotency key already used with a different operation, amount or currency",
				)
				return
			}

			if errors.Is(err, ErrIdempotencyKeyInProgress) {
				response.Conflict(ctx, "A wallet operation with this idempotency key is in progress, retry later")
				return
			}

			if errors.Is(err, ErrInsufficientBalance) {
				response.ErrorResponse(ctx, http.StatusBadRequest, response.ErrCodeInsufficientBalance, "Insufficient balance")
				return
			}

			if errors.Is(err, ErrWalletNotFound) {
				response.NotFound(ctx, "Wallet not found")
				return
			}

			response.InternalServerError(ctx, "Failed to process transaction")
			return
		}

		// Marks retries of an already applied operation, which keep the original 200 status
		if replayed {
			ctx.Header(idempotency.HeaderReplayed, "true")
		}

		// Returns success response with updated wallet
		response.Success(ctx, http.StatusOK, wallet)
	}
}
