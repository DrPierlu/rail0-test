package flows_test

import (
	"testing"
	"time"
)

// charge → dispute → close dispute. Disputes are payer-driven and signal-only,
// but follow the same prepare → sign → submit lifecycle as every other operation
// (the gateway broadcasts the signed tx). The indexer's on-chain event flips
// payment.disputed.
//
// The payer (buyer) wallet must hold native gas on the target chain — the
// dispute / close-dispute transactions are sent from the payer.
func TestDisputeOpenAndClose(t *testing.T) {
	payeeKey := env(t, "ACCOUNT_PRIVATE_KEY")
	buyerKey := env(t, "BUYER_PRIVATE_KEY")
	desc(t, "charge → dispute → close dispute",
		"1. Create payment + payer signature (charge mode)",
		"2. Charge (payee) — merchant now holds refundable funds",
		"3. Payer opens a dispute (prepare → sign → submit)",
		"4. Payer closes the dispute (prepare → sign → submit)")

	step(t, "1. create + sign (charge mode)")
	rail0Id := createSigned(t, "charge")

	step(t, "2. charge/prepare → sign → submit")
	runCLI(t, "payments", "charge", rail0Id, "-p", payeeKey)
	pollStatus(t, rail0Id, "charge", "charged")
	ok(t, "charged — merchant holds refundable funds")

	// Dispute endpoints require the payer: switch the CLI session to the buyer,
	// and restore the payee session afterwards for the other flows.
	login(t, buyerKey)
	defer login(t, payeeKey)

	step(t, "3. dispute/prepare → sign → submit (payer)")
	runCLI(t, "payments", "dispute", rail0Id, "-p", buyerKey)
	pollDisputed(t, rail0Id, true)
	ok(t, "dispute opened (disputed=true)")

	step(t, "4. dispute close/prepare → sign → submit (payer)")
	runCLI(t, "payments", "dispute", "close", rail0Id, "-p", buyerKey)
	pollDisputed(t, rail0Id, false)
	ok(t, "dispute closed (disputed=false)")
}

// pollDisputed polls until payment.disputed equals want (the indexer flips it
// after the on-chain dispute / close-dispute event).
func pollDisputed(t *testing.T, rail0Id string, want bool) {
	t.Helper()
	t.Logf("    [poll] waiting for disputed=%v …", want)
	deadline := time.Now().Add(pollTimeout)
	last := !want
	for {
		p := runCLI(t, "payments", "get", rail0Id)
		disputed, _ := p["disputed"].(bool)
		if disputed != last {
			t.Logf("    [poll] dispute: disputed=%v", disputed)
			last = disputed
		}
		if disputed == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("    ✗ timed out waiting for disputed=%v", want)
		}
		time.Sleep(pollInterval)
	}
}
