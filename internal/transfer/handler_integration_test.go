//go:build integration

package transfer

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cristianrisueo/gopay/internal/auth"
	"github.com/cristianrisueo/gopay/internal/wallet"
	"github.com/cristianrisueo/gopay/pkg/currency"
	"github.com/cristianrisueo/gopay/pkg/database"
	"github.com/cristianrisueo/gopay/pkg/idempotency"
	"github.com/cristianrisueo/gopay/pkg/response"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

const (
	postgresImage  = "postgres:16.3"
	requestTimeout = 30 * time.Second
	replayLogMsg   = "Transfer idempotent replay"
	mismatchLogMsg = "Idempotency key reused with different payload"
)

var (
	testDB   *sql.DB
	testDSN  string
	testAuth = auth.NewAuthService(uuid.NewString(), 1)
)

// TestMain starts one PostgreSQL container per run and applies all migrations to it
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

type transferEnvelope struct {
	Success bool              `json:"success"`
	Data    *TransferResponse `json:"data"`
	Error   *response.Error   `json:"error"`
}

// newTestRouter wires the real transfer routes, auth middleware and repositories on the given pool
func newTestRouter(db *sql.DB) (*gin.Engine, *observer.ObservedLogs) {
	core, logs := observer.New(zap.InfoLevel)

	service := NewService(NewPostgresRepository(db), wallet.NewPostgresRepository(db), zap.New(core))

	router := gin.New()
	RegisterRoutes(router.Group(""), service, testAuth)

	return router, logs
}

// createUser inserts a user with a wallet holding the given EUR balance and returns it with a valid JWT
func createUser(t *testing.T, balanceEUR string) testUser {
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

	if _, err := testDB.Exec(
		`INSERT INTO wallets (user_id, balance_eur) VALUES ($1, $2)`,
		userID, balanceEUR,
	); err != nil {
		t.Fatalf("failed to create wallet: %v", err)
	}

	token, err := testAuth.GenerateToken(userID, email, username)
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	return testUser{ID: userID, Token: token}
}

// postTransfer sends POST /transfer through the router. An empty key omits the Idempotency-Key header.
// It does not use *testing.T so it can be called from goroutines.
func postTransfer(router http.Handler, token, key, receiverID, amount, curr string) *httptest.ResponseRecorder {
	body := fmt.Sprintf(`{"receiver_id":%q,"amount":%q,"currency":%q}`, receiverID, amount, curr)

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/transfer", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	if key != "" {
		req.Header.Set(idempotency.HeaderKey, key)
	}

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	return rec
}

func decodeTransfer(t *testing.T, rec *httptest.ResponseRecorder) transferEnvelope {
	t.Helper()

	var envelope transferEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("failed to decode response %q: %v", rec.Body.String(), err)
	}

	return envelope
}

// requireCreated asserts a 201 and returns the decoded transfer
func requireCreated(t *testing.T, rec *httptest.ResponseRecorder, wantReplayed bool) *TransferResponse {
	t.Helper()

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	gotReplayed := rec.Header().Get(idempotency.HeaderReplayed) == "true"
	if gotReplayed != wantReplayed {
		t.Fatalf("%s header = %q, want replayed=%v", idempotency.HeaderReplayed, rec.Header().Get(idempotency.HeaderReplayed), wantReplayed)
	}

	envelope := decodeTransfer(t, rec)
	if envelope.Data == nil {
		t.Fatalf("response has no data: %s", rec.Body.String())
	}

	return envelope.Data
}

// requireError asserts the status code and the error code of a failed response
func requireError(t *testing.T, rec *httptest.ResponseRecorder, wantStatus int, wantCode string) {
	t.Helper()

	if rec.Code != wantStatus {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, wantStatus, rec.Body.String())
	}

	envelope := decodeTransfer(t, rec)
	if envelope.Error == nil || envelope.Error.Code != wantCode {
		t.Fatalf("error code = %+v, want %s; body: %s", envelope.Error, wantCode, rec.Body.String())
	}

	if rec.Header().Get(idempotency.HeaderReplayed) != "" {
		t.Fatalf("error response must not carry %s header", idempotency.HeaderReplayed)
	}
}

func countTransfersFrom(t *testing.T, senderID string) int {
	t.Helper()

	var count int
	if err := testDB.QueryRow(`SELECT COUNT(*) FROM transfers WHERE sender_id = $1`, senderID).Scan(&count); err != nil {
		t.Fatalf("failed to count transfers: %v", err)
	}

	return count
}

func balanceEUR(t *testing.T, userID string) decimal.Decimal {
	t.Helper()

	var balance string
	if err := testDB.QueryRow(`SELECT balance_eur FROM wallets WHERE user_id = $1`, userID).Scan(&balance); err != nil {
		t.Fatalf("failed to get balance: %v", err)
	}

	value, err := decimal.NewFromString(balance)
	if err != nil {
		t.Fatalf("failed to parse balance %q: %v", balance, err)
	}

	return value
}

func assertBalanceEUR(t *testing.T, userID, want string) {
	t.Helper()

	if got := balanceEUR(t, userID); !got.Equal(decimal.RequireFromString(want)) {
		t.Errorf("balance of %s = %s, want %s", userID, got, want)
	}
}

func assertTransferCount(t *testing.T, senderID string, want int) {
	t.Helper()

	if got := countTransfersFrom(t, senderID); got != want {
		t.Errorf("transfers created by sender = %d, want %d", got, want)
	}
}

func assertLogged(t *testing.T, logs *observer.ObservedLogs, msg, userID, key, transferID string, wantCount int) {
	t.Helper()

	entries := logs.FilterMessage(msg).All()
	if len(entries) != wantCount {
		t.Fatalf("log entries %q = %d, want %d", msg, len(entries), wantCount)
	}

	for _, entry := range entries {
		fields := entry.ContextMap()
		if fields["user_id"] != userID || fields["idempotency_key"] != key || fields["transfer_id"] != transferID {
			t.Errorf("log %q fields = %v, want user_id=%s idempotency_key=%s transfer_id=%s", msg, fields, userID, key, transferID)
		}
	}
}

// ==================== Tests ====================

// Criterion 1: same key twice in a row moves money once and the second answer is a replay
func TestTransferIdempotency_SequentialRetry(t *testing.T) {
	router, logs := newTestRouter(testDB)
	sender := createUser(t, "100")
	receiver := createUser(t, "0")
	key := uuid.NewString()

	original := requireCreated(t, postTransfer(router, sender.Token, key, receiver.ID, "10", "EUR"), false)

	// Moves the sender balance so the replay must report the current one, not the one after the transfer
	walletService := wallet.NewService(wallet.NewPostgresRepository(testDB))
	if _, err := walletService.Deposit(context.Background(), sender.ID, "50", currency.EUR); err != nil {
		t.Fatalf("failed to deposit: %v", err)
	}

	replay := requireCreated(t, postTransfer(router, sender.Token, key, receiver.ID, "10", "EUR"), true)

	if replay.TransferID != original.TransferID {
		t.Errorf("replay transfer_id = %s, want %s", replay.TransferID, original.TransferID)
	}
	if !replay.Amount.Equal(original.Amount) || replay.ReceiverID != original.ReceiverID ||
		replay.Currency != original.Currency || !replay.CreatedAt.Equal(original.CreatedAt) {
		t.Errorf("replay = %+v, want same data as original %+v", replay, original)
	}
	if !replay.SenderBalance.Equal(decimal.RequireFromString("140")) {
		t.Errorf("replay sender_balance = %s, want current balance 140", replay.SenderBalance)
	}

	assertTransferCount(t, sender.ID, 1)
	assertBalanceEUR(t, sender.ID, "140")
	assertBalanceEUR(t, receiver.ID, "10")
	assertLogged(t, logs, replayLogMsg, sender.ID, key, original.TransferID, 1)
}

// Criterion 2: 20 simultaneous requests with the same key move money once
func TestTransferIdempotency_ConcurrentSameKey(t *testing.T) {
	router, _ := newTestRouter(testDB)
	sender := createUser(t, "1000")
	receiver := createUser(t, "0")
	key := uuid.NewString()

	const workers = 20

	start := make(chan struct{})
	recs := make([]*httptest.ResponseRecorder, workers)

	var wg sync.WaitGroup
	for i := range workers {
		wg.Add(1)

		go func() {
			defer wg.Done()

			// Waits for the common signal so all requests leave at the same time
			<-start
			recs[i] = postTransfer(router, sender.Token, key, receiver.ID, "10", "EUR")
		}()
	}

	close(start)
	wg.Wait()

	assertTransferCount(t, sender.ID, 1)
	assertBalanceEUR(t, sender.ID, "990")
	assertBalanceEUR(t, receiver.ID, "10")

	originals, replays := 0, 0
	transferIDs := make(map[string]int)

	for i, rec := range recs {
		if rec.Code != http.StatusCreated {
			t.Errorf("request %d: status = %d, want %d; body: %s", i, rec.Code, http.StatusCreated, rec.Body.String())
			continue
		}

		if rec.Header().Get(idempotency.HeaderReplayed) == "true" {
			replays++
		} else {
			originals++
		}

		transferIDs[decodeTransfer(t, rec).Data.TransferID]++
	}

	if originals != 1 || replays != workers-1 {
		t.Errorf("originals = %d, replays = %d; want 1 and %d", originals, replays, workers-1)
	}
	if len(transferIDs) != 1 {
		t.Errorf("distinct transfer ids in responses = %d, want 1 (%v)", len(transferIDs), transferIDs)
	}
}

// Criterion 3: same key with a different amount, receiver or currency is rejected with 422
func TestTransferIdempotency_DifferentPayload(t *testing.T) {
	router, logs := newTestRouter(testDB)
	sender := createUser(t, "100")
	receiver := createUser(t, "0")
	otherReceiver := createUser(t, "0")
	key := uuid.NewString()

	original := requireCreated(t, postTransfer(router, sender.Token, key, receiver.ID, "10", "EUR"), false)

	tests := []struct {
		name       string
		receiverID string
		amount     string
		currency   string
	}{
		{name: "other amount", receiverID: receiver.ID, amount: "20", currency: "EUR"},
		{name: "other receiver", receiverID: otherReceiver.ID, amount: "10", currency: "EUR"},
		{name: "other currency", receiverID: receiver.ID, amount: "10", currency: "USD"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := postTransfer(router, sender.Token, key, tt.receiverID, tt.amount, tt.currency)
			requireError(t, rec, http.StatusUnprocessableEntity, response.ErrCodeIdempotencyKeyReused)
		})
	}

	assertTransferCount(t, sender.ID, 1)
	assertBalanceEUR(t, sender.ID, "90")
	assertBalanceEUR(t, receiver.ID, "10")
	assertBalanceEUR(t, otherReceiver.ID, "0")
	assertLogged(t, logs, mismatchLogMsg, sender.ID, key, original.TransferID, len(tests))
}

// A used key towards a receiver that does not exist answers 404 and moves nothing
func TestTransferIdempotency_UsedKeyUnknownReceiver(t *testing.T) {
	router, _ := newTestRouter(testDB)
	sender := createUser(t, "100")
	receiver := createUser(t, "0")
	key := uuid.NewString()

	requireCreated(t, postTransfer(router, sender.Token, key, receiver.ID, "10", "EUR"), false)

	rec := postTransfer(router, sender.Token, key, uuid.NewString(), "10", "EUR")
	requireError(t, rec, http.StatusNotFound, response.ErrCodeNotFound)

	assertTransferCount(t, sender.ID, 1)
	assertBalanceEUR(t, sender.ID, "90")
}

// Criterion 4: two senders can use the same key independently
func TestTransferIdempotency_SameKeyDifferentSenders(t *testing.T) {
	router, _ := newTestRouter(testDB)
	senderA := createUser(t, "100")
	senderB := createUser(t, "100")
	receiver := createUser(t, "0")
	key := uuid.NewString()

	transferA := requireCreated(t, postTransfer(router, senderA.Token, key, receiver.ID, "10", "EUR"), false)
	transferB := requireCreated(t, postTransfer(router, senderB.Token, key, receiver.ID, "10", "EUR"), false)

	if transferA.TransferID == transferB.TransferID {
		t.Errorf("both senders got transfer %s, want two different transfers", transferA.TransferID)
	}

	assertTransferCount(t, senderA.ID, 1)
	assertTransferCount(t, senderB.ID, 1)
	assertBalanceEUR(t, senderA.ID, "90")
	assertBalanceEUR(t, senderB.ID, "90")
	assertBalanceEUR(t, receiver.ID, "20")
}

// Criterion 5: a missing or non canonical UUID key is rejected with 400
func TestTransferIdempotency_InvalidKey(t *testing.T) {
	router, _ := newTestRouter(testDB)
	sender := createUser(t, "100")
	receiver := createUser(t, "0")
	key := uuid.NewString()

	tests := []struct {
		name string
		key  string
	}{
		{name: "missing", key: ""},
		{name: "not a uuid", key: "not-a-uuid"},
		{name: "without hyphens", key: strings.ReplaceAll(key, "-", "")},
		{name: "with braces", key: "{" + key + "}"},
		{name: "with urn prefix", key: "urn:uuid:" + key},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := postTransfer(router, sender.Token, tt.key, receiver.ID, "10", "EUR")
			requireError(t, rec, http.StatusBadRequest, response.ErrCodeBadRequest)
		})
	}

	assertTransferCount(t, sender.ID, 0)
	assertBalanceEUR(t, sender.ID, "100")
	assertBalanceEUR(t, receiver.ID, "0")
}

// Criterion 6: a failed first attempt does not register the key, so a later retry executes
func TestTransferIdempotency_RetryAfterInsufficientBalance(t *testing.T) {
	router, _ := newTestRouter(testDB)
	sender := createUser(t, "5")
	receiver := createUser(t, "0")
	key := uuid.NewString()

	rec := postTransfer(router, sender.Token, key, receiver.ID, "10", "EUR")
	requireError(t, rec, http.StatusBadRequest, response.ErrCodeInsufficientBalance)
	assertTransferCount(t, sender.ID, 0)

	walletService := wallet.NewService(wallet.NewPostgresRepository(testDB))
	if _, err := walletService.Deposit(context.Background(), sender.ID, "10", currency.EUR); err != nil {
		t.Fatalf("failed to deposit: %v", err)
	}

	requireCreated(t, postTransfer(router, sender.Token, key, receiver.ID, "10", "EUR"), false)

	assertTransferCount(t, sender.ID, 1)
	assertBalanceEUR(t, sender.ID, "5")
	assertBalanceEUR(t, receiver.ID, "10")
}

// Criterion 7: "10", "10.00" and "10.0000" with the same key are the same request
func TestTransferIdempotency_AmountNormalization(t *testing.T) {
	router, _ := newTestRouter(testDB)
	sender := createUser(t, "100")
	receiver := createUser(t, "0")
	key := uuid.NewString()

	original := requireCreated(t, postTransfer(router, sender.Token, key, receiver.ID, "10", "EUR"), false)

	for _, amount := range []string{"10.00", "10.0000"} {
		replay := requireCreated(t, postTransfer(router, sender.Token, key, receiver.ID, amount, "EUR"), true)
		if replay.TransferID != original.TransferID {
			t.Errorf("amount %s: transfer_id = %s, want %s", amount, replay.TransferID, original.TransferID)
		}
	}

	assertTransferCount(t, sender.ID, 1)
	assertBalanceEUR(t, sender.ID, "90")
}

// Criterion 8: different keys with the same receiver, amount and currency are two transfers
func TestTransferIdempotency_DistinctKeysSamePayload(t *testing.T) {
	router, _ := newTestRouter(testDB)
	sender := createUser(t, "100")
	receiver := createUser(t, "0")

	first := requireCreated(t, postTransfer(router, sender.Token, uuid.NewString(), receiver.ID, "10", "EUR"), false)
	second := requireCreated(t, postTransfer(router, sender.Token, uuid.NewString(), receiver.ID, "10", "EUR"), false)

	if first.TransferID == second.TransferID {
		t.Errorf("both requests got transfer %s, want two different transfers", first.TransferID)
	}

	assertTransferCount(t, sender.ID, 2)
	assertBalanceEUR(t, sender.ID, "80")
	assertBalanceEUR(t, receiver.ID, "20")
}

// The key is case insensitive: it is stored normalized to lowercase
func TestTransferIdempotency_KeyCaseInsensitive(t *testing.T) {
	router, _ := newTestRouter(testDB)
	sender := createUser(t, "100")
	receiver := createUser(t, "0")
	key := uuid.NewString()

	original := requireCreated(t, postTransfer(router, sender.Token, key, receiver.ID, "10", "EUR"), false)
	replay := requireCreated(t, postTransfer(router, sender.Token, strings.ToUpper(key), receiver.ID, "10", "EUR"), true)

	if replay.TransferID != original.TransferID {
		t.Errorf("replay transfer_id = %s, want %s", replay.TransferID, original.TransferID)
	}

	assertTransferCount(t, sender.ID, 1)
}

// The guarantee lives in the transfers table: a new process (new router and pool) still replays.
// This adds no new guarantee; criterion 9 holds by design because there is no purgeable store.
func TestTransferIdempotency_SurvivesRestart(t *testing.T) {
	router, _ := newTestRouter(testDB)
	sender := createUser(t, "100")
	receiver := createUser(t, "0")
	key := uuid.NewString()

	original := requireCreated(t, postTransfer(router, sender.Token, key, receiver.ID, "10", "EUR"), false)

	restartedDB, err := database.New(testDSN)
	if err != nil {
		t.Fatalf("failed to open a new pool: %v", err)
	}
	t.Cleanup(func() {
		if err := restartedDB.Close(); err != nil {
			t.Errorf("failed to close pool: %v", err)
		}
	})

	restartedRouter, _ := newTestRouter(restartedDB)
	replay := requireCreated(t, postTransfer(restartedRouter, sender.Token, key, receiver.ID, "10", "EUR"), true)

	if replay.TransferID != original.TransferID {
		t.Errorf("replay transfer_id = %s, want %s", replay.TransferID, original.TransferID)
	}

	assertTransferCount(t, sender.ID, 1)
	assertBalanceEUR(t, sender.ID, "90")
}

// Amounts must fit DECIMAL(19,4): at most 4 decimals and 15 integer digits
func TestTransferAmountPrecision(t *testing.T) {
	router, _ := newTestRouter(testDB)
	sender := createUser(t, "100")
	receiver := createUser(t, "0")

	tests := []struct {
		name       string
		amount     string
		wantStatus int
		wantCode   string
	}{
		{name: "five decimals", amount: "10.00001", wantStatus: http.StatusBadRequest, wantCode: response.ErrCodeBadRequest},
		{name: "sixteen integer digits", amount: "1000000000000000", wantStatus: http.StatusBadRequest, wantCode: response.ErrCodeBadRequest},
		{name: "largest valid amount", amount: "999999999999999.9999", wantStatus: http.StatusBadRequest, wantCode: response.ErrCodeInsufficientBalance},
		{name: "negative", amount: "-10", wantStatus: http.StatusBadRequest, wantCode: response.ErrCodeBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := postTransfer(router, sender.Token, uuid.NewString(), receiver.ID, tt.amount, "EUR")
			requireError(t, rec, tt.wantStatus, tt.wantCode)
		})
	}

	// Four decimals are still accepted
	requireCreated(t, postTransfer(router, sender.Token, uuid.NewString(), receiver.ID, "10.1234", "EUR"), false)

	assertTransferCount(t, sender.ID, 1)
	assertBalanceEUR(t, sender.ID, "89.8766")
}
