package credit

import (
	"errors"
	"net/http"

	"github.com/cristianrisueo/gopay/internal/auth"
	"github.com/cristianrisueo/gopay/pkg/response"
	"github.com/gin-gonic/gin"
)

// CreditHandler handles HTTP requests for credit operations
type CreditHandler struct {
	creditService Service
}

// NewCreditHandler creates a new credit handler instance
func NewCreditHandler(creditService Service) *CreditHandler {
	return &CreditHandler{
		creditService: creditService,
	}
}

// RequestCredit handles a credit creation
func (ch *CreditHandler) RequestCredit(ctx *gin.Context) {
	// Gets user ID from context (set by AuthMiddleware)
	userID, exists := ctx.Get(auth.ContextKeyUserID)
	if !exists {
		response.Unauthorized(ctx, "User not authenticated")
		return
	}

	// Binds and validates request
	var req CreditRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.BadRequest(ctx, "Invalid request format")
		return
	}

	// Calls service
	credit, err := ch.creditService.RequestCredit(ctx.Request.Context(), userID.(string), &req)
	if err != nil {
		ch.handleError(ctx, err)
		return
	}

	// Maps to response
	creditResp := ch.mapCreditToResponse(credit)

	response.Success(ctx, http.StatusCreated, creditResp)
}

// GetCredit handles a credit retrieval by ID if the user is the owner
func (ch *CreditHandler) GetCredit(ctx *gin.Context) {
	// Gets user ID from context
	userID, exists := ctx.Get(auth.ContextKeyUserID)
	if !exists {
		response.Unauthorized(ctx, "User not authenticated")
		return
	}

	// Gets credit ID from URL parameter
	creditID := ctx.Param("id")
	if creditID == "" {
		response.BadRequest(ctx, "Credit ID is required")
		return
	}

	// Calls service
	credit, err := ch.creditService.GetCredit(ctx.Request.Context(), userID.(string), creditID)
	if err != nil {
		ch.handleError(ctx, err)
		return
	}

	// Maps to response
	creditResp := ch.mapCreditToResponse(credit)

	response.Success(ctx, http.StatusOK, creditResp)
}

// ListMyCredits handles a retrieval of all credits from a user optionally filtered by status
func (ch *CreditHandler) ListMyCredits(ctx *gin.Context) {
	// Gets user ID from context
	userID, exists := ctx.Get(auth.ContextKeyUserID)
	if !exists {
		response.Unauthorized(ctx, "User not authenticated")
		return
	}

	// Gets optional status query parameter
	var statusFilter *CreditStatus

	statusParam := ctx.Query("status")
	if statusParam != "" {
		status := CreditStatus(statusParam)
		statusFilter = &status
	}

	// Calls service
	credits, err := ch.creditService.ListMyCredits(ctx.Request.Context(), userID.(string), statusFilter)
	if err != nil {
		response.InternalServerError(ctx, "Failed to list credits")
		return
	}

	// Maps to response
	creditResponses := make([]CreditResponse, 0, len(credits))
	for _, credit := range credits {
		creditResponses = append(creditResponses, ch.mapCreditToResponse(credit))
	}

	response.Success(ctx, http.StatusOK, gin.H{
		"credits": creditResponses,
		"count":   len(creditResponses),
	})
}

// AcceptCredit handles the acceptance of a credit
func (ch *CreditHandler) AcceptCredit(ctx *gin.Context) {
	// Gets user ID from context
	userID, exists := ctx.Get(auth.ContextKeyUserID)
	if !exists {
		response.Unauthorized(ctx, "User not authenticated")
		return
	}

	// Gets credit ID from URL parameter
	creditID := ctx.Param("id")
	if creditID == "" {
		response.BadRequest(ctx, "Credit ID is required")
		return
	}

	// Calls service
	err := ch.creditService.AcceptCredit(ctx.Request.Context(), userID.(string), creditID)
	if err != nil {
		ch.handleError(ctx, err)
		return
	}

	response.Success(ctx, http.StatusOK, gin.H{
		"message": "Credit accepted successfully",
	})
}

// MakePayment handles a credit payment
func (ch *CreditHandler) MakePayment(ctx *gin.Context) {
	// Gets user ID from context
	userID, exists := ctx.Get(auth.ContextKeyUserID)
	if !exists {
		response.Unauthorized(ctx, "User not authenticated")
		return
	}

	// Gets credit ID from URL parameter
	creditID := ctx.Param("id")
	if creditID == "" {
		response.BadRequest(ctx, "Credit ID is required")
		return
	}

	// Calls service
	payment, err := ch.creditService.MakePayment(ctx.Request.Context(), userID.(string), creditID)
	if err != nil {
		ch.handleError(ctx, err)
		return
	}

	// Maps to response
	paymentResp := ch.mapPaymentToResponse(payment)

	response.Success(ctx, http.StatusOK, paymentResp)
}

// GetCreditHistory handles the retrieval of a user's credit history
func (ch *CreditHandler) GetCreditHistory(ctx *gin.Context) {
	// Gets user ID from context
	userID, exists := ctx.Get(auth.ContextKeyUserID)
	if !exists {
		response.Unauthorized(ctx, "User not authenticated")
		return
	}

	// Calls service
	history, err := ch.creditService.GetCreditHistory(ctx.Request.Context(), userID.(string))
	if err != nil {
		ch.handleError(ctx, err)
		return
	}

	// Maps to response
	historyResp := ch.mapCreditHistoryToResponse(history)

	response.Success(ctx, http.StatusOK, historyResp)
}

//* Helper functions to map error and responses to HTTP

// handleError maps domain errors to HTTP responses
func (ch *CreditHandler) handleError(ctx *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrCreditNotFound):
		response.NotFound(ctx, "Credit not found")
	case errors.Is(err, ErrCreditHistoryNotFound):
		response.NotFound(ctx, "Credit history not found")
	case errors.Is(err, ErrInsufficientCollateral):
		response.ErrorResponse(ctx, http.StatusBadRequest, response.ErrCodeInsufficientBalance, "Insufficient collateral available")
	case errors.Is(err, ErrInsufficientLiquidity):
		response.ErrorResponse(ctx, http.StatusBadRequest, response.ErrCodeInsufficientBalance, "Insufficient pool liquidity")
	case errors.Is(err, ErrMaxActiveCreditsExceeded):
		response.ErrorResponse(ctx, http.StatusBadRequest, response.ErrCodeBadRequest, "Maximum active credits exceeded (3)")
	case errors.Is(err, ErrCreditAlreadyActive):
		response.ErrorResponse(ctx, http.StatusBadRequest, response.ErrCodeBadRequest, "Credit is already active")
	case errors.Is(err, ErrCreditNotApproved):
		response.ErrorResponse(ctx, http.StatusBadRequest, response.ErrCodeBadRequest, "Credit is not approved")
	case errors.Is(err, ErrPaymentNotDue):
		response.ErrorResponse(ctx, http.StatusBadRequest, response.ErrCodeBadRequest, "Payment is not pending")
	case errors.Is(err, ErrInsufficientBalance):
		response.ErrorResponse(ctx, http.StatusBadRequest, response.ErrCodeInsufficientBalance, "Insufficient balance for payment")
	case errors.Is(err, ErrUserHasDefaultedCredit):
		response.ErrorResponse(ctx, http.StatusBadRequest, response.ErrCodeBadRequest, "User has defaulted credits")
	case errors.Is(err, ErrInvalidCurrency):
		response.BadRequest(ctx, "Invalid currency")
	case errors.Is(err, ErrInvalidAmount):
		response.BadRequest(ctx, "Invalid amount")
	case errors.Is(err, ErrLLMEvaluation):
		response.InternalServerError(ctx, "Failed to evaluate credit request")
	case errors.Is(err, ErrLLMResponse):
		response.InternalServerError(ctx, "Invalid LLM response")
	case errors.Is(err, ErrUserNotFound):
		response.NotFound(ctx, "User not found")
	default:
		response.InternalServerError(ctx, "An error occurred")
	}
}

// mapCreditToResponse maps Credit domain model to CreditResponse
func (ch *CreditHandler) mapCreditToResponse(credit *Credit) CreditResponse {
	resp := CreditResponse{
		ID:             credit.ID,
		Amount:         credit.Amount.String(),
		Currency:       string(credit.Currency),
		Status:         string(credit.Status),
		APR:            credit.APR.String(),
		DurationMonths: credit.DurationMonths,
		RequestedAt:    credit.RequestedAt.Format("2006-01-02T15:04:05Z07:00"),
	}

	// Optional fields
	if !credit.CollateralPercent.IsZero() {
		resp.CollateralPercent = credit.CollateralPercent.String()
	}

	if !credit.CollateralLocked.IsZero() {
		resp.CollateralLocked = credit.CollateralLocked.String()
	}

	if credit.RiskLevel != nil {
		resp.RiskLevel = string(*credit.RiskLevel)
	}

	if credit.RejectionReason != nil {
		resp.RejectionReason = *credit.RejectionReason
	}

	if credit.ReviewedAt != nil {
		resp.ReviewedAt = credit.ReviewedAt.Format("2006-01-02T15:04:05Z07:00")
	}

	if credit.GivenAt != nil {
		resp.GivenAt = credit.GivenAt.Format("2006-01-02T15:04:05Z07:00")
	}

	if credit.CompletedAt != nil {
		resp.CompletedAt = credit.CompletedAt.Format("2006-01-02T15:04:05Z07:00")
	}

	if credit.LLMResponse != nil {
		resp.LLMResponse = credit.LLMResponse
	}

	return resp
}

// mapPaymentToResponse maps Payment domain model to PaymentResponse
func (ch *CreditHandler) mapPaymentToResponse(payment *Payment) PaymentResponse {
	resp := PaymentResponse{
		ID:              payment.ID,
		CreditID:        payment.CreditID,
		PaymentNumber:   payment.PaymentNumber,
		PrincipalAmount: payment.PrincipalAmount.String(),
		InterestAmount:  payment.InterestAmount.String(),
		TotalAmount:     payment.TotalAmount.String(),
		DueDate:         payment.DueDate.Format("2006-01-02T15:04:05Z07:00"),
		Status:          string(payment.Status),
	}

	if payment.PaidDate != nil {
		resp.PaidDate = payment.PaidDate.Format("2006-01-02T15:04:05Z07:00")
	}

	return resp
}

// mapCreditHistoryToResponse maps CreditHistory domain model to CreditHistoryResponse
func (ch *CreditHandler) mapCreditHistoryToResponse(history *CreditHistory) CreditHistoryResponse {
	resp := CreditHistoryResponse{
		CreditsAsked:       history.CreditsAsked,
		CreditsPaid:        history.CreditsPaid,
		CreditsDefaulted:   history.CreditsDefaulted,
		TotalBorrowed:      history.TotalBorrowed.String(),
		TotalRepaid:        history.TotalRepaid.String(),
		OnTimePaymentRate:  history.OnTimePaymentRate.String(),
		ActiveCreditsCount: history.ActiveCreditsCount,
	}

	if history.FirstCreditDate != nil {
		resp.FirstCreditDate = history.FirstCreditDate.Format("2006-01-02T15:04:05Z07:00")
	}

	if history.LastDefaultDate != nil {
		resp.LastDefaultDate = history.LastDefaultDate.Format("2006-01-02T15:04:05Z07:00")
	}

	if history.CreditScore != nil {
		resp.CreditScore = history.CreditScore.String()
	}

	return resp
}
