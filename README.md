# GoPay

> A high-performance P2P payment system supporting multi-currency transfers with ACID transaction guarantees

[![Go Version](https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![PostgreSQL](https://img.shields.io/badge/PostgreSQL-16+-336791?style=flat&logo=postgresql)](https://www.postgresql.org/)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)

## Overview

**GoPay** is a proof-of-concept P2P payment platform that enables users to transfer funds across three fiat currencies (EUR, USD, JPY). Built with Go and PostgreSQL, it emphasizes clean architecture, domain-driven design, and transactional integrity.

### Key Features

- 💰 **Multi-Currency Support** - EUR, USD, and JPY wallets
- 🔒 **ACID Transactions** - Pessimistic locking with `SELECT FOR UPDATE`
- 🚀 **High Performance** - Optimized database queries with strategic indexes
- 🔐 **JWT Authentication** - Secure, stateless authentication
- 📊 **Decimal Precision** - Accurate monetary calculations (never float64)
- 🎯 **Clean Architecture** - Domain-driven design with clear separation of concerns
- ✅ **Comprehensive Validation** - Input validation and business rule enforcement
- 🤖 **AI-Powered Credit System** - GPT-4 LLM integration for credit evaluation
- 💳 **Credit Management** - Full credit lifecycle with collateral management
- 📈 **Staking & Liquidity Pools** - DeFi-like features with interest distribution
- 🌱 **Database Seeding** - Pre-populated test data for development

## Tech Stack

- **Backend**: Go 1.25
- **Web Framework**: Gin
- **Database**: PostgreSQL 16+
- **Authentication**: JWT (golang-jwt/jwt)
- **Decimal Math**: shopspring/decimal
- **Logging**: Zap (uber-go)
- **Password Hashing**: bcrypt
- **Hot Reload**: Air (development)
- **LLM Integration**: OpenAI GPT-4 for credit evaluation
- **Database Seeding**: SQL seeding scripts with pre-populated test data

## Architecture

GoPay follows Clean Architecture principles with domain-driven design:

```
┌─────────────┐
│   HTTP/API  │ ← Gin handlers, JWT middleware
├─────────────┤
│   Service   │ ← Business logic, validation
├─────────────┤
│ Repository  │ ← Data access, ACID transactions
├─────────────┤
│ PostgreSQL  │ ← Persistence layer
└─────────────┘
```

### Project Structure

```
gopay/
├── cmd/
│   └── api/
│       ├── main.go              # Application entry point (56 lines)
│       └── setup.go             # Domain initialization & routing
├── internal/                    # Private application code
│   ├── auth/                    # JWT service & middleware
│   ├── user/                    # User domain (CRUD, search)
│   ├── wallet/                  # Wallet operations (deposit, withdraw)
│   ├── transfer/                # P2P transfers (ACID transactions)
│   ├── credit/                  # Credit system with LLM evaluation
│   ├── pool/                    # Liquidity pools domain
│   └── staking/                 # Staking operations domain
├── pkg/                         # Public reusable packages
│   ├── app/                     # Application infrastructure (NEW)
│   │   ├── config.go            # Configuration loading (JWT, LLM, env)
│   │   └── server.go            # Server lifecycle management
│   ├── database/                # PostgreSQL connection
│   ├── logger/                  # Structured logging
│   ├── response/                # Standard API responses
│   ├── idempotency/             # Idempotency-Key header contract and validation
│   ├── validator/               # Input validation
│   ├── currency/                # Currency type definitions
│   └── llm/                     # LLM client for credit evaluation
├── migrate/
│   ├── migrations/              # SQL migration files
│   └── seed/                    # Database seeding scripts
└── docker-compose.yml           # PostgreSQL setup
```

**Domain Structure Pattern:**

Each domain follows the same pattern:

- `handler.go` - HTTP handlers
- `service.go` - Business logic
- `repository.go` - Data access
- `types.go` - Domain models
- `routes.go` - Route registration

**Application Bootstrap:**

- `cmd/api/main.go` - Clean entry point with minimal boilerplate
- `cmd/api/setup.go` - GoPay-specific domain initialization
- `pkg/app/config.go` - Reusable configuration loading
- `pkg/app/server.go` - Reusable HTTP server lifecycle

### Code Organization Philosophy

GoPay follows a **hybrid approach** to code organization:

**Generic Infrastructure (`pkg/app/`):**

- ✅ Reusable across multiple commands (API, workers, CLI tools)
- ✅ No knowledge of specific domains (user, wallet, transfer, etc.)
- ✅ Infrastructure-only: configuration, server lifecycle, utilities
- ✅ Can be used in other Go projects

**Application-Specific Logic (`cmd/api/setup.go`):**

- ✅ Knows about all GoPay domains and their dependencies
- ✅ Orchestrates domain initialization in correct order
- ✅ Not reusable (specific to this application)
- ✅ Keeps `main.go` clean and focused

**Result**: `main.go` reduced from **174 lines to 56 lines** (68% reduction) while maintaining clarity and following the "Simplicity First" principle.

## Prerequisites

- **Go**: 1.21 or higher
- **PostgreSQL**: 16 or higher
- **Docker**: (optional, for containerized PostgreSQL)
- **Air**: (optional, for hot reload during development)

## Getting Started

### 1. Clone the Repository

```bash
git clone https://github.com/yourusername/gopay.git
cd gopay
```

### 2. Environment Setup

Create a `.env` file in the project root:

```env
# Server Configuration
PORT=8080
ENV=development

# Database Configuration
DB_DSN=postgres://admin:password@localhost:5432/gopay?sslmode=disable

# JWT Configuration
JWT_SECRET=your-super-secret-jwt-key-change-this-in-production
JWT_EXPIRATION_HOURS=24
```

### 3. Start PostgreSQL

Using Docker (recommended):

```bash
docker-compose up -d
```

Or use your local PostgreSQL instance and create the `gopay` database.

### 4. Run Database Migrations

```bash
# Install golang-migrate (if not installed)
brew install golang-migrate

# Run migrations
make migrate-up

# Or manually:
migrate -path migrate/migrations -database "postgres://admin:password@localhost:5432/gopay?sslmode=disable" up
```

### 4.5 Seed Database (Optional but Recommended)

```bash
# Seed the database with test data
make seed

# Or reset database completely (migrations + seed)
make reset-db
```

See [migrate/seed/README.md](migrate/seed/README.md) for seeding details.

### 5. Install Dependencies

```bash
go mod download
```

### 6. Run the Application

**Option A: With hot reload (recommended for development)**

```bash
# Install Air
go install github.com/cosmtrek/air@latest

# Run with Air
air
```

**Option B: Standard run**

```bash
go run cmd/api/main.go
```

The server will start on `http://localhost:8080`

### 7. Health Check

```bash
curl http://localhost:8080/health
```

Expected response:

```json
{
  "success": true,
  "data": {
    "status": "healthy",
    "service": "gopay-api",
    "version": "0.0.1"
  }
}
```

## API Documentation

### Authentication Endpoints

#### Register

```bash
POST /auth/register
Content-Type: application/json

{
  "username": "johndoe",
  "email": "john@example.com",
  "password": "SecurePass123"
}
```

#### Login

```bash
POST /auth/login
Content-Type: application/json

{
  "email": "john@example.com",
  "password": "SecurePass123"
}
```

Response includes JWT token:

```json
{
  "success": true,
  "data": {
    "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
    "user": {
      "id": "...",
      "username": "johndoe",
      "email": "john@example.com"
    }
  }
}
```

### User Endpoints

All user endpoints require JWT authentication (`Authorization: Bearer <token>`).

#### Get Profile

```bash
GET /users/me
Authorization: Bearer <token>
```

#### Search Users

```bash
GET /users/search?q=john
Authorization: Bearer <token>
```

#### Get User by ID

```bash
GET /users/:id
Authorization: Bearer <token>
```

#### Update Profile

```bash
PUT /users/me
Authorization: Bearer <token>
Content-Type: application/json

{
  "username": "johndoe_updated",
  "email": "newemail@example.com"
}
```

#### Delete Account

```bash
DELETE /users/me
Authorization: Bearer <token>
```

_Note: Account can only be deleted if all currency balances are zero._

### Wallet Endpoints

#### Get Balance

```bash
GET /wallet/balance
Authorization: Bearer <token>
```

Response:

```json
{
  "success": true,
  "data": {
    "eur": "1000.50",
    "usd": "500.00",
    "jpy": "0"
  }
}
```

#### Deposit Funds

```bash
POST /wallet/deposit
Authorization: Bearer <token>
Content-Type: application/json
Idempotency-Key: 0b8e4f2a-7c1d-4e9b-a3f6-5d2c8b1e9a07

{
  "amount": "1000.50",
  "currency": "EUR"
}
```

#### Withdraw Funds

```bash
POST /wallet/withdraw
Authorization: Bearer <token>
Content-Type: application/json
Idempotency-Key: 9d3a7b5c-2e4f-4a1b-8c6d-0f9e8a7b6c5d

{
  "amount": "100.00",
  "currency": "USD"
}
```

`amount` must be positive, with at most 4 decimals and at most `999999999999999.9999`;
otherwise `400 Bad Request`. Both routes return the updated wallet with `200 OK`.

> **Breaking change:** `Idempotency-Key` is now mandatory on `POST /wallet/deposit` and
> `POST /wallet/withdraw`. Clients that do not send it get `400 Bad Request` and no money moves.
> Amounts with more than 4 decimals, or above `999999999999999.9999`, are now rejected with
> `400 Bad Request` (before, they were silently rounded, or failed with `500` on overflow).

**Idempotency (required header):** the client generates a new UUID each time the user
starts a deposit or a withdrawal and reuses that same key in every retry of it (timeouts,
lost connection). A retry never moves money twice. The key is unique per user and shared by
deposits and withdrawals; it is independent of the keys used for transfers. It is stored with
the movement in `wallet_movements`, so the guarantee lasts as long as the movement exists.

| Situation | Response |
|---|---|
| New key | `200 OK`, the deposit or withdrawal is applied and recorded. |
| Key already used, same operation, amount and currency | `200 OK` with header `Idempotent-Replayed: true`. No money is moved. Body: the current wallet. Amounts are compared as decimals: `"10"` and `"10.0000"` are the same. This holds even if the balance no longer covers a replayed withdrawal: the replay is resolved before the balance check. |
| Key already used with a different operation (deposit/withdraw), amount or currency | `422 Unprocessable Entity`, code `IDEMPOTENCY_KEY_REUSED`. No money is moved. |
| Header missing, or not a UUID in canonical form (`8-4-4-4-12`, any case) | `400 Bad Request`. No money is moved. |
| The original request failed (validation, insufficient balance, missing wallet...) | The key is not registered: a retry with the same key is executed normally. |
| Several requests with the same key at the same time | Only one moves money; the others wait for it and get the replay response. `409 Conflict` is only a safety net if the original cannot be read. |

### Transfer Endpoints

#### Create Transfer

```bash
POST /transfer
Authorization: Bearer <token>
Content-Type: application/json
Idempotency-Key: 6f1c2a9e-3b4d-4e5f-8a7b-9c0d1e2f3a4b

{
  "receiver_id": "550e8400-e29b-41d4-a716-446655440000",
  "amount": "250.50",
  "currency": "EUR"
}
```

`amount` must be positive, with at most 4 decimals and 15 integer digits.

**Idempotency (required header):** the client generates a new UUID each time the user
presses "send" and reuses that same key in every retry of that send (timeouts, lost
connection). A retry never moves money twice. The key is unique per sender and is stored
with the transfer, so the guarantee lasts as long as the transfer exists.

| Situation | Response |
|---|---|
| New key | `201 Created`, the transfer is executed. |
| Key already used, same receiver, amount and currency | `201 Created` with header `Idempotent-Replayed: true`. No money is moved. Body: the original transfer (same `transfer_id`, amount, receiver, currency and `created_at`) and the current balances. Amounts are compared as decimals: `"10"` and `"10.00"` are the same. |
| Key already used with a different receiver, amount or currency | `422 Unprocessable Entity`, code `IDEMPOTENCY_KEY_REUSED`. No money is moved. |
| Header missing, or not a UUID in canonical form (`8-4-4-4-12`, any case) | `400 Bad Request`. No money is moved. |
| The original request failed (validation, insufficient balance, unknown receiver...) | The key is not registered: a retry with the same key is executed normally. |
| Several requests with the same key at the same time | Only one moves money; the others wait for it and get the replay response. `409 Conflict` is only a safety net if the original cannot be read. |

Response includes updated balances:

```json
{
  "success": true,
  "data": {
    "transfer_id": "...",
    "sender_id": "...",
    "receiver_id": "...",
    "amount": "250.50",
    "currency": "EUR",
    "status": "COMPLETED",
    "sender_balance": "750.00",
    "receiver_balance": "2250.50",
    "created_at": "2025-11-08T15:42:06.931947Z"
  }
}
```

#### Get Transfer by ID

```bash
GET /transfer/:id
Authorization: Bearer <token>
```

_Note: Only the sender or receiver can view a transfer._

#### Get Transfer History

```bash
GET /transfer/history?limit=50
Authorization: Bearer <token>
```

Returns both sent and received transfers:

```json
{
  "success": true,
  "data": [
    {
      "id": "...",
      "direction": "SENT",
      "other_user_id": "...",
      "amount": "250.50",
      "currency": "EUR",
      "status": "COMPLETED",
      "created_at": "2025-11-08T15:42:06.931947Z"
    }
  ]
}
```

### Credit System Endpoints

#### Request Credit (with LLM Evaluation)

```bash
POST /credit/request
Authorization: Bearer <token>
Content-Type: application/json

{
  "amount": "5000.00",
  "currency": "EUR",
  "duration_months": 12,
  "purpose": "Home renovation"
}
```

**Response (if APPROVED):**

```json
{
  "success": true,
  "data": {
    "id": "550e8400-e29b-41d4-a716-446655440001",
    "amount": "5000",
    "currency": "EUR",
    "status": "PENDING",
    "apr": "4.5",
    "duration_months": 12,
    "risk_level": "LOW",
    "requested_at": "2025-11-14T18:31:15Z",
    "llm_response": {
      /* full LLM evaluation */
    }
  }
}
```

#### Accept Approved Credit

```bash
POST /credit/:credit_id/accept
Authorization: Bearer <token>
```

Locks collateral and disburses loan amount to wallet.

#### Make Credit Payment

```bash
POST /credit/:credit_id/payment
Authorization: Bearer <token>
```

Automatically pays the next due payment from wallet.

#### Get Credit History

```bash
GET /credit/history
Authorization: Bearer <token>
```

Returns user's credit metrics:

```json
{
  "success": true,
  "data": {
    "credits_asked": 5,
    "credits_paid": 5,
    "credits_defaulted": 0,
    "total_amount_borrowed": "20000",
    "total_amount_repaid": "20603.19",
    "on_time_payment_rate": "100",
    "active_credits_count": 0,
    "credit_score": "750.00"
  }
}
```

### Liquidity Pool Endpoints

#### Get Pool Statistics

```bash
GET /pool/:currency
```

Returns pool stats:

```json
{
  "success": true,
  "data": {
    "currency": "EUR",
    "total_liquidity": "100000.0000",
    "total_used": "5000.0000",
    "available_liquidity": "95000.0000",
    "utilization_rate": "5.00",
    "current_apy": "2.50"
  }
}
```

### Staking Endpoints

#### Create Staking Position

```bash
POST /staking/create
Authorization: Bearer <token>
Content-Type: application/json

{
  "amount": "5000.00",
  "currency": "EUR"
}
```

Creates a 1-year locked staking position with interest accrual.

#### Withdraw Staking Position

```bash
POST /staking/:position_id/withdraw
Authorization: Bearer <token>
```

Withdraws stake and returns collateral after lockup period expires.

#### Get Staking Positions

```bash
GET /staking/positions?status=ACTIVE
Authorization: Bearer <token>
```

Lists all staking positions for the user.

## Business Rules

### Transfer Constraints

1. **Atomicity**: All transfers are ACID-compliant transactions
2. **Same Currency**: Only same-currency transfers allowed (EUR→EUR, USD→USD, JPY→JPY)
3. **Sufficient Balance**: Transfers fail if sender has insufficient funds
4. **Valid Receiver**: Receiver must have an active wallet
5. **No Self-Transfer**: Cannot transfer to yourself
6. **Idempotency**: Every transfer requires an `Idempotency-Key` header. A sender + key pair moves money at most once; uniqueness is enforced by a unique index on `transfers (sender_id, idempotency_key)` and the key is recorded in the same transaction that moves the money
7. **Amount Precision**: At most 4 decimals and 15 integer digits (`DECIMAL(19,4)`)

### Wallet Rules

1. **Multi-Currency**: Each user has one wallet with EUR, USD, and JPY balances
2. **Non-Negative**: Balances cannot go below zero
3. **Decimal Precision**: All amounts use `DECIMAL(19,4)` for precision
4. **Simulated Operations**: Deposits and withdrawals are simulated (no external gateway)
5. **Idempotency**: Every deposit and withdrawal requires an `Idempotency-Key` header. A user + key pair moves money at most once, across deposits and withdrawals; uniqueness is enforced by a unique index on `wallet_movements (user_id, idempotency_key)`
6. **Movement History**: Every successful deposit and withdrawal records one row in `wallet_movements` (user, type, currency, amount, key, time) in the same transaction as the balance change; a failed one records nothing. There is no API route for it: support reads the table. History starts with this table, and transfers, staking and credits do not record movements, so the movements of a wallet do not add up to its balance

### Credit System Rules

1. **LLM-Based Evaluation**: OpenAI GPT-4 evaluates creditworthiness
2. **Collateral Requirements**: Collateral locked in alternative currency (typically USD)
3. **Max Active Credits**: Users limited to 3 active credits at once
4. **No Defaulted Credits**: Users with defaults cannot request new credits
5. **Automatic Repayment**: Failed payments can be retried; automatic triggers after lockup
6. **Payment Schedule**: Amortized payments over loan duration
7. **Interest Accrual**: APR rate determined by LLM based on risk assessment

### Staking Rules

1. **Minimum Amount**: 1,000 units per staking position
2. **Lockup Period**: 1 year from position creation (non-custodial)
3. **Interest Accrual**: APY rate set at position creation
4. **Stake Increase**: Can add to position without resetting lockup
5. **Available Balance**: Calculated as total balance minus active staked amounts
6. **Liquidity Contribution**: Staked amounts added to liquidity pools

### Liquidity Pool Rules

1. **Per-Currency Pools**: Separate pools for EUR, USD, JPY
2. **Utilization Tracking**: Pool tracks total liquidity and used amounts
3. **APY Distribution**: Interest distributed via historical events
4. **Default APY**: 2% minimum APY if no historical data available
5. **Collateral Source**: Pools fund collateral requirements and credit loans

### Account Deletion

1. **Zero Balance Required**: All currency balances must be 0 to delete account
2. **Soft Delete**: Uses `deleted_at` timestamp
3. **Cascade**: Associated wallet is deleted automatically

## ACID Transaction Implementation

GoPay uses PostgreSQL transactions with pessimistic locking to guarantee atomicity:

```go
// Simplified transfer flow
tx.Begin()
  SELECT ... FROM wallets WHERE user_id = sender FOR UPDATE  // Lock sender wallet
  SELECT ... FROM wallets WHERE user_id = receiver FOR UPDATE  // Lock receiver wallet
  INSERT INTO transfers (..., idempotency_key) ...
    ON CONFLICT (sender_id, idempotency_key) DO NOTHING       // Key already used: rollback, replay original
  check sender balance                                        // Insufficient: rollback, key not registered
  UPDATE wallets SET balance = balance - amount WHERE user_id = sender
  UPDATE wallets SET balance = balance + amount WHERE user_id = receiver
tx.Commit()
```

This prevents race conditions and ensures:

- ✅ Both wallets update atomically
- ✅ No double-spending
- ✅ Consistent transfer records
- ✅ Rollback on any failure

## Security

- **Password Hashing**: bcrypt with salt
- **JWT Authentication**: Stateless, cryptographically signed tokens
- **Input Validation**: Gin validator with custom rules
- **SQL Injection Prevention**: Parameterized queries only
- **Environment Variables**: Sensitive data never hardcoded

## Development

### Running Tests

```bash
# Run all tests
go test ./...

# Run tests with coverage
go test -cover ./...

# Run tests for specific package
go test ./internal/transfer

# Run integration tests (requires Docker running)
make test-integration
```

Integration tests are tagged with `//go:build integration`, so `go test ./...` does not
compile them. `make test-integration` runs `go test -race -tags integration ./...`: it
starts a throwaway `postgres:16.3` container with testcontainers, applies every migration
in `migrate/migrations` and runs the tests against it. If Docker is not available, the
tests fail; they are never skipped.

### Database Migrations

```bash
# Create new migration
migrate create -ext sql -dir migrate/migrations -seq migration_name

# Apply migrations
make migrate-up

# Rollback migrations
make migrate-down

# Check migration version
migrate -path migrate/migrations -database $DB_DSN version
```

### Hot Reload with Air

Air watches for file changes and automatically rebuilds:

```bash
air
```

Configuration is in `.air.toml`.

## Design Principles

1. **Simplicity First**: Straightforward patterns over premature abstraction
2. **Domain-Driven Design**: Organized by business capabilities
3. **Dependency Injection**: Dependencies passed through constructors
4. **Interface-Based Repositories**: Testable, swappable data access
5. **Service Layer Validation**: Business rules in Go, not database
6. **Graceful Degradation**: Proper error handling and rollback

## Error Handling

All API responses follow a standard format:

**Success:**

```json
{
  "success": true,
  "data": { ... }
}
```

**Error:**

```json
{
  "success": false,
  "error": {
    "code": "INSUFFICIENT_BALANCE",
    "message": "Insufficient balance"
  }
}
```

Error codes include:

- `BAD_REQUEST`
- `UNAUTHORIZED`
- `FORBIDDEN`
- `NOT_FOUND`
- `CONFLICT`
- `INSUFFICIENT_BALANCE`
- `USER_ALREADY_EXISTS`
- `INVALID_TOKEN`
- `IDEMPOTENCY_KEY_REUSED`

## Performance Considerations

- **Connection Pooling**: PostgreSQL connection pool configured
- **Indexes**: Strategic indexes on frequently queried columns
- **Decimal Math**: shopspring/decimal for precision without performance penalty
- **Structured Logging**: Zap for minimal overhead logging

## Project Status & Phases

### ✅ Completed Phases

- **Phase 1**: Infrastructure setup (database, logger, response handlers)
- **Phase 2**: User authentication (JWT, registration, login)
- **Phase 3**: Wallet management (balance, deposits, withdrawals)
- **Phase 4**: P2P transfers (ACID transactions, transfer history)
- **Phase 5**: Liquidity Pools & Staking (DeFi-like features)
- **Phase 6**: Credit System with LLM (GPT-4 credit evaluation, collateral management, payments)
- **Phase 7**: Database Seeding (pre-populated test data for development)

### 🚀 Future Enhancements

- [ ] Currency conversion support (with exchange rate APIs)
- [ ] Transaction fees (configurable per operation)
- [ ] Webhooks for credit/transfer notifications
- [ ] Admin dashboard for monitoring
- [ ] Rate limiting (per-user API quotas)
- [ ] API versioning (v1, v2, backwards compatibility)
- [ ] GraphQL API alongside REST
- [ ] Microservices architecture (separate credit, pool, staking services)
- [ ] Redis caching (balances, pool stats)
- [ ] Prometheus metrics & observability
- [ ] Email notifications (credit approvals, payment reminders)
- [ ] SMS two-factor authentication
- [ ] Advanced credit scoring (historical data analysis)
- [ ] Default handling & credit recovery
- [ ] Mobile app (iOS/Android)

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Acknowledgments

- Built with ❤️ using Go and PostgreSQL
- Clean Architecture principles by Robert C. Martin
- Domain-Driven Design by Eric Evans

---

## Quick Links

- 📚 [CLAUDE.md](CLAUDE.md) - Development guidelines and architecture decisions
- 🧪 [CREDIT_API_TESTS.md](CREDIT_API_TESTS.md) - Comprehensive credit system testing guide
- 🌱 [migrate/seed/README.md](migrate/seed/README.md) - Database seeding documentation
- 🔑 **API Base URL**: `http://localhost:8080`

---

**Version**: 3.1.0
**Status**: Production-ready with clean architecture
**Last Updated**: November 15, 2025

### Recent Updates (v3.1.0)

- ✅ Major refactoring: `main.go` reduced from 174 to 56 lines (68% reduction)
- ✅ New `pkg/app/` package for reusable infrastructure (config, server lifecycle)
- ✅ Separation of concerns: generic infrastructure vs. domain-specific logic
- ✅ Improved maintainability and testability
- ✅ Enhanced code organization following "Simplicity First" principle

### Previous Updates (v3.0.0)

- ✅ Complete database seeding infrastructure (`migrate/seed/`)
- ✅ New `make seed` and `make reset-db` commands
- ✅ Pre-populated test data with 3 users and wallets
- ✅ Bug fixes for LLM response serialization/deserialization
- ✅ Account age calculation for credit evaluation
- ✅ End-to-end testing completed and verified
- ✅ Comprehensive CHANGELOG and documentation
