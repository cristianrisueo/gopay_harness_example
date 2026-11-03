---
name: audit
description: Read-only audit of a task against its plan and its raw evidence, in a new session. Remote mode - /audit <pr> on main posts one verdict comment on the feat/* PR and opens debt issues. Local mode (no origin) - /audit <change-id> on feat/<change-id> runs the gates and writes specs/sprints/audit.md with the verdict and the regression facts. Never edits code, commits or fixes anything.
argument-hint: <pr-number | change-id>
model: opus
effort: high
context: fork
disable-model-invocation: true
---

# /audit $0

## Which mode

Run `git remote get-url origin`.

- **It succeeds: remote mode.** `$0` is a PR number. Follow everything from "Remote mode" on; skip "Local mode".
- **It fails: local mode.** `$0` is a change id. Follow "Local mode" at the end of this file, and nothing from "Remote mode" to its step 9.

## Verdict markers

Both modes write the same two first lines: `<!-- harness:audit -->`, then exactly one of these verdict lines. `/ship` reads them verbatim, so never reword them:

| Verdict line                  | Means                                           |
| ----------------------------- | ----------------------------------------------- |
| `## Audit: APPROVED`          | no blocking findings: `/ship` may merge         |
| `## Audit: CHANGES REQUESTED` | blocking findings: the next step is a fix round |
| `## Audit: HALTED`            | round 3 or a contradiction: Cristian decides    |

The audit is the only approval the harness uses. GitHub does not let Cristian approve his own PR, so there is no formal review to wait for.

## Remote mode

You audit PR #$0. You are **read-only**: never edit a file, commit, push, switch branches or fix anything. Your only outputs are one PR comment and, for non-blocking debt, GitHub issues. You can use these tools:

- the file tools (Read, Glob, Grep), which see `main`;
- `git fetch origin <branch>`, `git cat-file -p <sha>:<path>`, `git cat-file -e <ref>:<path>`, `git ls-tree -r --name-only <sha> [<dir>]`;
- `gh pr view`, `gh pr diff`, `gh pr checks`, `gh pr comment`;
- `gh issue list`, `gh issue create`, `gh label create`.

The audit never shares a context with the session that built the task: it runs forked (`context: fork`), so it never sees the conversation that invoked it, and only its final report comes back. The rules you audit against are `main`'s `CLAUDE.md` and `specs/project-specs.md`, never the PR's copies.

## 1. Preconditions

Check all of these; if any fails, stop and say which and why:

- `git branch --show-current` is `main` and `git status --porcelain` prints nothing.
- After `git fetch origin main`, HEAD equals `origin/main`.
- PR #$0 is open, is not a draft, and its head branch is `feat/<change-id>`.

## 2. Read

1. `gh pr view $0 --json number,title,body,headRefName,headRefOid,isDraft,comments`
2. The change id comes from the head branch `feat/<change-id>`.
3. The PR body is the plan followed by the evidence:
   - `# Plan: <change-id>`, with "Problem and Affected Areas", "Non-negotiables" and the "Sprint / Task Checklist";
   - one `# Sprint <NN>` section per sprint, with its raw Red and Green output;
   - from round 2 on, a `## Fix round` section.
4. `gh pr diff $0` and `gh pr diff $0 --name-only`.
5. The PR's files are never checked out: run `git fetch origin <headRefName>`, then read them with `git cat-file -p <headRefOid>:<path>` and list them with `git ls-tree -r --name-only <headRefOid>`.
6. `CLAUDE.md` and `specs/project-specs.md` from the working directory (`main`), and the CI results with `gh pr checks $0`. No checks is fine: the repository may have no CI.

## 3. Round

Count the PR's previous comments that start with `<!-- harness:audit -->`. Round = that count + 1.

- **Round 1** is the full audit.
- **Round 2** only checks two things: every blocking finding and every "Claimed only" or "Missing" row of round 1 is resolved, with raw output in the `## Fix round` section, and the fix broke nothing. Anything new you see in round 2 is non-blocking.
- **Round 3 or later:** post a comment with the `<!-- harness:audit -->` marker, the verdict line `## Audit: HALTED` (see "Verdict markers") and the line `Halted: Cristian decides.`, then stop.

## 4. Evidence matrix

Make one row per sprint of the checklist and one per non-negotiable, and put each row in exactly one column:

- **Proven by raw output:** the body holds the exact output of a run that exercises exactly this row (a test that asserts it, or a `curl` whose response shows it), that run is green, and the diff contains that test or code. For a sprint, its Red run must also fail on assertions, not on compilation.
- **Claimed only:** the plan or a sprint says it is done, but no output proves it. Examples: there is no output; the output does not exercise this row; the test does not assert it; the output is summarized or reworded instead of pasted; the output shows a different command than the one claimed.
- **Missing:** neither the evidence nor the code is there.

Read the tests in the diff: a test named after a criterion that does not assert it makes the row "Claimed only". The output must be consistent with the diff (test names, packages, counts); when it is not, the row is "Claimed only".

## 5. Other checks

- The code does what the plan's problem and non-negotiables say, and every `specs/project-specs.md` requirement the plan cites still holds.
- Nothing outside the sprints' scope (their Scope and Out of scope) was changed.
- No rule of `CLAUDE.md` is broken.
- Test files that exist on `main` were not weakened: no deleted or loosened assertions, no skipped tests.
- No migration that exists on `main` was edited, moved or deleted: `git cat-file -e origin/main:<path>` for each `migrate/migrations/` path in the diff that is not new.
- The diff touches no harness file: `.claude/`, `CLAUDE.md`, or the harness targets of the `Makefile` (`check`, `test`, `harness-test`). Harness maintenance goes on `chore/*` branches.
- `specs/project-specs.md` changed only for a permanent architectural change, in its own format, without renaming existing requirements.
- No secret, credential or personal data is committed (`.env` files, tokens, keys, real customer data in `migrate/seed/`).

## 6. Classify

A finding is **blocking** only if it breaks a row of the matrix (any row not "Proven by raw output"), a non-negotiable, a `specs/project-specs.md` requirement, or a rule of `CLAUDE.md`, and it names which one. Everything else is **non-blocking**, however serious it sounds.

A contradiction between two parts of the plan, or between the plan and `specs/project-specs.md`, is not a finding. Post a comment with the `<!-- harness:audit -->` marker and the verdict line `## Audit: HALTED` (see "Verdict markers") that names both parts and ends with `Halted: Cristian decides.`, then stop.

## 7. Publish

Open the debt issues of step 8 first, so the comment can list them. Then post exactly one comment, filled from the skeleton below, with:

```
gh pr comment $0 --body-file - <<'EOF'
…
EOF
```

- The first line stays `<!-- harness:audit -->`.
- The second line is the verdict line, exactly as in "Verdict markers": `## Audit: APPROVED` when there are no blocking findings, so every matrix row is "Proven by raw output", and `## Audit: CHANGES REQUESTED` otherwise.
- Every filled matrix cell says why in a few words. For a proven row, name the sprint and the command whose output proves it.
- The findings table gives severity, `path:line`, the finding, and what it breaks.

## 8. Debt issues

For each non-blocking finding that calls for a code change:

1. `gh label create debt --color D93F0B --description "Non-blocking audit finding" --force`. It is idempotent.
2. Look for an open issue that already describes the same problem: `gh issue list --label debt --state open --search "<keywords>"`.
3. If none exists, `gh issue create --label debt --title "<short title>" --body "<finding, location, PR #$0>"`.

List these issues in the comment's "Debt issues" section.

## 9. Finish

End with one line: the verdict, the round, the matrix counts (proven, claimed only, missing), and the number of blocking findings and of debt issues.

## Local mode

No `origin`: there is no PR and no CI. You audit change `$0` on its branch `feat/$0`, in a new session, before `/ship`. You are **read-only towards the code**: never edit code, tests, migrations or specs, never commit, push, switch branches or fix anything. Your only output is the file `specs/sprints/audit.md` (gitignored, like the rest of `specs/sprints/`). The rules you audit against are `main`'s `CLAUDE.md` and `specs/project-specs.md`: read them with `git show main:<path>`.

### L1. Preconditions

Check all of these; if any fails, stop and say which and why:

- `git branch --show-current` is `feat/$0`, and `git status --porcelain` prints nothing.
- `specs/plan.md` exists, its first line is `# Plan: $0`, and every line of its checklist is `- [x]`.
- `specs/sprints/<NN>.md` exists for every sprint of the checklist.

### L2. Round

- **Round 1:** `specs/sprints/fix.md` does not exist.
- **Round 2:** `specs/sprints/fix.md` exists (the one fix round of `/build $0`). Check only that every blocking finding and every "Claimed only" or "Missing" row of round 1 is resolved, with raw output in `fix.md`, and that the fix broke nothing. Anything new is non-blocking.
- **Round 3:** `specs/sprints/audit.md` already says `**Round:** 2` and is not approved. Write `audit.md` with the two marker lines, `## Audit: HALTED` as the verdict line and `Halted: Cristian decides.`, then stop.

### L3. Read

1. `specs/plan.md`, every `specs/sprints/<NN>.md` in order, and `specs/sprints/fix.md` if it exists.
2. `git diff main...feat/$0` and `git diff main...feat/$0 --stat`.
3. `main`'s `CLAUDE.md` and `specs/project-specs.md` (`git show main:<path>`).
4. For the regression facts: `docker-compose.yml`, `.env.example`, the `Makefile`, `cmd/api/`, and the handlers and `routes.go` of every route the diff touches.

### L4. Gates

Run `make check`, `make test` and `make test-integration` (it needs Docker), one after the other, and keep each `$ <command>` line and its output exactly as printed. They go into `audit.md`, never summarized. A red gate is a blocking finding.

### L5. Evidence matrix

One row per acceptance criterion of the plan (every `Verified by:` line, cited as `Sprint NN AC<n>` with the requirement IDs it names), plus one per non-negotiable. Put each row in exactly one column, with the same rules as remote mode's step 4: **Proven by raw output** only when a sprint report, `fix.md` or your own gate runs hold the exact output of a green run that exercises exactly this row, and the diff contains that test or code; **Claimed only** when it is said but not proven; **Missing** when neither evidence nor code is there.

### L6. Other checks and classify

Apply remote mode's step 5 to the local diff (`git diff main...feat/$0` instead of `gh pr diff`), and its step 6 to classify findings. A contradiction between two parts of the plan, or between the plan and `specs/project-specs.md`, is a `## Audit: HALTED` verdict that names both parts. Debt goes in the "Verdict" section as non-blocking findings; there are no GitHub issues in local mode.

### L7. Write `specs/sprints/audit.md`

From the skeleton at the end of this file, in English:

- The first line is `<!-- harness:audit -->` and the second is the verdict line, exactly as in "Verdict markers". `/ship` reads them.
- **Audited commit:** the output of `git rev-parse HEAD`. `/ship` refuses an audit of an older commit.
- **Evidence matrix:** the matrix of L5, then the raw output of the L4 gates.
- **Scope:** the raw output of `git diff main...feat/$0 --stat`, then the full diff (`git diff main...feat/$0 -- <path>`) of every test file (`*_test.go`) that exists on `main` (`git cat-file -e main:<path>`) and was edited. Write "No test file that exists on main was edited." when there is none.
- **Deviations:** everything done outside the plan's scope and non-negotiables, from the diff and the sprints' own "Deviations" sections. "None." when there are none.
- **Verdict:** the verdict line again, the round, and the findings table.
- **Regression facts:** everything a script needs without access to the repository. Facts from the files of L3 only; write "unknown" rather than guess:
  - every route the diff touches: method, path, every required header (auth, `Idempotency-Key`, `Content-Type`), and a real request body that the handler accepts, plus the status and body shape of the success response and of each error the plan names;
  - how to get a token, if routes need auth: the register and login requests;
  - every table and column the diff touches (migrations and repository SQL);
  - the exact command and environment to start the API, and its port;
  - the database container name, user and database, and the `docker exec` command that opens `psql` in it;
  - every acceptance criterion with its requirement IDs, one line each.

### L8. Finish

End with one line: the verdict, the round and the matrix counts (proven, claimed only, missing). Then exactly one line:

- `## Audit: APPROVED`: `Next: build the regression script from specs/sprints/audit.md in the reviewer chat, run it, save its output to specs/sprints/regression.md, then /clear, then /ship $0`
- `## Audit: CHANGES REQUESTED`: `Next: /clear, then /build $0`
- `## Audit: HALTED`: `Next: Cristian decides`

## Skeleton: the audit comment (remote mode)

```markdown
<!-- harness:audit -->
## Audit: <APPROVED | CHANGES REQUESTED>

**PR:** #<number> · **Change:** `<change-id>` · **Round:** <1 | 2>

<Two or three lines: what was checked and why the verdict is what it is.>

### Evidence matrix

| Sprint or non-negotiable | Proven by raw output                                                  | Claimed only                                      | Missing                |
| ------------------------ | --------------------------------------------------------------------- | ------------------------------------------------- | ---------------------- |
| <Sprint 01 — goal>       | ✅ <Sprint 01, Green: `go test -run TestX ./internal/wallet/` → `ok`> |                                                   |                        |
| <Non-negotiable>         |                                                                       | ⚠️ <what is claimed, and why no output proves it> |                        |
| <Sprint 02 — goal>       |                                                                       |                                                   | ❌ <what is missing>   |

### Findings

| #   | Severity                   | Location      | Finding   | What it breaks                                                                          |
| --- | -------------------------- | ------------- | --------- | --------------------------------------------------------------------------------------- |
| 1   | <blocking \| non-blocking> | `<path:line>` | <finding> | <matrix row, non-negotiable, requirement name or CLAUDE.md rule; — when non-blocking> |

<Write "None." instead of the table when there are no findings.>

### Debt issues

- <#issue title. Write "None." when there are none.>
```

## Skeleton: `specs/sprints/audit.md` (local mode)

````markdown
<!-- harness:audit -->
## Audit: <APPROVED | CHANGES REQUESTED | HALTED>

**Change:** `<change-id>` · **Round:** <1 | 2 | 3> · **Audited commit:** `<git rev-parse HEAD>`

<Two or three lines: what was checked and why the verdict is what it is.>

## Evidence matrix

| Acceptance criterion or non-negotiable | Proven by raw output                          | Claimed only                 | Missing              |
| -------------------------------------- | --------------------------------------------- | ---------------------------- | -------------------- |
| <Sprint 01 AC1 (WAL-01): behaviour>    | ✅ <Sprint 01, Green: `go test -run TestX …`> |                              |                      |
| <Non-negotiable>                       |                                               | ⚠️ <why no output proves it> |                      |
| <Sprint 02 AC3 (TRF-02): behaviour>    |                                               |                              | ❌ <what is missing> |

### Audit gates (raw stdout)

```bash
$ make check
<output, exactly as printed>
$ make test
<output, exactly as printed>
$ make test-integration
<output, exactly as printed>
```

## Scope

```bash
$ git diff main...feat/<change-id> --stat
<output, exactly as printed>
```

<For each test file that exists on main and was edited, or "No test file that exists on main was edited.":>

```diff
$ git diff main...feat/<change-id> -- <path>
<output, exactly as printed>
```

## Deviations

- <Anything done outside the plan. "None." when there are none.>

## Verdict

**Verdict:** `## Audit: <APPROVED | CHANGES REQUESTED | HALTED>` · **Round:** <n>

| #   | Severity                   | Location      | Finding   | What it breaks                                                    |
| --- | -------------------------- | ------------- | --------- | ----------------------------------------------------------------- |
| 1   | <blocking \| non-blocking> | `<path:line>` | <finding> | <matrix row, requirement ID, CLAUDE.md rule; — when non-blocking> |

<Write "None." instead of the table when there are no findings.>

## Regression facts

### Start the API

- **Command and environment:** <e.g. `make docker-up`, or `DB_DSN=… go run ./cmd/api`>
- **Base URL:** <http://localhost:PORT>
- **Database:** container `<name>`, user `<user>`, database `<db>`; `docker exec -it <name> psql -U <user> -d <db>`

### Auth

<The register and login requests and where the token is in the response, or "No auth.">

### Routes touched

#### <METHOD> <path>

- **Headers:** <every required header>
- **Request body:** `<real JSON body>`
- **Success:** <status and body shape>
- **Errors:** <status, error code and trigger, for each error the plan names>

### Tables and columns touched

- `<table>`: <columns, constraints and indexes>

### Acceptance criteria

- <Sprint NN ACn (<requirement IDs>): observable behaviour>
````
