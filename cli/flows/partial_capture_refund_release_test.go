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
	quarter := quarterAmount(t)
	step(t, "3a. capture quarter #1 (%s)", quarter)
	runCLI(t, "payments", "capture", rail0Id, "-a", quarter, "-p", payeeKey)
	pollStatus(t, rail0Id, "capture #1", "partially_captured")
	step(t, "3b. capture quarter #2 (%s) — half escrow remains", quarter)
	runCLI(t, "payments", "capture", rail0Id, "-a", quarter, "-p", payeeKey)
	// The 2nd capture keeps status partially_captured, so wait on the tx itself:
	// the refund below seals its nonce to the live refundable, which must already
	// include both captures.
	waitForConfirmedCount(t, rail0Id, "capture", 2)
	ok(t, "captured half (2×%s), half still in escrow", quarter)

	step(t, "4a. refund quarter #1 (%s)", quarter)
	runCLI(t, "payments", "refund", rail0Id, "-a", quarter, "-p", payeeKey)
	pollStatus(t, rail0Id, "refund #1", "partially_refunded")
	step(t, "4b. refund quarter #2 (%s) — clears the captured half; escrow remains", quarter)
	runCLI(t, "payments", "refund", rail0Id, "-a", quarter, "-p", payeeKey)
	// The captured half is now fully refunded, but the other half is still in
	// escrow, so the payment is NOT closed: status advances to partially_refunded
	// (never refunded while escrow remains, and never back to a capture state).
	res := pollStatus(t, rail0Id, "refund #2", "partially_refunded")
	ok(t, "captured half refunded, escrow still in place — status=%s", res["status"])

	step(t, "5. release remaining escrow after authorizationExpiry")
	waitForAuthorizationExpiry(t, rail0Id)
	runCLI(t, "payments", "release", rail0Id, "-p", payeeKey)
	res = pollStatus(t, rail0Id, "release", "released")
	ok(t, "released — status=%s", res["status"])
}
