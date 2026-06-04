# frozen_string_literal: true

require_relative "../test_helper"

NONEXISTENT_ID = "00000000-0000-0000-0000-000000000000"

# ── GET /accounts/:id/payment-methods ─────────────────────────────────────────

describe "GET /accounts/:id/payment-methods" do
  it "returns 200 with an empty array for an unknown account" do
    get "/accounts/#{NONEXISTENT_ID}/payment-methods"
    assert_equal 200, last_response.status
    assert_equal [], json_response
  end

  it "returns a non-empty array for the seeded account" do
    get "/accounts/#{ACCOUNT_ID}/payment-methods"
    assert_equal 200, last_response.status
    assert_instance_of Array, json_response
    refute_empty json_response, "Seeded account must have at least one payment method"
  end

  it "each row includes the required keys" do
    get "/accounts/#{ACCOUNT_ID}/payment-methods"
    row = json_response.first
    %i[id token_id chain_id chain_name chain_slug explorer_url
       token_address token_symbol token_decimals wallet_address default].each do |key|
      assert row.key?(key), "Missing key :#{key} in payment-methods row"
    end
  end

  it "wallet_address is a valid 0x-prefixed Ethereum address" do
    get "/accounts/#{ACCOUNT_ID}/payment-methods"
    json_response.each do |row|
      assert_match(/\A0x[0-9a-fA-F]{40}\z/, row[:wallet_address],
                   "wallet_address must be a 40-char hex address")
    end
  end

  it "chain_id is a positive integer" do
    get "/accounts/#{ACCOUNT_ID}/payment-methods"
    json_response.each do |row|
      assert row[:chain_id].is_a?(Integer) && row[:chain_id] > 0,
             "chain_id must be a positive integer"
    end
  end

  it "filters by blockchain_slug" do
    get "/accounts/#{ACCOUNT_ID}/payment-methods?blockchain_slug=arc-testnet"
    assert_equal 200, last_response.status
    json_response.each do |row|
      assert_equal "arc-testnet", row[:chain_slug]
    end
  end

  it "returns empty array for a slug that matches no payment method" do
    get "/accounts/#{ACCOUNT_ID}/payment-methods?blockchain_slug=nonexistent-chain"
    assert_equal 200, last_response.status
    assert_equal [], json_response
  end
end

# ── GET /accounts/:id/wallets ─────────────────────────────────────────────────

describe "GET /accounts/:id/wallets" do
  it "returns 200 with an empty array for an unknown account" do
    get "/accounts/#{NONEXISTENT_ID}/wallets"
    assert_equal 200, last_response.status
    assert_equal [], json_response
  end

  it "returns a non-empty array for the seeded account" do
    get "/accounts/#{ACCOUNT_ID}/wallets"
    assert_equal 200, last_response.status
    assert_instance_of Array, json_response
    refute_empty json_response, "Seeded account must have at least one wallet"
  end

  it "each row includes the required keys" do
    get "/accounts/#{ACCOUNT_ID}/wallets"
    row = json_response.first
    %i[id wallet_id address default active
       token_id token_symbol token_address token_decimals
       chain_id chain_name chain_slug].each do |key|
      assert row.key?(key), "Missing key :#{key} in wallets row"
    end
  end

  it "id is the wallet_token id (uuid)" do
    get "/accounts/#{ACCOUNT_ID}/wallets"
    json_response.each do |row|
      assert_match(/\A[0-9a-f-]{36}\z/, row[:id].to_s, "id must be a UUID")
    end
  end

  it "wallet_id is a separate uuid from id" do
    get "/accounts/#{ACCOUNT_ID}/wallets"
    row = json_response.first
    refute_equal row[:id], row[:wallet_id],
                 "wallet_token.id and wallet.id must be different UUIDs"
  end

  it "address is a valid 0x-prefixed Ethereum address" do
    get "/accounts/#{ACCOUNT_ID}/wallets"
    json_response.each do |row|
      assert_match(/\A0x[0-9a-fA-F]{40}\z/, row[:address],
                   "address must be a 40-char hex address")
    end
  end

  it "active and default are booleans" do
    get "/accounts/#{ACCOUNT_ID}/wallets"
    json_response.each do |row|
      assert [true, false].include?(row[:active]),  ":active must be a boolean"
      assert [true, false].include?(row[:default]), ":default must be a boolean"
    end
  end

  it "token_decimals is a positive integer" do
    get "/accounts/#{ACCOUNT_ID}/wallets"
    json_response.each do |row|
      assert row[:token_decimals].is_a?(Integer) && row[:token_decimals] > 0,
             "token_decimals must be a positive integer"
    end
  end

  it "chain_id is a positive integer" do
    get "/accounts/#{ACCOUNT_ID}/wallets"
    json_response.each do |row|
      assert row[:chain_id].is_a?(Integer) && row[:chain_id] > 0,
             "chain_id must be a positive integer"
    end
  end
end

# ── GET /accounts/:id/wallets/:wallet_token_id ────────────────────────────────

describe "GET /accounts/:id/wallets/:wallet_token_id" do
  it "returns 422 for a nonexistent wallet_token_id" do
    get "/accounts/#{ACCOUNT_ID}/wallets/#{NONEXISTENT_ID}"
    assert_equal 422, last_response.status
    assert_equal "wallet_not_found", json_response[:code]
  end

  it "returns the wallet_token by id" do
    get "/accounts/#{ACCOUNT_ID}/wallets"
    first = json_response.first
    skip "No wallets for seeded account" unless first

    get "/accounts/#{ACCOUNT_ID}/wallets/#{first[:id]}"
    assert_equal 200, last_response.status
    row = json_response
    assert_equal first[:id],        row[:id]
    assert_equal first[:wallet_id], row[:wallet_id]
    assert_equal first[:address],   row[:address]
    assert_equal first[:token_id],  row[:token_id]
  end

  it "returns 422 when the wallet_token belongs to a different account" do
    get "/accounts/#{ACCOUNT_ID}/wallets"
    first = json_response.first
    skip "No wallets for seeded account" unless first

    get "/accounts/#{NONEXISTENT_ID}/wallets/#{first[:id]}"
    assert_equal 422, last_response.status
    assert_equal "wallet_not_found", json_response[:code]
  end
end
