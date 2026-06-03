# frozen_string_literal: true
# Flow: create → sign → authorize → capture → refund (EIP-3009 two-phase)
#
# Run:
#   set -a && source ../.env && set +a
#   bundle exec ruby flows/authorize_capture_test.rb

require "minitest/autorun"
require_relative "../support/helpers"

class AuthorizeCaptureTest < Minitest::Test
  include IntegrationHelpers

  def test_authorize_capture_refund
    desc "authorize → capture → refund (EIP-3009)",
         "1. Create payment + payer signature",
         "2. Authorize (payee)",
         "3. Capture full amount (payee)",
         "4. Refund full amount via EIP-3009 (payee)"

    # ── 1. Create + sign ──────────────────────────────────────────────────────
    step "1. Create payment and submit payer signature"
    payment_id, _pm = create_and_sign_payment(mode: "authorize")
    ok "payment_id=#{payment_id}"

    # ── 2. Authorize ──────────────────────────────────────────────────────────
    step "2. authorize/payload → sign → submit"
    prep = client.payments.authorize_prepare(payment_id)
    assert prep[:unsigned_transaction]

    client.payments.authorize(payment_id, signed_transaction: sign_eip1559(prep[:unsigned_transaction], account_key))
    auth = poll_until_status(payment_id, "authorized", waiting_for: "authorize")
    ok "authorized — capturable=#{auth.dig(:on_chain, :capturable_amount)}"

    # ── 3. Capture ────────────────────────────────────────────────────────────
    step "3. capture/payload → sign → submit"
    prep = client.payments.capture_prepare(payment_id, amount: amount)
    assert prep[:unsigned_transaction]

    client.payments.capture(payment_id, signed_transaction: sign_eip1559(prep[:unsigned_transaction], account_key))
    cap = poll_until_status(payment_id, "captured", "partially_captured", waiting_for: "capture")
    ok "captured — status=#{cap[:status]}"

    assert_equal "0", cap.dig(:on_chain, :capturable_amount), "capturable must be 0 after full capture"

    # ── 4. Refund (EIP-3009 two-phase) ────────────────────────────────────────
    step "4. refund/payload phase 1 (get signing payload)"
    phase1 = client.payments.refund_prepare(payment_id, amount: amount)
    assert phase1[:signing_payload], "phase 1 must return signing_payload"
    refute phase1[:unsigned_transaction], "phase 1 must NOT return unsigned_transaction"

    step "4. refund/payload phase 2 (get unsigned tx)"
    vrs    = sign_payload_vrs(ENV.fetch("ACCOUNT_PRIVATE_KEY"), phase1[:signing_payload])
    phase2 = client.payments.refund_prepare(payment_id, amount: amount, **vrs)
    assert phase2[:unsigned_transaction], "phase 2 must return unsigned_transaction"

    step "4. submit refund"
    client.payments.refund(payment_id, signed_transaction: sign_eip1559(phase2[:unsigned_transaction], account_key))
    final = poll_until_status(payment_id, "refunded", "partially_refunded", waiting_for: "refund")
    ok "refunded — status=#{final[:status]}"

    assert_equal "0", final.dig(:on_chain, :refundable_amount), "refundable must be 0 after full refund"
  end
end
