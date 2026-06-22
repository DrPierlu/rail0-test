package flows_test

import "testing"

// authorize → void: open an authorization, then cancel it (returns the full
// escrow to the payer). Driven through the rail0 CLI.
func TestAuthorizeVoid(t *testing.T) {
	payeeKey := env(t, "ACCOUNT_PRIVATE_KEY")
	desc(t, "authorize → void",
		"1. Create payment + payer signature",
		"2. Authorize — payee locks the escrow",
		"3. Void — cancel the authorization, return escrow to payer")

	step(t, "1. create + sign (authorize mode)")
	rail0Id := createSigned(t, "authorize")

	step(t, "2. authorize/prepare → sign → submit")
	runCLI(t, "payments", "authorize", rail0Id, "-p", payeeKey)
	pollStatus(t, rail0Id, "authorize", "authorized")
	ok(t, "authorized")

	step(t, "3. void/prepare → sign → submit")
	runCLI(t, "payments", "void", rail0Id, "-p", payeeKey)
	res := pollStatus(t, rail0Id, "void", "voided")
	ok(t, "voided — status=%s", res["status"])
}
