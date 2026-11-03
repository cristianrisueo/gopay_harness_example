package transfer

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/cristianrisueo/gopay/internal/auth"
	"github.com/cristianrisueo/gopay/pkg/idempotency"
	"github.com/cristianrisueo/gopay/pkg/response"
	"github.com/gin-gonic/gin"
)

// TransferHandler handles HTTP requests for transfer operations
type TransferHandler struct {
	transferService *TransferService
}

// NewTransferHandler creates a new transfer handler instance
func NewTransferHandler(transferService *TransferService) *TransferHandler {
	return &TransferHandler{
		transferService: transferService,
	}
}

// CreateTransfer handles P2P transfer requests
func (th *TransferHandler) CreateTransfer(ctx *gin.Context) {
	// Validates the idempotency key before reading the body
	idempotencyKey, err := idempotency.ParseKey(ctx.GetHeader(idempotency.HeaderKey))
	if err != nil {
		response.BadRequest(ctx, "Missing or invalid "+idempotency.HeaderKey+" header: must be a UUID")
		return
	}

	// Validates the input data
	var req TransferRequest

	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.BadRequest(ctx, "Invalid input: "+err.Error())
		return
	}

	// Gets user ID from context (set by AuthMiddleware)
	senderID, exists := ctx.Get(auth.ContextKeyUserID)
	if !exists {
		response.Unauthorized(ctx, "User not authenticated")
		return
	}

	// Performs the transfer
	transferResp, err := th.transferService.Transfer(
		ctx.Request.Context(),
		senderID.(string),
		req.ReceiverID,
		req.Amount,
		req.Currency,
		idempotencyKey,
	)

	if err != nil {
		if errors.Is(err, ErrSelfTransfer) {
			response.BadRequest(ctx, "Cannot transfer to yourself")
			return
		}

		if errors.Is(err, ErrInvalidCurrency) {
			response.BadRequest(ctx, "Invalid currency")
			return
		}

		if errors.Is(err, ErrInvalidAmount) {
			response.BadRequest(ctx, "Invalid amount: must be positive, with at most 4 decimals and 15 integer digits")
			return
		}

		if errors.Is(err, ErrIdempotencyKeyMismatch) {
			response.ErrorResponse(
				ctx,
				http.StatusUnprocessableEntity,
				response.ErrCodeIdempotencyKeyReused,
				"Idempotency key already used with a different receiver, amount or currency",
			)
			return
		}

		if errors.Is(err, ErrIdempotencyKeyInProgress) {
			response.Conflict(ctx, "A transfer with this idempotency key is in progress, retry later")
			return
		}

		if errors.Is(err, ErrInsufficientBalance) {
			response.ErrorResponse(
				ctx,
				http.StatusBadRequest,
				response.ErrCodeInsufficientBalance,
				"Insufficient balance",
			)
			return
		}

		if errors.Is(err, ErrSenderNotFound) {
			response.NotFound(ctx, "Sender wallet not found")
			return
		}

		if errors.Is(err, ErrReceiverNotFound) {
			response.NotFound(ctx, "Receiver wallet not found")
			return
		}

		response.InternalServerError(ctx, "Failed to process transfer")
		return
	}

	// Marks retries of an already executed transfer, which keep the original 201 status
	if transferResp.Replayed {
		ctx.Header(idempotency.HeaderReplayed, "true")
	}

	// Returns success response with transfer details
	response.Success(ctx, http.StatusCreated, transferResp)
}

// GetTransferByID retrieves a transfer by its ID
func (th *TransferHandler) GetTransferByID(ctx *gin.Context) {
	// Gets transfer ID from URL parameter
	transferID := ctx.Param("id")

	// Gets user ID from context (set by AuthMiddleware)
	userID, exists := ctx.Get(auth.ContextKeyUserID)
	if !exists {
		response.Unauthorized(ctx, "User not authenticated")
		return
	}

	// Retrieves the transfer
	transfer, err := th.transferService.GetTransferByID(ctx.Request.Context(), transferID)
	if err != nil {
		if errors.Is(err, ErrTransferNotFound) {
			response.NotFound(ctx, "Transfer not found")
			return
		}

		response.InternalServerError(ctx, "Failed to get transfer")
		return
	}

	// Verifies that the authenticated user is either sender or receiver
	userIDStr := userID.(string)

	if transfer.SenderID != userIDStr && transfer.ReceiverID != userIDStr {
		response.ErrorResponse(
			ctx,
			http.StatusForbidden,
			response.ErrCodeForbidden,
			"You don't have permission to view this transfer",
		)

		return
	}

	// Returns success response with transfer data
	response.Success(ctx, http.StatusOK, transfer)
}

// GetTransferHistory retrieves the transfer history for the authenticated user
func (th *TransferHandler) GetTransferHistory(ctx *gin.Context) {
	// Gets user ID from context (set by AuthMiddleware)
	userID, exists := ctx.Get(auth.ContextKeyUserID)
	if !exists {
		response.Unauthorized(ctx, "User not authenticated")
		return
	}

	// Gets optional limit parameter from query string
	limitStr := ctx.DefaultQuery("limit", "50")
	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 {
		limit = 50
	}

	// Retrieves transfer history
	history, err := th.transferService.GetHistory(ctx.Request.Context(), userID.(string), limit)
	if err != nil {
		response.InternalServerError(ctx, "Failed to get transfer history")
		return
	}

	// Returns success response with transfer history
	response.Success(ctx, http.StatusOK, history)
}
