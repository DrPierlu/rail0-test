package flows_test

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// End-to-end tests that drive the `rail0` CLI binary (rail0-cli) against a live
// gateway. The CLI signs locally with --private-key, so these tests only pass
// addresses/keys as flags — no SDK or signing code here.
//
// Required env (see .env.example): RAIL0_API_URL, BUYER_PRIVATE_KEY,
// BUYER_ADDRESS (payer), ACCOUNT_PRIVATE_KEY (payee), PAYEE_ADDRESS (payee
// wallet), TOKEN_SYMBOL, CHAIN_ID, AMOUNT (human decimal, e.g. "1.00").

const (
	pollTimeout  = 120 * time.Second
	pollInterval = 2 * time.Second
)

// cliBin is the path to the compiled rail0 binary, built once in TestMain.
var cliBin string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "rail0-cli-")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dir)

	cliBin = filepath.Join(dir, "rail0")
	build := exec.Command("go", "build", "-o", cliBin, ".")
	build.Dir = "../../../rail0-cli" // sibling repo: rail0-test/cli/flows → GitHub/rail0-cli
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		panic("build rail0-cli: " + err.Error())
	}

	// Payee operations (authorize/capture/…) require an authenticated session,
	// and the CLI addresses payments by rail0_id which it resolves through the
	// authenticated GET /payments?rail0_id= filter — so log in once up front.
	if err := setupAuth(); err != nil {
		panic("setup auth: " + err.Error())
	}
	os.Exit(m.Run())
}

// setupAuth makes the test account usable: it logs in as the payee, then
// registers the payer and payee wallets (idempotent — a 409 means the wallet
// already exists). Login must come first: wallet writes are authenticated
// (the gateway's ensure_account! guard), so registration needs the session
// JWT that `auth login` caches to the CLI's token file — picked up by every
// later CLI invocation. The account itself (RAIL0_ACCOUNT_ID) must already
// exist on the gateway, with the payee wallet linked so login resolves it.
func setupAuth() error {
	base := envOr("RAIL0_API_URL", "http://localhost:4567")
	account := os.Getenv("RAIL0_ACCOUNT_ID")
	if account == "" {
		account = os.Getenv("ACCOUNT_ID")
	}
	if account == "" {
		return fmt.Errorf("RAIL0_ACCOUNT_ID (or ACCOUNT_ID) must be set")
	}
	fmt.Printf("\n=== setup ===\ngateway: %s\naccount: %s\n", base, account)

	fmt.Printf("login: payee %s\n", os.Getenv("PAYEE_ADDRESS"))
	if err := loginCLI(os.Getenv("ACCOUNT_PRIVATE_KEY")); err != nil {
		return err
	}

	for _, addr := range []string{os.Getenv("BUYER_ADDRESS"), os.Getenv("PAYEE_ADDRESS")} {
		if addr == "" {
			continue
		}
		if err := ensureWallet(addr); err != nil {
			return err
		}
	}
	fmt.Printf("setup ok — logged in\n=============\n")
	return nil
}

// loginCLI runs `rail0 auth login` with the given key, caching the JWT to the
// CLI's token file (used by every later runCLI invocation). Switches the active
// session — e.g. a flow can log in as the payer for dispute operations.
func loginCLI(privateKey string) error {
	base := envOr("RAIL0_API_URL", "http://localhost:4567")
	out, err := exec.Command(cliBin, "--json", "--base-url", base,
		"auth", "login", "-p", privateKey).CombinedOutput()
	if err != nil {
		return fmt.Errorf("login: %s: %w", out, err)
	}
	return nil
}

// login is loginCLI for use inside a test (fails the test on error).
func login(t *testing.T, privateKey string) {
	t.Helper()
	if err := loginCLI(privateKey); err != nil {
		t.Fatalf("%v", err)
	}
}

// ensureWallet registers a wallet on the logged-in account via the CLI, which
// sends the cached session JWT (wallet writes are authenticated). A 409
// conflict (the account_id+address unique constraint) means the wallet is
// already registered, so the call is idempotent across reruns. Must run after
// loginCLI so the session token exists; the CLI derives the account from the
// session, so no account id is passed.
func ensureWallet(addr string) error {
	base := envOr("RAIL0_API_URL", "http://localhost:4567")
	out, err := exec.Command(cliBin, "--json", "--base-url", base,
		"wallets", "create", "--address", addr).CombinedOutput()
	if err != nil {
		if strings.Contains(string(out), "HTTP 409") {
			fmt.Printf("wallet: %s already registered\n", addr)
			return nil
		}
		return fmt.Errorf("register wallet %s: %s: %w", addr, out, err)
	}
	fmt.Printf("wallet: registered %s\n", addr)
	return nil
}

func env(t *testing.T, key string) string {
	t.Helper()
	v := os.Getenv(key)
	if v == "" {
		t.Fatalf("required env var %s is not set", key)
	}
	return v
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// runCLI runs `rail0 <args…> --json` and returns the parsed JSON object. It
// fails the test on a non-zero exit. --base-url defaults to RAIL0_API_URL.
func runCLI(t *testing.T, args ...string) map[string]any {
	t.Helper()
	full := append([]string{"--json", "--base-url", envOr("RAIL0_API_URL", "http://localhost:4567")}, args...)
	cmd := exec.Command(cliBin, full...)
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("rail0 %s\n%s\nerror: %v", strings.Join(args, " "), out, err)
	}
	var obj map[string]any
	if len(out) > 0 {
		if jerr := json.Unmarshal(out, &obj); jerr != nil {
			t.Fatalf("rail0 %s: non-JSON output: %s", strings.Join(args, " "), out)
		}
	}
	return obj
}

// step prints a clearly delimited progress marker so a terminal run reads as a
// sequence of named lifecycle phases. desc prints a boxed header with the flow
// title and the numbered plan of phases; step marks the start of a phase; ok
// records its successful outcome — mirroring the old Ruby integration suite's
// output so a terminal run reads as a clear, self-describing transcript.
func desc(t *testing.T, title string, plan ...string) {
	t.Helper()
	line := strings.Repeat("─", 70)
	// Emit each header line on its own single-line log: bin/test strips the
	// per-line "<file>.go:<line>:" prefix, leaving the box flush-left. A single
	// multi-line t.Logf would instead keep Go's continuation indentation on
	// every line but the first.
	t.Log("")
	t.Log(line)
	t.Log(title)
	t.Log(line)
	for _, p := range plan {
		t.Logf("  %s", p)
	}
	t.Log(line)
}

func step(t *testing.T, format string, args ...any) {
	t.Helper()
	t.Logf("  → "+format, args...)
}

func ok(t *testing.T, format string, args ...any) {
	t.Helper()
	t.Logf("    ✓ "+format, args...)
}

// txSummary renders the payment's embedded transactions as "operation:status"
// tokens (e.g. "authorize:submitted capture:confirmed"), so a poll line shows
// where each on-chain step stands without dumping the whole payload.
func txSummary(p map[string]any) string {
	txs, _ := p["transactions"].([]any)
	parts := make([]string, 0, len(txs))
	for _, it := range txs {
		m, _ := it.(map[string]any)
		op, _ := m["operation"].(string)
		st, _ := m["status"].(string)
		parts = append(parts, op+":"+st)
	}
	return strings.Join(parts, " ")
}

// pollStatus polls `rail0 payments get <rail0Id>` until status matches one of
// expected, failing on timeout or a "failed" status. It logs the initial state,
// every status transition, and every change in the transactions summary, so a
// terminal run shows the payment moving through its intermediate states (and,
// on a stall, exactly which transaction is stuck and at what stage).
func pollStatus(t *testing.T, rail0Id, waitingFor string, expected ...string) map[string]any {
	t.Helper()
	set := make(map[string]bool, len(expected))
	for _, s := range expected {
		set[s] = true
	}

	deadline := time.Now().Add(pollTimeout)
	var lastStatus, lastTx string
	for {
		p := runCLI(t, "payments", "get", rail0Id)
		status, _ := p["status"].(string)
		tx := txSummary(p)

		// Log on change so the transcript shows each intermediate state without
		// repeating an unchanged line every tick.
		if status != lastStatus || tx != lastTx {
			t.Logf("    [poll] %s: status=%s tx=[%s]", waitingFor, status, tx)
			lastStatus, lastTx = status, tx
		}

		if set[status] {
			return p
		}
		if status == "failed" {
			t.Fatalf("    ✗ payment %s failed (transactions: %s)", rail0Id, tx)
		}
		if time.Now().After(deadline) {
			t.Fatalf("    ✗ timed out after %s waiting for %s %v (last status=%q, transactions: %s)",
				pollTimeout, waitingFor, expected, status, tx)
		}
		time.Sleep(pollInterval)
	}
}

// waitForConfirmedCount blocks until at least n transactions for `op` are
// confirmed. Needed when an operation does not change the payment status (a
// second partial capture stays partially_captured) yet a later operation
// depends on it being settled on-chain: refund seals its EIP-3009 nonce to the
// LIVE refundable balance, so every prior capture must be confirmed before the
// refund is prepared — otherwise the nonce is stale and the refund reverts.
func waitForConfirmedCount(t *testing.T, rail0Id, op string, n int) {
	t.Helper()
	deadline := time.Now().Add(pollTimeout)
	for {
		p := runCLI(t, "payments", "get", rail0Id)
		confirmed := 0
		txs, _ := p["transactions"].([]any)
		for _, it := range txs {
			m, _ := it.(map[string]any)
			o, _ := m["operation"].(string)
			s, _ := m["status"].(string)
			if o != op {
				continue
			}
			if s == "confirmed" {
				confirmed++
			} else if s == "failed" {
				t.Fatalf("    ✗ %s transaction failed (transactions: %s)", op, txSummary(p))
			}
		}
		if confirmed >= n {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("    ✗ timed out waiting for %d confirmed %s (transactions: %s)", n, op, txSummary(p))
		}
		time.Sleep(pollInterval)
	}
}

// createSigned creates a payment and signs it with the buyer's key, returning
// the rail0_id. mode is "authorize" or "charge".
func createSigned(t *testing.T, mode string) string {
	t.Helper()
	out := runCLI(t, "payments", "create",
		"-p", env(t, "BUYER_PRIVATE_KEY"),
		"-F", env(t, "BUYER_ADDRESS"),
		"-T", env(t, "PAYEE_ADDRESS"),
		"-t", envOr("TOKEN_SYMBOL", "USDC"),
		"-a", envOr("AMOUNT", "1.00"),
		"-c", envOr("CHAIN_ID", "5042002"),
		"-m", mode,
	)
	id, _ := out["rail0_id"].(string)
	if id == "" {
		t.Fatalf("create: no rail0_id in response: %v", out)
	}
	ok(t, "payment_id=%s (%s, %s %s)", id, mode, envOr("AMOUNT", "1.00"), envOr("TOKEN_SYMBOL", "USDC"))
	return id
}

// quarterAmount returns a quarter of the configured human decimal AMOUNT (e.g.
// "1.00" → "0.25"), for partial capture/refund steps. The gateway converts the
// decimal to base units using the token's decimals.
func quarterAmount(t *testing.T) string {
	t.Helper()
	f, err := strconv.ParseFloat(envOr("AMOUNT", "1.00"), 64)
	if err != nil {
		t.Fatalf("invalid AMOUNT: %v", err)
	}
	return strconv.FormatFloat(f/4, 'f', -1, 64)
}

// waitForAuthorizationExpiry blocks until the payment's authorizationExpiry has
// passed, so release() is callable. Set POLICY_AUTHORIZATION_TTL low (e.g. 30)
// on the gateway, or this waits the full TTL.
func waitForAuthorizationExpiry(t *testing.T, rail0Id string) {
	t.Helper()
	expiry, _ := runCLI(t, "payments", "get", rail0Id)["authorization_expiry"].(float64)
	if expiry == 0 {
		t.Fatalf("no authorization_expiry on payment %s", rail0Id)
	}
	// release() is callable only after authorizationExpiry; +2s clears the edge.
	target := time.Unix(int64(expiry), 0).Add(2 * time.Second)
	if time.Until(target) <= 0 {
		return
	}
	step(t, "waiting %s for authorizationExpiry…", time.Until(target).Round(time.Second))

	const tick = 10 * time.Second
	for {
		remaining := time.Until(target)
		if remaining <= 0 {
			break
		}
		t.Logf("    [countdown] %s until authorizationExpiry", remaining.Round(time.Second))
		if remaining < tick {
			time.Sleep(remaining)
		} else {
			time.Sleep(tick)
		}
	}
	ok(t, "authorizationExpiry reached — release now callable")
}
