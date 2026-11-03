//go:build integration

package wallet

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/cristianrisueo/gopay/pkg/currency"
	"github.com/cristianrisueo/gopay/pkg/response"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// movement is one wallet_movements row as the tests read it
type movement struct {
	UserID         string
	Type           string
	Currency       string
	Amount         decimal.Decimal
	IdempotencyKey string
}

// movementsOf returns every wallet_movements row of the user, oldest first
func movementsOf(t *testing.T, userID string) []movement {
	t.Helper()

	rows, err := testDB.Query(
		`SELECT user_id, type, currency, amount, idempotency_key
		 FROM wallet_movements
		 WHERE user_id = $1
		 ORDER BY created_at, id`,
		userID,
	)
	if err != nil {
		t.Fatalf("failed to read wallet movements: %v", err)
	}
	defer rows.Close()

	var movements []movement

	for rows.Next() {
		var m movement
		var amount string

		if err := rows.Scan(&m.UserID, &m.Type, &m.Currency, &amount, &m.IdempotencyKey); err != nil {
			t.Fatalf("failed to scan wallet movement: %v", err)
		}

		m.Amount = decimal.RequireFromString(amount)
		movements = append(movements, m)
	}

	if err := rows.Err(); err != nil {
		t.Fatalf("failed to iterate wallet movements: %v", err)
	}

	return movements
}

// requireOneMovement asserts that the user has exactly one movement with the given fields
func requireOneMovement(t *testing.T, userID, wantType, wantCurrency, wantAmount string) movement {
	t.Helper()

	movements := movementsOf(t, userID)
	if len(movements) != 1 {
		t.Fatalf("movements of %s = %d, want 1: %+v", userID, len(movements), movements)
	}

	m := movements[0]

	if m.UserID != userID {
		t.Errorf("movement user_id = %s, want %s", m.UserID, userID)
	}

	if m.Type != wantType {
		t.Errorf("movement type = %s, want %s", m.Type, wantType)
	}

	if m.Currency != wantCurrency {
		t.Errorf("movement currency = %s, want %s", m.Currency, wantCurrency)
	}

	if !m.Amount.Equal(decimal.RequireFromString(wantAmount)) {
		t.Errorf("movement amount = %s, want %s", m.Amount, wantAmount)
	}

	if _, err := uuid.Parse(m.IdempotencyKey); err != nil {
		t.Errorf("movement idempotency_key = %q is not a UUID: %v", m.IdempotencyKey, err)
	}

	return m
}

// requireNoMovements asserts that the user has no movement at all
func requireNoMovements(t *testing.T, userID string) {
	t.Helper()

	if movements := movementsOf(t, userID); len(movements) != 0 {
		t.Fatalf("movements of %s = %d, want 0: %+v", userID, len(movements), movements)
	}
}

// AC1: a deposit through the route inserts one DEPOSIT row and raises the balance once
func TestDeposit_RecordsMovement(t *testing.T) {
	router := newTestRouter()
	user := createUser(t, balances{EUR: "100", USD: "0", JPY: "0"})

	wallet := requireOK(t, postWallet(router, user.Token, "deposit", "25.5", "EUR"))
	if !wallet.BalanceEUR.Equal(decimal.RequireFromString("125.5")) {
		t.Errorf("balance_eur = %s, want 125.5", wallet.BalanceEUR)
	}

	assertBalances(t, user.ID, balances{EUR: "125.5", USD: "0", JPY: "0"})
	requireOneMovement(t, user.ID, "DEPOSIT", "EUR", "25.5")
}

// AC2: a withdrawal through the route inserts one WITHDRAWAL row
func TestWithdraw_RecordsMovement(t *testing.T) {
	router := newTestRouter()
	user := createUser(t, balances{EUR: "0", USD: "100", JPY: "0"})

	wallet := requireOK(t, postWallet(router, user.Token, "withdraw", "30", "USD"))
	if !wallet.BalanceUSD.Equal(decimal.RequireFromString("70")) {
		t.Errorf("balance_usd = %s, want 70", wallet.BalanceUSD)
	}

	assertBalances(t, user.ID, balances{EUR: "0", USD: "70", JPY: "0"})
	requireOneMovement(t, user.ID, "WITHDRAWAL", "USD", "30")
}

// AC3: a failed deposit or withdrawal inserts no row and leaves the balance unchanged
func TestWalletMovement_FailuresRecordNothing(t *testing.T) {
	tests := []struct {
		name       string
		start      *balances // nil means the user has no wallet
		operation  string
		amount     string
		currency   string
		wantStatus int
		wantCode   string
	}{
		{name: "insufficient balance", start: &balances{EUR: "50", USD: "0", JPY: "0"}, operation: "withdraw", amount: "60", currency: "EUR", wantStatus: http.StatusBadRequest, wantCode: response.ErrCodeInsufficientBalance},
		{name: "negative wallet withdraw", start: &balances{EUR: "-40", USD: "0", JPY: "0"}, operation: "withdraw", amount: "10", currency: "EUR", wantStatus: http.StatusBadRequest, wantCode: response.ErrCodeInsufficientBalance},
		{name: "missing wallet deposit", operation: "deposit", amount: "10", currency: "EUR", wantStatus: http.StatusNotFound, wantCode: response.ErrCodeNotFound},
		{name: "missing wallet withdraw", operation: "withdraw", amount: "10", currency: "EUR", wantStatus: http.StatusNotFound, wantCode: response.ErrCodeNotFound},
		{name: "zero deposit", start: &balances{EUR: "50", USD: "0", JPY: "0"}, operation: "deposit", amount: "0", currency: "EUR", wantStatus: http.StatusBadRequest, wantCode: response.ErrCodeBadRequest},
		{name: "negative deposit", start: &balances{EUR: "50", USD: "0", JPY: "0"}, operation: "deposit", amount: "-10", currency: "EUR", wantStatus: http.StatusBadRequest, wantCode: response.ErrCodeBadRequest},
		{name: "non numeric deposit", start: &balances{EUR: "50", USD: "0", JPY: "0"}, operation: "deposit", amount: "abc", currency: "EUR", wantStatus: http.StatusBadRequest, wantCode: response.ErrCodeBadRequest},
		{name: "five decimals deposit", start: &balances{EUR: "50", USD: "0", JPY: "0"}, operation: "deposit", amount: "10.00001", currency: "EUR", wantStatus: http.StatusBadRequest, wantCode: response.ErrCodeBadRequest},
		{name: "zero withdraw", start: &balances{EUR: "50", USD: "0", JPY: "0"}, operation: "withdraw", amount: "0", currency: "EUR", wantStatus: http.StatusBadRequest, wantCode: response.ErrCodeBadRequest},
		{name: "negative withdraw", start: &balances{EUR: "50", USD: "0", JPY: "0"}, operation: "withdraw", amount: "-10", currency: "EUR", wantStatus: http.StatusBadRequest, wantCode: response.ErrCodeBadRequest},
		{name: "non numeric withdraw", start: &balances{EUR: "50", USD: "0", JPY: "0"}, operation: "withdraw", amount: "abc", currency: "EUR", wantStatus: http.StatusBadRequest, wantCode: response.ErrCodeBadRequest},
		{name: "five decimals withdraw", start: &balances{EUR: "50", USD: "0", JPY: "0"}, operation: "withdraw", amount: "10.00001", currency: "EUR", wantStatus: http.StatusBadRequest, wantCode: response.ErrCodeBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := newTestRouter()

			var user testUser
			if tt.start == nil {
				user = insertUser(t)
			} else {
				user = createUser(t, *tt.start)
			}

			rec := postWallet(router, user.Token, tt.operation, tt.amount, tt.currency)

			requireError(t, rec, tt.wantStatus, tt.wantCode, "")
			requireNoMovements(t, user.ID)

			if tt.start != nil {
				assertBalances(t, user.ID, *tt.start)
			}
		})
	}
}

// AC4: a deposit into a negative wallet is applied and recorded
func TestDeposit_IntoNegativeBalanceRecordsMovement(t *testing.T) {
	router := newTestRouter()
	user := createUser(t, balances{EUR: "-40", USD: "0", JPY: "0"})

	wallet := requireOK(t, postWallet(router, user.Token, "deposit", "10", "EUR"))
	if !wallet.BalanceEUR.Equal(decimal.RequireFromString("-30")) {
		t.Errorf("balance_eur = %s, want -30", wallet.BalanceEUR)
	}

	assertBalances(t, user.ID, balances{EUR: "-30", USD: "0", JPY: "0"})
	requireOneMovement(t, user.ID, "DEPOSIT", "EUR", "10")
}

// AC5: more than 4 decimals or above 999999999999999.9999 is a 400 on both routes; 4 decimals is accepted
func TestWalletAmountPrecision(t *testing.T) {
	start := balances{EUR: "100", USD: "0", JPY: "0"}

	for _, operation := range []string{"deposit", "withdraw"} {
		t.Run(operation, func(t *testing.T) {
			for _, amount := range []string{"10.00001", "1000000000000000"} {
				t.Run("rejects "+amount, func(t *testing.T) {
					router := newTestRouter()
					user := createUser(t, start)

					rec := postWallet(router, user.Token, operation, amount, "EUR")

					requireError(t, rec, http.StatusBadRequest, response.ErrCodeBadRequest, "")
					assertBalances(t, user.ID, start)
					requireNoMovements(t, user.ID)
				})
			}

			t.Run("accepts 10.1234", func(t *testing.T) {
				router := newTestRouter()
				user := createUser(t, start)

				requireOK(t, postWallet(router, user.Token, operation, "10.1234", "EUR"))

				want := map[string]balances{
					"deposit":  {EUR: "110.1234", USD: "0", JPY: "0"},
					"withdraw": {EUR: "89.8766", USD: "0", JPY: "0"},
				}[operation]
				wantType := map[string]string{"deposit": "DEPOSIT", "withdraw": "WITHDRAWAL"}[operation]

				assertBalances(t, user.ID, want)
				requireOneMovement(t, user.ID, wantType, "EUR", "10.1234")
			})
		})
	}
}

// AC6: under concurrent withdrawals the WITHDRAWAL rows match the accepted requests and the debit,
// and the balance never goes below zero
func TestWithdraw_ConcurrentMovementsMatchBalance(t *testing.T) {
	router := newTestRouter()
	user := createUser(t, balances{EUR: "100", USD: "0", JPY: "0"})

	const workers = 20

	recs := withdrawConcurrently(router, user.Token, "7", "EUR", workers)

	accepted := 0

	for i, rec := range recs {
		switch rec.Code {
		case http.StatusOK:
			accepted++
		case http.StatusBadRequest:
			requireError(t, rec, http.StatusBadRequest, response.ErrCodeInsufficientBalance, "Insufficient balance")
		default:
			t.Errorf("request %d: status = %d; body: %s", i, rec.Code, rec.Body.String())
		}
	}

	var finalBalance string
	if err := testDB.QueryRow(`SELECT balance_eur FROM wallets WHERE user_id = $1`, user.ID).Scan(&finalBalance); err != nil {
		t.Fatalf("failed to get balance: %v", err)
	}

	balance := decimal.RequireFromString(finalBalance)
	if balance.IsNegative() {
		t.Errorf("balance_eur = %s, want >= 0", balance)
	}

	movements := movementsOf(t, user.ID)

	sum := decimal.Zero
	for _, m := range movements {
		if m.Type != "WITHDRAWAL" || m.Currency != "EUR" {
			t.Errorf("unexpected movement %+v", m)
		}

		sum = sum.Add(m.Amount)
	}

	if len(movements) != accepted {
		t.Errorf("WITHDRAWAL rows = %d, want %d (one per 200 response)", len(movements), accepted)
	}

	debit := decimal.RequireFromString("100").Sub(balance)
	if !sum.Equal(debit) {
		t.Errorf("sum of WITHDRAWAL rows = %s, want the debit %s", sum, debit)
	}

	if accepted != 14 || !balance.Equal(decimal.RequireFromString("2")) {
		t.Errorf("accepted = %d, balance_eur = %s; want 14 and 2", accepted, balance)
	}
}

// Database Error Is Not Insufficient Balance, on the path production uses (service and route,
// through WithdrawWithMovement). A limited role makes the movement insert or the balance UPDATE fail:
// the withdrawal is a database error (500 at the route), never INSUFFICIENT_BALANCE, both when the
// balance covers the amount and when it does not, and it leaves no balance change and no movement.
func TestWithdraw_DatabaseErrorOnRealPath(t *testing.T) {
	roles := []struct {
		name      string
		grants    []string
		wantTable string
	}{
		{
			// Can lock and update the wallet, cannot insert the movement
			name:      "movement insert fails",
			grants:    []string{`GRANT SELECT, UPDATE ON wallets TO %s`},
			wantTable: "wallet_movements",
		},
		{
			// Can lock the wallet (UPDATE on one column is enough for FOR UPDATE) and insert the
			// movement, cannot update the balance: the inserted movement must be rolled back.
			// SELECT on wallet_movements is what INSERT … ON CONFLICT (user_id, idempotency_key) needs
			name: "balance update fails",
			grants: []string{
				`GRANT SELECT ON wallets TO %s`,
				`GRANT UPDATE (updated_at) ON wallets TO %s`,
				`GRANT SELECT, INSERT ON wallet_movements TO %s`,
			},
			wantTable: "wallets",
		},
	}

	amounts := []struct {
		name   string
		amount string
	}{
		{name: "amount covered by the balance", amount: "10"},
		{name: "amount above the balance", amount: "500"},
	}

	start := balances{EUR: "100", USD: "0", JPY: "0"}

	for _, role := range roles {
		t.Run(role.name, func(t *testing.T) {
			limitedDB := connectAsLimitedRole(t, role.grants...)
			service := NewService(NewPostgresRepository(limitedDB))

			for _, tt := range amounts {
				t.Run(tt.name, func(t *testing.T) {
					// Service
					user := createUser(t, start)

					_, err := service.Withdraw(context.Background(), user.ID, tt.amount, currency.EUR)
					requireDatabaseError(t, err, role.wantTable)
					assertBalances(t, user.ID, start)
					requireNoMovements(t, user.ID)

					// Route
					user = createUser(t, start)

					rec := postWallet(newTestRouterOn(limitedDB), user.Token, "withdraw", tt.amount, "EUR")

					requireError(t, rec, http.StatusInternalServerError, response.ErrCodeInternalServer, "Failed to process transaction")
					assertBalances(t, user.ID, start)
					requireNoMovements(t, user.ID)
				})
			}
		})
	}
}

// requireDatabaseError asserts that err is a permission error on the given table and not a domain error
func requireDatabaseError(t *testing.T, err error, table string) {
	t.Helper()

	if err == nil {
		t.Fatalf("error = nil, want permission denied for table %s", table)
	}

	if errors.Is(err, ErrInsufficientBalance) || errors.Is(err, ErrWalletNotFound) {
		t.Fatalf("error = %v, want a database error, not a domain error", err)
	}

	if !strings.Contains(err.Error(), "permission denied for table "+table) {
		t.Fatalf("error = %v, want permission denied for table %s", err, table)
	}
}

// An amount with more than 4 decimals or above the maximum gets a message that says why,
// not "must be positive"
func TestWalletAmountBounds_Message(t *testing.T) {
	const want = "Invalid amount: at most 4 decimals and at most 999999999999999.9999"

	for _, operation := range []string{"deposit", "withdraw"} {
		for _, amount := range []string{"10.00001", "1000000000000000"} {
			t.Run(operation+" "+amount, func(t *testing.T) {
				router := newTestRouter()
				user := createUser(t, balances{EUR: "100", USD: "0", JPY: "0"})

				rec := postWallet(router, user.Token, operation, amount, "EUR")

				requireError(t, rec, http.StatusBadRequest, response.ErrCodeBadRequest, want)
			})
		}
	}
}
