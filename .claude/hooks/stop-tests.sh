#!/usr/bin/env bash
# Stop hook. On feat/* branches Claude cannot finish while `make test` is red.
# Exit 0 lets Claude stop; exit 2 sends the failure back to Claude.
set -uo pipefail

project_dir=${CLAUDE_PROJECT_DIR:-$(pwd)}
branch=$(git -C "$project_dir" branch --show-current 2>/dev/null || true)
[[ $branch == feat/* ]] || exit 0

payload=$(cat)
if command -v jq >/dev/null 2>&1; then
  active=$(jq -r '.stop_hook_active // false' <<<"$payload" 2>/dev/null)
else
  active=false
  grep -Eq '"stop_hook_active"[[:space:]]*:[[:space:]]*true' <<<"$payload" && active=true
fi
# Already continuing because of this hook: let Claude stop (no endless loop).
[[ $active == true ]] && exit 0

out=$(make -C "$project_dir" test 2>&1) && exit 0
{
  tail -n 30 <<<"$out"
  printf '\n`make test` is failing. Fix it before finishing, or stop and explain why.\n'
} >&2
exit 2
