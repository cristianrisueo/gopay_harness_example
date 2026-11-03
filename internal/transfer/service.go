package transfer

import (
	"context"
	"fmt"
	"strings"

	"github.com/cristianrisueo/gopay/internal/wallet"
	"github.com/cristianrisueo/gopay/pkg/currency"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// Service defines the interface for transfer operations
type Service interface {
	Transfer(
		ctx context.Context,
		senderID, receiverID string,
		amountStr string,
		curr currency.Currency,
		idempotencyKey string,
	) (*TransferResponse, error)
	GetTransferByID(ctx context.Context, transferID string) (*Transfer, error)
	GetHistory(ctx context.Context, userID string, limit int) ([]TransferHistoryItem, error)
}

// Type assertion to ensure TransferService implements Service
var _ Service = (*TransferService)(nil)

// TransferService handles transfer business logic
type TransferService struct {
	transferRepo Repository
	walletRepo   wallet.Repository
	logger       *zap.Logger
}

// NewService creates a new TransferService instance
func NewService(transferRepo Repository, walletRepo wallet.Repository, logger *zap.Logger) *TransferService {
	return &TransferService{
		transferRepo: transferRepo,
		walletRepo:   walletRepo,
		logger:       logger,
	}
}

// Transfer performs a P2P transfer between two users. A retry with an idempotency key the
// sender already used returns the original transfer without moving money
func (ts *TransferService) Transfer(
	ctx context.Context,
	senderID, receiverID string,
	amountStr string,
	curr currency.Currency,
	idempotencyKey string,
) (*TransferResponse, error) {
	// Validates that sender and receiver are different
	if senderID == receiverID {
		return nil, ErrSelfTransfer
	}

	// Validates currency
	if !curr.IsValid() {
		return nil, ErrInvalidCurrency
	}

	// Parses amount to decimal from string
	amount, err := decimal.NewFromString(amountStr)
	if err != nil {
		return nil, ErrInvalidAmount
	}

	// Validates if amount is positive
	if amount.LessThanOrEqual(decimal.Zero) {
		return nil, ErrInvalidAmount
	}

	// Validates that amount fits the DECIMAL(19, 4) columns instead of being rounded or overflowing
	if !amount.Equal(amount.Truncate(maxAmountDecimals)) || amount.GreaterThan(maxAmount) {
		return nil, ErrInvalidAmount
	}

	// Validates that sender's wallet exists
	_, err = ts.walletRepo.GetByUserID(ctx, senderID)
	if err != nil {
		if err == wallet.ErrWalletNotFound {
			return nil, ErrSenderNotFound
		}

		return nil, fmt.Errorf("failed to get sender wallet: %w", err)
	}

	// Validates that receiver's wallet exists
	_, err = ts.walletRepo.GetByUserID(ctx, receiverID)
	if err != nil {
		if err == wallet.ErrWalletNotFound {
			return nil, ErrReceiverNotFound
		}

		return nil, fmt.Errorf("failed to get receiver wallet: %w", err)
	}

	// Performs the transfer using the repository (ACID transaction). The sufficient balance check
	// happens there, under the wallet lock and after the idempotency key, so a retry of a transfer
	// that emptied the balance is still answered as a replay
	transfer, replayed, err := ts.transferRepo.Transfer(ctx, senderID, receiverID, amount, curr, idempotencyKey)
	if err != nil {
		return nil, fmt.Errorf("failed to execute transfer: %w", err)
	}

	// A reused key must come with the same request: receiver, amount and currency
	if replayed {
		logFields := []zap.Field{
			zap.String("user_id", senderID),
			zap.String("idempotency_key", idempotencyKey),
			zap.String("transfer_id", transfer.ID),
		}

		if !strings.EqualFold(transfer.ReceiverID, receiverID) ||
			!transfer.Amount.Equal(amount) ||
			transfer.Currency != curr {
			ts.logger.Warn("Idempotency key reused with different payload", logFields...)
			return nil, ErrIdempotencyKeyMismatch
		}

		ts.logger.Info("Transfer idempotent replay", logFields...)
	}

	// Gets updated balances after transfer
	senderWalletUpdated, err := ts.walletRepo.GetByUserID(ctx, senderID)
	if err != nil {
		return nil, fmt.Errorf("failed to get updated sender wallet: %w", err)
	}

	receiverWalletUpdated, err := ts.walletRepo.GetByUserID(ctx, receiverID)
	if err != nil {
		return nil, fmt.Errorf("failed to get updated receiver wallet: %w", err)
	}

	// Gets the correct balance based on currency
	var senderNewBalance, receiverNewBalance decimal.Decimal

	switch curr {
	case currency.EUR:
		senderNewBalance = senderWalletUpdated.BalanceEUR
		receiverNewBalance = receiverWalletUpdated.BalanceEUR
	case currency.USD:
		senderNewBalance = senderWalletUpdated.BalanceUSD
		receiverNewBalance = receiverWalletUpdated.BalanceUSD
	case currency.JPY:
		senderNewBalance = senderWalletUpdated.BalanceJPY
		receiverNewBalance = receiverWalletUpdated.BalanceJPY
	}

	// Builds and returns the response of the transfer
	return &TransferResponse{
		TransferID:      transfer.ID,
		SenderID:        transfer.SenderID,
		ReceiverID:      transfer.ReceiverID,
		Amount:          transfer.Amount,
		Currency:        transfer.Currency,
		Status:          transfer.Status,
		SenderBalance:   senderNewBalance,
		ReceiverBalance: receiverNewBalance,
		CreatedAt:       transfer.CreatedAt,
		Replayed:        replayed,
	}, nil
}

// GetTransferByID retrieves a transfer by its ID
func (ts *TransferService) GetTransferByID(
	ctx context.Context,
	transferID string,
) (*Transfer, error) {
	transfer, err := ts.transferRepo.GetByID(ctx, transferID)
	if err != nil {
		return nil, fmt.Errorf("failed to get transfer: %w", err)
	}

	return transfer, nil
}

// GetHistory retrieves transfer history for a user
func (ts *TransferService) GetHistory(
	ctx context.Context,
	userID string,
	limit int,
) ([]TransferHistoryItem, error) {
	// Sets a default limit if none provided
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	history, err := ts.transferRepo.GetHistoryByUserID(ctx, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get transfer history: %w", err)
	}

	return history, nil
}
