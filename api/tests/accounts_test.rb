# frozen_string_literal: true

require_relative "../test_helper"

NONEXISTENT_ID = "00000000-0000-0000-0000-000000000000"

# The whole /accounts section is behind SIWE: every wallet read requires a
# Bearer JWT (401 without one) and only the session's OWN account is readable
# (403 for any other account id in the path). Public, buyer-facing discovery of
# a merchant's wallets/tokens lives on GET /payment_methods (see
# payment_methods_test.rb), not here.

# ── GET /accounts/:id/wallets (JWT, own account only) ─────────────────────────

describe "GET /accounts/:id/wallets" do
  it "returns 401 without a token" do
    get "/accounts/#{ACCOUNT_ID}/wallets"
    assert_equal 401, last_response.status
  end

  it "returns 403 for an account that is not the session's own" do
    token = obtain_jwt
    get "/accounts/#{NONEXISTENT_ID}/wallets", headers: auth_headers(token)
    assert_equal 403, last_response.status
  end

  it "returns a non-empty array for the seeded account (own session)" do
    token = obtain_jwt
    get "/accounts/#{ACCOUNT_ID}/wallets", headers: auth_headers(token)
    assert_equal 200, last_response.status
    assert_instance_of Array, json_response
    refute_empty json_response, "Seeded account must have at least one wallet"
  end

  it "each row carries the wallet keys, its holdings nested under :tokens" do
    token = obtain_jwt
    get "/accounts/#{ACCOUNT_ID}/wallets", headers: auth_headers(token)
    row = json_response.first
    %i[id address label active tokens].each do |key|
      assert row.key?(key), "Missing key :#{key} in wallets row"
    end
  end
end

# ── GET /accounts/:id/wallets/:wallet_id (JWT, own account only) ──────────────

describe "GET /accounts/:id/wallets/:wallet_id" do
  it "returns 401 without a token" do
    get "/accounts/#{ACCOUNT_ID}/wallets/#{NONEXISTENT_ID}"
    assert_equal 401, last_response.status
  end

  it "returns 404 for a nonexistent wallet_id on the own account" do
    token = obtain_jwt
    get "/accounts/#{ACCOUNT_ID}/wallets/#{NONEXISTENT_ID}", headers: auth_headers(token)
    assert_equal 404, last_response.status
    assert_equal "not_found", json_response[:status]
    assert_equal "wallet", json_response[:resource]
  end

  it "returns the wallet by id" do
    token = obtain_jwt
    get "/accounts/#{ACCOUNT_ID}/wallets", headers: auth_headers(token)
    first = json_response.first
    skip "No wallets for seeded account" unless first

    get "/accounts/#{ACCOUNT_ID}/wallets/#{first[:id]}", headers: auth_headers(token)
    assert_equal 200, last_response.status
    row = json_response
    assert_equal first[:id],      row[:id]
    assert_equal first[:address], row[:address]
    assert_equal first[:label],   row[:label]
  end

  it "returns 403 when the account in the path is not the session's own" do
    token = obtain_jwt
    get "/accounts/#{ACCOUNT_ID}/wallets", headers: auth_headers(token)
    first = json_response.first
    skip "No wallets for seeded account" unless first

    # Same wallet id, but a different account in the path — the SIWE guard
    # rejects on account ownership before any wallet lookup.
    get "/accounts/#{NONEXISTENT_ID}/wallets/#{first[:id]}", headers: auth_headers(token)
    assert_equal 403, last_response.status
  end
end
