# frozen_string_literal: true
# Flow: create (mode=charge) → sign → charge
#
# Run:
#   set -a && source ../.env && set +a
#   bundle exec ruby flows/charge_test.rb

require "minitest/autorun"
require_relative "../support/helpers"

class ChargeTest < Minitest::Test
  include IntegrationHelpers

  def test_charge
    desc "charge (one-shot)",
         "1. Create payment mode=charge + payer signature",
         "2. Charge (payee) — funds distributed immediately, no escrow"

    step "1. Create payment (mode=charge) and submit payer signature"
    payment_id, _pm = create_and_sign_payment(mode: "charge")
    ok "payment_id=#{payment_id}"

    step "2. charge/payload → sign → submit"
    prep = client.payments.charge_payload(payment_id)
    assert prep[:unsigned_transaction]

    client.payments.charge(payment_id, signed_transaction: sign_eip1559(prep[:unsigned_transaction], account_key))
    final = poll_until_status(payment_id, "charged", waiting_for: "charge")
    ok "charged — status=#{final[:status]}"
  end
end
