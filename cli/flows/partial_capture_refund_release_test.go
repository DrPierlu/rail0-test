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

	step(t, "create + sign (authorize mode)")
	rail0Id := createSigned(t, "authorize")

	step(t, "authorize — lock the escrow")
	runCLI(t, "payments", "authorize", rail0Id, "-p", payeeKey)
	pollStatus(t, rail0Id, "authorized")

	// Capture half the authorization in two quarters; half stays in escrow.
	quarter := quarterOf(t, paymentAmount(t, rail0Id))
	step(t, "capture #1 (quarter=%s) → partially_captured", quarter)
	runCLI(t, "payments", "capture", rail0Id, "-a", quarter, "-p", payeeKey)
	pollStatus(t, rail0Id, "partially_captured")
	step(t, "capture #2 (quarter=%s) → still partially_captured (half escrow remains)", quarter)
	runCLI(t, "payments", "capture", rail0Id, "-a", quarter, "-p", payeeKey)
	pollStatus(t, rail0Id, "partially_captured")

	step(t, "refund #1 (quarter=%s) → partially_refunded", quarter)
	runCLI(t, "payments", "refund", rail0Id, "-a", quarter, "-p", payeeKey)
	pollStatus(t, rail0Id, "partially_refunded")
	step(t, "refund #2 (quarter=%s) → captured half fully refunded", quarter)
	runCLI(t, "payments", "refund", rail0Id, "-a", quarter, "-p", payeeKey)
	pollStatus(t, rail0Id, "refunded", "partially_refunded")

	step(t, "waiting for authorizationExpiry, then release remaining escrow → released")
	waitForAuthorizationExpiry(t, rail0Id)
	runCLI(t, "payments", "release", rail0Id, "-p", payeeKey)
	pollStatus(t, rail0Id, "released")

	step(t, "done — partial capture ×2 → partial refund ×2 → release complete")
}
