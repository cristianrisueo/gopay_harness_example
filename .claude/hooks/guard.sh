#!/usr/bin/env bash
# PreToolUse guard (Bash|Edit|Write|NotebookEdit). Blocks what must never be
# left to the model, whatever the permission rules say: exit 0 allows, exit 2
# blocks and stderr goes to Claude. The rules:
#   G1  wiping or rolling back the local database: docker compose down -v/--volumes,
#       docker volume rm/prune, make reset-db, make migrate-down, migrate down/drop.
#   G2  git commit on main, except when HEAD does not exist yet (the baseline
#       commit /explore makes in a new repository), or the local-mode squash
#       commit of /ship: no origin remote, a pending squash merge (SQUASH_MSG,
#       written by 'git merge --squash'), and exactly 'git commit -F
#       specs/sprints/ship.txt'. Any other commit on main stays blocked.
#   G3  git push to main, force pushes, remote branch deletions, and
#       'gh repo create --push' (the first push of main is Cristian's, by hand).
#   G4  writing, moving or deleting application code (cmd/, internal/, pkg/,
#       migrate/, go.mod, go.sum) outside a feat/* branch. Only /build creates
#       feat/<id>, and only once specs/plan.md exists: no code without a plan.
#   G5  writing, moving or deleting a migration under migrate/migrations/ that exists on main.
#   G6  fail closed: jq missing, or an Edit/Write/NotebookEdit call without a path.
set -uo pipefail

block() {
  printf 'BLOCKED by .claude/hooks/guard.sh [%s]\n%s\n' "$1" "$2" >&2
  printf 'This block is final: adapt, or stop and report. Do not look for another way to do the same thing.\n' >&2
  exit 2
}

# G6: without jq the payload cannot be read, so every call is blocked.
if ! command -v jq >/dev/null 2>&1; then
  block G6 "jq is not installed, so the guard cannot read the tool call. Every call is blocked until jq is on the PATH. Ask Cristian to install jq."
fi

payload=$(cat)
project_dir=${CLAUDE_PROJECT_DIR:-$(pwd)}
tool=$(jq -r '.tool_name // ""' <<<"$payload" 2>/dev/null)
cwd=$(jq -r '.cwd // ""' <<<"$payload" 2>/dev/null)
[[ -n $cwd ]] || cwd=$project_dir

current_branch=$(git -C "$project_dir" branch --show-current 2>/dev/null || true)
branch=$current_branch

main_ref=""
if git -C "$project_dir" rev-parse --verify -q refs/heads/main >/dev/null 2>&1; then
  main_ref=main
elif git -C "$project_dir" rev-parse --verify -q refs/remotes/origin/main >/dev/null 2>&1; then
  main_ref=origin/main
fi

matches() { printf '%s\n' "$1" | grep -Eq "$2"; }

# Lexically resolves . and .. without touching the filesystem.
normalize() {
  local p=$1 lead="" part parts out=()
  [[ $p == /* ]] && lead=/
  IFS=/ read -r -a parts <<<"$p"
  for part in ${parts[@]+"${parts[@]}"}; do
    case $part in
      '' | .) ;;
      ..) if ((${#out[@]} > 0)); then unset "out[$((${#out[@]} - 1))]"; fi ;;
      *) out+=("$part") ;;
    esac
  done
  local IFS=/
  printf '%s%s' "$lead" "${out[*]-}"
}

# Path relative to the project; paths outside it stay absolute.
rel_path() {
  local p=$1
  p=${p//\"/}
  p=${p//\'/}
  [[ -n $p ]] || return 0
  [[ $p == /* ]] || p=$cwd/$p
  p=$(normalize "$p")
  local root
  root=$(normalize "$project_dir")
  if [[ $p == "$root" ]]; then
    p=.
  elif [[ $p == "$root"/* ]]; then
    p=${p#"$root"/}
  fi
  printf '%s' "$p"
}

# G4 and G5: checks one path that is about to be written, moved or deleted.
check_path() {
  local p
  p=$(rel_path "$1")
  [[ -n $p ]] || return 0

  if [[ $branch != feat/* ]] && matches "$p" '^(cmd|internal|pkg|migrate)(/.*)?$|^go\.(mod|sum)$'; then
    block G4 "Writing, moving or deleting '$p' on branch '${branch:-detached HEAD}'. Application code changes only on a feat/* branch, and only /build creates one, once specs/plan.md exists.
Allowed: /propose <id> writes specs/plan.md, then /build <id> 01 switches to feat/<id>. Files outside cmd/, internal/, pkg/, migrate/, go.mod and go.sum are not affected."
  fi

  if [[ -n $main_ref ]] && matches "$p" '^migrate/migrations(/.*)?$'; then
    if git -C "$project_dir" cat-file -e "$main_ref:$p" 2>/dev/null; then
      block G5 "Writing, moving or deleting '$p', which already exists on $main_ref. Merged migrations are immutable.
Allowed: create a new migration pair (NNN_<name>.up.sql and NNN_<name>.down.sql) with the next number."
    fi
  fi
}

# Arguments that are not options (do not start with '-').
operands() {
  local w
  for w in "$@"; do
    [[ $w == -* ]] || printf '%s\n' "$w"
  done
}

check_paths_of() {
  local t
  while IFS= read -r t; do
    check_path "$t"
  done < <(operands "$@")
}

check_docker() {
  local s=" $* "
  if matches "$s" ' volume( [^ ]+)* (rm|prune)( |$)'; then
    block G1 "'$*' deletes Docker volumes, which hold the PostgreSQL data.
Allowed: 'docker compose down' (or 'make docker-down') without -v/--volumes. Deleting data is Cristian's call, by hand."
  fi
  if matches "$s" ' down( |$)' && matches "$s" ' (-v|--volumes(=[^ ]*)?|-[A-Za-z]*v[A-Za-z]*)( |$)'; then
    block G1 "'$*' deletes the stack's Docker volumes (-v/--volumes), which hold the PostgreSQL data.
Allowed: 'docker compose down' (or 'make docker-down') without -v/--volumes."
  fi
}

check_make() {
  local w
  for w in "$@"; do
    case $w in
      reset-db | migrate-down)
        block G1 "'make $w' wipes or rolls back the local database.
Allowed: 'make migrate-up' to apply the migrations. Resetting or rolling back data is Cristian's call, by hand."
        ;;
    esac
  done
}

check_migrate() {
  local w
  for w in "$@"; do
    [[ $w == -* ]] && continue
    case $w in
      down | drop)
        block G1 "'migrate $w' rolls back or drops the database schema, and its data with it.
Allowed: 'make migrate-up'. Rolling back or dropping data is Cristian's call, by hand."
        ;;
    esac
  done
}

# The squash commit message file that /ship writes in local mode (ship/SKILL.md, L3).
ship_msg_file=specs/sprints/ship.txt

# True only for the exact arguments of /ship's squash commit: -F <file>,
# --file <file> or --file=<file>, where <file> is $ship_msg_file, and nothing else.
is_ship_commit() {
  local f
  case $# in
    1) [[ $1 == --file=* ]] || return 1; f=${1#--file=} ;;
    2) [[ $1 == -F || $1 == --file ]] || return 1; f=$2 ;;
    *) return 1 ;;
  esac
  [[ $(rel_path "$f") == "$ship_msg_file" ]]
}

# G3: gh repo create --push, in any argument order. The first push of main is Cristian's.
check_gh() {
  local w args=() words=()
  for w in "$@"; do
    w=${w//\"/}
    w=${w//\'/}
    args+=("$w")
    [[ $w == -* ]] || words+=("$w")
  done
  ((${#words[@]} >= 2)) && [[ ${words[0]} == repo && ${words[1]} == create ]] || return 0
  for w in ${args[@]+"${args[@]}"}; do
    if [[ $w == --push || $w == --push=* ]]; then
      block G3 "'gh repo create $w' pushes to the new remote. The first push of main is Cristian's, by hand.
Allowed: gh repo create <name> --private|--public --source=. --remote=origin (without --push); then Cristian runs '! git push -u origin main'."
    fi
  done
}

check_git() {
  local args=("$@") i=0 sub=""
  while ((i < ${#args[@]})); do
    case ${args[i]} in
      -C | -c | --git-dir | --work-tree | --namespace) i=$((i + 2)) ;;
      -*) i=$((i + 1)) ;;
      *)
        sub=${args[i]}
        i=$((i + 1))
        break
        ;;
    esac
  done
  local rest=()
  ((i < ${#args[@]})) && rest=("${args[@]:i}")

  case $sub in
    switch | checkout)
      # Track the branch a compound command moves to (git switch main && git commit).
      local w
      for w in ${rest[@]+"${rest[@]}"}; do
        [[ $w == -* || $w == -- ]] && continue
        branch=$w
        break
      done
      ;;
    commit)
      # An unborn HEAD means a new repository: its first commit is /explore's baseline.
      if [[ $branch == main ]] && git -C "$project_dir" rev-parse --verify -q HEAD >/dev/null 2>&1; then
        # Local mode (no origin): the squash commit /ship makes after 'git merge --squash feat/<id>',
        # and only with the message file /ship writes. Any other option or message stays blocked.
        local squash_msg
        squash_msg=$(git -C "$project_dir" rev-parse --path-format=absolute --git-path SQUASH_MSG 2>/dev/null || true)
        if ! git -C "$project_dir" remote get-url origin >/dev/null 2>&1 && [[ -n $squash_msg && -f $squash_msg ]] && is_ship_commit ${rest[@]+"${rest[@]}"}; then
          return 0
        fi
        block G2 "'git commit' on main. main only changes through a squash merge: a squash-merged PR in remote mode, or, in local mode, the squash commit /ship makes after 'git merge --squash feat/<id>', exactly 'git commit -F $ship_msg_file'.
Allowed: commit on a branch (/build creates feat/<id>; harness maintenance uses git switch -c chore/<topic>)."
      fi
      ;;
    push)
      local w refspecs=()
      for w in ${rest[@]+"${rest[@]}"}; do
        case $w in
          --force | --force-with-lease | --force-with-lease=* | --force-if-includes | --mirror | --all | --delete | -d | --prune)
            block G3 "'git push $w' rewrites or deletes remote history.
Allowed: git push -u origin feat/<id> (or chore/<topic>), then merge through the PR with /ship."
            ;;
          --*) ;;
          -*)
            if matches "$w" '^-[A-Za-z]*[fd][A-Za-z]*$'; then
              block G3 "'git push $w' force-pushes or deletes a remote branch.
Allowed: git push -u origin feat/<id> (or chore/<topic>)."
            fi
            ;;
          *) refspecs+=("$w") ;;
        esac
      done
      local n=${#refspecs[@]} j
      for ((j = 1; j < n; j++)); do
        w=${refspecs[j]}
        if [[ $w == +* ]]; then
          block G3 "'$w' is a force refspec.
Allowed: git push -u origin feat/<id> (or chore/<topic>)."
        fi
        if matches "$w" '^(.*:)?(refs/heads/)?main$' || { [[ $branch == main ]] && matches "$w" '^(HEAD|@)$'; }; then
          block G3 "'git push ... $w' targets main. main only changes through squash-merged PRs.
Allowed: git push -u origin feat/<id> (or chore/<topic>), then /ship."
        fi
      done
      if ((n <= 1)) && [[ $branch == main ]]; then
        block G3 "'git push' from main pushes main. main only changes through squash-merged PRs.
Allowed: switch to your branch and git push -u origin feat/<id> (or chore/<topic>)."
      fi
      ;;
    rm | mv)
      check_paths_of ${rest[@]+"${rest[@]}"}
      ;;
  esac
}

check_segment() {
  local words=() i=0 w prog
  read -r -a words <<<"$1"
  # Skip leading variable assignments and wrappers that run their argument.
  while ((i < ${#words[@]})); do
    w=${words[i]}
    case $w in
      [A-Za-z_]*=*) i=$((i + 1)) ;;
      sudo | command | builtin | exec | nohup | time | nice | noglob | env | xargs | then | do | else | if | while | until | '!' | '{' | '(') i=$((i + 1)) ;;
      timeout) i=$((i + 2)) ;;
      *) break ;;
    esac
  done
  ((i < ${#words[@]})) || return 0
  words=("${words[@]:i}")
  prog=${words[0]//\"/}
  prog=${prog//\'/}
  prog=${prog##*/}
  local args=()
  ((${#words[@]} > 1)) && args=("${words[@]:1}")

  case $prog in
    docker | docker-compose) check_docker "${words[@]}" ;;
    make | gmake) check_make ${args[@]+"${args[@]}"} ;;
    migrate) check_migrate ${args[@]+"${args[@]}"} ;;
    git) check_git ${args[@]+"${args[@]}"} ;;
    gh) check_gh ${args[@]+"${args[@]}"} ;;
    rm | rmdir | truncate | tee | mv | unlink | shred) check_paths_of ${args[@]+"${args[@]}"} ;;
    cp | install)
      local last=""
      while IFS= read -r w; do last=$w; done < <(operands ${args[@]+"${args[@]}"})
      check_path "$last"
      ;;
    sed | gsed)
      for w in ${args[@]+"${args[@]}"}; do
        if matches "$w" '^(-[A-Za-z]*i|--in-place)'; then
          check_paths_of ${args[@]+"${args[@]}"}
          break
        fi
      done
      ;;
  esac
}

# Heredoc bodies are data (PR bodies, comments), not commands, so they are
# dropped before the checks, unless a shell reads them (bash <<EOF).
# Here-strings (<<<) are not heredocs and are left alone.
strip_heredocs() {
  local line term="" dash="" keep=0 spec cmp
  while IFS= read -r line || [[ -n $line ]]; do
    if [[ -n $term ]]; then
      cmp=$line
      [[ -n $dash ]] && cmp=${cmp#"${cmp%%[!$'\t']*}"}
      if [[ $cmp == "$term" ]]; then
        term=""
      elif ((keep)); then
        printf '%s\n' "$line"
      fi
      continue
    fi
    printf '%s\n' "$line"
    spec=$(printf '%s\n' "$line" | sed -nE "s/(^|.*[^<])<<(-?)[[:space:]]*['\"]?([A-Za-z_][A-Za-z0-9_]*)['\"]?.*/\2 \3/p")
    if [[ -n $spec ]]; then
      dash=${spec% *}
      term=${spec#* }
      keep=0
      matches "${line%%<<*}" '(^|[^[:alnum:]_.-])(ba|z|da|k)?sh[[:space:]]*$|(^|[^[:alnum:]_.-])(ba|z|da|k)?sh[[:space:]]+(-[a-z]+[[:space:]]*)*$' && keep=1
    fi
  done <<<"$1"
}

check_bash() {
  local cmd seg t
  cmd=$(strip_heredocs "$1")
  # Pass 1 splits on ; & | and newlines. Pass 2 also splits on ( ) and
  # backticks to reach commands inside subshells and substitutions.
  for pass in ';&|' ';&|()`'; do
    branch=$current_branch
    while IFS= read -r seg; do
      check_segment "$seg"
    done < <(printf '%s\n' "$cmd" | tr "$pass" '\n\n\n\n\n\n')
  done
  branch=$current_branch
  # Redirections: > file and >> file (not >&2 or 2>&1).
  while IFS= read -r t; do
    t=${t#>}
    t=${t#>}
    t=${t#"${t%%[![:space:]]*}"}
    check_path "$t"
  done < <(printf '%s\n' "$cmd" | grep -oE '>>?[[:space:]]*[^[:space:]&|;<>()]+' || true)
}

case $tool in
  Bash)
    check_bash "$(jq -r '.tool_input.command // ""' <<<"$payload")"
    ;;
  *)
    path=$(jq -r '.tool_input.file_path // .tool_input.notebook_path // ""' <<<"$payload" 2>/dev/null)
    if [[ -z $path ]]; then
      block G6 "The ${tool:-unknown} call has neither file_path nor notebook_path, so the guard cannot check what it writes (fail closed).
Allowed: call the tool with an explicit file_path (Edit/Write) or notebook_path (NotebookEdit)."
    fi
    check_path "$path"
    ;;
esac

exit 0
