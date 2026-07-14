#!/usr/bin/env bash
# Integration test runner.
#
# Usage:
#   cp .env.example .env && $EDITOR .env
#   ./run.sh [suite]
#
# Suites: api | go | cli | ruby | all (default)
#
# The script sources .env automatically if present.

set -euo pipefail

SUITE="${1:-all}"
ROOT="$(cd "$(dirname "$0")" && pwd)"

# ── Load .env ─────────────────────────────────────────────────────────────────
if [[ -f "$ROOT/.env" ]]; then
  set -a
  # shellcheck disable=SC1091
  source "$ROOT/.env"
  set +a
fi

# ── Colour helpers ─────────────────────────────────────────────────────────────
GREEN='\033[0;32m'; RED='\033[0;31m'; BOLD='\033[1m'; RESET='\033[0m'
pass() { echo -e "${GREEN}✓ $1${RESET}"; }
fail() { echo -e "${RED}✗ $1${RESET}"; }
header() { echo -e "\n${BOLD}━━━ $1 ━━━${RESET}"; }

FAILED=()

run_suite() {
  local name="$1"; shift
  header "$name"
  if "$@"; then
    pass "$name passed"
  else
    fail "$name FAILED"
    FAILED+=("$name")
  fi
}

# ── API (direct HTTP — no SDK) ────────────────────────────────────────────────
run_api() {
  cd "$ROOT/api"
  bundle check > /dev/null 2>&1 || bundle install --quiet
  bundle exec ruby -Itests -e '
    Dir["tests/*_test.rb"].sort.each { |f| require_relative f }
  '
}

# ── Go (rail0-go SDK) ──────────────────────────────────────────────────────────
run_go() {
  cd "$ROOT/go"
  go test ./flows/ -v -timeout 300s
}

# ── Ruby (rail0-ruby SDK) ───────────────────────────────────────────────────────
run_ruby() {
  cd "$ROOT/ruby"
  bundle check > /dev/null 2>&1 || bundle install --quiet
  # Each flow broadcasts real on-chain transactions and polls for confirmation,
  # so allow a wide ceiling (Minitest has no global timeout of its own here).
  bundle exec ruby -Iflows -e '
    Dir["flows/*_test.rb"].sort.each { |f| require_relative f }
  '
}

# ── CLI (rail0-cli binary) ─────────────────────────────────────────────────────
run_cli() {
  cd "$ROOT/cli"
  # The flows run serially and each broadcasts real on-chain transactions, then
  # polls for confirmation (the release flow also waits out authorizationExpiry),
  # so the whole suite needs well over the per-flow time — keep a wide ceiling.
  go test ./flows/ -v -timeout 900s
}

# ── Dispatch ──────────────────────────────────────────────────────────────────
case "$SUITE" in
  api)  run_suite "API"  run_api  ;;
  go)   run_suite "Go"   run_go   ;;
  cli)  run_suite "CLI"  run_cli  ;;
  ruby) run_suite "Ruby" run_ruby ;;
  all)
    run_suite "API"  run_api
    run_suite "Go"   run_go
    run_suite "Ruby" run_ruby
    run_suite "CLI"  run_cli
    ;;
  *)
    echo "Unknown suite: $SUITE  (api | go | cli | ruby | all)"
    exit 1
    ;;
esac

# ── Summary ───────────────────────────────────────────────────────────────────
echo ""
if [[ ${#FAILED[@]} -eq 0 ]]; then
  echo -e "${GREEN}${BOLD}All suites passed.${RESET}"
else
  echo -e "${RED}${BOLD}Failed suites: ${FAILED[*]}${RESET}"
  exit 1
fi
