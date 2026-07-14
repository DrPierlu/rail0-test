# frozen_string_literal: true

# Flow: create → sign → authorize → capture → refund (EIP-3009), via rail0-ruby.
#
# Run:
#   set -a && source ../../.env && set +a
#   bundle exec ruby -Iflows flows/authorize_capture_test.rb

require_relative "test_helper"

describe "authorize → capture → refund (rail0-ruby)" do
  it "settles a payment through the full lifecycle" do
    client   = new_client
    payee_pk = env("ACCOUNT_PRIVATE_KEY")
    pm       = discover_payment_method(client)

    # ── Create + sign (payer, public) ─────────────────────────────────────────
    puts "→ creating payment and submitting payer signature"
    payment_id = create_and_sign(client, pm, "authorize")
    puts "  payment_id=#{payment_id}"

    # ── Authorize (payee, JWT) ──────────────────────────────────────────────────
    puts "→ authorize/prepare"
    prep = payee_client.payments.authorize_prepare(payment_id)
    payee_client.payments.authorize(payment_id, { signed_transaction: sign_tx(prep[:unsigned_transaction], payee_pk) })
    auth = poll_until_status(client, payment_id, "authorized")
    puts "  authorized — capturable=#{auth[:capturable_amount]}"

    # ── Capture (full) ──────────────────────────────────────────────────────────
    puts "→ capture/prepare"
    prep = payee_client.payments.capture_prepare(payment_id, amount)
    payee_client.payments.capture(payment_id, { signed_transaction: sign_tx(prep[:unsigned_transaction], payee_pk) })
    cap = poll_until_status(client, payment_id, "captured", "partially_captured")
    puts "  captured — status=#{cap[:status]}"
    assert_equal "0", cap[:capturable_amount], "capturable_amount after full capture"

    # ── Refund (EIP-3009 two-phase) ─────────────────────────────────────────────
    puts "→ refund/prepare phase 1"
    phase1 = payee_client.payments.refund_prepare(payment_id, amount: amount)
    refute_nil phase1[:signing_payload], "phase 1 must return signing_payload"
    assert_nil phase1[:unsigned_transaction], "phase 1 must NOT return unsigned_transaction"

    puts "→ signing EIP-3009 refund payload"
    refund_sig = Rail0::Signing.sign_payload(payee_pk, phase1[:signing_payload])

    puts "→ refund/prepare phase 2"
    phase2 = payee_client.payments.refund_prepare(payment_id, amount: amount, signature: refund_sig.to_hex)
    refute_nil phase2[:unsigned_transaction], "phase 2 must return unsigned_transaction"

    puts "→ submitting refund"
    payee_client.payments.refund(payment_id, { signed_transaction: sign_tx(phase2[:unsigned_transaction], payee_pk) })
    final = poll_until_status(client, payment_id, "refunded", "partially_refunded")
    puts "  refunded — status=#{final[:status]} refundable=#{final[:refundable_amount]}"
    assert_equal "0", final[:refundable_amount], "refundable_amount after full refund"
  end
end
