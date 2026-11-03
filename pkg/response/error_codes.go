package response

const (
	// HTTP-level error codes
	ErrCodeBadRequest     = "BAD_REQUEST"
	ErrCodeUnauthorized   = "UNAUTHORIZED"
	ErrCodeForbidden      = "FORBIDDEN"
	ErrCodeNotFound       = "NOT_FOUND"
	ErrCodeConflict       = "CONFLICT"
	ErrCodeInternalServer = "INTERNAL_SERVER_ERROR"

	// Business logic error codes
	ErrCodeInsufficientBalance     = "INSUFFICIENT_BALANCE"
	ErrCodeInvalidCurrency         = "INVALID_CURRENCY"
	ErrCodeInvalidEmail            = "INVALID_EMAIL"
	ErrCodeInvalidToken            = "INVALID_TOKEN"
	ErrCodeTokenExpired            = "TOKEN_EXPIRED"
	ErrCodeSameCurrencyRequired    = "SAME_CURRENCY_REQUIRED"
	ErrCodeCannotDeleteUser        = "CANNOT_DELETE_USER"
	ErrCodeInvalidCredentials      = "INVALID_CREDENTIALS"
	ErrCodeUserAlreadyExists       = "USER_ALREADY_EXISTS"
	ErrCodeWalletNotFound          = "WALLET_NOT_FOUND"
	ErrCodeSelfTransfer            = "SELF_TRANSFER_NOT_ALLOWED"
	ErrCodeInvalidAmount           = "INVALID_AMOUNT"
	ErrCodeInsufficientCreditScore = "INSUFFICIENT_CREDIT_SCORE"
	ErrCodeMaxActiveCredits        = "MAX_ACTIVE_CREDITS_EXCEEDED"
	ErrCodeCreditNotFound          = "CREDIT_NOT_FOUND"
	ErrCodeCreditAlreadyActive     = "CREDIT_ALREADY_ACTIVE"
	ErrCodeInsufficientCollateral  = "INSUFFICIENT_COLLATERAL"
	ErrCodePaymentNotDue           = "PAYMENT_NOT_DUE"
	ErrCodeLLMEvaluation           = "LLM_EVALUATION_FAILED"
	ErrCodeCreditNotApproved       = "CREDIT_NOT_APPROVED"
	ErrCodeDefaultedCredit         = "HAS_DEFAULTED_CREDIT"
	ErrCodeNotImplemented          = "NOT_IMPLEMENTED"
	ErrCodeIdempotencyKeyReused    = "IDEMPOTENCY_KEY_REUSED"
)
