package flows_test

import "testing"

// Full lifecycle authorize → capture → refund, driven entirely through the
// rail0 CLI. The CLI signs locally with -p (the payee key) and broadcasts at
// each step; the refund command runs the two-phase EIP-3009 flow internally.
func TestAuthorizeCaptureRefund(t *testing.T) {
	payeeKey := env(t, "ACCOUNT_PRIVATE_KEY")
	desc(t, "authorize → capture → refund (EIP-3009)",
		"1. Create payment + payer signature",
		"2. Authorize — payee locks the escrow",
		"3. Capture full amount (payee)",
		"4. Refund full amount via EIP-3009 (payee)")

	step(t, "1. create + sign (authorize mode)")
	rail0Id := createSigned(t, "authorize")

	step(t, "2. authorize/prepare → sign → submit")
	runCLI(t, "payments", "authorize", rail0Id, "-p", payeeKey)
	pollStatus(t, rail0Id, "authorize", "authorized")
	ok(t, "authorized")

	// Amounts are human decimals (e.g. "1.00"); the gateway converts to the
	// token's base units using its decimals.
	amount := envOr("AMOUNT", "1.00")

	step(t, "3. capture/prepare → sign → submit (amount %s)", amount)
	runCLI(t, "payments", "capture", rail0Id, "-a", amount, "-p", payeeKey)
	res := pollStatus(t, rail0Id, "capture", "captured")
	ok(t, "captured — status=%s", res["status"])

	step(t, "4. refund/prepare (EIP-3009 two-phase) → sign → submit (amount %s)", amount)
	runCLI(t, "payments", "refund", rail0Id, "-a", amount, "-p", payeeKey)
	res = pollStatus(t, rail0Id, "refund", "refunded")
	ok(t, "refunded — status=%s", res["status"])
}
