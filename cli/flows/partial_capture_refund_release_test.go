package flows_test

import "testing"

// authorize → partial capture ×2 → partial refund ×2 → release, through the
// rail0 CLI. Captures half the authorization in two steps, refunds that half in
// two steps, then releases the remaining escrow to the payer after
// authorizationExpiry.
//
// Requires a short authorization TTL on the gateway so release becomes callable
// within the test window — set POLICY_AUTHORIZATION_TTL (e.g. 30) before
// starting it; otherwise the release step waits the full TTL.
func TestPartialCaptureRefundRelease(t *testing.T) {
	payeeKey := env(t, "ACCOUNT_PRIVATE_KEY")
	desc(t, "authorize → partial capture ×2 → partial refund ×2 → release",
		"1. Create payment + payer signature",
		"2. Authorize — payee locks the escrow",
		"3. Capture half the escrow in two quarters",
		"4. Refund the captured half in two quarters",
		"5. Release the remaining escrow after authorizationExpiry")

	step(t, "1. create + sign (authorize mode)")
	rail0Id := createSigned(t, "authorize")

	step(t, "2. authorize/prepare → sign → submit")
	runCLI(t, "payments", "authorize", rail0Id, "-p", payeeKey)
	pollStatus(t, rail0Id, "authorize", "authorized")
	ok(t, "authorized")

	// Capture half the authorization in two quarters; half stays in escrow.
	// Wait on the confirmed-capture COUNT, not the payment status: both captures
	// leave the payment "partially_captured", so a status poll would not wait for
	// the 2nd to settle — and the following refund must not be signed until
	// refundableAmount has stopped moving (it seals the refund nonce).
	quarter := quarterAmount(t)
	step(t, "3a. capture quarter #1 (%s)", quarter)
	runCLI(t, "payments", "capture", rail0Id, "-a", quarter, "-p", payeeKey)
	pollOpConfirmed(t, rail0Id, "capture #1", "capture", 1)
	step(t, "3b. capture quarter #2 (%s) — half escrow remains", quarter)
	runCLI(t, "payments", "capture", rail0Id, "-a", quarter, "-p", payeeKey)
	pollOpConfirmed(t, rail0Id, "capture #2", "capture", 2)
	ok(t, "captured half (2×%s), half still in escrow", quarter)

	step(t, "4a. refund quarter #1 (%s)", quarter)
	runCLI(t, "payments", "refund", rail0Id, "-a", quarter, "-p", payeeKey)
	pollStatus(t, rail0Id, "refund #1", "partially_refunded")
	step(t, "4b. refund quarter #2 (%s)", quarter)
	runCLI(t, "payments", "refund", rail0Id, "-a", quarter, "-p", payeeKey)
	res := pollStatus(t, rail0Id, "refund #2", "refunded", "partially_refunded")
	ok(t, "captured half refunded — status=%s", res["status"])

	step(t, "5. release remaining escrow after authorizationExpiry")
	waitForAuthorizationExpiry(t, rail0Id)
	runCLI(t, "payments", "release", rail0Id, "-p", payeeKey)
	res = pollStatus(t, rail0Id, "release", "released")
	ok(t, "released — status=%s", res["status"])
}
