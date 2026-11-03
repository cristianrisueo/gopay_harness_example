#!/usr/bin/env bash
# PostToolUse formatter (Edit|Write). Formats an edited Go file with gofmt,
# then runs go vet on its package when the file is inside the module.
# A missing tool is a silent skip; a gofmt or go vet failure exits 2 with
# the last 30 lines of its output.
set -uo pipefail

command -v jq >/dev/null 2>&1 || exit 0
payload=$(cat)
project_dir=${CLAUDE_PROJECT_DIR:-$(pwd)}
file=$(jq -r '.tool_input.file_path // ""' <<<"$payload" 2>/dev/null)
[[ -n $file && -f $file ]] || exit 0
rel=${file#"$project_dir"/}

fail() {
  printf '%s failed on %s:\n' "$1" "$rel" >&2
  tail -n 30 <<<"$2" >&2
  exit 2
}

case $file in
  *.go)
    command -v gofmt >/dev/null 2>&1 || exit 0
    out=$(gofmt -w "$file" 2>&1) || fail gofmt "$out"
    if [[ $rel != "$file" && -f $project_dir/go.mod ]] && command -v go >/dev/null 2>&1; then
      out=$(cd "$(dirname "$file")" && go vet . 2>&1) || fail "go vet" "$out"
    fi
    ;;
esac

exit 0
