#!/usr/bin/env bash
# Integration test runner.
#
# Usage:
#   cp .env.example .env && $EDITOR .env
#   ./run.sh [suite]
#
# Suites: api | ruby | go | python | rust | typescript | cross | all (default)
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

# ── Ruby ──────────────────────────────────────────────────────────────────────
run_ruby() {
  cd "$ROOT/ruby"
  bundle check > /dev/null 2>&1 || bundle install --quiet
  bundle exec ruby -Iflows -e '
    Dir["flows/*_test.rb"].each { |f| require_relative f }
  '
}

# ── Go ────────────────────────────────────────────────────────────────────────
run_go() {
  cd "$ROOT/go"
  go test ./flows/ -v -timeout 300s
}

# ── Python ────────────────────────────────────────────────────────────────────
run_python() {
  cd "$ROOT/python"
  if [[ ! -d .venv ]]; then
    python3 -m venv .venv
    .venv/bin/pip install -q -r requirements.txt
  fi
  .venv/bin/pytest flows/ -v
}

# ── Rust ──────────────────────────────────────────────────────────────────────
run_rust() {
  cd "$ROOT/rust"
  cargo test -- --nocapture 2>&1
}

# ── TypeScript ────────────────────────────────────────────────────────────────
run_typescript() {
  cd "$ROOT/typescript"
  if [[ ! -d node_modules ]]; then
    npm install --silent
  fi
  npm test
}

# ── Cross-SDK ─────────────────────────────────────────────────────────────────
run_cross() {
  cd "$ROOT/cross_sdk"
  go test . -v -timeout 300s
}

# ── Dispatch ──────────────────────────────────────────────────────────────────
case "$SUITE" in
  api)        run_suite "API"         run_api        ;;
  ruby)       run_suite "Ruby"        run_ruby       ;;
  go)         run_suite "Go"          run_go         ;;
  python)     run_suite "Python"      run_python     ;;
  rust)       run_suite "Rust"        run_rust       ;;
  typescript) run_suite "TypeScript"  run_typescript ;;
  cross)      run_suite "Cross-SDK"   run_cross      ;;
  all)
    run_suite "API"        run_api
    run_suite "Ruby"       run_ruby
    run_suite "Go"         run_go
    run_suite "Python"     run_python
    run_suite "Rust"       run_rust
    run_suite "TypeScript" run_typescript
    run_suite "Cross-SDK"  run_cross
    ;;
  *)
    echo "Unknown suite: $SUITE  (api | ruby | go | python | rust | typescript | cross | all)"
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
