# gopay project specification

## Purpose

GoPay is a proof-of-concept P2P payment API: users hold wallets in EUR, USD and JPY, transfer money to each other, stake funds into per-currency liquidity pools, and request credits that an LLM (OpenAI) evaluates. It is written in Go with Gin and PostgreSQL. This is the only permanent spec. `/ship` updates it, and only when a task changes the architecture permanently.

## Architecture

- **Entry point:** `cmd/api`, the HTTP server. It loads `.env` and the configuration, the logger and the database, wires every domain and its routes, and shuts down gracefully within 5 seconds.
- **Domains:** one package per domain under `internal/<domain>/`, each with `handler.go`, `service.go`, `repository.go`, `types.go` and `routes.go`. `internal/auth` holds the JWT code and the auth middleware.
- **Shared packages** (`pkg/`):
  - `app`: configuration and server lifecycle
  - `currency`: the supported currencies
  - `database`: the Postgres connection pool
  - `idempotency`: parses the Idempotency-Key header
  - `llm`: the OpenAI client and the credit prompt
  - `logger`: zap
  - `response`: the response envelope and the error codes
- **Tests:**
  - Unit tests: `pkg/idempotency`, no build tag, run by `make test`.
  - Integration tests: `internal/wallet` and `internal/transfer`, tagged `//go:build integration`, run by `make test-integration`. They need Docker: each package starts its own testcontainers `postgres:16.3` and applies every up migration.
  - The user, pool, staking and credit domains have no tests.

## Domains

| Domain     | Owns                                                                                   |
| ---------- | -------------------------------------------------------------------------------------- |
| `auth`     | Issues and validates HS256 JWTs; the Bearer middleware that identifies the user        |
| `user`     | Registration (with wallet creation), login, profile, user search, account soft delete  |
| `wallet`   | One wallet per user with EUR, USD and JPY balances; idempotent deposits and withdrawals, recorded as movements |
| `transfer` | Idempotent same-currency P2P transfers, reading one transfer, transfer history        |
| `pool`     | Per-currency liquidity pools: stats, APY, interest distribution history                |
| `staking`  | Staking positions that move wallet funds into pool liquidity, with a 1-year lockup     |
| `credit`   | LLM-evaluated credits: request, accept (collateral and disbursement), payments, credit history |

## Data

- PostgreSQL 16.3. Migrations live in `migrate/migrations/NNN_<name>.{up,down}.sql` and are applied with golang-migrate. `migrate/seed/seed.sql` holds development data: pools, test users, wallets, credit history and interest distributions.
- `users`: UUID id. `username` and `email` are UNIQUE. `deleted_at` marks a soft delete.
- `wallets`: one row per user (UNIQUE `user_id`, cascades on user delete). Three balances, DECIMAL(19,4), default 0. There is no CHECK that a balance stays ≥ 0.
- `transfers`: sender, receiver, amount DECIMAL(19,4), currency, type (default P2P), status (default COMPLETED), and a nullable `idempotency_key`. The UNIQUE index (`sender_id`, `idempotency_key`) makes a key single-use per sender.
- `wallet_movements`: one row per deposit or withdrawal. CHECKs: type is DEPOSIT or WITHDRAWAL, the currency is supported, amount > 0. `idempotency_key` is NOT NULL, and the UNIQUE index (`user_id`, `idempotency_key`) makes a key single-use per user.
- `liquidity_pools`: one pool per currency (UNIQUE currency), with `total_liquidity`, `total_used` and `fee_percentage`.
- `interest_distributions`: per-pool interest records with an `apy_rate`.
- `staking_positions`: user, amount, accumulated interest, APY DECIMAL(5,2), lockup end, status (no CHECK), `withdrawn_at`.
- `credit_history`: one row per user (UNIQUE `user_id`), with counters, on-time payment rate and credit score.
- `credits`: amount, currency, status (no CHECK), APR, collateral fields and the raw LLM response (JSONB).
- `credit_payments`: the repayment schedule of a credit: one row per payment with a due date and a status (default PENDING).
- `credit_collateral`: the collateral locked for a credit, with `released_at`.
- History: 001 initial_schema · 002 liquidity_pools · 003 staking_positions · 004 credits · 005 transfer idempotency_key column · 006 unique (sender_id, idempotency_key) index · 007 wallet_movements

## Business rules (invariants)

### Requirement: AUT-01 JWT on protected routes

Every route except `/health`, `/auth/register` and `/auth/login` MUST require a valid `Authorization: Bearer` HS256 token issued by `gopay`.

#### Scenario: Missing or invalid token

- **WHEN** a protected route is called with no Bearer header, a malformed one, an invalid token or an expired token
- **THEN** it returns 401 with `UNAUTHORIZED`, `INVALID_TOKEN` or `TOKEN_EXPIRED`
- **AND** the handler does not run

### Requirement: WAL-01 Withdrawal never overdraws

A withdrawal SHALL lock the wallet row and debit it only if the balance in that currency is at least the amount.

#### Scenario: Withdrawal above the balance

- **WHEN** a user withdraws more than their balance in a currency
- **THEN** the response is 400 `INSUFFICIENT_BALANCE`
- **AND** the balance is unchanged and no movement is recorded

### Requirement: WAL-02 A wallet key moves money at most once

A deposit or withdrawal with an Idempotency-Key already used by the same user, with the same type, currency and amount, MUST NOT move money again. It SHALL return the original result with the header `Idempotent-Replayed: true`.

#### Scenario: Replayed deposit

- **WHEN** a user repeats a deposit with the same key and the same body
- **THEN** the response is 200 with `Idempotent-Replayed: true`
- **AND** the balance was credited only once

### Requirement: WAL-03 Reused wallet key with different parameters

A wallet key reused by the same user with a different type, currency or amount MUST be rejected.

#### Scenario: Same key, different amount

- **WHEN** a user sends a deposit with a key already used for a different amount, currency or operation type
- **THEN** the response is 422 `IDEMPOTENCY_KEY_REUSED`
- **AND** no money moves

### Requirement: WAL-04 Wallet replay is decided before the balance check

The replay of a wallet key SHALL be decided before the balance is checked, so a replay succeeds even if the balance has since dropped.

#### Scenario: Replay after the balance was spent

- **WHEN** a user replays a successful withdrawal after the balance went below its amount
- **THEN** the response is the original result, marked as replayed
- **AND** it is not answered as `INSUFFICIENT_BALANCE`

### Requirement: WAL-05 A failed wallet operation leaves no trace

A deposit or withdrawal that fails MUST leave neither a movement nor a balance change, and its key stays unused.

#### Scenario: Failed withdrawal

- **WHEN** a withdrawal fails (insufficient balance or a database error)
- **THEN** the transaction is rolled back
- **AND** no `wallet_movements` row exists for its key and the balance is unchanged

### Requirement: WAL-06 A database error is never reported as insufficient balance

A database failure during a wallet operation MUST be returned as 500, never as `INSUFFICIENT_BALANCE`.

#### Scenario: Database failure during a withdrawal

- **WHEN** a database statement of a withdrawal fails
- **THEN** the response is 500 `INTERNAL_SERVER_ERROR`

### Requirement: WAL-07 Wallet amount bounds

A deposit or withdrawal amount MUST be positive, have at most 4 decimals and be at most 999999999999999.9999.

#### Scenario: Invalid amount

- **WHEN** the amount is zero, negative, has more than 4 decimals or exceeds the maximum
- **THEN** the response is 400 `BAD_REQUEST`
- **AND** no money moves

### Requirement: WAL-08 One wallet per user

Each user SHALL have exactly one wallet, created when the user registers.

#### Scenario: Registration

- **WHEN** a user registers
- **THEN** one wallet row with zero balances exists for that user
- **AND** the database rejects a second wallet for the same user

### Requirement: IDM-01 Idempotency-Key format

Deposit, withdrawal and transfer MUST require an `Idempotency-Key` header that is a canonical UUID. The key SHALL be stored lowercase, so case variants of one UUID are the same key.

#### Scenario: Missing or malformed key

- **WHEN** a deposit, withdrawal or transfer has no Idempotency-Key or one that is not a canonical UUID
- **THEN** the response is 400 `BAD_REQUEST`
- **AND** no money moves

#### Scenario: Uppercase variant

- **WHEN** a request repeats a used key in uppercase
- **THEN** it is treated as the same key

### Requirement: IDM-02 Keys are scoped per user and per operation family

Idempotency keys SHALL be unique per user, and wallet keys SHALL be independent from transfer keys.

#### Scenario: Same key, different user or family

- **WHEN** a key already used by one user is sent by another user, or a wallet key is reused for a transfer
- **THEN** it is treated as a new key

### Requirement: IDM-03 Key in progress

When a concurrent request holds the same key and its row is not yet visible, the request MUST be answered with 409 `CONFLICT`, not processed.

#### Scenario: Concurrent requests with one key

- **WHEN** two requests with the same key run at the same time and one cannot see the other's committed row
- **THEN** that one returns 409 `CONFLICT`
- **AND** money moves at most once

### Requirement: TRF-01 A transfer key moves money at most once

A transfer with a key already used by the same sender, with the same receiver, amount and currency, MUST NOT move money again. It SHALL return the original transfer with `Idempotent-Replayed: true`.

#### Scenario: Replayed transfer

- **WHEN** a sender repeats a transfer with the same key and the same body
- **THEN** the response is 201 with the original transfer and `Idempotent-Replayed: true`
- **AND** the balances moved only once

### Requirement: TRF-02 Reused transfer key with different parameters

A transfer key reused by the same sender with a different receiver, amount or currency MUST be rejected.

#### Scenario: Same key, different receiver

- **WHEN** a sender reuses a key with a different receiver, amount or currency
- **THEN** the response is 422 `IDEMPOTENCY_KEY_REUSED`
- **AND** no money moves

### Requirement: TRF-03 Transfer balance is checked under lock

A transfer SHALL lock both wallets and check the sender's balance under the lock. A transfer that fails MUST NOT consume its key.

#### Scenario: Insufficient balance

- **WHEN** the sender's balance in the currency is below the amount
- **THEN** the response is 400 `INSUFFICIENT_BALANCE`
- **AND** no transfer row exists and the key can be used again

### Requirement: TRF-04 Transfer amount bounds

A transfer amount MUST be positive, have at most 4 decimals and at most 15 integer digits.

#### Scenario: Too many decimals

- **WHEN** a transfer amount has more than 4 decimals or more than 15 integer digits
- **THEN** the response is 400 `BAD_REQUEST`

### Requirement: TRF-05 No self-transfer

A user MUST NOT transfer to themselves.

#### Scenario: Receiver is the sender

- **WHEN** the receiver id equals the sender id
- **THEN** the response is 400 `BAD_REQUEST`

### Requirement: TRF-06 Unknown receiver is reported as not found

A transfer to a receiver with no wallet SHALL return 404, even when the key was already used, because the wallets are checked before the replay.

#### Scenario: Unknown receiver

- **WHEN** a transfer names a receiver that does not exist, with a new key or with a used one
- **THEN** the response is 404 `NOT_FOUND`
- **AND** no money moves

### Requirement: TRF-07 Only participants can read a transfer

A transfer SHALL be readable only by its sender or its receiver.

#### Scenario: Third party reads a transfer

- **WHEN** a user who is neither sender nor receiver requests `GET /transfer/:id`
- **THEN** the response is 403 `FORBIDDEN`

### Requirement: USR-01 Account deletion requires empty balances

A user MUST NOT delete their account while any wallet balance is not zero. Deletion is a soft delete.

#### Scenario: Delete with funds

- **WHEN** a user with a non-zero balance in any currency calls `DELETE /users/me`
- **THEN** the response is 400 `CANNOT_DELETE_USER`
- **AND** the account is not deleted

### Requirement: USR-02 Unique username and email

Usernames and emails MUST be unique.

#### Scenario: Duplicate registration

- **WHEN** a user registers, or updates their profile, with a username or email already taken
- **THEN** the response is 409 `USER_ALREADY_EXISTS`

### Requirement: POL-01 One pool per currency

There SHALL be at most one liquidity pool per currency.

#### Scenario: Pool lookup

- **WHEN** a pool is read by currency
- **THEN** exactly one pool or none is found

### Requirement: STK-01 Stake amount bounds

A new stake MUST be at least 1000 and at most the user's wallet balance in that currency.

#### Scenario: Stake below the minimum

- **WHEN** a user stakes less than 1000
- **THEN** the response is 400 `BAD_REQUEST`

#### Scenario: Stake above the balance

- **WHEN** a user stakes more than their balance
- **THEN** the response is 400 `INSUFFICIENT_BALANCE`

### Requirement: STK-02 One-year lockup

A staking position SHALL be locked for 1 year from its creation, and MUST NOT be withdrawn before the lockup ends.

#### Scenario: Early withdrawal

- **WHEN** the owner withdraws a position before its lockup end
- **THEN** the response is 400
- **AND** the position stays ACTIVE

### Requirement: STK-03 Only the owner acts on an active position

Only the owner of a staking position MAY increase or withdraw it, and only while it is ACTIVE.

#### Scenario: Another user's position

- **WHEN** a user increases or withdraws a position they do not own
- **THEN** the response is 401 `UNAUTHORIZED`

#### Scenario: Position not active

- **WHEN** the owner increases or withdraws a position that is not ACTIVE
- **THEN** the response is 400

### Requirement: CRD-01 Credit duration

A credit request MUST have a duration of 1 to 60 months.

#### Scenario: Duration out of range

- **WHEN** a credit is requested for 0 or more than 60 months
- **THEN** the response is 400 `BAD_REQUEST`

### Requirement: CRD-02 Credit request eligibility

A user with any defaulted credit, or with 3 or more active credits, MUST NOT request a new credit.

#### Scenario: Too many active credits

- **WHEN** a user with 3 active credits requests another
- **THEN** the response is 400 `BAD_REQUEST`
- **AND** the LLM is not called

### Requirement: CRD-03 Accepting a credit

Accepting a credit SHALL lock the credit, the user's wallet and the pool. Under those locks it re-checks that the credit is PENDING, that the collateral balance is enough and that the pool has enough available liquidity. Then, in one transaction, it debits the collateral, credits the amount to the wallet, increases the pool's `total_used`, creates the payment schedule and the collateral record, and updates the credit history.

#### Scenario: Not pending

- **WHEN** a user accepts a credit that is not PENDING
- **THEN** the response is 400 `BAD_REQUEST`
- **AND** nothing changes

#### Scenario: Collateral or liquidity short

- **WHEN** the collateral balance or the pool's available liquidity is below what the credit needs
- **THEN** the response is 400 `INSUFFICIENT_BALANCE`
- **AND** nothing changes

### Requirement: CRD-04 Paying a credit

A payment SHALL lock the payment, the credit, the wallet and the pool. It requires a PENDING payment, an ACTIVE credit and enough balance for the payment amount. The last payment completes the credit and returns the collateral to the wallet.

#### Scenario: Insufficient balance for a payment

- **WHEN** the wallet balance is below the payment amount
- **THEN** the response is 400 `INSUFFICIENT_BALANCE`
- **AND** the payment stays PENDING

#### Scenario: Last payment

- **WHEN** the last pending payment is paid
- **THEN** the credit becomes COMPLETED
- **AND** the collateral is released and credited back to the wallet

### Requirement: CRD-05 A credit is visible only to its owner

A credit owned by another user SHALL be reported as not found.

#### Scenario: Another user's credit

- **WHEN** a user reads, accepts or pays a credit they do not own
- **THEN** the response is 404 `NOT_FOUND`

### Requirement: CRD-06 LLM decision is validated

The LLM evaluation MUST return a decision of APPROVED or REJECTED with its required fields; any other response is an error. An approved credit is stored as PENDING, a rejected one as REJECTED.

#### Scenario: Invalid LLM response

- **WHEN** the LLM returns another decision, a missing field or invalid JSON
- **THEN** the request fails with 500 `INTERNAL_SERVER_ERROR`

### Requirement: CRD-07 One credit history per user

Each user SHALL have at most one credit history row.

#### Scenario: First credit request

- **WHEN** a user requests their first credit
- **THEN** one credit history row is created for them, and later requests reuse it

## Dependencies

Go 1.26.0 (`go.mod`) · PostgreSQL 16.3 via `database/sql` and `lib/pq` · docker compose (`db` and `app`) · OpenAI Chat Completions API · gin (HTTP router) · golang-jwt v5 (auth) · shopspring/decimal (all money) · google/uuid (ids and keys) · bcrypt from x/crypto (passwords) · zap (logging) · godotenv (`.env`) · testcontainers-go (integration tests) · golang-migrate CLI (migrations)

## API surface

Every route marked "yes" can also return 401 `UNAUTHORIZED`, `INVALID_TOKEN` or `TOKEN_EXPIRED` (AUT-01). Every route can return 500 `INTERNAL_SERVER_ERROR`.

| Method | Path                               | Auth | Error codes |
| ------ | ---------------------------------- | ---- | ----------- |
| GET    | /health                            | no   | 200 · 500 (database ping fails) |
| POST   | /auth/register                     | no   | 201 · 400 BAD_REQUEST · 409 USER_ALREADY_EXISTS |
| POST   | /auth/login                        | no   | 200 · 400 BAD_REQUEST · 401 UNAUTHORIZED |
| GET    | /users/me                          | yes  | 200 · 404 NOT_FOUND |
| PUT    | /users/me                          | yes  | 200 · 400 BAD_REQUEST · 404 NOT_FOUND · 409 USER_ALREADY_EXISTS |
| DELETE | /users/me                          | yes  | 200 · 400 CANNOT_DELETE_USER · 404 NOT_FOUND |
| GET    | /users/search                      | yes  | 200 · 400 BAD_REQUEST (query under 2 characters) |
| GET    | /users/:id                         | yes  | 200 · 404 NOT_FOUND |
| GET    | /wallet/balance                    | yes  | 200 · 404 NOT_FOUND |
| POST   | /wallet/deposit                    | yes  | 200 (replay: `Idempotent-Replayed: true`) · 400 BAD_REQUEST · 404 NOT_FOUND · 409 CONFLICT · 422 IDEMPOTENCY_KEY_REUSED |
| POST   | /wallet/withdraw                   | yes  | as deposit · 400 INSUFFICIENT_BALANCE |
| POST   | /transfer                          | yes  | 201 (replay: `Idempotent-Replayed: true`) · 400 BAD_REQUEST · 400 INSUFFICIENT_BALANCE · 404 NOT_FOUND · 409 CONFLICT · 422 IDEMPOTENCY_KEY_REUSED |
| GET    | /transfer/:id                      | yes  | 200 · 403 FORBIDDEN · 404 NOT_FOUND |
| GET    | /transfer/history                  | yes  | 200 (limit default 50, max 100) |
| GET    | /pool/stats                        | yes  | 200 |
| GET    | /pool/stats/:currency              | yes  | 200 · 400 BAD_REQUEST · 404 NOT_FOUND |
| GET    | /pool/interest-history/:currency   | yes  | 200 · 400 BAD_REQUEST · 404 NOT_FOUND |
| POST   | /staking/stake                     | yes  | 201 · 400 BAD_REQUEST · 400 INSUFFICIENT_BALANCE |
| POST   | /staking/:id/increase              | yes  | 200 · 400 BAD_REQUEST · 400 INSUFFICIENT_BALANCE · 401 UNAUTHORIZED (not the owner) · 404 NOT_FOUND |
| GET    | /staking/positions                 | yes  | 200 · 400 BAD_REQUEST (invalid status filter) |
| POST   | /staking/:id/withdraw              | yes  | 200 · 400 BAD_REQUEST (not active, still locked) · 401 UNAUTHORIZED (not the owner) · 404 NOT_FOUND |
| POST   | /credit/request                    | yes  | 201 · 400 BAD_REQUEST · 400 INSUFFICIENT_BALANCE (pool liquidity) · 404 NOT_FOUND · 500 (LLM failure) |
| GET    | /credit/:id                        | yes  | 200 · 404 NOT_FOUND |
| GET    | /credit/my-credits                 | yes  | 200 |
| POST   | /credit/:id/accept                 | yes  | 200 · 400 BAD_REQUEST · 400 INSUFFICIENT_BALANCE · 404 NOT_FOUND |
| POST   | /credit/:id/payment                | yes  | 200 · 400 BAD_REQUEST · 400 INSUFFICIENT_BALANCE · 404 NOT_FOUND |
| GET    | /credit/history                    | yes  | 200 · 404 NOT_FOUND |

## Lifecycles

### Transfer

- **States:** COMPLETED (PENDING and FAILED are defined in code but never written)
- **Transitions:** none; every transfer is created COMPLETED
- **Guard:** nothing does; there is no CHECK on the column

### Staking position

- **States:** ACTIVE · WITHDRAWN (COMPLETED is defined but never set)
- **Transitions:** created ACTIVE on stake; ACTIVE → WITHDRAWN on withdraw
- **Guard:** a service check before the transaction; the UPDATE has no status condition and there is no CHECK

### Credit

- **States:** PENDING · REJECTED · ACTIVE · COMPLETED (DEFAULTED is defined but never set)
- **Transitions:** created PENDING (LLM approves) or REJECTED (LLM rejects) on request; PENDING → ACTIVE on accept; ACTIVE → COMPLETED on the last payment
- **Guard:** accept and pay lock the credit row and re-check its status; there is no CHECK

### Credit payment

- **States:** PENDING · PAID (OVERDUE is defined but never set)
- **Transitions:** created PENDING on accept; PENDING → PAID on payment
- **Guard:** the payment row is locked and its status re-checked

### Credit collateral

- **States:** locked (`released_at` NULL) · released
- **Transitions:** locked → released on the last payment
- **Guard:** the UPDATE only applies where `released_at IS NULL`

### User

- **States:** active · deleted (`deleted_at` set)
- **Transitions:** active → deleted on `DELETE /users/me`
- **Guard:** the UPDATE only applies where `deleted_at IS NULL`

## Cross-domain flows

All money is `shopspring/decimal`, stored as DECIMAL(19,4). Only wallet and transfer amounts are validated to 4 decimals; staking and credit amounts are rounded by PostgreSQL on insert. Every flow below runs in one READ COMMITTED transaction.

### Deposit and withdrawal

The wallet balance in the currency is credited (deposit) or debited (withdrawal), and one `wallet_movements` row is inserted with the key.

### Transfer

The sender's balance is debited and the receiver's balance credited in the same currency, and one `transfers` row is inserted with the key.

### Stake

The wallet is debited by the amount, the pool's `total_liquidity` grows by the amount, and an ACTIVE position is inserted with the pool's current APY.

### Increase stake

The wallet is debited, the position's amount grows and the pool's `total_liquidity` grows by the same amount.

### Withdraw stake

The pool's `total_liquidity` shrinks by the principal, and the wallet is credited the principal plus `accumulated_interest`.

### Accept credit

The collateral is debited from the wallet in the collateral currency, the credit amount is credited to the wallet in the credit currency, and the pool's `total_used` grows by the amount. The payment schedule and the collateral record are inserted, and the credit history counts one more active credit and the amount borrowed.

### Pay credit

The wallet is debited the payment's total amount, the pool's `total_used` shrinks by the principal part, the payment becomes PAID and the history adds the amount repaid. On the last payment the credit becomes COMPLETED, the collateral is released and credited back to the wallet, and the history moves one credit from active to paid.

### Interest and APY

- **Pool APY:** the `apy_rate` of the newest `interest_distributions` row. Pool stats look back 365 days, and staking looks back 30 days. With none, the APY defaults to 0.0200. Utilization is `total_used / total_liquidity`.
- **Staking interest:** the position stores the APY at creation. Nothing computes or accrues `accumulated_interest`, so there is no formula, time base or schedule, and a withdrawal pays principal plus 0. Nothing in the code writes `interest_distributions`; only the seed does.
- **Credit interest:** the APR and the amortized repayment schedule come from the LLM; the code does not compute them.

## Locking order

- Wallet movement: wallets[user] → wallet_movements key → update wallets[user]
- Transfer: wallets[sender] → wallets[receiver] → transfers key → update sender → update receiver
- Stake: liquidity_pools[currency] → insert position → wallets[user]
- Increase stake: staking_positions[id] → liquidity_pools[currency] → wallets[user]
- Withdraw stake: staking_positions[id] (update) → liquidity_pools[currency] → wallets[user]
- Accept credit: credits[id] → wallets[user] → liquidity_pools[currency] → payments, collateral, wallets, pool, credit_history
- Pay credit: credit_payments[id] → credits[id] → wallets[user] → liquidity_pools[currency] → wallet, pool, payment, history, and on the last payment the credit, the collateral and the wallet
- **Global order:** none. A transfer A→B and a transfer B→A lock the same two wallets in opposite orders. The staking transactions lock the pool before the wallet, while accept and pay credit lock the wallet before the pool.

## External dependencies

- **OpenAI Chat Completions:**
  - Request: `POST https://api.openai.com/v1/chat/completions` with a Bearer key, temperature 0.3, 2000 max tokens and a JSON response format. The applicant data goes as float amounts in the loan currency.
  - Response: JSON with `decision` (APPROVED or REJECTED), `credit_terms` (APR as a percentage, collateral amount in the collateral currency), a repayment schedule with `YYYY-MM-DD` due dates, and totals.
  - Timeout: 30 seconds (HTTP client) plus the request context.
  - On failure: no retry. Every error, including rate limits, timeouts and an empty API key, ends as 500 `INTERNAL_SERVER_ERROR`.
- **Clock:**
  - Contract: `time.Now`, used for the staking lockup, the account age sent to the LLM and the JWT expiry.
  - Timeout: not applicable.
  - On failure: not injected, no fallback.

## Configuration

| Variable               | Controls                                         | Default                                                          |
| ---------------------- | ------------------------------------------------ | ---------------------------------------------------------------- |
| `ENV`                  | Logger mode and Gin release mode                 | `development` (compose sets `production`)                        |
| `PORT`                 | HTTP listen port                                 | `8080`                                                           |
| `DB_DSN`               | PostgreSQL connection string                     | `postgres://admin:password@localhost:5432/gopay?sslmode=disable` |
| `JWT_SECRET`           | HMAC secret for tokens                           | `dev-secret-key-change-in-production`                            |
| `JWT_EXPIRATION_HOURS` | Token lifetime in hours                          | `24` (also when unparsable)                                      |
| `OPENAI_API_KEY`       | OpenAI authentication                            | empty (credit requests fail)                                     |
| `OPENAI_MODEL`         | OpenAI model                                     | `gpt-4o`                                                         |
| `POSTGRES_DB`, `POSTGRES_USER`, `POSTGRES_PASSWORD` | Database container setup (compose)  | `gopay` / `admin` / `password`                         |
| `DB_PORT`              | Host port of the database container (compose)   | `5432`                                                           |
| `LOG_LEVEL`, `DB_HOST` | Nothing: listed in `.env.example` and compose, never read by the code | —                                        |

## Known gaps

- **Staking withdraw has no SQL guard:** the UPDATE has no `status = 'ACTIVE'` condition and the status check runs outside the transaction, so two concurrent withdrawals of one position can both credit the wallet and debit the pool.
- **Stake and increase can overdraw:** the balance check runs outside the transaction, the wallet debit is unconditional, and no CHECK keeps balances ≥ 0.
- **Increase on a withdrawn position:** increase locks the position but does not re-check its status under the lock.
- **Pool liquidity can go below what is used:** a staking withdrawal subtracts the principal from `total_liquidity` without comparing it against `total_used`.
- **Staking interest never accrues:** `accumulated_interest` is never updated, so a withdrawal returns only the principal. Nothing writes `interest_distributions`.
- **APY unit is inconsistent:** the default 0.0200 is a fraction, the seed stores percentages (2.4–3.2), and staking stores the APY in DECIMAL(5,2), which drops any fraction beyond 2 decimals (0.0250 becomes 0.03). The intended unit is undecided.
- **Credit interest is lost:** the interest part of a payment leaves the wallet but is credited to no balance or pool column.
- **`credits.collateral_locked` is never set:** it stays 0 after accept.
- **Active-credit limit only at request time:** the 3-active-credits limit (CRD-02) is not re-checked on accept, and PENDING credits have no limit.
- **Credit terms live only in the prompt:** the APR range (4–12%), the collateral range (10–50%) and collateral in another currency exist only in the LLM prompt; accept checks only that the currency is valid. The LLM's `approved_amount` and `duration_months` are ignored.
- **Credit request is not transactional:** the credit history is created before the LLM call, and the credit insert and its review update are separate statements.
- **Payment due dates are never checked:** payments can be made early, and nothing sets OVERDUE or DEFAULTED.
- **No pending payment returns 500:** paying a credit with no pending payment returns a plain error, not a 4xx.
- **Registration is not transactional:** a failed wallet insert is compensated with a soft delete, so the username and email stay taken.
- **Account deletion is incomplete:** the balance check runs outside any transaction and ignores staked funds and active credits.
- **Deleted users keep access:** a soft-deleted user's token still works on the wallet, transfer, pool, staking and credit routes; only the user routes filter `deleted_at`.
- **Errors swallowed:**
  - login answers database errors as 401;
  - pool stats and APY fall back to defaults when the query fails;
  - pool decimal parse errors are ignored on accept;
  - the transfer rollback error is ignored.
- **Staking errors are mapped by string comparison** of `err.Error()`, and an increase with amount ≤ 0 is reported as "at least 1000".
- **Malformed ids return 500:** a non-UUID `:id` on the user, transfer, staking or credit routes returns 500 instead of 400 or 404.
- **No server or query timeouts:** `http.Server` sets no read, write or header timeouts, and no query has a deadline.
- **Unused pool code:** `AddLiquidity` and `RemoveLiquidity` do an unlocked read-modify-write; they have no callers.
- **Self-transfer check is case-sensitive:** it compares raw id strings, while the replay check ignores case.
- **Two wallet tests assert an unused path:** `TestWithdraw_UpdateFails` and `TestWithdraw_LookupFailsAfterNoRowsUpdated` prove their failure through the repository's `Withdraw`, which production never calls; through the HTTP route the 500 comes from a different statement.
- **Untested rules:** AUT-01 (no direct test), WAL-08, IDM-03, TRF-05, TRF-07, USR-01, USR-02, POL-01, STK-01 to STK-03 and CRD-01 to CRD-07 have no test. The user, pool, staking and credit domains have no tests at all.
