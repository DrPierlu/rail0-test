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
	rail0Id := createSigned(t, "authorize")
	t.Logf("created+signed payment %s", rail0Id)

	runCLI(t, "payments", "authorize", rail0Id, "-p", payeeKey)
	pollStatus(t, rail0Id, "authorized")

	// Capture half the authorization in two quarters; half stays in escrow.
	quarter := quarterOf(t, paymentAmount(t, rail0Id))
	runCLI(t, "payments", "capture", rail0Id, "-a", quarter, "-p", payeeKey)
	pollStatus(t, rail0Id, "partially_captured")
	runCLI(t, "payments", "capture", rail0Id, "-a", quarter, "-p", payeeKey)
	pollStatus(t, rail0Id, "partially_captured")

	// Refund the captured half in two quarters.
	runCLI(t, "payments", "refund", rail0Id, "-a", quarter, "-p", payeeKey)
	pollStatus(t, rail0Id, "partially_refunded")
	runCLI(t, "payments", "refund", rail0Id, "-a", quarter, "-p", payeeKey)
	pollStatus(t, rail0Id, "refunded", "partially_refunded")

	// Release the remaining escrow to the payer (only after authorizationExpiry).
	waitForAuthorizationExpiry(t, rail0Id)
	runCLI(t, "payments", "release", rail0Id, "-p", payeeKey)
	pollStatus(t, rail0Id, "released")
}
