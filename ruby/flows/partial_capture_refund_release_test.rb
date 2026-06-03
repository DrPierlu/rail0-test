# frozen_string_literal: true
# Flow: authorize → partial capture ×2 → partial refund ×2 → release
#
# The release() step requires authorization_expiry to have passed.
# Run the API with a short TTL:
#   POLICY_AUTHORIZATION_TTL=30 bundle exec puma -C config/puma.rb
#
# Then run this test with the matching TTL so the poller waits correctly:
#   POLICY_AUTHORIZATION_TTL=30 set -a && source ../.env && set +a
#   POLICY_AUTHORIZATION_TTL=30 bundle exec ruby flows/partial_capture_refund_release_test.rb

require "minitest/autorun"
require_relative "../support/helpers"

class PartialCaptureRefundReleaseTest < Minitest::Test
  include IntegrationHelpers

  AUTHORIZATION_TTL = ENV.fetch("POLICY_AUTHORIZATION_TTL", "604800").to_i

  def test_partial_capture_refund_release
    desc "authorize → partial capture ×2 → partial refund ×2 → release",
         "1. Create + sign",
         "2. Authorize",
         "3. Partial capture 0.30",
         "4. Partial capture 0.20",
         "5. Partial refund 0.10 (EIP-3009)",
         "6. Partial refund 0.15 (EIP-3009)",
         "7. Wait for authorization_expiry",
         "8. Buyer release",
         "9. Verify final state"

    # ── 1. Create + sign ──────────────────────────────────────────────────────
    step "1. Create payment and submit payer signature"
    payment_id, _pm = create_and_sign_payment(mode: "authorize")
    ok "payment_id=#{payment_id}"

    # ── 2. Authorize ──────────────────────────────────────────────────────────
    step "2. Authorize"
    prep = client.payments.authorize_prepare(payment_id)
    client.payments.authorize(payment_id, signed_transaction: sign_eip1559(prep[:unsigned_transaction], account_key))
    auth = poll_until_status(payment_id, "authorized", waiting_for: "authorize")
    ok "authorized — capturable=#{auth.dig(:on_chain, :capturable_amount)}"

    # ── 3. Partial capture 0.30 ───────────────────────────────────────────────
    step "3. Partial capture 0.30"
    prep = client.payments.capture_prepare(payment_id, amount: "0.30")
    client.payments.capture(payment_id, signed_transaction: sign_eip1559(prep[:unsigned_transaction], account_key))
    s = poll_until_status(payment_id, "partially_captured", waiting_for: "capture#1")
    ok "partially_captured — capturable=#{s.dig(:on_chain, :capturable_amount)}"

    # ── 4. Partial capture 0.20 ───────────────────────────────────────────────
    step "4. Partial capture 0.20"
    prep = client.payments.capture_prepare(payment_id, amount: "0.20")
    client.payments.capture(payment_id, signed_transaction: sign_eip1559(prep[:unsigned_transaction], account_key))
    s = poll_until_status(payment_id, "partially_captured", waiting_for: "capture#2")
    ok "partially_captured — capturable=#{s.dig(:on_chain, :capturable_amount)}"

    # ── 5. Partial refund 0.10 (EIP-3009) ────────────────────────────────────
    step "5. Partial refund 0.10"
    refund_eip3009(payment_id, "0.10")
    poll_until_status(payment_id, "partially_refunded", waiting_for: "refund#1")
    ok "partially_refunded"

    # ── 6. Partial refund 0.15 (EIP-3009) ────────────────────────────────────
    step "6. Partial refund 0.15"
    refund_eip3009(payment_id, "0.15")
    s = poll_until_status(payment_id, "partially_refunded", waiting_for: "refund#2")
    ok "partially_refunded — refundable=#{s.dig(:on_chain, :refundable_amount)}"

    # ── 7. Wait for authorization_expiry ──────────────────────────────────────
    expiry    = client.payments.get(payment_id)[:authorization_expiry].to_i
    wait_secs = expiry - Time.now.to_i + 3
    if wait_secs > 0
      step "7. Waiting #{wait_secs}s for authorization_expiry"
      wait_secs.times do |i|
        $stdout.print "\r    ⏳ #{wait_secs - i}s remaining...   "
        $stdout.flush
        sleep 1
      end
      $stdout.puts
    else
      step "7. authorization_expiry already passed"
    end
    ok "authorization_expiry passed"

    # ── 8. Buyer release ──────────────────────────────────────────────────────
    step "8. release/payload → sign (as buyer) → submit"
    prep = client.payments.release_prepare(payment_id, caller_address: buyer_address)
    assert prep[:unsigned_transaction]

    client.payments.release(payment_id, signed_transaction: sign_eip1559(prep[:unsigned_transaction], buyer_key))
    poll_until_status(payment_id, "released", waiting_for: "release")
    ok "released"

    # ── 9. Verify ─────────────────────────────────────────────────────────────
    step "9. Verify final state"
    final = client.payments.get(payment_id)

    assert_equal "released", final[:status]
    assert_equal "0", final.dig(:on_chain, :capturable_amount), "capturable must be 0 after release"
    ok "DB status=#{final[:status]} capturable=#{final.dig(:on_chain, :capturable_amount)}"
  end

  private

  # EIP-3009 two-phase refund helper.
  def refund_eip3009(payment_id, refund_amount)
    phase1 = client.payments.refund_prepare(payment_id, amount: refund_amount)
    assert phase1[:signing_payload], "phase 1 must return signing_payload"

    vrs    = sign_payload_vrs(ENV.fetch("ACCOUNT_PRIVATE_KEY"), phase1[:signing_payload])
    phase2 = client.payments.refund_prepare(payment_id, amount: refund_amount, **vrs)
    assert phase2[:unsigned_transaction], "phase 2 must return unsigned_transaction"

    client.payments.refund(payment_id, signed_transaction: sign_eip1559(phase2[:unsigned_transaction], account_key))
  end
end
