package flows_test

// Flow: create → sign → authorize → capture → refund (EIP-3009)
//
// Run:
//   set -a && source ../../.env && set +a
//   go test ./flows/ -run TestAuthorizeCaptureRefund -v

import (
	"context"
	"testing"

	rail0 "github.com/rail0/go-sdk"
)

func TestAuthorizeCaptureRefund(t *testing.T) {
	client     := newClient(t)
	accountKey := loadKey(t, "ACCOUNT_PRIVATE_KEY")
	pm         := discoverPaymentMethod(t, client)
	amount     := envOr("AMOUNT", "1000000")

	// ── Create + sign ──────────────────────────────────────────────────────────
	t.Log("→ creating payment and submitting payer signature")
	paymentID := createAndSign(t, client, pm, "authorize")
	t.Logf("  payment_id=%s", paymentID)

	// ── Authorize ──────────────────────────────────────────────────────────────
	t.Log("→ authorize/payload")
	prep, err := client.Payments.AuthorizePrepare(context.Background(), paymentID)
	if err != nil {
		t.Fatalf("AuthorizePrepare: %v", err)
	}
	signedAuth := signEIP1559(t, prep.UnsignedTransaction, accountKey)
	if _, err := client.Payments.Authorize(context.Background(), paymentID, rail0.SubmitTransactionRequest{
		SignedTransaction: signedAuth,
	}); err != nil {
		t.Fatalf("Authorize: %v", err)
	}
	auth := pollUntilStatus(t, client, paymentID, "authorized")
	t.Logf("  authorized — capturable=%s", auth.OnChain.CapturableAmount)

	// ── Capture ────────────────────────────────────────────────────────────────
	t.Log("→ capture/payload")
	prep, err = client.Payments.CapturePrepare(context.Background(), paymentID, rail0.CapturePaymentRequest{
		Amount: amount,
	})
	if err != nil {
		t.Fatalf("CapturePrepare: %v", err)
	}
	signedCap := signEIP1559(t, prep.UnsignedTransaction, accountKey)
	if _, err := client.Payments.Capture(context.Background(), paymentID, rail0.SubmitTransactionRequest{
		SignedTransaction: signedCap,
	}); err != nil {
		t.Fatalf("Capture: %v", err)
	}
	cap := pollUntilStatus(t, client, paymentID, "captured", "partially_captured")
	t.Logf("  captured — status=%s", cap.Status)
	if cap.OnChain.CapturableAmount != "0" {
		t.Errorf("capturable_amount after full capture: got %s, want 0", cap.OnChain.CapturableAmount)
	}

	// ── Refund (EIP-3009 two-phase) ────────────────────────────────────────────
	t.Log("→ refund/payload phase 1")
	phase1, err := client.Payments.RefundPrepare(context.Background(), paymentID, rail0.RefundPayloadRequest{
		Amount: amount,
	})
	if err != nil {
		t.Fatalf("RefundPrepare phase1: %v", err)
	}
	if phase1.SigningPayload == nil {
		t.Fatal("phase 1 must return signing_payload")
	}
	if phase1.UnsignedTransaction != "" {
		t.Fatal("phase 1 must NOT return unsigned_transaction")
	}

	t.Log("→ signing EIP-3009 refund payload")
	refundSig, err := rail0.SignPayload(accountKey, *phase1.SigningPayload)
	if err != nil {
		t.Fatalf("SignPayload for refund: %v", err)
	}

	t.Log("→ refund/payload phase 2")
	phase2, err := client.Payments.RefundPrepare(context.Background(), paymentID, rail0.RefundPayloadRequest{
		Amount: amount,
		V:      refundSig.V,
		R:      refundSig.R,
		S:      refundSig.S,
	})
	if err != nil {
		t.Fatalf("RefundPrepare phase2: %v", err)
	}
	if phase2.UnsignedTransaction == "" {
		t.Fatal("phase 2 must return unsigned_transaction")
	}

	t.Log("→ submitting refund")
	signedRef := signEIP1559(t, phase2.UnsignedTransaction, accountKey)
	if _, err := client.Payments.Refund(context.Background(), paymentID, rail0.SubmitTransactionRequest{
		SignedTransaction: signedRef,
	}); err != nil {
		t.Fatalf("Refund: %v", err)
	}
	final := pollUntilStatus(t, client, paymentID, "refunded", "partially_refunded")
	t.Logf("  refunded — status=%s refundable=%s", final.Status, final.OnChain.RefundableAmount)
	if final.OnChain.RefundableAmount != "0" {
		t.Errorf("refundable_amount after full refund: got %s, want 0", final.OnChain.RefundableAmount)
	}
}
