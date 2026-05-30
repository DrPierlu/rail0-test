//! Shared helpers for all Rust integration tests.

use std::env;
use std::time::{Duration, SystemTime, UNIX_EPOCH};

use alloy::primitives::B256;
use alloy::signers::local::PrivateKeySigner;
use alloy::network::{EthereumWallet, TransactionBuilder};
use alloy::consensus::TxEnvelope;
use alloy::rpc::types::TransactionRequest;

use rail0::{
    ClientOptions, PaymentMethod, Rail0Client,
    CreatePaymentRequest, PaymentInput, PayerSignatureRequest,
    SignPaymentParams, TokenDomain,
    signing::{sign_authorize, sign_charge, hex_to_private_key},
};

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
    let chain_id: u64 = get_env_or("CHAIN_ID", "5042002").parse().unwrap();
    let amount        = get_env_or("AMOUNT", "1000000");
    let buyer_key     = buyer_private_key_bytes();
    let buyer_addr    = format!("0x{}", hex::encode(
        alloy::signers::local::PrivateKeySigner::from_slice(&buyer_key)
            .unwrap()
            .address()
            .as_slice()
    )).to_lowercase();

    let create = client.payments.create_payment(&CreatePaymentRequest {
        payment: PaymentInput {
            payer:  buyer_addr.clone(),
            payee:  pm.wallet_address.clone(),
            token:  pm.token_address.clone(),
            amount: amount.clone(),
        },
        chain_id: chain_id as i64,
        mode: mode.to_string(),
    }).await.expect("create_payment failed");

    // Sign EIP-3009 payload
    let domain = TokenDomain {
        name:               create.signing_payload.domain.name.clone(),
        version:            create.signing_payload.domain.version.clone(),
        chain_id:           create.signing_payload.domain.chain_id as u64,
        verifying_contract: create.signing_payload.domain.verifying_contract.clone(),
    };
    let sign_fn = if mode == "charge" { sign_charge } else { sign_authorize };
    let sig = sign_fn(&SignPaymentParams {
        private_key:      buyer_key,
        payment:          create.payment.clone(),
        amount:           amount.parse().unwrap(),
        nonce:            create.signing_payload.message.nonce.clone(),
        contract_address: create.rail0_contract.clone(),
        token_domain:     domain,
        valid_after:      None,
        valid_before:     None,
    }).expect("sign_authorize failed");

    let sign_resp = client.payments.sign(&create.rail0_id, &PayerSignatureRequest {
        v: sig.v,
        r: sig.r.clone(),
        s: sig.s.clone(),
    }).await.expect("sign failed");

    assert_eq!(sign_resp.status, "signature_stored", "unexpected sign status");
    create.rail0_id
}

// ── EIP-1559 tx signing ───────────────────────────────────────────────────────

pub async fn sign_eip1559(unsigned_hex: &str, signer: &PrivateKeySigner) -> String {
    use alloy::consensus::TxEnvelope;
    use alloy::rlp::Decodable;

    let raw = hex::decode(unsigned_hex.trim_start_matches("0x"))
        .expect("unsigned_hex is not valid hex");

    let tx = TxEnvelope::decode(&mut raw.as_slice())
        .expect("failed to decode unsigned tx");

    let wallet = EthereumWallet::from(signer.clone());
    let signed = wallet.sign_transaction(tx.into()).await
        .expect("sign_transaction failed");

    let mut buf = Vec::new();
    signed.encode_2718(&mut buf);
    format!("0x{}", hex::encode(buf))
}

// ── Polling ───────────────────────────────────────────────────────────────────

pub async fn poll_until_status<'a>(
    client: &Rail0Client,
    payment_id: &str,
    expected: &[&'a str],
    waiting_for: &str,
) -> rail0::PaymentResponse {
    let deadline = std::time::Instant::now() + POLL_TIMEOUT;
    loop {
        let state = client.payments.get(payment_id).await
            .expect("poll get failed");
        println!("  [poll] {waiting_for}: status={}", state.status);
        if expected.contains(&state.status.as_str()) {
            return state;
        }
        assert_ne!(state.status, "failed",
            "payment failed: {} — {}", state.failure_code, state.failure_message);
        assert!(std::time::Instant::now() < deadline,
            "timed out waiting for {:?} (last: {})", expected, state.status);
        tokio::time::sleep(POLL_INTERVAL).await;
    }
}
