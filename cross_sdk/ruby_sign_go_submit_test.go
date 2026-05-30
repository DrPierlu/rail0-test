package cross_sdk_test

// Cross-SDK compatibility test: Ruby signs the EIP-3009 payload, Go submits authorize.
//
// This verifies that the EIP-3009 signature produced by the Ruby SDK is valid
// when submitted via the Go SDK — i.e. both SDKs produce identical digests for
// the same typed-data payload.
//
// Run:
//   set -a && source ../.env && set +a
//   go test . -run TestRubySignGoSubmit -v
//
// Requires:
//   - ruby with the rail0-ruby SDK available (bundle exec ruby)
//   - RAIL0_API_URL, BUYER_PRIVATE_KEY, ACCOUNT_PRIVATE_KEY, ACCOUNT_ID set

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"crypto/ecdsa"
	"encoding/hex"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	rail0 "github.com/rail0/go-sdk"
)

const (
	pollTimeout  = 120 * time.Second
	pollInterval = 2 * time.Second
)

func env(t *testing.T, key string) string {
	t.Helper()
	v := os.Getenv(key)
	if v == "" {
		t.Fatalf("required env var %s is not set", key)
	}
	return v
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func newClient(t *testing.T) *rail0.Client {
	t.Helper()
	return rail0.NewClient(rail0.ClientOptions{
		BaseURL: envOr("RAIL0_API_URL", "http://localhost:4567"),
	})
}

func loadKey(t *testing.T, envKey string) *ecdsa.PrivateKey {
	t.Helper()
	raw := strings.TrimPrefix(env(t, envKey), "0x")
	b, err := hex.DecodeString(raw)
	if err != nil {
		t.Fatalf("loadKey %s: %v", envKey, err)
	}
	k, err := crypto.ToECDSA(b)
	if err != nil {
		t.Fatalf("loadKey %s: %v", envKey, err)
	}
	return k
}

func addressOf(k *ecdsa.PrivateKey) string {
	return strings.ToLower(crypto.PubkeyToAddress(k.PublicKey).Hex())
}

func signEIP1559(t *testing.T, unsignedHex string, key *ecdsa.PrivateKey) string {
	t.Helper()
	raw, _ := hex.DecodeString(strings.TrimPrefix(unsignedHex, "0x"))
	tx := new(types.Transaction)
	if err := tx.UnmarshalBinary(raw); err != nil {
		t.Fatalf("signEIP1559 unmarshal: %v", err)
	}
	signer := types.LatestSignerForChainID(tx.ChainId())
	signed, err := types.SignTx(tx, signer, key)
	if err != nil {
		t.Fatalf("signEIP1559 sign: %v", err)
	}
	b, _ := signed.MarshalBinary()
	return "0x" + hex.EncodeToString(b)
}

func pollUntilStatus(t *testing.T, client *rail0.Client, paymentID string, expected ...string) *rail0.PaymentResponse {
	t.Helper()
	deadline := time.Now().Add(pollTimeout)
	set := make(map[string]bool)
	for _, s := range expected {
		set[s] = true
	}
	for {
		state, err := client.Payments.Get(context.Background(), paymentID)
		if err != nil {
			t.Fatalf("poll Get: %v", err)
		}
		t.Logf("  [poll] status=%s", state.Status)
		if set[state.Status] {
			return state
		}
		if state.Status == "failed" {
			t.Fatalf("payment failed: %s — %s", state.FailureCode, state.FailureMessage)
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %v (last: %s)", expected, state.Status)
		}
		time.Sleep(pollInterval)
	}
}

// rubySign invokes a small Ruby one-liner via the rail0-ruby SDK to produce the
// EIP-3009 signature for the given signingPayload JSON, using BUYER_PRIVATE_KEY.
// Returns the 65-byte "0x"-prefixed hex signature (r+s+v).
func rubySign(t *testing.T, signingPayloadJSON string) string {
	t.Helper()
	script := fmt.Sprintf(`
require "json"
$LOAD_PATH.unshift File.expand_path("../../rail0-ruby/lib", __dir__)
require "rail0/signing"

payload = JSON.parse('%s', symbolize_names: true)
sig = Rail0::Signing.sign_payload(ENV.fetch("BUYER_PRIVATE_KEY"), payload)
puts "0x#{sig.r[2..]}#{sig.s[2..]}#{sig.v.to_s(16).rjust(2, '0')}"
`, strings.ReplaceAll(signingPayloadJSON, "'", `'"'"'`))

	cmd := exec.Command("ruby", "-e", script)
	cmd.Dir = "../"
	cmd.Env = append(os.Environ())
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if ok := false; ok {
			_ = exitErr
		}
		t.Fatalf("ruby sign script failed: %v\nstderr: %s", err, cmd.Stderr)
	}
	return strings.TrimSpace(string(out))
}

func TestRubySignGoSubmit(t *testing.T) {
	client     := newClient(t)
	accountKey := loadKey(t, "ACCOUNT_PRIVATE_KEY")
	buyerKey   := loadKey(t, "BUYER_PRIVATE_KEY")
	accountID  := env(t, "ACCOUNT_ID")
	chainSlug  := envOr("CHAIN_SLUG", "arc-testnet")
	symbol     := envOr("TOKEN_SYMBOL", "USDC")
	chainID, _ := func() (int, error) {
		var n int
		_, err := fmt.Sscanf(envOr("CHAIN_ID", "5042002"), "%d", &n)
		return n, err
	}()
	amount := envOr("AMOUNT", "1000000")

	// ── Discover payment method ────────────────────────────────────────────────
	t.Log("→ discovering payment method")
	methods, err := client.Accounts.PaymentMethods(context.Background(), accountID)
	if err != nil {
		t.Fatalf("PaymentMethods: %v", err)
	}
	var pm rail0.PaymentMethod
	for _, m := range methods {
		if m.ChainSlug == chainSlug && m.TokenSymbol == symbol {
			pm = m
			break
		}
	}
	if pm.WalletAddress == "" {
		t.Fatalf("no %s payment method on %s", symbol, chainSlug)
	}

	// ── Create payment (Go SDK) ────────────────────────────────────────────────
	t.Log("→ creating payment via Go SDK")
	create, err := client.Payments.CreatePayment(context.Background(), rail0.CreatePaymentRequest{
		Payment: rail0.PaymentInput{
			Payer:  addressOf(buyerKey),
			Payee:  pm.WalletAddress,
			Token:  pm.TokenAddress,
			Amount: amount,
		},
		ChainId: chainID,
		Mode:    "authorize",
	})
	if err != nil {
		t.Fatalf("CreatePayment: %v", err)
	}
	t.Logf("  payment_id=%s", create.Rail0Id)

	// ── Sign with Ruby SDK ─────────────────────────────────────────────────────
	t.Log("→ signing EIP-3009 payload with Ruby SDK")
	payloadJSON, err := json.Marshal(create.SigningPayload)
	if err != nil {
		t.Fatalf("marshal signing payload: %v", err)
	}
	signature := rubySign(t, string(payloadJSON))
	t.Logf("  Ruby signature: %s…", signature[:12])

	// ── Submit signature via Go SDK ────────────────────────────────────────────
	t.Log("→ submitting signature via Go SDK")
	signResp, err := client.Payments.Sign(context.Background(), create.Rail0Id, rail0.PayerSignatureRequest{
		Signature: signature,
	})
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	if signResp.Status != "signature_stored" {
		t.Fatalf("expected signature_stored, got %s", signResp.Status)
	}
	t.Logf("  recovered_payer=%s", signResp.RecoveredPayer)

	// Verify the API recovered the correct buyer address.
	if strings.ToLower(signResp.RecoveredPayer) != addressOf(buyerKey) {
		t.Errorf("recovered payer mismatch: got %s, want %s", signResp.RecoveredPayer, addressOf(buyerKey))
	}

	// ── Authorize via Go SDK ───────────────────────────────────────────────────
	t.Log("→ authorize/payload (Go SDK)")
	prep, err := client.Payments.AuthorizePayload(context.Background(), create.Rail0Id)
	if err != nil {
		t.Fatalf("AuthorizePayload: %v", err)
	}
	signed := signEIP1559(t, prep.UnsignedTransaction, accountKey)
	if _, err := client.Payments.Authorize(context.Background(), create.Rail0Id, rail0.SubmitTransactionRequest{
		SignedTransaction: signed,
	}); err != nil {
		t.Fatalf("Authorize: %v", err)
	}
	final := pollUntilStatus(t, client, create.Rail0Id, "authorized")
	t.Logf("  authorized — capturable=%s", final.OnChain.CapturableAmount)
}
