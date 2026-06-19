package flows_test

import (
	"encoding/json"
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
	os.Exit(m.Run())
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
