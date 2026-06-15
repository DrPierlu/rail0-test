package flows_test

import (
	"context"
	"crypto/ecdsa"
	"encoding/hex"
	"fmt"
	"math/big"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	rail0 "github.com/rail0/go-sdk"
)

const (
	pollTimeout  = 120 * time.Second
	pollInterval = 2 * time.Second
)

// env fetches a required environment variable, failing the test if absent.
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

// newClient returns a Rail0 client pointed at RAIL0_API_URL.
func newClient(t *testing.T) *rail0.Client {
	t.Helper()
	return rail0.NewClient(rail0.ClientOptions{
		BaseURL: envOr("RAIL0_API_URL", "http://localhost:4567"),
	})
}

// loadKey parses a 0x-prefixed hex private key.
func loadKey(t *testing.T, envKey string) *ecdsa.PrivateKey {
	t.Helper()
	raw := strings.TrimPrefix(env(t, envKey), "0x")
	b, err := hex.DecodeString(raw)
	if err != nil {
		t.Fatalf("loadKey %s: %v", envKey, err)
	}
	key, err := crypto.ToECDSA(b)
	if err != nil {
		t.Fatalf("loadKey %s: %v", envKey, err)
	}
	return key
}

func addressOf(key *ecdsa.PrivateKey) string {
	return strings.ToLower(crypto.PubkeyToAddress(key.PublicKey).Hex())
}

// discoverPaymentMethod returns the wallet token matching the configured chain and token symbol.
func discoverPaymentMethod(t *testing.T, client *rail0.Client) rail0.WalletToken {
	t.Helper()
	accountID := env(t, "ACCOUNT_ID")
	chainSlug := envOr("CHAIN_SLUG", "arc-testnet")
	symbol    := envOr("TOKEN_SYMBOL", "USDC")

	tokens, err := client.Accounts.Wallets(context.Background(), accountID)
	if err != nil {
		t.Fatalf("Wallets: %v", err)
	}
	for _, m := range tokens {
		if m.ChainSlug == chainSlug && m.TokenSymbol == symbol {
			return m
		}
	}
	t.Fatalf("no %s wallet token on %s for account %s", symbol, chainSlug, accountID)
	panic("unreachable")
}

// createAndSign creates a payment and submits the payer's EIP-3009 signature.
// Returns the rail0_id.
func createAndSign(t *testing.T, client *rail0.Client, pm rail0.WalletToken, mode string) string {
	t.Helper()
	buyerKey  := loadKey(t, "BUYER_PRIVATE_KEY")
	chainID, _ := strconv.Atoi(envOr("CHAIN_ID", "5042002"))
	amount     := envOr("AMOUNT", "1000000")

	create, err := client.Payments.CreatePayment(context.Background(), rail0.CreatePaymentRequest{
		Payment: rail0.PaymentInput{
			Payer:  strings.ToLower(addressOf(buyerKey)),
			Payee:  pm.Address,
			Token:  pm.TokenAddress,
			Amount: amount,
		},
		ChainId: chainID,
		Mode:    mode,
	})
	if err != nil {
		t.Fatalf("CreatePayment: %v", err)
	}

	privBytes, err := rail0.HexToPrivateKey(hex.EncodeToString(crypto.FromECDSA(buyerKey)))
	if err != nil {
		t.Fatalf("HexToPrivateKey: %v", err)
	}
	sp := create.SigningPayload
	value := new(big.Int)
	value.SetString(sp.Message.Value, 10)
	validAfter := new(big.Int)
	validAfter.SetString(sp.Message.ValidAfter, 10)
	validBefore := new(big.Int)
	validBefore.SetString(sp.Message.ValidBefore, 10)
	domain := rail0.TokenDomain{
		Name:              sp.Domain.Name,
		Version:           sp.Domain.Version,
		ChainID:           uint64(sp.Domain.ChainId),
		VerifyingContract: sp.Domain.VerifyingContract,
	}
	sig, err := rail0.SignTransferWithAuthorization(privBytes, domain, rail0.SignTransferParams{
		From:        sp.Message.From,
		To:          sp.Message.To,
		Value:       value,
		ValidAfter:  validAfter,
		ValidBefore: validBefore,
		Nonce:       sp.Message.Nonce,
	})
	if err != nil {
		t.Fatalf("SignTransferWithAuthorization: %v", err)
	}
	rBytes, _ := hex.DecodeString(strings.TrimPrefix(sig.R, "0x"))
	sBytes, _ := hex.DecodeString(strings.TrimPrefix(sig.S, "0x"))
	sigBytes := append(append(rBytes, sBytes...), byte(sig.V))
	signature := "0x" + hex.EncodeToString(sigBytes)

	signResp, err := client.Payments.Sign(context.Background(), create.Rail0Id, rail0.PayerSignatureRequest{
		Signature: signature,
	})
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	if signResp.Status != "signature_stored" {
		t.Fatalf("expected signature_stored, got %s", signResp.Status)
	}
	return create.Rail0Id
}

// signEIP1559 signs an unsigned EIP-1559 transaction hex with the given key.
func signEIP1559(t *testing.T, unsignedHex string, key *ecdsa.PrivateKey) string {
	t.Helper()
	raw, err := hex.DecodeString(strings.TrimPrefix(unsignedHex, "0x"))
	if err != nil {
		t.Fatalf("signEIP1559 decode: %v", err)
	}
	tx := new(types.Transaction)
	if err := tx.UnmarshalBinary(raw); err != nil {
		t.Fatalf("signEIP1559 unmarshal: %v", err)
	}
	signer := types.LatestSignerForChainID(tx.ChainId())
	signed, err := types.SignTx(tx, signer, key)
	if err != nil {
		t.Fatalf("signEIP1559 sign: %v", err)
	}
	b, err := signed.MarshalBinary()
	if err != nil {
		t.Fatalf("signEIP1559 marshal: %v", err)
	}
	return "0x" + hex.EncodeToString(b)
}

// pollUntilStatus polls GET /payments/:id until status matches one of expected.
func pollUntilStatus(t *testing.T, client *rail0.Client, paymentID string, expected ...string) *rail0.PaymentResponse {
	t.Helper()
	deadline := time.Now().Add(pollTimeout)
	set := make(map[string]bool, len(expected))
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
			t.Fatalf("payment failed: code=%s msg=%s", state.FailureCode, state.FailureMessage)
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %v (last: %s)", expected, state.Status)
		}
		time.Sleep(pollInterval)
	}
}

// mustGetEnvChainID parses CHAIN_ID or returns the default.
func mustGetEnvChainID() *big.Int {
	s := envOr("CHAIN_ID", "5042002")
	n, _ := new(big.Int).SetString(s, 10)
	return n
}

func logf(t *testing.T, format string, args ...any) {
	t.Helper()
	t.Log(fmt.Sprintf(format, args...))
}
