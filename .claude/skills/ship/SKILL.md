---
name: ship
description: Ship a task. Update specs/project-specs.md only for a permanent architectural change. Remote mode - open the feat/<change-id> PR with specs/plan.md and specs/sprints/ as its body, delete them locally, then wait for green checks and an APPROVED audit, ask Cristian and squash-merge. Local mode - require an APPROVED specs/sprints/audit.md for HEAD, write the plan, the sprints, the audit and the regression output into the squash commit message, ask Cristian about the regression script and the merge, run the squash merge and delete them.
argument-hint: <change-id>
model: sonnet
effort: medium
disable-model-invocation: true
---

# /ship $0

You publish task `$0` and close it, only when the gates are green and Cristian says yes. In remote mode the plan and the sprints go to the PR body, and you merge the PR. In local mode they go to the commit message of the squash merge, which you run once Cristian says yes. **Never look for another way to merge** than the command in step 7 (remote mode) or the commands in L5 (local mode).

## Before anything: remote or local

Detect the mode first:

- **Remote mode:** `git remote get-url origin` succeeds and `gh auth status` succeeds. Follow "Which PR state" and everything after it, down to step 8.
- **Local mode:** `git remote get-url origin` fails, so no remote is wired. Do not stop: follow "Local mode" at the end of this file, and nothing from "Which PR state" to step 8.
- **origin exists but `gh auth status` fails:** the GitHub CLI is not logged in. Stop, tell Cristian, and give the fix: `! gh auth login`. Do nothing else.

## Which PR state

Run `gh pr list --head feat/$0 --state open --json number,url`.

- **No open PR:** follow "Open the PR", then "Gates".
- **An open PR:** the task is already published (a rerun after `/audit` or after a fix round). Go straight to "Gates".

## Open the PR

### 1. Preconditions

Check all of these; if any fails, stop and say which and why:

- `git branch --show-current` is `feat/$0`, and `git status --porcelain` prints nothing.
- `specs/plan.md` exists, its first line is `# Plan: $0`, and every line of its checklist is `- [x]`.
- `specs/sprints/<NN>.md` exists for every sprint of the checklist, and for no other.
- `make check` and `make test` are green. Run them now.

### 2. Permanent specs

`specs/project-specs.md` is the only permanent document. Update it **only** if the task made a permanent architectural change:

- a new domain under `internal/` or a new package under `pkg/`;
- a new external dependency or service;
- a new business invariant, or a change to an existing requirement;
- a migration that changes the data model: a new table, a new constraint, a column with business meaning.

Bug fixes, refactors, new tests and new endpoints inside an existing domain are not architectural: leave the file alone.

When it applies:

- Add or change requirements in the file's own format: `### Requirement: <ID> <Name>`, SHALL or MUST, and at least one `#### Scenario:` with WHEN and THEN lines. A new requirement takes the next free number of its domain prefix (`WAL-07` after `WAL-06`).
- Keep existing requirement IDs and names: plans, tests and audits cite them. Never renumber or reuse an ID.
- A known gap is never written as a requirement: it goes only under "Known gaps".
- Commit it as `docs($0): update project specs`.

### 3. Push

`git push -u origin feat/$0`

### 4. Open the PR

The body is the plan followed by every sprint's evidence, in order:

```bash
{ cat specs/plan.md; for f in specs/sprints/*.md; do printf '\n---\n\n'; cat "$f"; done; } \
  | gh pr create --head feat/$0 --base main --title "feat($0): <task goal>" --body-file -
```

The title follows Conventional Commits (`feat`, `fix`, `refactor`…) and becomes the squash commit. Confirm the PR exists with `gh pr view feat/$0 --json number,url`.

### 5. Delete the ephemeral files

Only once step 4 confirmed the PR:

```bash
rm -rf specs/plan.md specs/sprints/
```

From now on the PR body is the only copy of the plan and the evidence. If the PR was not created, delete nothing and stop.

## Gates

### 6. Checks and audit verdict

1. Run `gh pr view <pr> --json number,title,headRefName,body,comments,state`. If the PR is not open, stop.
2. If `specs/plan.md` still exists and its first line is `# Plan: $0`, an earlier run stopped before step 5. Run step 5 now.
3. Run `gh pr checks <pr> --watch`.
   - If any check is red, stop and report which one and its link.
   - If it reports no checks, the repository has no CI: say so in one line and go on.
4. Do not wait for a formal GitHub review, and never read `reviewDecision`: GitHub does not let Cristian approve his own PR. The only approval is the `/audit` comment. Read the latest PR comment whose first line is `<!-- harness:audit -->`, and match its verdict line verbatim against the "Verdict markers" table of `.claude/skills/audit/SKILL.md`:
   - `## Audit: APPROVED`: go on.
   - `## Audit: CHANGES REQUESTED`: blocked. Stop and report it. The next step is the fix round; end with exactly the line `Next: /clear, then /build $0`.
   - `## Audit: HALTED`: blocked. Stop and report it. Cristian decides.
   - No audit comment yet, or a verdict line that matches none of the three: stop and report it. The next step is the audit, then `/ship $0` again; end with exactly the line `Next: /clear, then /audit <pr>`.

### 7. Show and merge

1. Do not write a summary. Show Cristian the plan (the PR body up to its first `---` line), then the latest audit comment, then one line: `Audit: APPROVED (round N)`.
2. Ask with `AskUserQuestion`: "Merge PR #<pr> into main?"
3. Only on a yes, run `gh pr merge <pr> --squash --delete-branch`. Claude Code always prompts Cristian for this command, and that prompt is his green light. If he declines either question, stop.

### 8. Finish

1. Run `git switch main`, then `git pull`, as two separate commands.
   - If either fails with `unable to unlink old`, the sandbox stopped it from replacing a protected file under `.claude/skills/` or `.claude/hooks/`. Offer to rerun **that one command** unsandboxed, and wait for Cristian's approval. Retry nothing else.
   - Never add git commands to `sandbox.excludedCommands` to avoid this.
2. Run `git fetch --prune`.
3. Delete the local branch `feat/$0`:
   - If `git branch --list feat/$0` prints nothing, `gh pr merge --delete-branch` already deleted it. Skip this step.
   - Otherwise run `git branch -D feat/$0`.
   - Never delete `main`.
4. Report: task `$0` is merged into `main`. End with exactly this line:

   `Next: /clear, then /propose <change-id>`

## Local mode

No `origin`: there is no PR and no CI. `/audit $0` has already run in a clean session and written `specs/sprints/audit.md`, and Cristian has run the regression script that the reviewer chat built from it. The task closes with a squash merge that you run in L5, only after Cristian says yes. The only commands you run on `main` are the four of L5, in that order; never any other commit, merge or branch deletion.

### L1. Preconditions

Check all of these; if any fails, stop and say which and why:

- `git branch --show-current` is `feat/$0`, and `git status --porcelain` prints nothing.
- `specs/plan.md` exists, its first line is `# Plan: $0`, and every line of its checklist is `- [x]`.
- `specs/sprints/<NN>.md` exists for every sprint of the checklist, and for no other `<NN>`. The only other files allowed in `specs/sprints/` are `audit.md`, `fix.md`, `regression.md` and `ship.txt`.
- `specs/sprints/audit.md` exists, its first line is `<!-- harness:audit -->` and its second line is exactly `## Audit: APPROVED`, the verdict line of the "Verdict markers" table of `.claude/skills/audit/SKILL.md`. Anything else (no file, `## Audit: CHANGES REQUESTED`, `## Audit: HALTED`, any other line): stop and report it. With no file, end with exactly `Next: open a new session, then /audit $0`.
- The `**Audited commit:**` of `audit.md` equals `git rev-parse HEAD`. If it differs, the branch changed after the audit: stop, and end with exactly `Next: open a new session, then /audit $0`. Check this before L2, whose commit moves HEAD.

### L2. Permanent specs

Apply step 2 ("Permanent specs") exactly as in remote mode: update `specs/project-specs.md` **only** for a permanent architectural change, and commit it on `feat/$0` as `docs($0): update project specs`. Otherwise leave it alone.

### L3. Squash commit message

Write `specs/sprints/ship.txt`: the first line is a Conventional Commits subject for the whole task (for example `feat($0): <task goal>`), then a blank line, then `specs/plan.md`, every sprint report `specs/sprints/<NN>.md` in order, `fix.md` if it exists, `audit.md`, and `regression.md` if it exists (Cristian saves the regression script's output there), separated by `---`:

```bash
{ printf '%s\n\n' "feat($0): <task goal>"; cat specs/plan.md; for f in specs/sprints/[0-9][0-9].md specs/sprints/fix.md specs/sprints/audit.md specs/sprints/regression.md; do [ -f "$f" ] || continue; printf '\n---\n\n'; cat "$f"; done; } > specs/sprints/ship.txt
```

Confirm the file exists, its first line is the subject, and it contains the audit's `## Audit: APPROVED` line.

### L4. Gates

Run `make check`, then `make test`, and paste their raw output exactly, never summarized. If either is red, stop: report which one, and do not print the merge commands.

### L5. Show and merge

1. Do not write a summary. Show Cristian the subject line of `specs/sprints/ship.txt`, the plan (`specs/plan.md`), the "Verdict" section of `audit.md`, and one line: `Audit: APPROVED (round N)`, plus `Regression: specs/sprints/regression.md included` or `Regression: no regression.md`.
2. Ask with `AskUserQuestion`: "Did you run the regression script built from specs/sprints/audit.md?", with "Yes, it passed" and "No". On "No", or if he says it failed, stop and change nothing: end with `Next: run the regression script, save its output to specs/sprints/regression.md, then /ship $0`. If he ran it after L3, rerun L3 so `regression.md` is in the commit message.
3. Ask with `AskUserQuestion`: "Squash-merge feat/$0 into main?" If he declines, stop and change nothing.
4. Only on a yes, run these commands, **each as its own Bash call**, in this order, and stop at the first that fails:

   ```
   git switch main
   git merge --squash feat/$0
   git commit -F specs/sprints/ship.txt
   git branch -D feat/$0
   ```

   - Separate calls are required: the guard allows `git commit` on `main` only in local mode while the squash is pending (`SQUASH_MSG` exists), and it checks each call before it runs.
   - Run the commit exactly as written: the guard (G2) accepts only `git commit -F specs/sprints/ship.txt` (the L3 file), with no other option. `-m`, `-a`, `--amend` or any other message file stay blocked on `main`.
   - If `git merge --squash` reports conflicts, stop: run nothing else and report the conflicting files. Cristian decides.
   - If a command fails with `unable to unlink old` or `cannot lock ref`, the sandbox stopped it from writing a protected file. Offer to rerun **that one command** unsandboxed, and wait for Cristian's approval. Retry nothing else.
5. Confirm with `git log --oneline -1` that the subject is the squash commit, and with `git branch --list feat/$0` that the branch is gone.

### L6. Finish

1. Only once L5 confirmed the squash commit on `main`, delete the ephemeral files:

   ```bash
   rm -rf specs/plan.md specs/sprints/
   ```

   If any L5 command failed, delete nothing and stop.
2. Report: task `$0` is merged into `main`. End with exactly this line:

   `Next: /clear, then /propose <change-id>`
