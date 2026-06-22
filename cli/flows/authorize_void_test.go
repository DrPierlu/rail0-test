package flows_test

import "testing"

// authorize → void: open an authorization, then cancel it (returns the full
// escrow to the payer). Driven through the rail0 CLI.
func TestAuthorizeVoid(t *testing.T) {
	payeeKey := env(t, "ACCOUNT_PRIVATE_KEY")

	step(t, "create + sign (authorize mode)")
	rail0Id := createSigned(t, "authorize")

	step(t, "authorize — lock the escrow")
	runCLI(t, "payments", "authorize", rail0Id, "-p", payeeKey)
	pollStatus(t, rail0Id, "authorized")

	step(t, "void — cancel the authorization, return escrow → voided")
	runCLI(t, "payments", "void", rail0Id, "-p", payeeKey)
	pollStatus(t, rail0Id, "voided")

	step(t, "done — authorize → void complete")
}
