//! Flow: create (mode=charge) → sign → charge
//!
//! Run:
//!   set -a && source ../../.env && set +a
//!   cargo test test_charge -- --nocapture

mod helpers;
use helpers::*;

use rail0::SubmitTransactionRequest;

#[tokio::test]
async fn test_charge() {
    let _ = dotenvy::dotenv();
    let client      = new_client();
    let account_key = account_signer();
    let pm          = discover_payment_method(&client).await;

    println!("→ creating payment (mode=charge) and submitting payer signature");
    let payment_id = create_and_sign(&client, &pm, "charge").await;
    println!("  payment_id={payment_id}");

    println!("→ charge/payload");
    let prep = client.payments.charge_payload(&payment_id).await
        .expect("charge_payload failed");
    let signed = sign_eip1559(&prep.unsigned_transaction, &account_key).await;
    client.payments.charge(&payment_id, &SubmitTransactionRequest { signed_transaction: signed })
        .await.expect("charge failed");

    let final_state = poll_until_status(&client, &payment_id, &["charged"], "charge").await;
    println!("  charged — status={}", final_state.status);
}
