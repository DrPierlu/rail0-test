# frozen_string_literal: true
# Flow: create → sign → authorize → void
#
# Run:
#   set -a && source ../.env && set +a
#   bundle exec ruby flows/authorize_void_test.rb

require "minitest/autorun"
require_relative "../support/helpers"

class AuthorizeVoidTest < Minitest::Test
  include IntegrationHelpers

  def test_authorize_void
    desc "authorize → void",
         "1. Create payment + payer signature",
         "2. Authorize (payee)",
         "3. Void (payee)"

    step "1. Create payment and submit payer signature"
    payment_id, _pm = create_and_sign_payment(mode: "authorize")
    ok "payment_id=#{payment_id}"

    step "2. authorize/payload → sign → submit"
    prep = client.payments.authorize_payload(payment_id)
    assert prep[:unsigned_transaction]

    client.payments.authorize(payment_id, signed_transaction: sign_eip1559(prep[:unsigned_transaction], account_key))
    auth = poll_until_status(payment_id, "authorized", waiting_for: "authorize")
    ok "authorized — capturable=#{auth.dig(:on_chain, :capturable_amount)}"

    step "3. void/payload → sign → submit"
    prep = client.payments.void_payload(payment_id)
    assert prep[:unsigned_transaction]

    client.payments.void(payment_id, signed_transaction: sign_eip1559(prep[:unsigned_transaction], account_key))
    final = poll_until_status(payment_id, "voided", waiting_for: "void")
    ok "voided — capturable=#{final.dig(:on_chain, :capturable_amount)}"

    assert_equal "0", final.dig(:on_chain, :capturable_amount), "capturable must be 0 after void"
  end
end
