package credit

import (
	"errors"
	"time"

	"github.com/cristianrisueo/gopay/pkg/currency"
	"github.com/shopspring/decimal"
)

// Domain errors
var (
	ErrCreditNotFound           = errors.New("credit not found")
	ErrInsufficientCollateral   = errors.New("insufficient collateral available")
	ErrInsufficientLiquidity    = errors.New("insufficient pool liquidity")
	ErrMaxActiveCreditsExceeded = errors.New("maximum active credits exceeded")
	ErrCreditAlreadyActive      = errors.New("credit is already active")
	ErrCreditNotApproved        = errors.New("credit is not approved")
	ErrPaymentNotDue            = errors.New("payment is not pending")
	ErrInsufficientBalance      = errors.New("insufficient balance for payment")
	ErrUserHasDefaultedCredit   = errors.New("user has defaulted credits")
	ErrInvalidCurrency          = errors.New("invalid currency")
	ErrInvalidAmount            = errors.New("amount must be positive")
	ErrCreditHistoryNotFound    = errors.New("credit history not found")
	ErrLLMEvaluation            = errors.New("llm evaluation failed")
	ErrLLMResponse              = errors.New("invalid llm response")
	ErrUserNotFound             = errors.New("user not found")
)

// CreditStatus represents the status of a credit
type CreditStatus string

const (
	StatusPending   CreditStatus = "PENDING"
	StatusActive    CreditStatus = "ACTIVE"
	StatusRejected  CreditStatus = "REJECTED"
	StatusCompleted CreditStatus = "COMPLETED"
	StatusDefaulted CreditStatus = "DEFAULTED"
)

// PaymentStatus represents the status of a payment
type PaymentStatus string

const (
	PaymentPending PaymentStatus = "PENDING"
	PaymentPaid    PaymentStatus = "PAID"
	PaymentOverdue PaymentStatus = "OVERDUE"
)

// RiskLevel represents the risk evaluation of a credit
type RiskLevel string

const (
	RiskLevelLow    RiskLevel = "LOW"
	RiskLevelMedium RiskLevel = "MEDIUM"
	RiskLevelHigh   RiskLevel = "HIGH"
)

// RepaymentType represents the repayment type
type RepaymentType string

const (
	RepaymentTypeAmortized RepaymentType = "AMORTIZED"
)

// Credit represents a credit record in the database
type Credit struct {
	ID                string            `json:"id"`
	UserID            string            `json:"user_id"`
	Amount            decimal.Decimal   `json:"amount"`
	Currency          currency.Currency `json:"currency"`
	Status            CreditStatus      `json:"status"`
	APR               decimal.Decimal   `json:"apr"`
	DurationMonths    int               `json:"duration_months"`
	RepaymentType     RepaymentType     `json:"repayment_type"`
	CollateralPercent decimal.Decimal   `json:"collateral_percentage"`
	CollateralLocked  decimal.Decimal   `json:"collateral_locked"`
	RejectionReason   *string           `json:"rejection_reason,omitempty"`
	RiskLevel         *RiskLevel        `json:"risk_level,omitempty"`
	RequestedAt       time.Time         `json:"requested_at"`
	ReviewedAt        *time.Time        `json:"reviewed_at,omitempty"`
	GivenAt           *time.Time        `json:"given_at,omitempty"`
	CompletedAt       *time.Time        `json:"completed_at,omitempty"`
	LLMResponse       interface{}       `json:"llm_response,omitempty"`
	CreatedAt         time.Time         `json:"created_at"`
	UpdatedAt         time.Time         `json:"updated_at"`
}

// CreditHistory represents user's credit history and score
type CreditHistory struct {
	ID                 string           `json:"id"`
	UserID             string           `json:"user_id"`
	CreditsAsked       int              `json:"credits_asked"`
	CreditsPaid        int              `json:"credits_paid"`
	CreditsDefaulted   int              `json:"credits_defaulted"`
	TotalBorrowed      decimal.Decimal  `json:"total_amount_borrowed"`
	TotalRepaid        decimal.Decimal  `json:"total_amount_repaid"`
	OnTimePaymentRate  decimal.Decimal  `json:"on_time_payment_rate"`
	ActiveCreditsCount int              `json:"active_credits_count"`
	FirstCreditDate    *time.Time       `json:"first_credit_date,omitempty"`
	LastDefaultDate    *time.Time       `json:"last_default_date,omitempty"`
	CreditScore        *decimal.Decimal `json:"credit_score,omitempty"`
	CreatedAt          time.Time        `json:"created_at"`
	UpdatedAt          time.Time        `json:"updated_at"`
}

// Payment represents a credit payment record
type Payment struct {
	ID              string          `json:"id"`
	CreditID        string          `json:"credit_id"`
	PaymentNumber   int             `json:"payment_number"`
	PrincipalAmount decimal.Decimal `json:"principal_amount"`
	InterestAmount  decimal.Decimal `json:"interest_amount"`
	TotalAmount     decimal.Decimal `json:"total_amount"`
	DueDate         time.Time       `json:"due_date"`
	PaidDate        *time.Time      `json:"paid_date,omitempty"`
	Status          PaymentStatus   `json:"status"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

// Collateral represents locked collateral for a credit
type Collateral struct {
	ID         string            `json:"id"`
	CreditID   string            `json:"credit_id"`
	UserID     string            `json:"user_id"`
	Currency   currency.Currency `json:"currency"`
	Amount     decimal.Decimal   `json:"amount"`
	LockedAt   time.Time         `json:"locked_at"`
	ReleasedAt *time.Time        `json:"released_at,omitempty"`
	CreatedAt  time.Time         `json:"created_at"`
	UpdatedAt  time.Time         `json:"updated_at"`
}

// CreditRequest represents a credit request from the API
type CreditRequest struct {
	Amount         string `json:"amount" binding:"required"`
	Currency       string `json:"currency" binding:"required,oneof=EUR USD JPY"`
	DurationMonths int    `json:"duration_months" binding:"required,min=1,max=60"`
	Purpose        string `json:"purpose,omitempty"`
}

// CreditResponse represents a credit response to the API
type CreditResponse struct {
	ID                string      `json:"id"`
	Amount            string      `json:"amount"`
	Currency          string      `json:"currency"`
	Status            string      `json:"status"`
	APR               string      `json:"apr"`
	DurationMonths    int         `json:"duration_months"`
	CollateralPercent string      `json:"collateral_percentage,omitempty"`
	CollateralLocked  string      `json:"collateral_locked,omitempty"`
	RiskLevel         string      `json:"risk_level,omitempty"`
	RejectionReason   string      `json:"rejection_reason,omitempty"`
	RequestedAt       string      `json:"requested_at"`
	ReviewedAt        string      `json:"reviewed_at,omitempty"`
	GivenAt           string      `json:"given_at,omitempty"`
	CompletedAt       string      `json:"completed_at,omitempty"`
	LLMResponse       interface{} `json:"llm_response,omitempty"`
}

// PaymentResponse represents a payment response to the API
type PaymentResponse struct {
	ID              string `json:"id"`
	CreditID        string `json:"credit_id"`
	PaymentNumber   int    `json:"payment_number"`
	PrincipalAmount string `json:"principal_amount"`
	InterestAmount  string `json:"interest_amount"`
	TotalAmount     string `json:"total_amount"`
	DueDate         string `json:"due_date"`
	PaidDate        string `json:"paid_date,omitempty"`
	Status          string `json:"status"`
}

// CreditHistoryResponse represents a credit history response to the API
type CreditHistoryResponse struct {
	CreditsAsked       int    `json:"credits_asked"`
	CreditsPaid        int    `json:"credits_paid"`
	CreditsDefaulted   int    `json:"credits_defaulted"`
	TotalBorrowed      string `json:"total_amount_borrowed"`
	TotalRepaid        string `json:"total_amount_repaid"`
	OnTimePaymentRate  string `json:"on_time_payment_rate"`
	ActiveCreditsCount int    `json:"active_credits_count"`
	FirstCreditDate    string `json:"first_credit_date,omitempty"`
	LastDefaultDate    string `json:"last_default_date,omitempty"`
	CreditScore        string `json:"credit_score,omitempty"`
}
