package flows_test

import "testing"

// authorize → capture, driven entirely through the rail0 CLI.
func TestAuthorizeCapture(t *testing.T) {
	payeeKey := env(t, "ACCOUNT_PRIVATE_KEY")
	rail0Id := createSigned(t, "authorize")
	t.Logf("created+signed payment %s", rail0Id)

	// Payee authorizes (CLI prepares, signs locally with -p, and broadcasts).
	runCLI(t, "payments", "authorize", rail0Id, "-p", payeeKey)
	pollStatus(t, rail0Id, "authorized")

	// Capture the full amount. The capture endpoint takes base units, so read
	// the payment's stored amount (base units) rather than the human create value.
	p := runCLI(t, "payments", "get", rail0Id)
	amount, _ := p["amount"].(string)
	if amount == "" {
		t.Fatalf("no amount on payment %s: %v", rail0Id, p)
	}
	runCLI(t, "payments", "capture", rail0Id, "-a", amount, "-p", payeeKey)
	pollStatus(t, rail0Id, "captured")
}
