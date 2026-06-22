package flows_test

import "testing"

// authorize → void: open an authorization, then cancel it (returns the full
// escrow to the payer). Driven through the rail0 CLI.
func TestAuthorizeVoid(t *testing.T) {
	payeeKey := env(t, "ACCOUNT_PRIVATE_KEY")
	rail0Id := createSigned(t, "authorize")
	t.Logf("created+signed payment %s", rail0Id)

	runCLI(t, "payments", "authorize", rail0Id, "-p", payeeKey)
	pollStatus(t, rail0Id, "authorized")

	runCLI(t, "payments", "void", rail0Id, "-p", payeeKey)
	pollStatus(t, rail0Id, "voided")
}
