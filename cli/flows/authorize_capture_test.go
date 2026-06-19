package flows_test

import "testing"

// authorize → capture, driven entirely through the rail0 CLI.
func TestAuthorizeCapture(t *testing.T) {
	payeeKey := env(t, "PAYEE_PRIVATE_KEY")
	rail0Id := createSigned(t, "authorize")
	t.Logf("created+signed payment %s", rail0Id)

	// Payee authorizes (CLI prepares, signs locally with -p, and broadcasts).
	runCLI(t, "payments", "authorize", rail0Id, "-p", payeeKey)
	pollStatus(t, rail0Id, "authorized")

	// Payee captures the full amount.
	runCLI(t, "payments", "capture", rail0Id, "-a", envOr("AMOUNT", "1000000"), "-p", payeeKey)
	pollStatus(t, rail0Id, "captured")
}
