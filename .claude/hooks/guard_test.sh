#!/usr/bin/env bash
# Tests for guard.sh. Builds a throwaway git repository under $TMPDIR, pipes
# one payload per case into the guard and asserts its exit code (0 allow,
# 2 block). Never touches the real repository.
set -uo pipefail

guard="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/guard.sh"
unset GIT_DIR GIT_WORK_TREE GIT_INDEX_FILE

repo=$(mktemp -d "${TMPDIR:-/tmp}/guard-test.XXXXXX")
# A second repository whose HEAD does not exist yet (no commits), for the G2 baseline exception.
unborn=$(mktemp -d "${TMPDIR:-/tmp}/guard-test-unborn.XXXXXX")
# A third repository on main with one commit, for the G2 local-mode squash exception.
squash=$(mktemp -d "${TMPDIR:-/tmp}/guard-test-squash.XXXXXX")
trap 'rm -rf "$repo" "$unborn" "$squash"' EXIT

g() { git -C "$repo" -c core.hooksPath=/dev/null -c commit.gpgsign=false "$@"; }
s() { git -C "$squash" -c core.hooksPath=/dev/null -c commit.gpgsign=false -c user.email=guard-test@example.com -c user.name=guard-test "$@"; }

g init -q
g symbolic-ref HEAD refs/heads/main
g config user.email guard-test@example.com
g config user.name guard-test
mkdir -p "$repo/internal/wallet" "$repo/migrate/migrations" "$repo/specs"
echo "package wallet" >"$repo/internal/wallet/service.go"
echo "CREATE TABLE wallets ();" >"$repo/migrate/migrations/001_init.up.sql"
echo "module example.com/demo" >"$repo/go.mod"
echo "# Specs" >"$repo/specs/project-specs.md"
g add -A
g commit -q -m init
g branch feat/demo
g branch chore/harness

git -C "$unborn" init -q
git -C "$unborn" symbolic-ref HEAD refs/heads/main

s init -q
s symbolic-ref HEAD refs/heads/main
s commit -q --allow-empty -m init

pass=0
fail=0

# run <G-row> <block|allow> <branch> <label> <payload>
# DIR selects another repository (default $repo); its branch is not switched.
# TAG replaces the branch in the printed line, to describe the repository state.
run() {
  local row=$1 want=$2 branch=$3 label=${4//$'\n'/ ⏎ } payload=$5 code dir=${DIR:-$repo}
  [[ $dir == "$repo" ]] && g switch -q "$branch"
  if [[ ${JQ_MISSING:-} == 1 ]]; then
    CLAUDE_PROJECT_DIR=$dir PATH=/nonexistent /bin/bash "$guard" <<<"$payload" >/dev/null 2>&1
  else
    CLAUDE_PROJECT_DIR=$dir bash "$guard" <<<"$payload" >/dev/null 2>&1
  fi
  code=$?
  local expected=0
  [[ $want == block ]] && expected=2
  if [[ $code == "$expected" ]]; then
    pass=$((pass + 1))
    printf 'PASS %s %-5s [%s] %s\n' "$row" "$want" "${TAG:-$branch}" "$label"
  else
    fail=$((fail + 1))
    printf 'FAIL %s %-5s [%s] %s (exit %s, want %s)\n' "$row" "$want" "${TAG:-$branch}" "$label" "$code" "$expected"
  fi
}

bash_case() { # <G-row> <block|allow> <branch> <command>
  run "$1" "$2" "$3" "$4" "$(jq -nc --arg c "$4" --arg d "${DIR:-$repo}" '{tool_name:"Bash",tool_input:{command:$c},cwd:$d}')"
}

file_case() { # <G-row> <block|allow> <branch> <tool> <relative path>
  run "$1" "$2" "$3" "$4 $5" "$(jq -nc --arg t "$4" --arg p "$repo/$5" --arg d "$repo" \
    '{tool_name:$t,tool_input:(if $t == "NotebookEdit" then {notebook_path:$p,new_source:"x"} else {file_path:$p,content:"x",old_string:"a",new_string:"b"} end),cwd:$d}')"
}

F=feat/demo
C=chore/harness

# G1: wiping or rolling back the local database
bash_case G1 block main 'docker compose down -v'
bash_case G1 block main 'docker compose -f docker-compose.yml down --volumes'
bash_case G1 block main 'docker-compose down -v --remove-orphans'
bash_case G1 block main 'docker volume rm gopay_postgres_data'
bash_case G1 block main 'docker volume prune -f'
bash_case G1 block main 'go build ./... && docker compose down -v'
bash_case G1 block main 'make reset-db'
bash_case G1 block main 'make migrate-down'
bash_case G1 block $F 'make docker-up && make reset-db'
bash_case G1 block main 'migrate -path=./migrate/migrations -database=$DB down'
bash_case G1 block main 'migrate -path ./migrate/migrations -database "$DB" drop -f'
# settings.json allows 'docker compose *'; the guard still blocks G1 on every branch.
bash_case G1 block $F 'docker compose down -v'
bash_case G1 block $C 'docker compose down -v'
bash_case G1 block $F 'docker compose down --volumes --remove-orphans'
bash_case G1 allow main 'docker compose down'
bash_case G1 allow main 'docker compose up -d'
bash_case G1 allow main 'make docker-down'
bash_case G1 allow main 'make migrate-up'
bash_case G1 allow main 'migrate -path=./migrate/migrations -database=$DB up'
bash_case G1 allow $F 'make test-integration'

# G2: commits on main
bash_case G2 block main "git commit -m 'feat: x'"
bash_case G2 block main 'git -C . commit -am x'
bash_case G2 block $F 'git switch main && git commit -m x'
bash_case G2 allow $F "git commit -m 'feat(demo): x'"
bash_case G2 allow $C "git commit -m 'chore: tune the guard'"
# The one exception: the first commit on main of a new repository (/explore's baseline).
DIR=$unborn bash_case G2 allow 'main, unborn HEAD' 'git add -A && git commit -m "baseline"'
bash_case G2 block main 'git add -A && git commit -m "baseline"'
# Local mode (no origin): the squash commit /ship makes while a squash merge is pending.
# A separate repository: run() would switch $repo's branch, and git switch deletes SQUASH_MSG.
touch "$squash/.git/SQUASH_MSG"
DIR=$squash TAG='main, local, squash pending' bash_case G2 allow main 'git commit -F specs/sprints/ship.txt'
DIR=$squash TAG='main, local, squash pending' bash_case G2 allow main 'git commit --file=specs/sprints/ship.txt'
# Only the message file /ship writes: any other commit on main stays blocked, even with SQUASH_MSG.
DIR=$squash TAG='main, local, squash pending' bash_case G2 block main "git commit -m 'feat: x'"
DIR=$squash TAG='main, local, squash pending' bash_case G2 block main 'git commit'
DIR=$squash TAG='main, local, squash pending' bash_case G2 block main 'git commit -F notes.txt'
DIR=$squash TAG='main, local, squash pending' bash_case G2 block main 'git commit -F specs/sprints/01.md'
DIR=$squash TAG='main, local, squash pending' bash_case G2 block main 'git commit -a -F specs/sprints/ship.txt'
DIR=$squash TAG='main, local, squash pending' bash_case G2 block main 'git commit --amend -F specs/sprints/ship.txt'
s remote add origin https://example.com/demo.git
DIR=$squash TAG='main, origin, squash pending' bash_case G2 block main 'git commit -F specs/sprints/ship.txt'
s remote remove origin
rm -f "$squash/.git/SQUASH_MSG"
DIR=$squash TAG='main, local, no squash' bash_case G2 block main 'git commit -F specs/sprints/ship.txt'

# G3: pushes to main, force pushes and remote deletions
bash_case G3 block $F 'git push origin main'
bash_case G3 block $F 'git push origin HEAD:main'
bash_case G3 block $F 'git push origin feat/demo:refs/heads/main'
bash_case G3 block $F 'git push --force origin feat/demo'
bash_case G3 block $F 'git push -f'
bash_case G3 block $F 'git push --force-with-lease origin feat/demo'
bash_case G3 block $F 'git push origin +feat/demo'
bash_case G3 block $F 'git push origin --delete feat/demo'
bash_case G3 block main 'git push'
bash_case G3 block main 'git push -u origin HEAD'
bash_case G3 allow $F 'git push -u origin feat/demo'
bash_case G3 allow $F 'git push origin feat/demo'
bash_case G3 allow $C 'git push -u origin chore/harness'
# gh repo create --push, in any argument order, on any branch: the first push of main is Cristian's.
bash_case G3 block main 'gh repo create gopay --private --source=. --remote=origin --push'
bash_case G3 block main 'gh repo create --push --source=. --remote=origin gopay --public'
bash_case G3 block main 'gh repo create gopay --source=. --push=true --private'
bash_case G3 block $F 'gh repo create gopay --private --source=. --remote=origin --push'
bash_case G3 block $C 'gh repo create "gopay" "--push" --source=. --private'
bash_case G3 allow main 'gh repo create gopay --private --source=. --remote=origin'
bash_case G3 allow main 'gh repo create --public --source=. --remote=origin gopay'
bash_case G3 allow $F 'gh pr create --title t --body push'

# G4: application code outside a feat/* branch
file_case G4 block main Edit internal/wallet/service.go
file_case G4 block main Write cmd/api/main.go
file_case G4 block main Write go.mod
file_case G4 block $C Edit pkg/response/response.go
bash_case G4 block main "echo 'package x' > internal/x/x.go"
bash_case G4 block main "sed -i '' 's/a/b/' go.sum"
bash_case G4 block main 'rm -rf internal/wallet'
bash_case G4 block main 'git mv internal/wallet internal/wallets'
bash_case G4 block main 'cp /tmp/x.go pkg/x.go'
bash_case G4 block $C 'printf x | tee migrate/seed/seed.sql'
file_case G4 allow $F Edit internal/wallet/service.go
file_case G4 allow $F Write cmd/api/main.go
bash_case G4 allow $F "echo 'package x' > internal/x/x.go"
file_case G4 allow main Write specs/plan.md
file_case G4 allow main Write specs/sprints/01.md
file_case G4 allow main Edit README.md
file_case G4 allow $C Edit .claude/hooks/guard.sh
bash_case G4 allow main 'cat internal/wallet/service.go'
bash_case G4 allow main 'cp internal/wallet/service.go /tmp/service.go'
bash_case G4 allow main 'go test ./internal/...'
bash_case G4 allow main 'rm -rf specs/plan.md specs/sprints/'

# G5: migrations that already exist on main
file_case G5 block $F Edit migrate/migrations/001_init.up.sql
bash_case G5 block $F 'rm migrate/migrations/001_init.up.sql'
bash_case G5 block $F "echo 'DROP TABLE wallets;' >> migrate/migrations/001_init.up.sql"
bash_case G5 block $F 'rm -rf migrate/migrations'
bash_case G5 block $F 'git mv migrate/migrations/001_init.up.sql migrate/migrations/003_init.up.sql'
file_case G5 allow $F Write migrate/migrations/002_ledger.up.sql
file_case G5 allow $F Edit migrate/seed/seed.sql
bash_case G5 allow $F 'cat migrate/migrations/001_init.up.sql'
bash_case G5 allow $F "echo 'CREATE INDEX x;' > migrate/migrations/002_index.up.sql"

# Heredoc bodies are data (PR bodies, audit comments), unless a shell reads them.
bash_case G3 allow $F $'gh pr comment 7 --body-file - <<\'EOF\'\nNever `git push --force` or `make reset-db`.\nrm -rf migrate/migrations\nEOF'
bash_case G2 allow main $'gh pr create --title t --body-file - <<EOF\ngit commit -m x\nEOF'
bash_case G1 block main $'bash <<\'EOF\'\nmake reset-db\nEOF'
bash_case G3 block $F $'cat <<\'EOF\' >/dev/null\nnotes\nEOF\ngit push --force origin feat/demo'
bash_case G3 block $F $'cat <<<notes\ngit push -f'

# The commands /ship and /build run.
bash_case G3 allow $F "{ cat specs/plan.md; for f in specs/sprints/*.md; do printf '\\n---\\n\\n'; cat \"\$f\"; done; } | gh pr create --head feat/demo --base main --title 'feat(demo): x' --body-file -"
bash_case G3 allow $F $'{ gh pr view 7 --json body -q .body; cat <<\'EOF\'\n## Fix round\nNever `git push --force`.\nEOF\n} | gh pr edit 7 --body-file -'

# G6: fail closed
run G6 block $F 'Edit without file_path' '{"tool_name":"Edit","tool_input":{"old_string":"a","new_string":"b"}}'
run G6 block $F 'NotebookEdit without notebook_path' '{"tool_name":"NotebookEdit","tool_input":{"new_source":"x"}}'
run G6 block $F 'payload that is not JSON' 'not json'
JQ_MISSING=1 run G6 block $F 'jq missing (Write README.md)' "{\"tool_name\":\"Write\",\"tool_input\":{\"file_path\":\"$repo/README.md\"}}"
JQ_MISSING=1 run G6 block $F 'jq missing (Bash ls)' '{"tool_name":"Bash","tool_input":{"command":"ls"}}'
file_case G6 allow $F Write README.md
file_case G6 allow $F NotebookEdit notebooks/explore.ipynb
bash_case G6 allow $F 'ls -la'

printf '\nguard_test.sh: %d passed, %d failed\n' "$pass" "$fail"
((fail == 0))
