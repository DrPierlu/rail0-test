//! Shared helpers for all Rust integration tests.

use std::env;
use std::time::Duration;

use alloy::consensus::{SignableTransaction, TxEip1559};
use alloy::eips::eip2718::Encodable2718;
use alloy::network::TxSignerSync;
use alloy::rlp::Decodable;
use alloy::signers::local::PrivateKeySigner;

use rail0::{
    ClientOptions, CreatePaymentInput, CreatePaymentRequest, PaymentMethod,
    PayerSignatureRequest, Rail0Client, SignPaymentParams, TokenDomain,
    signing::{sign_authorize, sign_charge, hex_to_private_key},
};

pub use rail0::types_gen::PaymentSummary;

pub const POLL_TIMEOUT: Duration  = Duration::from_secs(120);
pub const POLL_INTERVAL: Duration = Duration::from_secs(2);

// ── Environment ──────────────────────────────────────────────────────────────

pub fn get_env(key: &str) -> String {
    env::var(key).unwrap_or_else(|_| panic!("required env var {key} is not set"))
}

pub fn get_env_or(key: &str, default: &str) -> String {
    env::var(key).unwrap_or_else(|_| default.to_string())
}

// ── Client ───────────────────────────────────────────────────────────────────

pub fn new_client() -> Rail0Client {
    Rail0Client::new(ClientOptions {
        base_url: get_env_or("RAIL0_API_URL", "http://localhost:4567"),
        ..Default::default()
    })
}

// ── Signers ──────────────────────────────────────────────────────────────────

pub fn buyer_signer() -> PrivateKeySigner {
    let key = get_env("BUYER_PRIVATE_KEY");
    key.trim_start_matches("0x").parse().expect("invalid BUYER_PRIVATE_KEY")
}

pub fn account_signer() -> PrivateKeySigner {
    let key = get_env("ACCOUNT_PRIVATE_KEY");
    key.trim_start_matches("0x").parse().expect("invalid ACCOUNT_PRIVATE_KEY")
}

pub fn buyer_private_key_bytes() -> Vec<u8> {
    let k = get_env("BUYER_PRIVATE_KEY");
    hex_to_private_key(&k).expect("invalid BUYER_PRIVATE_KEY")
}

pub fn account_private_key_bytes() -> Vec<u8> {
    let k = get_env("ACCOUNT_PRIVATE_KEY");
    hex_to_private_key(&k).expect("invalid ACCOUNT_PRIVATE_KEY")
}

// ── Payment method discovery ─────────────────────────────────────────────────

pub async fn discover_payment_method(client: &Rail0Client) -> PaymentMethod {
    let account_id = get_env("ACCOUNT_ID");
    let chain_slug  = get_env_or("CHAIN_SLUG", "arc-testnet");
    let symbol      = get_env_or("TOKEN_SYMBOL", "USDC");

    let methods = client.accounts.payment_methods(&account_id).await
        .expect("failed to fetch payment methods");

    methods.into_iter()
        .find(|m| m.chain_slug == chain_slug && m.token_symbol == symbol)
        .unwrap_or_else(|| panic!("no {symbol} payment method on {chain_slug}"))
}

// ── Create + sign ─────────────────────────────────────────────────────────────

pub async fn create_and_sign(
    client: &Rail0Client,
    pm: &PaymentMethod,
    mode: &str,
) -> String {
    let chain_id: i64 = get_env_or("CHAIN_ID", "5042002").parse().unwrap();
    let amount        = get_env_or("AMOUNT", "1.00");
    let buyer_key     = buyer_private_key_bytes();
    let buyer_addr    = format!("{}", buyer_signer().address());

    let create = client.payments.create(&CreatePaymentRequest {
        payment: CreatePaymentInput {
            payer:  buyer_addr.clone(),
            payee:  pm.wallet_address.clone(),
            token:  pm.token_address.clone(),
            amount: amount.clone(),
        },
        chain_id,
        mode: mode.to_string(),
    }).await.expect("create failed");

    let sp = &create.signing_prepare;
    let domain = TokenDomain {
        name:               sp.domain.name.clone(),
        version:            sp.domain.version.clone(),
        chain_id:           sp.domain.chain_id as u64,
        verifying_contract: sp.domain.verifying_contract.clone(),
    };

    let sign_fn = if mode == "charge" { sign_charge } else { sign_authorize };
    let sig = sign_fn(&SignPaymentParams {
        private_key:      buyer_key,
        payment:          create.payment.clone(),
        amount:           sp.message.value.parse().unwrap(),
        nonce:            sp.message.nonce.clone(),
        contract_address: create.rail0_contract.clone(),
        token_domain:     domain,
    }).expect("sign failed");

    let sign_resp = client.payments.sign(&create.payment_id, &PayerSignatureRequest {
        v: sig.v,
        r: sig.r.clone(),
        s: sig.s.clone(),
    }).await.expect("sign failed");

    assert_eq!(sign_resp.status, "signature_stored", "unexpected sign status");
    create.payment_id
}

// ── EIP-1559 tx signing ───────────────────────────────────────────────────────

pub fn sign_eip1559(unsigned_hex: &str, signer: &PrivateKeySigner) -> String {
    let raw = alloy::hex::decode(unsigned_hex.trim_start_matches("0x"))
        .expect("invalid hex");
    // EIP-2718 typed tx: first byte is the type (0x02 for EIP-1559)
    assert_eq!(raw.first().copied(), Some(2), "expected EIP-1559 tx (type 0x02)");
    let mut tx = TxEip1559::decode(&mut &raw[1..])
        .expect("failed to decode EIP-1559 tx body");
    let sig = signer.sign_transaction_sync(&mut tx)
        .expect("signing failed");
    let signed = tx.into_signed(sig);
    let mut buf = Vec::new();
    signed.encode_2718(&mut buf);
    format!("0x{}", alloy::hex::encode(buf))
}

// ── Polling ───────────────────────────────────────────────────────────────────

pub async fn poll_until_status(
    client: &Rail0Client,
    payment_id: &str,
    expected: &[&str],
    waiting_for: &str,
) -> PaymentSummary {
    let deadline = std::time::Instant::now() + POLL_TIMEOUT;
    loop {
        let state = client.payments.get(payment_id).await
            .expect("poll get failed");
        let capturable = state.on_chain.as_ref()
            .and_then(|v| v.get("capturable_amount"))
            .and_then(|v| v.as_str())
            .unwrap_or("-");
        println!("  [poll] {waiting_for}: status={} capturable={capturable}", state.status);
        if expected.contains(&state.status.as_str()) {
            return state;
        }
        if state.status == "failed" {
            panic!(
                "payment failed: {} — {}",
                state.failure_code.as_deref().unwrap_or("?"),
                state.failure_message.as_deref().unwrap_or("?")
            );
        }
        assert!(std::time::Instant::now() < deadline,
            "timed out waiting for {:?} (last: {})", expected, state.status);
        tokio::time::sleep(POLL_INTERVAL).await;
    }
}
