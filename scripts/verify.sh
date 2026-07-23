#!/usr/bin/env bash
#
# Runs the four workflow scanners against this repository. The pipeline is
# expected to be at zero findings on all four; run this before pushing any
# change to .github/workflows/.
#
#   actionlint  syntax, expressions, shellcheck on run: blocks
#   zizmor      workflow security (injection, unpinned uses, permissions)
#   poutine     CI/CD exploitation chains
#   plumber     trust graph, governance, branch protection (reads the remote)
#
# plumber needs a GitHub token: `gh auth login`, or export GH_TOKEN.
set -euo pipefail

cd "$(dirname "$0")/.."

status=0
run() {
  local name=$1
  shift
  printf '\n\033[1m==> %s\033[0m\n' "$name"
  if "$@"; then
    printf '\033[32m    %s: clean\033[0m\n' "$name"
  else
    printf '\033[31m    %s: findings (exit %d)\033[0m\n' "$name" "$?"
    status=1
  fi
}

run actionlint actionlint
run zizmor zizmor --offline .github/workflows/
run poutine poutine analyze_local .
run plumber plumber analyze

# harden-runner is the blind spot of all four scanners: none of them reports its
# absence, so a repo can be at zero findings with no egress control anywhere.
printf '\n\033[1m==> harden-runner coverage\033[0m\n'
missing=$(grep -L harden-runner .github/workflows/*.yml || true)
if [ -n "$missing" ]; then
  printf '\033[31m    missing in:\033[0m\n%s\n' "$missing"
  status=1
else
  printf '\033[32m    every workflow hardens its runner\033[0m\n'
fi

exit "$status"
