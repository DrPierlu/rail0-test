package flows_test

import "testing"

// authorize → capture (settle): the core authorize-capture happy path — lock the
// escrow, then capture the full amount so the funds settle to the merchant. No
// refund. Driven through the rail0 CLI.
func TestAuthorizeCapture(t *testing.T) {
	payeeKey := env(t, "ACCOUNT_PRIVATE_KEY")
	desc(t, "authorize → capture (settle)",
		"1. Create payment + payer signature",
		"2. Authorize — payee locks the escrow",
		"3. Capture the full amount → captured (funds settle to merchant)")

	step(t, "1. create + sign (authorize mode)")
	rail0Id := createSigned(t, "authorize")

	step(t, "2. authorize/prepare → sign → submit")
	runCLI(t, "payments", "authorize", rail0Id, "-p", payeeKey)
	pollStatus(t, rail0Id, "authorize", "authorized")
	ok(t, "authorized")

	amount := envOr("AMOUNT", "1.00")
	step(t, "3. capture/prepare → sign → submit (amount %s)", amount)
	runCLI(t, "payments", "capture", rail0Id, "-a", amount, "-p", payeeKey)
	res := pollStatus(t, rail0Id, "capture", "captured")
	ok(t, "captured — status=%s", res["status"])
}
