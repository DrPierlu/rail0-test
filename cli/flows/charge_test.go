package flows_test

import "testing"

// charge (immediate pay-through, no escrow), driven through the rail0 CLI.
func TestCharge(t *testing.T) {
	payeeKey := env(t, "ACCOUNT_PRIVATE_KEY")

	step(t, "create + sign (charge mode)")
	rail0Id := createSigned(t, "charge")

	step(t, "charge — pay through to payee, no escrow → charged")
	runCLI(t, "payments", "charge", rail0Id, "-p", payeeKey)
	pollStatus(t, rail0Id, "charged")

	step(t, "done — charge complete")
}
