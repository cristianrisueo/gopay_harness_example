package wallet

import (
	"context"
	"fmt"

	"github.com/cristianrisueo/gopay/pkg/currency"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// Service defines the interface for wallet operations
type Service interface {
	CreateWallet(ctx context.Context, userID string) (*Wallet, error)
	GetBalance(ctx context.Context, userID string) (*BalanceResponse, error)
	Deposit(ctx context.Context, userID, amountStr string, curr currency.Currency) (*Wallet, error)
	Withdraw(ctx context.Context, userID, amountStr string, curr currency.Currency) (*Wallet, error)
	DepositWithKey(ctx context.Context, userID, amountStr string, curr currency.Currency, key string) (*Wallet, bool, error)
	WithdrawWithKey(ctx context.Context, userID, amountStr string, curr currency.Currency, key string) (*Wallet, bool, error)
}

// Type assertion to ensure WalletService implements Service
var _ Service = (*WalletService)(nil)

// WalletService handles wallet business logic
type WalletService struct {
	walletRepo Repository
}

// NewService creates a new WalletService instance
func NewService(walletRepo Repository) *WalletService {
	return &WalletService{walletRepo: walletRepo}
}

// CreateWallet creates a new wallet for a user
func (ws *WalletService) CreateWallet(ctx context.Context, userID string) (*Wallet, error) {
	wallet, err := ws.walletRepo.Create(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to create wallet: %w", err)
	}

	return wallet, nil
}

// GetBalance retrieves the balance for all currencies in a user's wallet
func (ws *WalletService) GetBalance(ctx context.Context, userID string) (*BalanceResponse, error) {
	wallet, err := ws.walletRepo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get wallet: %w", err)
	}

	return &BalanceResponse{
		EUR: wallet.BalanceEUR,
		USD: wallet.BalanceUSD,
		JPY: wallet.BalanceJPY,
	}, nil
}

// Deposit adds funds to a user's wallet for a specific currency, recorded with a server-generated key
func (ws *WalletService) Deposit(ctx context.Context, userID, amountStr string, curr currency.Currency) (*Wallet, error) {
	wallet, _, err := ws.apply(ctx, MovementDeposit, userID, amountStr, curr, uuid.NewString())

	return wallet, err
}

// Withdraw removes funds from a user's wallet for a specific currency, recorded with a server-generated key
func (ws *WalletService) Withdraw(ctx context.Context, userID, amountStr string, curr currency.Currency) (*Wallet, error) {
	wallet, _, err := ws.apply(ctx, MovementWithdrawal, userID, amountStr, curr, uuid.NewString())

	return wallet, err
}

// DepositWithKey adds funds once per idempotency key. A retry with a key the user already used
// moves no money and returns the current wallet with replayed = true
func (ws *WalletService) DepositWithKey(
	ctx context.Context,
	userID, amountStr string,
	curr currency.Currency,
	key string,
) (*Wallet, bool, error) {
	return ws.apply(ctx, MovementDeposit, userID, amountStr, curr, key)
}

// WithdrawWithKey removes funds once per idempotency key. A retry with a key the user already used
// moves no money and returns the current wallet with replayed = true, even if the balance no longer covers it
func (ws *WalletService) WithdrawWithKey(
	ctx context.Context,
	userID, amountStr string,
	curr currency.Currency,
	key string,
) (*Wallet, bool, error) {
	return ws.apply(ctx, MovementWithdrawal, userID, amountStr, curr, key)
}

// apply validates a deposit or a withdrawal, runs it with its key in one transaction and returns
// the updated wallet. If the key was already used, a retry of the same operation, currency and
// amount (compared as decimals) is a replay; anything else is ErrIdempotencyKeyMismatch
func (ws *WalletService) apply(
	ctx context.Context,
	movementType MovementType,
	userID, amountStr string,
	curr currency.Currency,
	key string,
) (*Wallet, bool, error) {
	// Validates currency
	if !curr.IsValid() {
		return nil, false, ErrInvalidCurrency
	}

	// Parses amount to decimal from string
	amount, err := decimal.NewFromString(amountStr)
	if err != nil {
		return nil, false, ErrInvalidAmount
	}

	// Validates if amount is positive
	if amount.LessThanOrEqual(decimal.Zero) {
		return nil, false, ErrInvalidAmount
	}

	// Validates that amount fits the DECIMAL(19, 4) columns instead of being rounded or overflowing
	if !amount.Equal(amount.Truncate(maxAmountDecimals)) || amount.GreaterThan(maxAmount) {
		return nil, false, ErrAmountOutOfRange
	}

	// Changes the balance and records the movement in one transaction. A withdrawal subtracts the
	// amount only if the balance covers it, checked atomically in the database
	var existing *Movement

	switch movementType {
	case MovementDeposit:
		existing, err = ws.walletRepo.DepositWithMovement(ctx, userID, curr, amount, key)
		if err != nil {
			return nil, false, fmt.Errorf("failed to deposit: %w", err)
		}
	case MovementWithdrawal:
		existing, err = ws.walletRepo.WithdrawWithMovement(ctx, userID, curr, amount, key)
		if err != nil {
			return nil, false, fmt.Errorf("failed to withdraw: %w", err)
		}
	default:
		return nil, false, fmt.Errorf("unknown movement type %q", movementType)
	}

	// A reused key must come with the same request: operation, currency and amount
	replayed := existing != nil
	if replayed && (existing.Type != movementType || existing.Currency != curr || !existing.Amount.Equal(amount)) {
		return nil, false, ErrIdempotencyKeyMismatch
	}

	// Gets and returns the updated wallet (the current one on a replay)
	wallet, err := ws.walletRepo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, false, fmt.Errorf("failed to get updated wallet: %w", err)
	}

	return wallet, replayed, nil
}
