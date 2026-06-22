package flows_test

import (
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
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

// setupAuth makes the test account usable: it registers the payer and payee
// wallets (idempotent — a 409 means the wallet already exists) and logs in as
// the payee. `auth login` caches the JWT to the CLI's token file, which every
// later runCLI invocation picks up automatically. The account itself
// (RAIL0_ACCOUNT_ID) must already exist on the gateway.
func setupAuth() error {
	base := envOr("RAIL0_API_URL", "http://localhost:4567")
	account := os.Getenv("RAIL0_ACCOUNT_ID")
	if account == "" {
		account = os.Getenv("ACCOUNT_ID")
	}
	if account == "" {
		return fmt.Errorf("RAIL0_ACCOUNT_ID (or ACCOUNT_ID) must be set")
	}
	for _, addr := range []string{os.Getenv("BUYER_ADDRESS"), os.Getenv("PAYEE_ADDRESS")} {
		if addr == "" {
			continue
		}
		if err := ensureWallet(base, account, addr); err != nil {
			return err
		}
	}

	out, err := exec.Command(cliBin, "--json", "--base-url", base,
		"auth", "login", "-p", os.Getenv("ACCOUNT_PRIVATE_KEY")).CombinedOutput()
	if err != nil {
		return fmt.Errorf("login: %s: %w", out, err)
	}
	return nil
}

// ensureWallet registers a wallet on the account, treating HTTP 409 (the
// account_id+address unique constraint) as "already registered" so the call is
// idempotent across reruns.
func ensureWallet(base, account, addr string) error {
	body := strings.NewReader(fmt.Sprintf(`{"address":%q}`, addr))
	req, err := http.NewRequest(http.MethodPost, base+"/accounts/"+account+"/wallets", body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusConflict {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("register wallet %s: HTTP %d: %s", addr, resp.StatusCode, b)
	}
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

// pollStatus polls `rail0 payments get <rail0Id>` until status matches one of
// expected, failing on timeout or a "failed" status.
func pollStatus(t *testing.T, rail0Id string, expected ...string) {
	t.Helper()
	set := make(map[string]bool, len(expected))
	for _, s := range expected {
		set[s] = true
	}
	deadline := time.Now().Add(pollTimeout)
	for {
		p := runCLI(t, "payments", "get", rail0Id)
		status, _ := p["status"].(string)
		t.Logf("  [poll] status=%s", status)
		if set[status] {
			return
		}
		if status == "failed" {
			t.Fatalf("payment %s failed", rail0Id)
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %v (last: %s)", expected, status)
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
	return id
}

// quarterOf returns amount/4 as a base-units string (integer division).
func quarterOf(t *testing.T, amount string) string {
	t.Helper()
	n, ok := new(big.Int).SetString(amount, 10)
	if !ok {
		t.Fatalf("quarterOf: not an integer amount: %q", amount)
	}
	return new(big.Int).Div(n, big.NewInt(4)).String()
}

// paymentAmount reads the payment's stored amount (base units).
func paymentAmount(t *testing.T, rail0Id string) string {
	t.Helper()
	amount, _ := runCLI(t, "payments", "get", rail0Id)["amount"].(string)
	if amount == "" {
		t.Fatalf("no amount on payment %s", rail0Id)
	}
	return amount
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
	wait := time.Until(time.Unix(int64(expiry), 0)) + 2*time.Second
	if wait > 0 {
		t.Logf("  waiting %s for authorizationExpiry…", wait.Round(time.Second))
		time.Sleep(wait)
	}
}
