//! Flow: create → sign → authorize → capture → refund (EIP-3009)
//!
//! Run:
//!   set -a && source ../../.env && set +a
//!   cargo test authorize_capture_refund -- --nocapture

mod helpers;
use helpers::*;

use rail0::{CapturePaymentRequest, RefundPayloadRequest, SubmitTransactionRequest};

#[tokio::test]
async fn authorize_capture_refund() {
    let _ = dotenvy::dotenv();
    let client      = new_client();
    let account_key = account_signer();
    let pm          = discover_payment_method(&client).await;
    let amount      = get_env_or("AMOUNT", "1000000");

    // ── Create + sign ──────────────────────────────────────────────────────────
    println!("→ creating payment and submitting payer signature");
    let payment_id = create_and_sign(&client, &pm, "authorize").await;
    println!("  payment_id={payment_id}");

    // ── Authorize ──────────────────────────────────────────────────────────────
    println!("→ authorize/payload");
    let prep = client.payments.authorize_payload(&payment_id).await
        .expect("authorize_payload failed");
    let signed = sign_eip1559(&prep.unsigned_transaction, &account_key).await;
    client.payments.authorize(&payment_id, &SubmitTransactionRequest { signed_transaction: signed })
        .await.expect("authorize failed");

    let auth = poll_until_status(&client, &payment_id, &["authorized"], "authorize").await;
    println!("  authorized — capturable={}", auth.on_chain.capturable_amount);

    // ── Capture ────────────────────────────────────────────────────────────────
    println!("→ capture/payload");
    let prep = client.payments.capture_payload(&payment_id, &CapturePaymentRequest { amount: amount.clone() })
        .await.expect("capture_payload failed");
    let signed = sign_eip1559(&prep.unsigned_transaction, &account_key).await;
    client.payments.capture(&payment_id, &SubmitTransactionRequest { signed_transaction: signed })
        .await.expect("capture failed");

    let cap = poll_until_status(&client, &payment_id, &["captured", "partially_captured"], "capture").await;
    assert_eq!(cap.on_chain.capturable_amount, "0", "capturable must be 0 after full capture");
    println!("  captured — status={}", cap.status);

    // ── Refund (EIP-3009 two-phase) ────────────────────────────────────────────
    println!("→ refund/payload phase 1");
    let phase1 = client.payments.refund_payload(&payment_id, &RefundPayloadRequest {
        amount: amount.clone(), v: None, r: None, s: None,
    }).await.expect("refund_payload phase1 failed");
    assert!(phase1.signing_payload.is_some(), "phase 1 must return signing_payload");
    assert!(phase1.unsigned_transaction.is_none() || phase1.unsigned_transaction.as_deref() == Some(""),
        "phase 1 must NOT return unsigned_transaction");

    println!("→ signing EIP-3009 refund payload");
    let sp = phase1.signing_payload.as_ref().unwrap();
    let account_key_bytes = account_private_key_bytes();
    let domain = rail0::TokenDomain {
        name:               sp.domain.name.clone(),
        version:            sp.domain.version.clone(),
        chain_id:           sp.domain.chain_id as u64,
        verifying_contract: sp.domain.verifying_contract.clone(),
    };
    // The refund signing payload uses ReceiveWithAuthorization — same EIP-3009 encoding.
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

    println!("→ refund/payload phase 2");
    let phase2 = client.payments.refund_payload(&payment_id, &RefundPayloadRequest {
        amount: amount.clone(),
        v: Some(refund_sig.v),
        r: Some(refund_sig.r.clone()),
        s: Some(refund_sig.s.clone()),
    }).await.expect("refund_payload phase2 failed");
    assert!(phase2.unsigned_transaction.as_deref().map(|s| !s.is_empty()).unwrap_or(false),
        "phase 2 must return unsigned_transaction");

    println!("→ submitting refund");
    let signed = sign_eip1559(phase2.unsigned_transaction.as_deref().unwrap(), &account_key).await;
    client.payments.refund(&payment_id, &SubmitTransactionRequest { signed_transaction: signed })
        .await.expect("refund failed");

    let final_state = poll_until_status(&client, &payment_id, &["refunded", "partially_refunded"], "refund").await;
    assert_eq!(final_state.on_chain.refundable_amount, "0", "refundable must be 0 after full refund");
    println!("  refunded — status={}", final_state.status);
}
