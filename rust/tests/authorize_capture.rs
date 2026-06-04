//! Flow: create → sign → authorize → capture → refund (EIP-3009)
//!
//! Run:
//!   set -a && source ../../.env && set +a
//!   cargo test authorize_capture_refund -- --nocapture

mod helpers;
use helpers::*;

use rail0::{CapturePaymentRequest, RefundPayloadRequest, RefundPrepareResponse, SubmitTransactionRequest};

#[tokio::test]
async fn authorize_capture_refund() {
    let _ = dotenvy::dotenv();
    let client      = new_client();
    let account_key = account_signer();
    let pm          = discover_payment_method(&client).await;
    let amount      = get_env_or("AMOUNT", "1.00");

    // ── Create + sign ──────────────────────────────────────────────────────────
    println!("→ creating payment and submitting payer signature");
    let payment_id = create_and_sign(&client, &pm, "authorize").await;
    println!("  payment_id={payment_id}");

    // ── Authorize ──────────────────────────────────────────────────────────────
    println!("→ authorize/prepare");
    let prep = client.payments.authorize_prepare(&payment_id).await
        .expect("authorize_prepare failed");
    let signed = sign_eip1559(&prep.unsigned_transaction, &account_key);
    client.payments.authorize(&payment_id, &SubmitTransactionRequest { signed_transaction: signed })
        .await.expect("authorize failed");

    let auth = poll_until_status(&client, &payment_id, &["authorized"], "authorize").await;
    let capturable = auth.on_chain.as_ref()
        .and_then(|v| v.get("capturable_amount"))
        .and_then(|v| v.as_str())
        .unwrap_or("-");
    println!("  authorized — capturable={capturable}");

    // ── Capture ────────────────────────────────────────────────────────────────
    println!("→ capture/prepare");
    let prep = client.payments.capture_prepare(&payment_id, &CapturePaymentRequest { amount: amount.clone() })
        .await.expect("capture_prepare failed");
    let signed = sign_eip1559(&prep.unsigned_transaction, &account_key);
    client.payments.capture(&payment_id, &SubmitTransactionRequest { signed_transaction: signed })
        .await.expect("capture failed");

    let cap = poll_until_status(&client, &payment_id, &["captured", "partially_captured"], "capture").await;
    let cap_capturable = cap.on_chain.as_ref()
        .and_then(|v| v.get("capturable_amount"))
        .and_then(|v| v.as_str())
        .unwrap_or("-");
    assert_eq!(cap_capturable, "0", "capturable must be 0 after full capture");
    println!("  captured — status={}", cap.status);

    // ── Refund (EIP-3009 two-phase) ────────────────────────────────────────────
    println!("→ refund/prepare phase 1");
    let phase1 = client.payments.refund_prepare(&payment_id, &RefundPayloadRequest {
        amount: amount.clone(), v: None, r: None, s: None,
    }).await.expect("refund_prepare phase1 failed");
    assert!(phase1.signing_payload.is_some(), "phase 1 must return signing_payload");

    println!("→ signing EIP-3009 refund payload");
    let sp = phase1.signing_payload.as_ref().unwrap();
    let account_key_bytes = account_private_key_bytes();
    let domain = rail0::TokenDomain {
        name:               sp.domain.name.clone(),
        version:            sp.domain.version.clone(),
        chain_id:           sp.domain.chain_id as u64,
        verifying_contract: sp.domain.verifying_contract.clone(),
    };
    let refund_sig = rail0::signing::sign_transfer_with_authorization(
        &account_key_bytes,
        &domain,
        rail0::signing::SignTransferParams {
            from:         sp.message.from.clone(),
            to:           sp.message.to.clone(),
            value:        sp.message.value.parse().unwrap(),
            valid_after:  Some(sp.message.valid_after.parse().unwrap()),
            valid_before: sp.message.valid_before.parse().unwrap(),
            nonce:        sp.message.nonce.clone(),
        },
    ).expect("sign_transfer_with_authorization failed");

    println!("→ refund/prepare phase 2");
    let phase2 = client.payments.refund_prepare(&payment_id, &RefundPayloadRequest {
        amount: amount.clone(),
        v: Some(refund_sig.v),
        r: Some(refund_sig.r.clone()),
        s: Some(refund_sig.s.clone()),
    }).await.expect("refund_prepare phase2 failed");
    assert!(phase2.unsigned_transaction.as_deref().map(|s| !s.is_empty()).unwrap_or(false),
        "phase 2 must return unsigned_transaction");

    println!("→ submitting refund");
    let signed = sign_eip1559(phase2.unsigned_transaction.as_deref().unwrap(), &account_key);
    client.payments.refund(&payment_id, &SubmitTransactionRequest { signed_transaction: signed })
        .await.expect("refund failed");

    let final_state = poll_until_status(&client, &payment_id, &["refunded", "partially_refunded"], "refund").await;
    let refundable = final_state.on_chain.as_ref()
        .and_then(|v| v.get("refundable_amount"))
        .and_then(|v| v.as_str())
        .unwrap_or("-");
    assert_eq!(refundable, "0", "refundable must be 0 after full refund");
    println!("  refunded — status={}", final_state.status);
}
