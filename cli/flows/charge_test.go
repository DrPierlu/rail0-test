package flows_test

import "testing"

// charge (immediate pay-through, no escrow), driven through the rail0 CLI.
func TestCharge(t *testing.T) {
	payeeKey := env(t, "ACCOUNT_PRIVATE_KEY")
	desc(t, "charge (immediate pay-through, no escrow)",
		"1. Create payment + payer signature (charge mode)",
		"2. Charge — pay through to payee in one call (payee)")

	step(t, "1. create + sign (charge mode)")
	rail0Id := createSigned(t, "charge")

	step(t, "2. charge/prepare → sign → submit")
	runCLI(t, "payments", "charge", rail0Id, "-p", payeeKey)
	res := pollStatus(t, rail0Id, "charge", "charged")
	ok(t, "charged — status=%s", res["status"])
}
