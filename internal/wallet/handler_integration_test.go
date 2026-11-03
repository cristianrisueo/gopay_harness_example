//go:build integration

package wallet

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cristianrisueo/gopay/internal/auth"
	"github.com/cristianrisueo/gopay/pkg/currency"
	"github.com/cristianrisueo/gopay/pkg/database"
	"github.com/cristianrisueo/gopay/pkg/idempotency"
	"github.com/cristianrisueo/gopay/pkg/response"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

const (
	postgresImage  = "postgres:16.3"
	requestTimeout = 30 * time.Second
)

var (
	testDB   *sql.DB
	testDSN  string
	testAuth = auth.NewAuthService(uuid.NewString(), 1)
)

// TestMain starts one PostgreSQL container per run and applies all migrations to it.
// The tests only ever connect to this container, never to DB_DSN or .env.
func TestMain(m *testing.M) {
	os.Exit(runIntegrationTests(m))
}

func runIntegrationTests(m *testing.M) int {
	gin.SetMode(gin.TestMode)
	ctx := context.Background()

	container, err := postgres.Run(ctx, postgresImage,
		postgres.WithDatabase("gopay"),
		postgres.BasicWaitStrategies(),
	)
	defer func() {
		if err := testcontainers.TerminateContainer(container); err != nil {
			fmt.Fprintf(os.Stderr, "failed to terminate postgres container: %v\n", err)
		}
	}()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to start postgres container (is Docker running?): %v\n", err)
		return 1
	}

	testDSN, err = container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to get postgres connection string: %v\n", err)
		return 1
	}

	testDB, err = database.New(testDSN)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to connect to postgres: %v\n", err)
		return 1
	}
	defer func() {
		if err := testDB.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "failed to close database: %v\n", err)
		}
	}()

	if err := applyMigrations(testDB); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 1
	}

	return m.Run()
}

// applyMigrations runs every up migration in order, one file per Exec and outside an
// explicit transaction, because CREATE INDEX CONCURRENTLY cannot run inside one
func applyMigrations(db *sql.DB) error {
	files, err := filepath.Glob(filepath.Join("..", "..", "migrate", "migrations", "*.up.sql"))
	if err != nil {
		return fmt.Errorf("failed to list migrations: %w", err)
	}

	if len(files) == 0 {
		return errors.New("no migrations found")
	}

	sort.Strings(files)

	for _, file := range files {
		content, err := os.ReadFile(file)
		if err != nil {
			return fmt.Errorf("failed to read migration %s: %w", file, err)
		}

		if _, err := db.Exec(string(content)); err != nil {
			return fmt.Errorf("failed to apply migration %s: %w", filepath.Base(file), err)
		}
	}

	return nil
}

// ==================== Helpers ====================

type testUser struct {
	ID    string
	Token string
}

type walletEnvelope struct {
	Success bool            `json:"success"`
	Data    *Wallet         `json:"data"`
	Error   *response.Error `json:"error"`
}

type balances struct {
	EUR, USD, JPY string
}

// newTestRouter wires the real wallet routes, auth middleware and repository on the test database
func newTestRouter() *gin.Engine {
	return newTestRouterOn(testDB)
}

// newTestRouterOn wires the real wallet routes, auth middleware and repository on the given pool
func newTestRouterOn(db *sql.DB) *gin.Engine {
	router := gin.New()
	RegisterRoutes(router.Group(""), NewService(NewPostgresRepository(db)), testAuth)

	return router
}

// connectAsLimitedRole creates a login role in the test container with only the given grants
// (each one a format string with %s for the role) and returns a pool connected as that role.
// It is used to make real database calls fail without changing production code.
func connectAsLimitedRole(t *testing.T, grants ...string) *sql.DB {
	t.Helper()

	role := "wallet_limited_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	password := uuid.NewString()

	if _, err := testDB.Exec(fmt.Sprintf(`CREATE ROLE %s LOGIN PASSWORD '%s'`, role, password)); err != nil {
		t.Fatalf("failed to create role: %v", err)
	}

	t.Cleanup(func() {
		if _, err := testDB.Exec(fmt.Sprintf(`DROP OWNED BY %s`, role)); err != nil {
			t.Errorf("failed to drop privileges of role %s: %v", role, err)
		}
		if _, err := testDB.Exec(fmt.Sprintf(`DROP ROLE %s`, role)); err != nil {
			t.Errorf("failed to drop role %s: %v", role, err)
		}
	})

	for _, grant := range grants {
		if _, err := testDB.Exec(fmt.Sprintf(grant, role)); err != nil {
			t.Fatalf("failed to grant %q: %v", grant, err)
		}
	}

	// Same container, same database, only the credentials change
	dsn, err := url.Parse(testDSN)
	if err != nil {
		t.Fatalf("failed to parse test DSN: %v", err)
	}
	dsn.User = url.UserPassword(role, password)

	db, err := database.New(dsn.String())
	if err != nil {
		t.Fatalf("failed to connect as role %s: %v", role, err)
	}

	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("failed to close database of role %s: %v", role, err)
		}
	})

	return db
}

// requirePermissionDenied asserts that err is a database permission error and not a domain error
func requirePermissionDenied(t *testing.T, err error) {
	t.Helper()

	if err == nil {
		t.Fatal("error = nil, want a permission denied error")
	}

	if errors.Is(err, ErrInsufficientBalance) || errors.Is(err, ErrWalletNotFound) {
		t.Fatalf("error = %v, want a database error, not a domain error", err)
	}

	if !strings.Contains(err.Error(), "permission denied for table wallets") {
		t.Fatalf("error = %v, want permission denied for table wallets", err)
	}
}

// insertUser inserts a user without a wallet and returns it with a valid JWT
func insertUser(t *testing.T) testUser {
	t.Helper()

	suffix := strings.ReplaceAll(uuid.NewString(), "-", "")
	username := "u_" + suffix
	email := suffix + "@example.com"

	var userID string

	err := testDB.QueryRow(
		`INSERT INTO users (username, email, password_hash) VALUES ($1, $2, $3) RETURNING id`,
		username, email, "not-a-real-hash",
	).Scan(&userID)
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	token, err := testAuth.GenerateToken(userID, email, username)
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	return testUser{ID: userID, Token: token}
}

// createUser inserts a user with a wallet holding the given balances. The wallet is written
// directly so tests can start from states the API cannot reach, like a negative balance.
func createUser(t *testing.T, start balances) testUser {
	t.Helper()

	user := insertUser(t)

	if _, err := testDB.Exec(
		`INSERT INTO wallets (user_id, balance_eur, balance_usd, balance_jpy) VALUES ($1, $2, $3, $4)`,
		user.ID, start.EUR, start.USD, start.JPY,
	); err != nil {
		t.Fatalf("failed to create wallet: %v", err)
	}

	return user
}

// postWallet sends POST /wallet/<operation> through the router.
// It does not use *testing.T so it can be called from goroutines.
func postWallet(router http.Handler, token, operation, amount, curr string) *httptest.ResponseRecorder {
	body := fmt.Sprintf(`{"amount":%q,"currency":%q}`, amount, curr)

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/wallet/"+operation, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set(idempotency.HeaderKey, uuid.NewString())

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	return rec
}

// withdrawConcurrently sends the same withdrawal from n goroutines released at the same time
func withdrawConcurrently(router http.Handler, token, amount, curr string, n int) []*httptest.ResponseRecorder {
	start := make(chan struct{})
	recs := make([]*httptest.ResponseRecorder, n)

	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)

		go func() {
			defer wg.Done()

			// Waits for the common signal so all requests leave at the same time
			<-start
			recs[i] = postWallet(router, token, "withdraw", amount, curr)
		}()
	}

	close(start)
	wg.Wait()

	return recs
}

func decodeWallet(t *testing.T, rec *httptest.ResponseRecorder) walletEnvelope {
	t.Helper()

	var envelope walletEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("failed to decode response %q: %v", rec.Body.String(), err)
	}

	return envelope
}

// requireOK asserts a 200 and returns the decoded wallet
func requireOK(t *testing.T, rec *httptest.ResponseRecorder) *Wallet {
	t.Helper()

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	envelope := decodeWallet(t, rec)
	if !envelope.Success || envelope.Data == nil {
		t.Fatalf("response is not a successful wallet: %s", rec.Body.String())
	}

	return envelope.Data
}

// requireError asserts the status code, the error code and the error message of a failed response.
// An empty wantMessage skips the message check.
func requireError(t *testing.T, rec *httptest.ResponseRecorder, wantStatus int, wantCode, wantMessage string) {
	t.Helper()

	if rec.Code != wantStatus {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, wantStatus, rec.Body.String())
	}

	envelope := decodeWallet(t, rec)
	if envelope.Success || envelope.Error == nil || envelope.Error.Code != wantCode {
		t.Fatalf("error = %+v, want code %s; body: %s", envelope.Error, wantCode, rec.Body.String())
	}

	if wantMessage != "" && envelope.Error.Message != wantMessage {
		t.Fatalf("error message = %q, want %q", envelope.Error.Message, wantMessage)
	}
}

// assertBalances compares the wallet stored in the database with the expected balances
func assertBalances(t *testing.T, userID string, want balances) {
	t.Helper()

	var eur, usd, jpy string
	err := testDB.QueryRow(
		`SELECT balance_eur, balance_usd, balance_jpy FROM wallets WHERE user_id = $1`,
		userID,
	).Scan(&eur, &usd, &jpy)
	if err != nil {
		t.Fatalf("failed to get balances: %v", err)
	}

	got := []string{eur, usd, jpy}
	wanted := []string{want.EUR, want.USD, want.JPY}

	for i, curr := range []string{"EUR", "USD", "JPY"} {
		if !decimal.RequireFromString(got[i]).Equal(decimal.RequireFromString(wanted[i])) {
			t.Errorf("balance %s of %s = %s, want %s", curr, userID, got[i], wanted[i])
		}
	}
}

// ==================== Tests ====================

// The incident reported by Finance: two simultaneous withdrawals of 70 from a balance of 100
// must not both succeed. One is accepted and the other gets the insufficient balance error.
func TestWithdraw_ConcurrentOverdraft(t *testing.T) {
	router := newTestRouter()

	// Repeats the race with fresh users so the two requests overlap at least once
	const rounds = 10

	for round := range rounds {
		user := createUser(t, balances{EUR: "100", USD: "0", JPY: "0"})

		recs := withdrawConcurrently(router, user.Token, "70", "EUR", 2)

		accepted, rejected := 0, 0

		for i, rec := range recs {
			switch rec.Code {
			case http.StatusOK:
				accepted++

				wallet := requireOK(t, rec)
				if !wallet.BalanceEUR.Equal(decimal.RequireFromString("30")) {
					t.Errorf("round %d request %d: balance_eur = %s, want 30", round, i, wallet.BalanceEUR)
				}
			case http.StatusBadRequest:
				rejected++

				requireError(t, rec, http.StatusBadRequest, response.ErrCodeInsufficientBalance, "Insufficient balance")
			default:
				t.Errorf("round %d request %d: status = %d; body: %s", round, i, rec.Code, rec.Body.String())
			}
		}

		if accepted != 1 || rejected != 1 {
			t.Errorf("round %d: accepted = %d, rejected = %d; want 1 and 1", round, accepted, rejected)
		}

		assertBalances(t, user.ID, balances{EUR: "30", USD: "0", JPY: "0"})
	}
}

// Many simultaneous withdrawals drain the balance exactly to zero and never below, in every currency
func TestWithdraw_ConcurrentManyRequests(t *testing.T) {
	tests := []struct {
		currency string
		start    balances
	}{
		{currency: "EUR", start: balances{EUR: "100", USD: "0", JPY: "0"}},
		{currency: "USD", start: balances{EUR: "0", USD: "100", JPY: "0"}},
		{currency: "JPY", start: balances{EUR: "0", USD: "0", JPY: "100"}},
	}

	for _, tt := range tests {
		t.Run(tt.currency, func(t *testing.T) {
			router := newTestRouter()
			user := createUser(t, tt.start)

			const workers = 20

			recs := withdrawConcurrently(router, user.Token, "10", tt.currency, workers)

			accepted, rejected := 0, 0

			for i, rec := range recs {
				switch rec.Code {
				case http.StatusOK:
					accepted++
				case http.StatusBadRequest:
					rejected++

					requireError(t, rec, http.StatusBadRequest, response.ErrCodeInsufficientBalance, "Insufficient balance")
				default:
					t.Errorf("request %d: status = %d; body: %s", i, rec.Code, rec.Body.String())
				}
			}

			if accepted != 10 || rejected != workers-10 {
				t.Errorf("accepted = %d, rejected = %d; want 10 and %d", accepted, rejected, workers-10)
			}

			assertBalances(t, user.ID, balances{EUR: "0", USD: "0", JPY: "0"})
		})
	}
}

// Withdrawing the whole balance is allowed and leaves it at zero
func TestWithdraw_WholeBalance(t *testing.T) {
	router := newTestRouter()
	user := createUser(t, balances{EUR: "50", USD: "0", JPY: "0"})

	wallet := requireOK(t, postWallet(router, user.Token, "withdraw", "50", "EUR"))
	if !wallet.BalanceEUR.IsZero() {
		t.Errorf("balance_eur = %s, want 0", wallet.BalanceEUR)
	}

	assertBalances(t, user.ID, balances{EUR: "0", USD: "0", JPY: "0"})
}

// A single withdrawal above the balance is rejected and moves no money
func TestWithdraw_InsufficientBalance(t *testing.T) {
	router := newTestRouter()
	user := createUser(t, balances{EUR: "50", USD: "0", JPY: "0"})

	rec := postWallet(router, user.Token, "withdraw", "60", "EUR")

	requireError(t, rec, http.StatusBadRequest, response.ErrCodeInsufficientBalance, "Insufficient balance")
	assertBalances(t, user.ID, balances{EUR: "50", USD: "0", JPY: "0"})
}

// A wallet that is already negative is left as it is: withdrawals are rejected and deposits still work
func TestWithdraw_AlreadyNegativeWallet(t *testing.T) {
	router := newTestRouter()
	user := createUser(t, balances{EUR: "-40", USD: "0", JPY: "0"})

	rec := postWallet(router, user.Token, "withdraw", "10", "EUR")

	requireError(t, rec, http.StatusBadRequest, response.ErrCodeInsufficientBalance, "Insufficient balance")
	assertBalances(t, user.ID, balances{EUR: "-40", USD: "0", JPY: "0"})

	wallet := requireOK(t, postWallet(router, user.Token, "deposit", "10", "EUR"))
	if !wallet.BalanceEUR.Equal(decimal.RequireFromString("-30")) {
		t.Errorf("balance_eur after deposit = %s, want -30", wallet.BalanceEUR)
	}

	assertBalances(t, user.ID, balances{EUR: "-30", USD: "0", JPY: "0"})
}

// A user without a wallet still gets 404
func TestWithdraw_WalletNotFound(t *testing.T) {
	router := newTestRouter()
	user := insertUser(t)

	rec := postWallet(router, user.Token, "withdraw", "10", "EUR")

	requireError(t, rec, http.StatusNotFound, response.ErrCodeNotFound, "Wallet not found")
}

// Invalid input is rejected with 400 and never changes the balance. A negative amount
// matters most: subtracting it in SQL would add money to the wallet.
func TestWithdraw_InvalidInput(t *testing.T) {
	tests := []struct {
		name        string
		amount      string
		currency    string
		wantMessage string
	}{
		{name: "zero amount", amount: "0", currency: "EUR", wantMessage: "Invalid amount: must be positive"},
		{name: "negative amount", amount: "-10", currency: "EUR", wantMessage: "Invalid amount: must be positive"},
		{name: "non numeric amount", amount: "abc", currency: "EUR", wantMessage: "Invalid amount: must be positive"},
		{name: "invalid currency", amount: "10", currency: "GBP"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := newTestRouter()
			user := createUser(t, balances{EUR: "100", USD: "0", JPY: "0"})

			rec := postWallet(router, user.Token, "withdraw", tt.amount, tt.currency)

			requireError(t, rec, http.StatusBadRequest, response.ErrCodeBadRequest, tt.wantMessage)
			assertBalances(t, user.ID, balances{EUR: "100", USD: "0", JPY: "0"})
		})
	}
}

// A failure in the withdrawal UPDATE is a 500 and moves no money, also when the amount is above
// the balance: a database error is never reported as an insufficient balance.
// The role can read wallets but not update them, so the UPDATE itself fails.
func TestWithdraw_UpdateFails(t *testing.T) {
	limitedDB := connectAsLimitedRole(t, `GRANT SELECT ON wallets TO %s`)

	tests := []struct {
		name   string
		amount string
	}{
		{name: "amount covered by the balance", amount: "10"},
		{name: "amount above the balance", amount: "500"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := newTestRouterOn(limitedDB)
			user := createUser(t, balances{EUR: "100", USD: "0", JPY: "0"})

			// Checks that the repository fails in the UPDATE, so the 500 below has that cause
			err := NewPostgresRepository(limitedDB).Withdraw(context.Background(), user.ID, currency.EUR, decimal.RequireFromString(tt.amount))
			requirePermissionDenied(t, err)

			rec := postWallet(router, user.Token, "withdraw", tt.amount, "EUR")

			requireError(t, rec, http.StatusInternalServerError, response.ErrCodeInternalServer, "Failed to process transaction")
			assertBalances(t, user.ID, balances{EUR: "100", USD: "0", JPY: "0"})
		})
	}
}

// When the UPDATE matches no rows and the lookup that follows fails, the answer is a 500,
// never 400 INSUFFICIENT_BALANCE. The role can run the withdrawal UPDATE (it may read and write
// the balance columns) but cannot read the other columns that GetByUserID selects.
func TestWithdraw_LookupFailsAfterNoRowsUpdated(t *testing.T) {
	limitedDB := connectAsLimitedRole(t,
		`GRANT SELECT (user_id, balance_eur, balance_usd, balance_jpy) ON wallets TO %s`,
		`GRANT UPDATE (balance_eur, balance_usd, balance_jpy, updated_at) ON wallets TO %s`,
	)
	repo := NewPostgresRepository(limitedDB)

	// Precondition: the role can run the withdrawal UPDATE, so a 0-row result is a real 0-row result
	covered := createUser(t, balances{EUR: "100", USD: "0", JPY: "0"})
	if err := repo.Withdraw(context.Background(), covered.ID, currency.EUR, decimal.RequireFromString("10")); err != nil {
		t.Fatalf("withdrawal UPDATE with the limited role failed: %v", err)
	}
	assertBalances(t, covered.ID, balances{EUR: "90", USD: "0", JPY: "0"})

	// Precondition: the role cannot run the lookup
	_, err := repo.GetByUserID(context.Background(), covered.ID)
	requirePermissionDenied(t, err)

	// The amount is above the balance, so the UPDATE matches no rows and the lookup runs and fails
	user := createUser(t, balances{EUR: "50", USD: "0", JPY: "0"})

	err = repo.Withdraw(context.Background(), user.ID, currency.EUR, decimal.RequireFromString("60"))
	requirePermissionDenied(t, err)

	rec := postWallet(newTestRouterOn(limitedDB), user.Token, "withdraw", "60", "EUR")

	requireError(t, rec, http.StatusInternalServerError, response.ErrCodeInternalServer, "Failed to process transaction")
	assertBalances(t, user.ID, balances{EUR: "50", USD: "0", JPY: "0"})
}

// The repository rejects non-positive amounts before touching the database. Subtracting a
// negative amount would add money, so this barrier must hold even if the service check is bypassed.
func TestRepositoryWithdraw_NonPositiveAmount(t *testing.T) {
	repo := NewPostgresRepository(testDB)

	for _, amount := range []string{"0", "-10"} {
		t.Run(amount, func(t *testing.T) {
			user := createUser(t, balances{EUR: "100", USD: "0", JPY: "0"})

			err := repo.Withdraw(context.Background(), user.ID, currency.EUR, decimal.RequireFromString(amount))
			if !errors.Is(err, ErrInvalidAmount) {
				t.Errorf("error = %v, want %v", err, ErrInvalidAmount)
			}

			assertBalances(t, user.ID, balances{EUR: "100", USD: "0", JPY: "0"})
		})
	}
}
