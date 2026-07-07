package flows_test

// Flow: public payment-methods discovery (no on-chain, no session needed).
//
// Run:
//   set -a && source ../../.env && set +a
//   go test ./flows/ -run TestPaymentMethodsDiscovery -v

import (
	"strings"
	"testing"
)

// TestPaymentMethodsDiscovery exercises the public `payment-methods` command
// (gateway GET /payment_methods) — the buyer-facing counterpart to `wallets`,
// which needs no session. It looks the merchant up by its payee wallet address
// and asserts that wallet, with its token holdings, comes back.
func TestPaymentMethodsDiscovery(t *testing.T) {
	desc(t, "Payment methods — public discovery by address",
		"1. rail0 payment-methods --address <PAYEE_ADDRESS>",
		"2. assert the payee wallet is returned with its token holdings")

	payee := strings.ToLower(env(t, "PAYEE_ADDRESS"))

	step(t, "discovering payment methods for %s", payee)
	wallets := runCLIList(t, "payment-methods", "--address", payee)
	if len(wallets) == 0 {
		t.Fatalf("expected at least one wallet for %s", payee)
	}

	// The address handle returns just that one wallet — verify it is the payee
	// and that its token holdings are present (buyers pay a wallet×token).
	first, ok0 := wallets[0].(map[string]any)
	if !ok0 {
		t.Fatalf("unexpected wallet shape: %#v", wallets[0])
	}
	if addr, _ := first["address"].(string); strings.ToLower(addr) != payee {
		t.Fatalf("expected wallet address %s, got %q", payee, addr)
	}
	if tokens, _ := first["tokens"].([]any); len(tokens) == 0 {
		t.Fatalf("expected the payee wallet to expose at least one token holding")
	}
	ok(t, "discovered the merchant wallet %s with its token holdings", payee)
}
