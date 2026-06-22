package flows_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	rail0 "github.com/rail0/go-sdk"
)

// charge → dispute → close dispute. Disputes are buyer-driven and signal-only:
// the gateway only prepares the transaction; the payer signs it and broadcasts
// it directly to the chain, and the indexer reports the on-chain dispute /
// close-dispute event back to the gateway (which flips payment.disputed).
//
// The payer (buyer) wallet must hold native gas on the target chain to broadcast
// these transactions — unlike the EIP-3009 flows, where only the payee pays gas.
func TestDisputeOpenAndClose(t *testing.T) {
	payeeKey := env(t, "ACCOUNT_PRIVATE_KEY")
	desc(t, "charge → dispute → close dispute",
		"1. Create payment + payer signature (charge mode)",
		"2. Charge (payee) — merchant now holds refundable funds",
		"3. Payer opens a dispute (signal-only, payer-signed & broadcast)",
		"4. Payer closes the dispute")

	step(t, "1. create + sign (charge mode)")
	rail0Id := createSigned(t, "charge")

	step(t, "2. charge/prepare → sign → submit")
	runCLI(t, "payments", "charge", rail0Id, "-p", payeeKey)
	pollStatus(t, rail0Id, "charge", "charged")
	ok(t, "charged — merchant holds refundable funds")

	// Disputes require the payer (buyer) to authenticate, then sign and broadcast
	// the prepared transaction themselves.
	buyer := buyerClient(t)
	uuid := resolvePaymentUUID(t, buyer, rail0Id)

	step(t, "3. dispute/prepare (payer) → sign → broadcast on-chain")
	prep, err := buyer.Payments.DisputePrepare(context.Background(), uuid)
	if err != nil {
		t.Fatalf("dispute prepare: %v", err)
	}
	broadcastUnsignedTx(t, prep.UnsignedTransaction, env(t, "BUYER_PRIVATE_KEY"))
	pollDisputed(t, rail0Id, true)
	ok(t, "dispute opened (disputed=true)")

	step(t, "4. dispute/close prepare (payer) → sign → broadcast on-chain")
	closePrep, err := buyer.Payments.CloseDisputePrepare(context.Background(), uuid)
	if err != nil {
		t.Fatalf("close dispute prepare: %v", err)
	}
	broadcastUnsignedTx(t, closePrep.UnsignedTransaction, env(t, "BUYER_PRIVATE_KEY"))
	pollDisputed(t, rail0Id, false)
	ok(t, "dispute closed (disputed=false)")
}

// buyerClient logs in as the payer (buyer) over SIWE and returns an authenticated
// SDK client — dispute endpoints require the payer, while the CLI session is the
// payee.
func buyerClient(t *testing.T) *rail0.Client {
	t.Helper()
	base := envOr("RAIL0_API_URL", "http://localhost:4567")
	key, err := rail0.HexToPrivateKey(env(t, "BUYER_PRIVATE_KEY"))
	if err != nil {
		t.Fatalf("buyer key: %v", err)
	}
	host := strings.TrimPrefix(strings.TrimPrefix(base, "https://"), "http://")
	resp, err := rail0.NewClient(rail0.ClientOptions{BaseURL: base}).
		Auth.Login(context.Background(), key, host)
	if err != nil {
		t.Fatalf("buyer login: %v", err)
	}
	return rail0.NewClient(rail0.ClientOptions{
		BaseURL: base,
		Headers: map[string]string{"Authorization": "Bearer " + resp.Token},
	})
}

// resolvePaymentUUID turns a rail0_id into the payment's technical UUID via the
// authenticated list filter (path operations address payments by UUID).
func resolvePaymentUUID(t *testing.T, c *rail0.Client, rail0Id string) string {
	t.Helper()
	res, err := c.Payments.List(context.Background(), rail0.ListPaymentsParams{Rail0ID: rail0Id, PerPage: 1})
	if err != nil {
		t.Fatalf("resolve uuid: %v", err)
	}
	if len(res.Data) == 0 {
		t.Fatalf("no payment found for rail0_id %s", rail0Id)
	}
	return res.Data[0].ID
}

// broadcastUnsignedTx signs the gateway's unsigned (signal-only) transaction with
// the payer key and broadcasts it straight to the chain via eth_sendRawTransaction.
func broadcastUnsignedTx(t *testing.T, unsigned map[string]any, privKeyHex string) {
	t.Helper()
	b, err := json.Marshal(unsigned)
	if err != nil {
		t.Fatalf("marshal unsigned tx: %v", err)
	}
	key, err := rail0.HexToPrivateKey(privKeyHex)
	if err != nil {
		t.Fatalf("payer key: %v", err)
	}
	signed, err := rail0.SignTransaction(string(b), key)
	if err != nil {
		t.Fatalf("sign tx: %v", err)
	}

	url := envOr("RAIL0_RPC_URL", "https://rpc.testnet.arc.network")
	body := fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"eth_sendRawTransaction","params":[%q]}`, signed)
	resp, err := http.Post(url, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("broadcast: %v", err)
	}
	defer resp.Body.Close()
	var r struct {
		Result string `json:"result"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&r)
	if r.Error != nil {
		t.Fatalf("eth_sendRawTransaction: %s", r.Error.Message)
	}
	t.Logf("    broadcast %s", r.Result)
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
