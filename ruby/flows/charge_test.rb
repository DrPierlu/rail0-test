# frozen_string_literal: true

# Flow: create (mode=charge) → sign → charge, via rail0-ruby.
#
# Run:
#   set -a && source ../../.env && set +a
#   bundle exec ruby -Iflows flows/charge_test.rb

require_relative "test_helper"

describe "charge (rail0-ruby)" do
  it "charges a payment in one shot" do
    client   = new_client
    payee_pk = env("ACCOUNT_PRIVATE_KEY")
    pm       = discover_payment_method(client)

    puts "→ creating payment (mode=charge) and submitting payer signature"
    payment_id = create_and_sign(client, pm, "charge")
    puts "  payment_id=#{payment_id}"

    puts "→ charge/prepare"
    prep = payee_client.payments.charge_prepare(payment_id)
    payee_client.payments.charge(payment_id, { signed_transaction: sign_tx(prep[:unsigned_transaction], payee_pk) })
    final = poll_until_status(client, payment_id, "charged")
    puts "  charged — status=#{final[:status]}"
  end
end
