---
name: build
description: Build one sprint of specs/plan.md tests first on feat/<change-id>, commit, and record its deviations, scope and raw red and green output in specs/sprints/<NN>.md; or run the task's single fix round when the latest audit (the PR comment, or specs/sprints/audit.md in local mode) says CHANGES REQUESTED. One sprint per session, starting in plan mode.
argument-hint: <change-id> [<NN>]
model: opus
effort: high
disable-model-invocation: true
---

# /build $0 $1

Change `$0`, sprint `$1`. The branch is `feat/$0`. Abbreviations below: `<id>` = `$0`, `<NN>` = `$1`.

## Which mode

If `git remote get-url origin` fails, there is no remote (local mode):

- **`specs/sprints/audit.md` exists and its second line is `## Audit: CHANGES REQUESTED`:** follow "Fix round (local mode)" (`$1` is ignored).
- **Otherwise:** this is a **new sprint**, and `$1` is required. Follow "New sprint".

Otherwise run `gh pr list --head feat/$0 --state open --json number`.

- **No open PR:** this is a **new sprint**, and `$1` is required. Follow "New sprint".
- **An open PR:** `/ship` has already published the task. Read its latest audit comment with `gh pr view <pr> --json comments`. Audit comments start with `<!-- harness:audit -->`.
  - `## Audit: CHANGES REQUESTED`: follow "Fix round (remote mode)" (`$1` is ignored).
  - Anything else: stop and say what state the PR is in.

## New sprint

### 1. Preconditions

Check all of these; if any fails, stop and say which and why:

- `specs/plan.md` exists and its first line is `# Plan: $0`.
- `- [ ] Sprint $1 — …` exists, unchecked, in its section 3.
- Every earlier sprint is checked (`- [x]`) and has its `specs/sprints/<MM>.md`.
- `specs/sprints/$1.md` does not exist.
- Git:
  - Sprint `01`: on `main`, `git status --porcelain` is empty, and no branch `feat/$0` exists. Only if `git remote get-url origin` succeeds: after `git fetch origin main`, HEAD equals `origin/main`.
  - Any later sprint: on `feat/$0`, and `git status --porcelain` is empty.

### 2. Read and plan

Read only:

- sections 1 and 2 of `specs/plan.md`, and sprint `$1` in section 3;
- the requirements of `specs/project-specs.md` that the non-negotiables cite;
- the previous sprint's evidence, `specs/sprints/<NN-1>.md`, if any;
- the code the sprint touches. Use the built-in `Explore` subagent if it spans more than a few files.

Record the sprint base, the commit the sprint starts from: `main` for sprint `01`, otherwise the output of `git rev-parse HEAD` now, before any change. Step 8 uses it.

Present the plan with `ExitPlanMode`: the test you will write for each `Verified by:` line, the files you will touch, and how the sprint stays inside its scope. Wait for approval.

### 3. Branch

Sprint `01` only: `git switch -c feat/$0`. The guard blocks application code (`cmd/`, `internal/`, `pkg/`, `migrate/`, `go.mod`, `go.sum`) on any other kind of branch, so this comes before the first test. `specs/plan.md` is gitignored, so it stays in place across the switch.

### 4. Red

- Write the tests named in the `Verified by:` lines first, with compiling stubs for the code they call.
- Run them. They must **fail on assertions, not on compilation**. Fix the stubs until they do.
- Keep the command and its output exactly as printed: it goes into the evidence file.

### 5. Green

- Implement the minimum that makes the tests pass, inside the sprint's scope.
- Never widen the scope to make a test pass. If a criterion cannot be met within scope, stop and say so.
- Test files merged to `main` are not weakened or deleted. Migrations merged to `main` are not edited: a schema change is a new migration pair (`NNN_<name>.up.sql` and `NNN_<name>.down.sql`) with the next number.

### 6. Verify

- Run `make check` and `make test`. Add `make test-integration` when the sprint touches a `repository.go`, a handler or `migrate/migrations/` (it needs Docker).
- When the sprint changes an endpoint, also call it with `curl` against `make docker-up`, and keep the request and the response.
- Everything must be green. Keep every command and its output exactly as printed.

### 7. Commit

- Commit the code and the tests on `feat/$0` with Conventional Commits (`feat($0): <sprint goal>`, `test($0): …`, `refactor($0): …`); small logical commits are fine. Commit before step 8, so the scope diff sees every new file.
- `specs/plan.md` and `specs/sprints/` are gitignored and never committed on their own: `/ship` publishes them. In remote mode the plan and the sprints go to the PR body; in local mode they go to the commit message of the squash merge that `/ship` runs.
- Do not push. In remote mode `/ship` pushes and opens the PR; in local mode nothing is pushed, and `/ship` runs the squash merge once Cristian says yes.

### 8. Evidence

Write `specs/sprints/$1.md` from the skeleton below. It is the proof `/audit` checks every acceptance criterion against, and Cristian checks it before the next sprint, so:

- **Deviations** comes first: everything done outside the plan (a file outside the sprint's scope, a changed signature the plan did not name, a criterion verified another way, a test added or skipped), each with its reason. Write "None." when there are none.
- **Scope:** the raw output of `git diff main...HEAD --stat`, then the full diff (`git diff <sprint base>..HEAD -- <path>`) of every test file (`*_test.go`) that exists on `main` (`git cat-file -e main:<path>`) and was edited in this sprint (it appears in `git diff --name-only <sprint base>..HEAD`). Write "No test file that exists on main was edited in this sprint." when there is none.
- Paste output **exactly as printed**: the `$ <command>` line, then its output. Never summarize, reword or reformat it, and never write "tests pass" without the output that shows it.
- A long output may lose whole lines only, marked `[… N lines cut …]`. Never cut the lines that show the result (`--- FAIL`, `--- PASS`, `ok`, `FAIL`, the HTTP status line). Never cut a test-file diff.
- **Red** holds the run from step 4. **Green** holds every run from step 6, the sprint's own tests first.

Then tick the sprint in `specs/plan.md`: `- [ ] Sprint $1` becomes `- [x] Sprint $1`. Change nothing else in the plan. Its problem, non-negotiables and other sprints are frozen once approved; a design change is a new `/propose`.

### 9. Finish

Tell Cristian:

- what the sprint delivered, and each `Verified by:` line with its result;
- the sprint's deviations, or that there were none. He checks the sprint before the next one.

End with exactly one line:

- while sprints remain: `Next: /clear, then /build $0 <NN+1>`;
- after the last sprint, in local mode: `Next: open a new session, then /audit $0`;
- after the last sprint, in remote mode: `Next: /clear, then /ship $0` (remote `/audit` needs the PR that `/ship` opens).

## Fix round (local mode)

Run this on `feat/$0`, with a clean tree, when `specs/sprints/audit.md` says `## Audit: CHANGES REQUESTED`.

1. If `specs/sprints/fix.md` already exists, stop: the task has had its one fix round, and Cristian decides.
2. Address every **blocking** finding in the "Verdict" section of `audit.md`, and every row of its evidence matrix under "Claimed only" or "Missing". Fix non-blocking findings only if they are trivial and inside the plan's scope. Existing tests stay as they are.
3. For a test the audit says is missing or does not assert its criterion, run steps 4 and 5 first. Then re-run step 6 (Verify).
4. Commit `fix($0): address audit round 1`. Do not push.
5. Write `specs/sprints/fix.md` from the fix-round skeleton below, starting with a "Deviations" section and the scope diff of step 8 (sprint base: the HEAD before step 2). Never edit `audit.md` or the sprint reports.
6. Tell Cristian that `/audit $0` runs round 2 in a new session. End with exactly this line: `Next: open a new session, then /audit $0`.

## Fix round (remote mode)

Run this on `feat/$0`, after `git switch feat/$0 && git pull origin feat/$0`. Name the branch explicitly: the sandbox cannot write `.git/config`, so `git push -u` does not record an upstream.

1. Read the PR body with `gh pr view <pr> --json body`. If it already has a `## Fix round` section, stop: the task has had its one fix round, and Cristian decides.
2. Address every **blocking** finding of the latest audit comment, and every row of its evidence matrix under "Claimed only" or "Missing". Fix non-blocking findings only if they are trivial and inside the plan's scope. Existing tests stay as they are.
3. For a test the audit says is missing or does not assert its criterion, run steps 4 and 5 first. Then re-run step 6 (Verify).
4. Append a `## Fix round` section to the PR body, from the skeleton below, keeping everything above it as it is:
   ```bash
   { gh pr view <pr> --json body -q .body; cat <<'EOF'

   ## Fix round
   …
   EOF
   } | gh pr edit <pr> --body-file -
   ```
5. Commit `fix($0): address audit round 1`, then `git push origin feat/$0`.
6. Tell Cristian that `/audit <pr>` runs round 2, and `/ship $0` comes after it. End with exactly this line: `Next: /clear, then /audit <pr>`.

## Skeleton: `specs/sprints/<NN>.md`

````markdown
# Sprint <NN> — <sprint goal>

## Deviations

- <Anything done outside the plan, with its reason. "None." when there are none.>

## Scope

```bash
$ git diff main...HEAD --stat
<output, exactly as printed>
```

<For each test file that exists on main and was edited in this sprint, or "No test file that exists on main was edited in this sprint.":>

```diff
$ git diff <sprint base>..HEAD -- <path>
<output, exactly as printed>
```

## Execution evidence (raw stdout)

### Red

```bash
$ <command run against the stubs>
<its output, exactly as printed, failing on assertions>
```

### Green

```bash
$ <command: the sprint's tests first, then make check, make test, make test-integration or curl>
<its output, exactly as printed>
```
````

## Skeleton: the fix round, appended to the PR body (remote) or written to `specs/sprints/fix.md` (local)

In local mode, `fix.md` starts with the "Deviations" and "Scope" sections of the sprint skeleton, then this table and the output.

````markdown
## Fix round

| Finding | Change | Evidence |
| ------- | ------ | -------- |
| <# and finding, as the audit names it> | <what changed, `path:line`> | <the command below that proves it> |

```bash
$ <each command re-run in step 3>
<its output, exactly as printed>
```
````
