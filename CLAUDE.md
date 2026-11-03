# gopay

## Project

What this system is lives in `specs/project-specs.md`; read it before any task. The harness lives in `.claude/` (hooks and skills) and `specs/` (permanent specs and the task in flight).

## Language

Talk to Cristian in Spanish (Castilian): every reply, summary, question and interview. Files (specs, plans, sprint reports), code, comments and commit messages stay in English.

## How we work

A repository starts with `/explore`, once. After that, every piece of work is a task, closed as one squash merge into `main`:

`/propose` → `/build` per sprint (Cristian checks each sprint, `/clear` between them) → `/audit` in a clean session → Cristian runs a regression script that the reviewer chat writes from the audit report → `/ship`.

In remote mode the merge is a squash-merged PR, with the plan and the sprints as its body (there, `/ship` opens the PR before `/audit` and merges after it). In local mode it is a squash merge that `/ship` runs once Cristian says yes, with the plan, the sprints, the audit and the regression output as its commit message.

| Skill                  | Runs                                                   | Does                                                                                                                                                                                                                                                                                                                                                                                                                                             |
| ---------------------- | ------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `/explore`             | locally, once, before anything else                    | Step zero: with no `.git`, runs `git init -b main` and one `baseline` commit of the code as found. With no `origin`, offers to create a GitHub repository as `origin` (`gh repo create` without `--push`; local only is the default). Never pushes: Cristian runs the first push of `main` himself. Creates `specs/`, the `Makefile` if missing, and writes `specs/project-specs.md` from what the code does, with a stable ID per requirement                                                                                                                                                                                                 |
| `/propose <id> [idea]` | locally, on `main`                                     | Interviews Cristian and writes `specs/plan.md`: 1. Problem and Affected Areas, 2. Non-negotiables (with the decisions taken), 3. Sprint / Task Checklist. Plan-review changes become acceptance criteria; an API contract change adds a documentation item. No code, no PR, no merge                                                                                                                                                                                                                                                               |
| `/build <id> <NN>`     | locally                                                | Builds one sprint tests-first on `feat/<id>`, commits, and writes `specs/sprints/<NN>.md`: deviations, scope (`git diff main --stat` and edited test files) and raw red and green output; `/build <id>` runs the one fix round after a `CHANGES REQUESTED` audit |
| `/ship <id>`           | locally                                                | Updates `specs/project-specs.md` if the architecture changed. Remote mode: opens the PR, then waits for checks and the `## Audit: APPROVED` verdict of the latest `/audit` comment (no formal GitHub review), asks Cristian, squash-merges. Local mode: requires an approved `specs/sprints/audit.md` for HEAD, asks whether the regression script ran, puts plan, sprints, audit and `regression.md` in the squash commit message, runs the squash merge once Cristian says yes |
| `/audit <pr \| id>`    | locally, in a new session, both modes                  | Read-only towards the code. Evidence matrix: proven by raw output, claimed only, missing. Remote (`<pr>`, on `main`): verdict comment on the PR. Local (`<id>`, on `feat/<id>`): runs the gates and writes `specs/sprints/audit.md` with scope, deviations, verdict and the regression facts |

- `/explore` is an absolute prerequisite: `/propose` never runs in a repository without `specs/project-specs.md`. The `origin` remote is only needed from `/ship` on: `/explore` offers to create it when there is none, and Cristian can also wire it later.
- The mode is decided only by whether the `origin` remote exists. With it, the harness runs in remote mode. Without it, it runs in local mode: `/audit` writes `specs/sprints/audit.md` instead of a PR comment, and `/ship` closes the task with a squash merge it runs once Cristian says yes. Whoever creates `origin` (`/explore` or Cristian), the first push of `main` is always Cristian's, by hand; the guard blocks `gh repo create --push`.
- Run /clear between skills; each skill tells you the next one.
- One sprint per session.
- Work always starts in plan mode: present the plan and wait for approval before editing.
- State lives in files, not in the conversation: `specs/project-specs.md` (permanent), `specs/plan.md` and `specs/sprints/` (the task in flight), then the PR body (remote mode) or the squash commit message (local mode).

## Commands

Use these verbs. Never call the underlying tool directly when a verb exists.

| Verb                                  | Runs                                                        |
| ------------------------------------- | ----------------------------------------------------------- |
| `make check`                          | `make harness-test`, then `gofmt -l` + `go vet ./...`       |
| `make test`                           | `go test -race -count=1 ./...`                              |
| `make test-integration`               | The integration tests (`-tags integration`); needs Docker   |
| `make docker-up` / `make docker-down` | `docker compose up -d` / `docker compose down` (never `-v`) |
| `make migrate-up`                     | Applies the migrations to the local database                |
| `make harness-test`                   | Tests of the guard hook                                     |

## Specs

- `specs/project-specs.md` is what the system is today: purpose, architecture, domains, data, the business rules (invariants) every change must keep, the API surface, lifecycles, cross-domain flows, locking order, external dependencies, configuration and known gaps. `/explore` writes it once (and rewrites it only if Cristian confirms the overwrite); after that only `/ship` edits it, and only for a permanent architectural change.
- Every requirement has a stable ID (domain prefix plus number, e.g. `WAL-01`) that plans, tests and audits cite. A known gap is never a requirement: it lives only under Known gaps.
- `specs/plan.md` is the contract of the task in flight. Once approved, its problem, non-negotiables and sprints do not change: `/build` only ticks a sprint. A design change is a new `/propose`.
- `specs/plan.md` and `specs/sprints/` are ephemeral and gitignored. In remote mode, `/ship` publishes them as the PR body and deletes them locally before the task reaches `main`. In local mode, `/ship` writes them into the squash commit message and deletes them locally once the squash commit is on `main`.
- If code and spec disagree, stop and say so. Never silently pick one.

## Git

- Never commit or push on `main`. In remote mode it changes only through squash-merged PRs; in local mode, only through the squash commit `/ship` makes after `git merge --squash feat/<id>`, once Cristian says yes (the guard allows `git commit` on `main` only with no `origin`, a pending squash, and exactly `git commit -F specs/sprints/ship.txt`). There are two other exceptions, both from `/explore`: its `baseline` commit in a repository whose HEAD does not exist yet, and the `docs: add project specs` commit of `specs/project-specs.md` that Cristian runs himself afterwards. The guard allows nothing else.
- A task's branch is `feat/<id>`. Application code (`cmd/`, `internal/`, `pkg/`, `migrate/`, `go.mod`, `go.sum`) changes only there; the guard blocks it anywhere else.
- Harness maintenance (`.claude/`, `CLAUDE.md`, the harness targets of the `Makefile`) goes on `chore/*` branches. A `feat/*` task never touches it; in remote mode `/audit` checks this from the PR diff.
- Use Conventional Commits (`feat:`, `fix:`, `test:`, `docs:`, `chore:`, `refactor:`). The squash commit follows the same convention: in remote mode it is the PR title, in local mode the subject line `/ship` writes.
- Never force push. Never merge except through `/ship`.

## Testing

- Write tests first. The red run fails on assertions, not on compilation, before the implementation.
- Every sprint records the raw output of its red and green runs in `specs/sprints/<NN>.md`, pasted exactly and never summarized. Without that output, the sprint is not done.
- Test files that exist on `main` are never weakened or deleted. If one is wrong, report it.
- Migrations that exist on `main` are never edited: a schema change is a new migration pair with the next number.

## Safety

- Never wipe or roll back the local database (`docker compose down -v`, `docker volume rm`, `docker volume prune`, `make reset-db`, `make migrate-down`, `migrate … down`). That is Cristian's call, by hand.
- A hook block is final: adapt, or stop and report. Never look for another way to do the same thing.
- Never commit secrets, credentials or personal data. `.env` stays local.

## Compaction

When compacting, preserve: the change id and sprint in progress, the list of modified files, the commands that verify the sprint, and any open decision.
