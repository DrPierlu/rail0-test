package flows_test

import "testing"

// Full lifecycle authorize → capture → refund, driven entirely through the
// rail0 CLI. The CLI signs locally with -p (the payee key) and broadcasts at
// each step; the refund command runs the two-phase EIP-3009 flow internally.
func TestAuthorizeCaptureRefund(t *testing.T) {
	payeeKey := env(t, "ACCOUNT_PRIVATE_KEY")

	step(t, "create + sign (authorize mode)")
	rail0Id := createSigned(t, "authorize")

	step(t, "authorize — payee locks the escrow")
	runCLI(t, "payments", "authorize", rail0Id, "-p", payeeKey)
	pollStatus(t, rail0Id, "authorized")

	// Amounts are base units server-side; read the payment's stored amount.
	p := runCLI(t, "payments", "get", rail0Id)
	amount, _ := p["amount"].(string)
	if amount == "" {
		t.Fatalf("no amount on payment %s: %v", rail0Id, p)
	}

	step(t, "capture full amount (%s) → captured", amount)
	runCLI(t, "payments", "capture", rail0Id, "-a", amount, "-p", payeeKey)
	pollStatus(t, rail0Id, "captured")

	step(t, "refund full amount (%s) → refunded", amount)
	// Two-phase: payee signs the EIP-3009 authorization, then the unsigned tx,
	// both handled by the CLI.
	runCLI(t, "payments", "refund", rail0Id, "-a", amount, "-p", payeeKey)
	pollStatus(t, rail0Id, "refunded")

	step(t, "done — authorize → capture → refund complete")
}
