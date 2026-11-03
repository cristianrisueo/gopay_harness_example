---
name: propose
description: Propose a new task. Interview Cristian, then write specs/plan.md (problem, non-negotiables, sprint checklist) for /build to follow. Run on main, in plan mode. Writes no code and opens no PR.
argument-hint: <change-id> [one-line idea]
model: opus
effort: xhigh
disable-model-invocation: true
---

# /propose $0

Change id: `$0`. Full arguments: `$ARGUMENTS` (everything after the id is the idea, possibly empty).

You turn an idea into an approved plan, cut into sprints. `specs/plan.md` is the contract every later session builds from, and it is published when the task closes, so be precise: in remote mode the plan and the sprints go to the PR body; in local mode they go to the commit message of the squash merge that `/ship` runs. Work in plan mode until the interview is done. You write no code: the guard blocks application code outside a `feat/*` branch, and only `/build` creates one.

## 1. Preconditions

Check all of these; if any fails, stop and say which and why:

- `specs/project-specs.md` exists. If it does not, the repository has not been initialized: the next step is `/explore`.
- `$0` is a kebab-case noun (`^[a-z][a-z0-9]*(-[a-z0-9]+)*$`), e.g. `wallet-limits`.
- `git branch --show-current` is `main` and `git status --porcelain` prints nothing.
- Only if `git remote get-url origin` succeeds: after `git fetch origin main`, `git rev-parse HEAD` equals `git rev-parse origin/main`. A repository with no remote yet skips this check.
- `specs/plan.md` does not exist. If it does, another task is in flight: name it (its `# Plan:` line) and stop.
- No branch `feat/$0` exists locally (`git branch --list feat/$0`), nor on origin when there is one (`git ls-remote --heads origin feat/$0`).

## 2. Read

- Read `specs/project-specs.md`: the architecture, the domains, and the requirements the system holds today.
- Map the parts of the code this idea touches with the built-in `Explore` subagent (`cmd/api/`, `internal/<domain>/`, `pkg/`, `migrate/migrations/`). Do not read the whole codebase yourself.

## 3. Interview Cristian

Use `AskUserQuestion`, one open decision at a time.

- Bring a concrete proposal and its reason, not an open question. Offer two or three real options only where there is a genuine choice, recommended one first.
- Argue: if an answer creates a contradiction or a cost Cristian may not see, say so. Do not transcribe.
- Spend the questions on what is expensive to change later: data model and migrations, money and balance invariants, concurrency, idempotency, error semantics (status and error codes in `pkg/response`). For everything else, propose a default, state it, and move on.
- Stop when every non-negotiable can be written with a SHALL/MUST, and the work can be cut into sprints whose acceptance criteria each name a test or a command.

Then present the plan with `ExitPlanMode`: the problem, the non-negotiables, and the sprints with their goals. Write nothing before it is approved.

**Changes from the plan review.** When Cristian brings changes from reviewing the plan (his feedback on `ExitPlanMode`, or what he brings from the reviewer chat before approval), add each one to the plan as an acceptance criterion of the sprint it belongs to, with its own `Verified by:` line naming the test or command that proves it. Never as a note, a decision or a comment: if a change cannot be written as a criterion that a test or command verifies, say so and ask him how it should be verified.

## 4. Write `specs/plan.md`

Copy the headings of the skeleton below exactly and replace every `<placeholder>`. The plan is the first thing Cristian reads in the PR body (remote mode) or the squash commit message (local mode), so write it in short sentences, with no jargon the reader has to look up.

- `# Plan: $0`: the change id, exactly. `/build`, `/ship` and `/audit` match on it.
- **1. Problem and Affected Areas:** two or three lines on what is wrong or missing today and who notices, then the domains, packages, endpoints, tables and migrations it touches.
- **2. Non-negotiables:** each one a SHALL/MUST statement that a test can prove.
  - Cite by ID and name every requirement of `specs/project-specs.md` the task touches, e.g. "WAL-01 Withdrawal Never Overdraws still holds".
  - Record the decisions taken in the interview, each with its reason, or "None.".
- **3. Sprint / Task Checklist:** one unchecked `- [ ] Sprint NN — <goal>` line per sprint, numbered `01`, `02`… without gaps.
  - A sprint is one coherent unit of at most about a day of work: an endpoint with its tests, a migration, a repository change.
  - Each sprint has a scope, an out of scope, and one `Verified by:` line per acceptance criterion.
  - Every criterion names the test (function and file) or the exact command that verifies it, and the requirement IDs of `specs/project-specs.md` it proves (e.g. `WAL-03`), when it proves one. "Works well" is not a criterion.
  - If the task changes the public API contract (a route, a header, a status code, an error code, a request or response body), the checklist includes a documentation item: the README, and the Postman collection or any other API collection in the repository, with a `Verified by:` line (e.g. a `grep` that shows the new header in each file).

`specs/plan.md` is gitignored on purpose: it is never committed on its own. `/ship` publishes it: in remote mode the plan and the sprints go to the PR body; in local mode they go to the commit message of the squash merge that `/ship` runs. Never `git add -f` it.

## 5. Finish

Tell Cristian, in a few lines:

- the sprints the plan defines;
- that `/build $0 01` starts the first sprint and creates `feat/$0`;
- that the plan stays local until `/ship $0` publishes it: in the PR body in remote mode, or in the commit message of the squash merge `/ship` runs in local mode;
- the flow after it: `/build $0 <NN>` per sprint (he checks each sprint, `/clear` between them), then `/audit` in a new session, then the regression script, then `/ship $0`.

End with exactly this line:

`Next: /clear, then /build $0 01`

## Skeleton: `specs/plan.md`

```markdown
# Plan: <change-id>

## 1. Problem and Affected Areas

<Two or three lines: what is wrong or missing today, and who notices.>

**Affected:** <domains, packages, endpoints, tables and migrations this touches>

## 2. Non-negotiables

- <An invariant with SHALL or MUST, e.g. "A withdrawal MUST NOT leave a negative balance, even under concurrent requests.">
- <Each project-specs.md requirement this task touches, by ID and name, e.g. "TRF-04 Transfer Idempotency Key Unique Per Sender still holds.">

**Decisions:** <the choice, and why. Write "None." when there are none.>

## 3. Sprint / Task Checklist

- [ ] Sprint 01 — <goal: what is true after this sprint that was not true before>
  - **Scope:** <the endpoint, function or migration, with its tests>
  - **Out of scope:** <what waits for a later sprint>
  - **Verified by:** <AC1 (requirement IDs, e.g. WAL-01): observable behaviour, e.g. POST /wallets/{id}/withdraw over the balance responds 422> → <test and file, e.g. TestWithdrawOverBalance in internal/wallet/handler_integration_test.go>
  - **Verified by:** <AC2: observable behaviour> → <test and file, or the exact command>
- [ ] Sprint 02 — <goal>
  - **Scope:** <…>
  - **Out of scope:** <…>
  - **Verified by:** <…>
```
