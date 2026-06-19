package flows_test

import "testing"

// charge (immediate pay-through, no escrow), driven through the rail0 CLI.
func TestCharge(t *testing.T) {
	payeeKey := env(t, "PAYEE_PRIVATE_KEY")
	rail0Id := createSigned(t, "charge")
	t.Logf("created+signed charge payment %s", rail0Id)

	runCLI(t, "payments", "charge", rail0Id, "-p", payeeKey)
	pollStatus(t, rail0Id, "charged")
}
