//go:build integration

package wallet

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/cristianrisueo/gopay/pkg/idempotency"
	"github.com/cristianrisueo/gopay/pkg/response"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// invalidKeyMessage is the 400 message for a missing or malformed Idempotency-Key header
const invalidKeyMessage = "Missing or invalid Idempotency-Key header: must be a UUID"

// postWalletWithKey sends POST /wallet/<operation> with the given Idempotency-Key header.
// An empty key sends no header. It does not use *testing.T so it can be called from goroutines.
func postWalletWithKey(router http.Handler, token, operation, amount, curr, key string) *httptest.ResponseRecorder {
	body := fmt.Sprintf(`{"amount":%q,"currency":%q}`, amount, curr)

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/wallet/"+operation, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	if key != "" {
		req.Header.Set(idempotency.HeaderKey, key)
	}

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	return rec
}

// postConcurrentlyWithKey sends the same request with the same key from n goroutines released at the same time
func postConcurrentlyWithKey(router http.Handler, token, operation, amount, curr, key string, n int) []*httptest.ResponseRecorder {
	start := make(chan struct{})
	recs := make([]*httptest.ResponseRecorder, n)

	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)

		go func() {
			defer wg.Done()

			// Waits for the common signal so all requests leave at the same time
			<-start
			recs[i] = postWalletWithKey(router, token, operation, amount, curr, key)
		}()
	}

	close(start)
	wg.Wait()

	return recs
}

// isReplayed reports whether the response carries Idempotent-Replayed: true
func isReplayed(rec *httptest.ResponseRecorder) bool {
	return rec.Header().Get(idempotency.HeaderReplayed) == "true"
}

// requireReplayed asserts whether the response is marked as a replay
func requireReplayed(t *testing.T, rec *httptest.ResponseRecorder, want bool) {
	t.Helper()

	if got := isReplayed(rec); got != want {
		t.Fatalf("%s header = %q, want replayed = %v; body: %s",
			idempotency.HeaderReplayed, rec.Header().Get(idempotency.HeaderReplayed), want, rec.Body.String())
	}
}

// requireMovementWithKey asserts that the user has exactly one movement stored with the key
// (compared in lowercase, as it is stored) and that it has the given fields
func requireMovementWithKey(t *testing.T, userID, key, wantType, wantCurrency, wantAmount string) {
	t.Helper()

	var matches []movement

	for _, m := range movementsOf(t, userID) {
		if m.IdempotencyKey == strings.ToLower(key) {
			matches = append(matches, m)
		}
	}

	if len(matches) != 1 {
		t.Fatalf("movements of %s with key %s = %d, want 1: %+v", userID, key, len(matches), movementsOf(t, userID))
	}

	m := matches[0]

	if m.Type != wantType || m.Currency != wantCurrency || !m.Amount.Equal(decimal.RequireFromString(wantAmount)) {
		t.Fatalf("movement with key %s = %+v, want %s %s %s", key, m, wantType, wantCurrency, wantAmount)
	}
}

// requireMovementCount asserts how many movements the user has
func requireMovementCount(t *testing.T, userID string, want int) {
	t.Helper()

	if movements := movementsOf(t, userID); len(movements) != want {
		t.Fatalf("movements of %s = %d, want %d: %+v", userID, len(movements), want, movements)
	}
}

// operationCase is a deposit or a withdrawal of 10 EUR from a balance of 100, and what it leaves
type operationCase struct {
	operation    string
	movementType string
	after        balances
}

var operationCases = []operationCase{
	{operation: "deposit", movementType: "DEPOSIT", after: balances{EUR: "110", USD: "0", JPY: "0"}},
	{operation: "withdraw", movementType: "WITHDRAWAL", after: balances{EUR: "90", USD: "0", JPY: "0"}},
}

// AC1: a missing, malformed or non-canonical key is a 400 on both routes, moves no money and records nothing
func TestWalletIdempotency_InvalidKey(t *testing.T) {
	keys := []struct {
		name string
		key  string
	}{
		{name: "missing", key: ""},
		{name: "not a uuid", key: "not-a-uuid"},
		{name: "no hyphens", key: "6f1c2a9e3b4d4e5f8a7b9c0d1e2f3a4b"},
		{name: "braces", key: "{6f1c2a9e-3b4d-4e5f-8a7b-9c0d1e2f3a4b}"},
		{name: "urn prefix", key: "urn:uuid:6f1c2a9e-3b4d-4e5f-8a7b-9c0d1e2f3a4b"},
		{name: "35 characters", key: "6f1c2a9e-3b4d-4e5f-8a7b-9c0d1e2f3a4"},
		{name: "non hex character", key: "6f1c2a9e-3b4d-4e5f-8a7b-9c0d1e2f3a4g"},
	}

	start := balances{EUR: "100", USD: "0", JPY: "0"}

	for _, op := range operationCases {
		for _, tt := range keys {
			t.Run(op.operation+" "+tt.name, func(t *testing.T) {
				router := newTestRouter()
				user := createUser(t, start)

				rec := postWalletWithKey(router, user.Token, op.operation, "10", "EUR", tt.key)

				requireError(t, rec, http.StatusBadRequest, response.ErrCodeBadRequest, invalidKeyMessage)
				assertBalances(t, user.ID, start)
				requireNoMovements(t, user.ID)
			})
		}
	}
}

// AC2: a sequential retry with the same key is a 200 replay; money moves once and one row exists
func TestWalletIdempotency_SequentialRetry(t *testing.T) {
	for _, op := range operationCases {
		t.Run(op.operation, func(t *testing.T) {
			router := newTestRouter()
			user := createUser(t, balances{EUR: "100", USD: "0", JPY: "0"})
			key := uuid.NewString()

			first := postWalletWithKey(router, user.Token, op.operation, "10", "EUR", key)
			requireOK(t, first)
			requireReplayed(t, first, false)

			retry := postWalletWithKey(router, user.Token, op.operation, "10", "EUR", key)
			wallet := requireOK(t, retry)
			requireReplayed(t, retry, true)

			// The replay returns the current wallet, the same one the first request left
			if !wallet.BalanceEUR.Equal(decimal.RequireFromString(op.after.EUR)) {
				t.Errorf("replayed balance_eur = %s, want %s", wallet.BalanceEUR, op.after.EUR)
			}

			assertBalances(t, user.ID, op.after)
			requireMovementCount(t, user.ID, 1)
			requireMovementWithKey(t, user.ID, key, op.movementType, "EUR", "10")
		})
	}
}

// AC3: concurrent identical requests with one key all get 200, exactly one is not a replay,
// money moves once and one row exists
func TestWalletIdempotency_ConcurrentSameKey(t *testing.T) {
	const workers = 20

	for _, op := range operationCases {
		t.Run(op.operation, func(t *testing.T) {
			router := newTestRouter()
			user := createUser(t, balances{EUR: "100", USD: "0", JPY: "0"})
			key := uuid.NewString()

			recs := postConcurrentlyWithKey(router, user.Token, op.operation, "10", "EUR", key, workers)

			applied := 0

			for i, rec := range recs {
				if rec.Code != http.StatusOK {
					t.Errorf("request %d: status = %d, want 200; body: %s", i, rec.Code, rec.Body.String())
					continue
				}

				if !isReplayed(rec) {
					applied++
				}
			}

			if applied != 1 {
				t.Errorf("responses without %s = %d, want 1", idempotency.HeaderReplayed, applied)
			}

			assertBalances(t, user.ID, op.after)
			requireMovementCount(t, user.ID, 1)
			requireMovementWithKey(t, user.ID, key, op.movementType, "EUR", "10")
		})
	}
}

// AC4: the same key with a different amount, currency or type is a 422 and moves no money
func TestWalletIdempotency_DifferentPayload(t *testing.T) {
	router := newTestRouter()
	user := createUser(t, balances{EUR: "100", USD: "100", JPY: "0"})
	key := uuid.NewString()

	requireOK(t, postWalletWithKey(router, user.Token, "deposit", "10", "EUR", key))

	after := balances{EUR: "110", USD: "100", JPY: "0"}
	assertBalances(t, user.ID, after)

	tests := []struct {
		name      string
		operation string
		amount    string
		currency  string
	}{
		{name: "different amount", operation: "deposit", amount: "11", currency: "EUR"},
		{name: "different currency", operation: "deposit", amount: "10", currency: "USD"},
		{name: "different type", operation: "withdraw", amount: "10", currency: "EUR"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := postWalletWithKey(router, user.Token, tt.operation, tt.amount, tt.currency, key)

			requireError(t, rec, http.StatusUnprocessableEntity, response.ErrCodeIdempotencyKeyReused, "")
			assertBalances(t, user.ID, after)
			requireMovementCount(t, user.ID, 1)
			requireMovementWithKey(t, user.ID, key, "DEPOSIT", "EUR", "10")
		})
	}
}

// AC5: amounts are compared as decimals, so "10" then "10.0000" with the same key is a replay
func TestWalletIdempotency_AmountNormalization(t *testing.T) {
	router := newTestRouter()
	user := createUser(t, balances{EUR: "100", USD: "0", JPY: "0"})
	key := uuid.NewString()

	first := postWalletWithKey(router, user.Token, "deposit", "10", "EUR", key)
	requireOK(t, first)
	requireReplayed(t, first, false)

	retry := postWalletWithKey(router, user.Token, "deposit", "10.0000", "EUR", key)
	requireOK(t, retry)
	requireReplayed(t, retry, true)

	assertBalances(t, user.ID, balances{EUR: "110", USD: "0", JPY: "0"})
	requireMovementCount(t, user.ID, 1)
	requireMovementWithKey(t, user.ID, key, "DEPOSIT", "EUR", "10")
}

// AC6: keys that differ only in case are the same key; it is stored in lowercase
func TestWalletIdempotency_KeyCaseInsensitive(t *testing.T) {
	router := newTestRouter()
	user := createUser(t, balances{EUR: "100", USD: "0", JPY: "0"})
	key := strings.ToLower(uuid.NewString())

	first := postWalletWithKey(router, user.Token, "deposit", "10", "EUR", key)
	requireOK(t, first)
	requireReplayed(t, first, false)

	retry := postWalletWithKey(router, user.Token, "deposit", "10", "EUR", strings.ToUpper(key))
	requireOK(t, retry)
	requireReplayed(t, retry, true)

	assertBalances(t, user.ID, balances{EUR: "110", USD: "0", JPY: "0"})
	requireMovementCount(t, user.ID, 1)
	requireMovementWithKey(t, user.ID, key, "DEPOSIT", "EUR", "10")
}

// AC7: a withdrawal that fails with INSUFFICIENT_BALANCE does not register its key, so the retry
// with the same key after a deposit runs as a new withdrawal
func TestWalletIdempotency_RetryAfterInsufficientBalance(t *testing.T) {
	router := newTestRouter()
	user := createUser(t, balances{EUR: "50", USD: "0", JPY: "0"})
	key := uuid.NewString()

	rec := postWalletWithKey(router, user.Token, "withdraw", "60", "EUR", key)
	requireError(t, rec, http.StatusBadRequest, response.ErrCodeInsufficientBalance, "Insufficient balance")
	assertBalances(t, user.ID, balances{EUR: "50", USD: "0", JPY: "0"})
	requireNoMovements(t, user.ID)

	requireOK(t, postWalletWithKey(router, user.Token, "deposit", "20", "EUR", uuid.NewString()))

	retry := postWalletWithKey(router, user.Token, "withdraw", "60", "EUR", key)
	requireOK(t, retry)
	requireReplayed(t, retry, false)

	assertBalances(t, user.ID, balances{EUR: "10", USD: "0", JPY: "0"})
	requireMovementCount(t, user.ID, 2)
	requireMovementWithKey(t, user.ID, key, "WITHDRAWAL", "EUR", "60")
}

// AC8: the key is scoped per user, and the wallet and transfer key spaces are independent
func TestWalletIdempotency_KeyScope(t *testing.T) {
	router := newTestRouter()

	t.Run("two users, same key", func(t *testing.T) {
		alice := createUser(t, balances{EUR: "100", USD: "0", JPY: "0"})
		bob := createUser(t, balances{EUR: "100", USD: "0", JPY: "0"})
		key := uuid.NewString()

		for _, user := range []testUser{alice, bob} {
			rec := postWalletWithKey(router, user.Token, "deposit", "10", "EUR", key)
			requireOK(t, rec)
			requireReplayed(t, rec, false)

			assertBalances(t, user.ID, balances{EUR: "110", USD: "0", JPY: "0"})
			requireMovementWithKey(t, user.ID, key, "DEPOSIT", "EUR", "10")
		}
	})

	t.Run("key used by a transfer", func(t *testing.T) {
		sender := createUser(t, balances{EUR: "100", USD: "0", JPY: "0"})
		receiver := createUser(t, balances{EUR: "0", USD: "0", JPY: "0"})
		key := uuid.NewString()

		// A transfer row of the sender with the key, written directly: importing internal/transfer
		// from package wallet would be an import cycle
		if _, err := testDB.Exec(
			`INSERT INTO transfers (sender_id, receiver_id, amount, currency, idempotency_key) VALUES ($1, $2, $3, $4, $5)`,
			sender.ID, receiver.ID, "5", "EUR", key,
		); err != nil {
			t.Fatalf("failed to insert transfer: %v", err)
		}

		rec := postWalletWithKey(router, sender.Token, "deposit", "10", "EUR", key)
		requireOK(t, rec)
		requireReplayed(t, rec, false)

		assertBalances(t, sender.ID, balances{EUR: "110", USD: "0", JPY: "0"})
		requireMovementWithKey(t, sender.ID, key, "DEPOSIT", "EUR", "10")
	})
}

// AC9: the replay is resolved before the balance check, so retrying a withdrawal that emptied
// the balance is a replay, not INSUFFICIENT_BALANCE
func TestWalletIdempotency_ReplayAfterBalanceSpent(t *testing.T) {
	router := newTestRouter()
	user := createUser(t, balances{EUR: "50", USD: "0", JPY: "0"})
	key := uuid.NewString()

	first := postWalletWithKey(router, user.Token, "withdraw", "50", "EUR", key)
	requireOK(t, first)
	requireReplayed(t, first, false)
	assertBalances(t, user.ID, balances{EUR: "0", USD: "0", JPY: "0"})

	retry := postWalletWithKey(router, user.Token, "withdraw", "50", "EUR", key)
	wallet := requireOK(t, retry)
	requireReplayed(t, retry, true)

	if !wallet.BalanceEUR.IsZero() {
		t.Errorf("replayed balance_eur = %s, want 0", wallet.BalanceEUR)
	}

	assertBalances(t, user.ID, balances{EUR: "0", USD: "0", JPY: "0"})
	requireMovementCount(t, user.ID, 1)
	requireMovementWithKey(t, user.ID, key, "WITHDRAWAL", "EUR", "50")
}
