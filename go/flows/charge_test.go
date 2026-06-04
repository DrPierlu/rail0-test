package flows_test

// Flow: create (mode=charge) → sign → charge
//
// Run:
//   set -a && source ../../.env && set +a
//   go test ./flows/ -run TestCharge -v

import (
	"context"
	"testing"

	rail0 "github.com/rail0/go-sdk"
)

func TestCharge(t *testing.T) {
	client     := newClient(t)
	accountKey := loadKey(t, "ACCOUNT_PRIVATE_KEY")
	pm         := discoverPaymentMethod(t, client)

	t.Log("→ creating payment (mode=charge) and submitting payer signature")
	paymentID := createAndSign(t, client, pm, "charge")
	t.Logf("  payment_id=%s", paymentID)

	t.Log("→ charge/payload")
	prep, err := client.Payments.ChargePrepare(context.Background(), paymentID)
	if err != nil {
		t.Fatalf("ChargePrepare: %v", err)
	}
	signed := signEIP1559(t, prep.UnsignedTransaction, accountKey)
	if _, err := client.Payments.Charge(context.Background(), paymentID, rail0.SubmitTransactionRequest{
		SignedTransaction: signed,
	}); err != nil {
		t.Fatalf("Charge: %v", err)
	}
	final := pollUntilStatus(t, client, paymentID, "charged")
	t.Logf("  charged — status=%s", final.Status)
}
