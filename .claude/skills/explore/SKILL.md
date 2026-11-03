---
name: explore
description: Step zero for a new repository. Explore the codebase through a subagent and write the permanent specs/project-specs.md from what the code actually does. When there is no .git, create a local repository with a single baseline commit of the code as found; an existing repository is left untouched. When there is no origin remote, offer to create a GitHub repository as origin (never pushes). Run once, before the first /propose. Writes no application code.
model: opus
effort: high
disable-model-invocation: true
---

# /explore

You prepare a repository for the harness. `/propose` cannot run until this has: it needs a git repository to branch from, and `specs/project-specs.md` to plan against. When there is no `origin`, step 6 offers to create a GitHub repository as `origin`; the mode of every later skill is decided only by whether `origin` exists (with it, remote mode; without it, local mode). `/explore` never pushes: the first push of `main` is always Cristian's, by hand. That file is the only permanent spec; every later plan and audit cites it, so write only what the code shows.

Every session starts in plan mode. Steps 1 to 4 only read and ask; step 5 presents the plan; steps 6 to 9 write.

## 1. Preconditions

Check all of these; if any fails, stop and say which and why:

- `specs/plan.md` does not exist.
- If this is already a git repository (`git rev-parse --is-inside-work-tree` succeeds), `git rev-parse --show-toplevel` is the project directory, not a parent of it.

Then, if `specs/project-specs.md` already exists, the repository was explored before. Ask Cristian with `AskUserQuestion` whether to overwrite it, before anything else: "Overwrite specs/project-specs.md?", with "No, stop (recommended)" first and "Yes, rewrite it from the code" second. On no, stop and remind him that changes to the specs normally go through `/ship`. On yes, go on: step 9 replaces the file entirely, and step 10 uses the commit message `docs: update project specs` instead of `docs: add project specs`.

## 2. Git state

Read it; change nothing yet.

1. **`.git` exists:** git is left exactly as it is: no init, no commit. The only change step 6 can make is adding `origin`, and only when there is none and Cristian chooses it. The one check is read-only: the harness works on `main`, so `git branch --list main` must print it; if the default branch has another name (`master`), stop and say so. Renaming it is Cristian's call.
2. **No `.git`:** step 6 runs `git init -b main` and makes one `baseline` commit of the code as found.
3. **Remote:** run `git remote get-url origin` (read-only). If it succeeds, the remote choice of step 6 is skipped entirely: never ask about it. If it fails (or there is no `.git` yet), step 6 asks.

Never run `git ls-remote`, never change an existing remote, never push.

## 3. Explore

Delegate all codebase reading to **one** built-in `Explore` subagent with the Agent tool, so the files stay out of this session. Do not read source files yourself. Give it this brief:

> Map this Go codebase and return a **dense summary**: as long as the code demands and no longer, terse lines and tables, no code excerpts, no file dumps. Cite `path:line` for every claim. Use exactly these sections, in this order; when a section does not apply, say so in one line:
> 1. **Stack:** language and Go version (`go.mod`), database, router, local infrastructure.
> 2. **Directory structure:** the entry points under `cmd/`, the packages under `internal/` and `pkg/`, the layers each domain has, and where unit and integration tests live (build tags, what they need to run).
> 3. **Domains:** one line per domain package: what it owns.
> 4. **Invariants found in code:** each rule the code enforces (a balance check, a row lock, a transaction, a database constraint, an idempotency key, an error that must never be confused with another), one line each: the rule, the `path:line` that enforces it, and the test that asserts it (`path:line`) or "no test".
> 5. **Migrations:** the tool and file naming, then one line per migration: the tables, keys and constraints it adds.
> 6. **Dependencies:** the direct dependencies in `go.mod` that shape the design, and external services.
> 7. **API surface:** every route registered in the router, one table row each: method, path, whether auth is required (and which middleware enforces it), the handler (`path:line`), and every HTTP status and error code the route can return, traced through the handler and the service errors it maps.
> 8. **Lifecycles:** every entity with a status, state or phase column or field: its states, each allowed transition with the operation that triggers it (`path:line`), and whether anything prevents a disallowed transition (a `WHERE status = …`, a check constraint, a service guard) or nothing does.
> 9. **Cross-domain flows:** how money and liquidity move between domains: for each operation that touches more than one domain or table (transfer, deposit into a pool, stake, borrow, repay, reward payout…), the balances debited and credited, in which transaction, and the rounding and precision used. Include how interest and APY are computed: the formula, the time base, when it accrues, and where (`path:line`).
> 10. **Locking order:** for each transaction, the rows or tables it locks (`SELECT … FOR UPDATE`, advisory locks, isolation level) in the order it takes them. Then state whether one global lock order exists across all transactions, and name any two transactions that lock the same rows in opposite orders.
> 11. **External dependencies:** each service or process the code calls outside its own database (price oracle, HTTP API, queue, cache, clock…): the contract (request, response, units), the timeout configured or "none", and what the code does when it fails or times out (retry, fallback, error code, nothing).
> 12. **Configuration:** every environment variable the code reads (`os.Getenv`, config structs, `docker-compose.yml`, `.env.example`): name, what it controls, default if any or "required", and where it is read (`path:line`).
> 13. **Known gaps:** facts only, no fixes: an operation that moves money with no transaction or no lock, a route with no auth that changes state, a business rule with no test, an error swallowed, a missing timeout, a status transition nothing guards.
> 14. **Open questions:** anything the code leaves ambiguous: a rule enforced in one place and not another, a test that contradicts the code, no stated purpose.
>
> Read `go.mod`, the README, the router and every handler, service and repository, the migrations, `docker-compose.yml` and any `.env.example` and, when there is history, `git log --oneline -30` (commit messages often name the rules the code was changed to enforce). Every claim must come from a file you read; write "unknown" rather than guess.

The summary and Cristian's answers are the only inputs for the rest of the skill.

If there is no application code yet, skip the subagent. Step 4 asks Cristian what the system is for, and every other section of the specs says "None yet.".

## 4. Open questions

Use `AskUserQuestion`, one at a time, only where the summary cannot answer, starting with its open questions:

- the purpose of the system, if neither the code nor the README states it;
- a rule the code enforces in one place and not in another, or that a test contradicts: show both locations and ask which one is the rule. Never silently pick one.
- a behaviour that may be a rule or a bug: ask which. A bug goes only under "Known gaps", never as a requirement.

## 5. Plan

Present the plan with `ExitPlanMode`:

- the git actions step 6 will take (`git init -b main` and the `baseline` commit), or "none: git is left untouched"; and whether step 6 will ask the remote choice (only when there is no `origin`);
- whether step 8 creates a `Makefile` (only when the project has none);
- the outline of `specs/project-specs.md`: the domains, the tables, each business rule by ID and name with one line on where the code enforces it, and one line per later section (API surface, lifecycles, cross-domain flows, locking order, external dependencies, configuration, known gaps) on what it will cover, or that it does not apply.

Write nothing before it is approved.

## 6. Git

### 6.1 Initialize

Only if step 2 found no `.git`; otherwise skip to 6.2.

1. `git init -b main`
2. One commit of the codebase as found, before steps 7 to 9 write anything: `git add -A && git commit -m "baseline"`. The guard allows this commit on `main` only because HEAD does not exist yet; any later commit on `main` is blocked.

If `git init` fails with `could not lock config file` or `Operation not permitted`, the sandbox stopped it from writing `.git/config`. Offer to rerun **that one command** unsandboxed and wait for Cristian's approval. Retry nothing else. If the commit fails because git has no `user.name` or `user.email`, stop and give Cristian the two `git config --global` commands.

### 6.2 Remote choice

The last git step. Only if `git remote get-url origin` fails, in a repository 6.1 just created or one that already existed. If `origin` exists, skip this entirely and never ask.

1. Ask with `AskUserQuestion`: "This repository has no origin remote. How should the harness run?", with "Local only (Recommended)" first and "Create a GitHub repository" second.
2. **Local only:** change nothing. The harness runs in local mode.
3. **Create a GitHub repository:**
   1. Run `gh auth status`. If it fails, tell Cristian to run `! gh auth login`, stay in local mode (create nothing), and continue with step 7.
   2. Ask with `AskUserQuestion`, in one call, for the repository name (default: the project folder name, `basename "$(git rev-parse --show-toplevel)"`, offered first as "(Recommended)"; Cristian types another with "Other") and the visibility ("Private (Recommended)" first, "Public" second).
   3. Run `gh repo create <name> --<private|public> --source=. --remote=origin`, **without `--push`**. The guard blocks `gh repo create --push` (G3).
   4. Confirm with `git remote get-url origin`. If the command failed, report its output, stay in local mode, and continue with step 7.
   5. Never push. Step 10 prints the first push of `main` for Cristian to run himself.

If `gh repo create` fails with `could not lock config file` or `Operation not permitted`, the sandbox stopped it from writing `.git/config`. Offer to rerun **that one command** unsandboxed and wait for Cristian's approval. Retry nothing else.

Never run `git ls-remote`, never change an existing remote, never push.

## 7. Create `specs/`

1. `mkdir -p specs`
2. Make sure `.gitignore` ignores the task in flight. If either line is missing, append both under this comment:
   ```
   # The task in flight: ephemeral, published by /ship (PR body or squash commit message)
   specs/plan.md
   specs/sprints/
   ```

## 8. Create Makefile (if missing)

The harness drives everything through `make` verbs: `stop-tests.sh` runs `make test`, and `/build`, `/ship` and `CLAUDE.md` use the rest. This step makes the harness plug and play in a project that has no `Makefile`.

1. Check whether a `Makefile` exists at the project root (`test -f Makefile`).
2. **If it already exists, leave it untouched** and go to step 9. Do not add, reorder or rewrite targets. If it lacks any of the targets below, list the missing ones in your report (step 10); adding them is Cristian's call.
3. **If it does not exist,** write it from the skeleton below, with **exactly** these targets and nothing else:
   - `check`: the linters and code checks: `make harness-test`, then `gofmt -l` and `go vet ./...`;
   - `test`: the unit tests, `go test -race -count=1 ./...`;
   - `test-integration`: the integration tests, `go test -tags integration ./...`;
   - `docker-up` and `docker-down`: `docker compose up -d` and `docker compose down` (never `-v`);
   - `migrate-up`: the migrations, with `golang-migrate`. Set `MIGRATIONS_PATH` to where the summary of step 3 found them;
   - `harness-test`: the guard tests, `bash .claude/hooks/guard_test.sh`.

   Recipe lines start with a tab, not spaces.

Creating the `Makefile` triggers a confirmation prompt for Cristian, because `Edit(/Makefile)` is in the `ask` list of `.claude/settings.json`. The prompt is expected: wait for his answer. If he declines, do not write the file any other way. Go on to step 9 and say in the report that the project has no `Makefile`.

```make
# The verbs the harness and Cristian use. Created by /explore.

MIGRATIONS_PATH ?= ./migrate/migrations

# ==================== Docker Commands ====================

.PHONY: docker-up
docker-up:
	@docker compose up -d

.PHONY: docker-down
docker-down:
	@docker compose down

# ==================== Database Commands ====================

.PHONY: migrate-up
migrate-up:
	@test -n "$(DATABASE_URL)" || { echo "DATABASE_URL is not set"; exit 1; }
	@migrate -path=$(MIGRATIONS_PATH) -database="$(DATABASE_URL)" up

# ==================== Test Commands ====================

.PHONY: check
check: harness-test
	@unformatted="$$(gofmt -l .)"; \
	if [ -n "$$unformatted" ]; then echo "gofmt -l lists unformatted files:"; echo "$$unformatted"; exit 1; fi
	@go vet ./...

.PHONY: test
test:
	@go test -race -count=1 ./...

.PHONY: test-integration
test-integration:
	@go test -tags integration ./...

# ==================== Harness Commands ====================

.PHONY: harness-test
harness-test:
	@bash .claude/hooks/guard_test.sh
```

## 9. Write `specs/project-specs.md`

Copy the headings of the skeleton below exactly, in this order, and replace every `<placeholder>`. Write short sentences, with no jargon the reader has to look up.

- **Only what the code shows.** Write the file from the subagent's summary of step 3 plus Cristian's answers of step 4: every line must trace back to one of them. What the code does not show stays out. Intentions belong in a future `/propose`.
- **Business rules (invariants):** one `### Requirement: <ID> <Name>` per rule the code enforces today, with SHALL or MUST and at least one `#### Scenario:` with WHEN and THEN lines.
  - Every requirement gets a stable ID: a short uppercase prefix for its domain plus a two-digit number, e.g. `WAL-01`, `TRF-03`, `USR-02`. Number each domain from `01`, in the order the requirements appear. A rule that spans domains takes the prefix of the domain that enforces it.
  - IDs and names are unique in the file and never change or get reused: plans, tests and audits cite the IDs.
  - **A known gap is never a requirement.** Write a requirement only for behaviour that is correct and intended. When the code does something wrong (a missing lock, a swallowed error, an unchecked balance, a status nothing guards), it goes only under "Known gaps", never as a requirement or a scenario, so no requirement enshrines a bug as behaviour. If you cannot tell whether a behaviour is a rule or a bug, ask Cristian in step 4.
  - Describe behaviour, not code. The `path:line` evidence goes in your report to Cristian, not in the file.
  - Prefer the rules a test asserts. A rule the code enforces with no test still goes in; say so in the report.
- **Data:** the tables and the constraints that carry business meaning, not every column.
- Cover every section to the depth the code demands. Omit a section only when it does not apply, and say so in one line. Never pad. Every claim must come from code you read, not from guesses.

## 10. Finish

Beyond the `baseline` commit of step 6.1 and the `origin` that step 6.2 may create, `/explore` commits nothing and never pushes. Now that HEAD exists, the guard blocks Claude from committing on `main`, and `/propose` needs a clean tree. Tell Cristian, in a few lines:

- the domains and the business rules found, each with where the code enforces it (`path:line`), whether a test asserts it, and any rule the code enforces without a test;
- the mode: remote (`origin` exists, or step 6.2 created it) or local (no `origin`);
- the commands to run himself, with the `!` prefix, in this order:
  ```
  ! git add .gitignore specs/project-specs.md && git commit -m "docs: add project specs"
  ! git push -u origin main
  ```
  Use `docs: update project specs` when step 1 overwrote an existing file, and add `Makefile` to the `git add` when step 8 created it. Print the push line only when `origin` exists and `main` is not on it yet (step 6.2 created `origin`, or `git rev-parse --verify -q origin/main` prints nothing): the first push of `main` is always Cristian's. Never run either command yourself;
- in local mode, that he can still wire a remote later (`git remote add origin <url>`, then `! git push -u origin main`); from then on the harness runs in remote mode.

End with exactly this line:

`Next: /clear, then /propose <change-id>`

## Skeleton: `specs/project-specs.md`

```markdown
# <project> project specification

## Purpose

<Two or three lines: what the system does, for whom, and on what stack.> This is the only permanent spec. `/ship` updates it, and only when a task changes the architecture permanently.

## Architecture

- **Entry point:** <the binaries under cmd/, one line each>
- **Domains:** <how the code is organized, e.g. one package per domain under internal/<domain>/, and the layers each one has>
- **Shared packages** (`pkg/`): <each package, a few words each>
- **Tests:** <where unit and integration tests live, their build tags, and what they need to run>

## Domains

| Domain        | Owns                                     |
| ------------- | ---------------------------------------- |
| `<package>`   | <what it is responsible for, one line>   |

## Data

- <The database, the migration tool and the file naming, e.g. PostgreSQL; migrate/migrations/NNN_<name>.{up,down}.sql applied with golang-migrate>
- <One line per table or group of tables: what it holds and the constraints that carry business meaning>
- History: <NNN name · NNN name · …>

## Business rules (invariants)

### Requirement: <ID, e.g. WAL-01> <Name, unique in this file>

<The rule, with SHALL or MUST, as the code enforces it today.>

#### Scenario: <name>

- **WHEN** <trigger>
- **THEN** <observable outcome>
- **AND** <further outcome>

## Dependencies

<The Go version (go.mod), the database, the local infrastructure (docker compose), external services, and the direct dependencies that shape the design (router, driver, decimal library, JWT…), one item each, separated by " · ".>

## API surface

| Method   | Path     | Auth     | Error codes                                             |
| -------- | -------- | -------- | ------------------------------------------------------- |
| <METHOD> | <path>   | <yes/no> | <every status and error code it returns, e.g. 400 INVALID_AMOUNT · 404 WALLET_NOT_FOUND> |

## Lifecycles

### <Entity>

- **States:** <state> · <state> · …
- **Transitions:** <from> → <to> on <operation>; <one per allowed transition>
- **Guard:** <what prevents any other transition, or "nothing does">

## Cross-domain flows

### <Flow, e.g. Stake>

<The domains involved, the balances debited and credited, in one transaction or several, and the rounding used.>

<For interest and APY: the formula in words, the time base, when it accrues and when it is paid.>

## Locking order

- <Transaction>: <lock 1> → <lock 2> → …
- **Global order:** <the one order all transactions follow, or "none", naming the transactions that lock the same rows in opposite orders>

## External dependencies

- **<Dependency>:** <the contract: what is sent, what comes back, in which units> · Timeout: <value or "none"> · On failure: <what the code does>

## Configuration

| Variable | Controls                 | Default                   |
| -------- | ------------------------ | ------------------------- |
| `<NAME>` | <what it controls>       | <value, or "required">    |

## Known gaps

- <A fact: where the code lacks a safeguard or a test. No fix.>
```
