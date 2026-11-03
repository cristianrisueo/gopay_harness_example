package llm

import (
	"errors"
	"time"
)

// Exchange rate constants (hardcoded)
const (
	USDToEUR = 0.92
	JPYToEUR = 0.0062
)

// Domain errors
var (
	ErrAPIKeyMissing   = errors.New("openai api key is missing")
	ErrInvalidResponse = errors.New("invalid llm response format")
	ErrAPITimeout      = errors.New("openai api timeout")
	ErrRateLimited     = errors.New("openai rate limit exceeded")
	ErrInvalidInput    = errors.New("invalid credit evaluation input")
)

// CreditEvaluationRequest  contains all data needed for LLM evaluation
type CreditEvaluationRequest struct {
	Request         CreditRequest   `json:"request"`
	UserProfile     UserProfile     `json:"user_profile"`
	CreditHistory   CreditHistory   `json:"credit_history"`
	FinancialStatus FinancialStatus `json:"financial_status"`
	PoolStatus      PoolStatus      `json:"pool_status"`
}

// CreditRequest represents the credit request details
type CreditRequest struct {
	Amount         float64 `json:"amount"`
	Currency       string  `json:"currency"`
	DurationMonths int     `json:"duration_months"`
	Purpose        string  `json:"purpose,omitempty"`
}

// UserProfile contains user account information
type UserProfile struct {
	UserID           string    `json:"user_id"`
	Username         string    `json:"username"`
	Email            string    `json:"email"`
	RegistrationDate time.Time `json:"registration_date"`
	AccountAgeDays   int       `json:"account_age_days"`
}

// CreditHistory contains user's credit history data
type CreditHistory struct {
	CreditsAsked       int        `json:"credits_asked"`
	CreditsPaid        int        `json:"credits_paid"`
	CreditsDefaulted   int        `json:"credits_defaulted"`
	TotalBorrowed      float64    `json:"total_amount_borrowed"`
	TotalRepaid        float64    `json:"total_amount_repaid"`
	OnTimePaymentRate  float64    `json:"on_time_payment_rate"`
	ActiveCreditsCount int        `json:"active_credits_count"`
	FirstCreditDate    *time.Time `json:"first_credit_date,omitempty"`
	LastDefaultDate    *time.Time `json:"last_default_date,omitempty"`
	CreditScore        *float64   `json:"credit_score,omitempty"`
}

// FinancialStatus contains user's current financial status
type FinancialStatus struct {
	WalletBalanceEUR    float64 `json:"wallet_balance_eur"`
	WalletBalanceUSD    float64 `json:"wallet_balance_usd"`
	WalletBalanceJPY    float64 `json:"wallet_balance_jpy"`
	StakedAmountEUR     float64 `json:"staked_amount_eur"`
	StakedAmountUSD     float64 `json:"staked_amount_usd"`
	StakedAmountJPY     float64 `json:"staked_amount_jpy"`
	AvailableBalanceEUR float64 `json:"available_balance_eur"`
	AvailableBalanceUSD float64 `json:"available_balance_usd"`
	AvailableBalanceJPY float64 `json:"available_balance_jpy"`
	LoanToBalanceRatio  float64 `json:"loan_to_balance_ratio"`
}

// PoolStatus contains liquidity pool information
type PoolStatus struct {
	Currency           string  `json:"currency"`
	TotalLiquidity     float64 `json:"total_liquidity"`
	TotalUsed          float64 `json:"total_used"`
	AvailableLiquidity float64 `json:"available_liquidity"`
	UtilizationRate    float64 `json:"utilization_rate"`
	CurrentAPY         float64 `json:"current_apy"`
}

// CreditEvaluationResponse represents the LLM's evaluation decision
type CreditEvaluationResponse struct {
	Decision          string                `json:"decision"`
	RejectionReason   *string               `json:"rejection_reason,omitempty"`
	CreditTerms       *CreditTerms          `json:"credit_terms,omitempty"`
	RepaymentSchedule []PaymentScheduleItem `json:"repayment_schedule,omitempty"`
	TotalRepayment    *float64              `json:"total_repayment,omitempty"`
	TotalInterest     *float64              `json:"total_interest,omitempty"`
}

// CreditTerms contains the approved credit terms
type CreditTerms struct {
	ApprovedAmount               float64  `json:"approved_amount"`
	APR                          float64  `json:"apr"`
	DurationMonths               int      `json:"duration_months"`
	RepaymentType                string   `json:"repayment_type"`
	CollateralRequiredPercentage float64  `json:"collateral_required_percentage"`
	CollateralAmount             float64  `json:"collateral_amount"`
	CollateralCurrency           string   `json:"collateral_currency"`
	RiskLevel                    string   `json:"risk_level"`
	Conditions                   []string `json:"conditions,omitempty"`
	Reasoning                    string   `json:"reasoning"`
}

// PaymentScheduleItem represents a single payment in the schedule
type PaymentScheduleItem struct {
	PaymentNumber   int     `json:"payment_number"`
	DueDate         string  `json:"due_date"`
	PrincipalAmount float64 `json:"principal_amount"`
	InterestAmount  float64 `json:"interest_amount"`
	TotalAmount     float64 `json:"total_amount"`
}
