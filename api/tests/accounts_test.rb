# frozen_string_literal: true

require_relative "../test_helper"

NONEXISTENT_ID = "00000000-0000-0000-0000-000000000000"


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
    %i[id address label active tokens].each do |key|
      assert row.key?(key), "Missing key :#{key} in wallets row"
    end
    holding = row[:tokens].first
    %i[token active default].each do |key|
      assert holding.key?(key), "Missing key :#{key} in token holding"
    end
    %i[chain_id symbol address decimals].each do |key|
      assert holding[:token].key?(key), "Missing key :#{key} in holding token"
    end
  end

  it "id is the wallet id (uuid)" do
    get "/accounts/#{ACCOUNT_ID}/wallets"
    json_response.each do |row|
      assert_match(/\A[0-9a-f-]{36}\z/, row[:id].to_s, "id must be a UUID")
    end
  end

  it "address is a valid 0x-prefixed Ethereum address" do
    get "/accounts/#{ACCOUNT_ID}/wallets"
    json_response.each do |row|
      assert_match(/\A0x[0-9a-fA-F]{40}\z/, row[:address],
                   "address must be a 40-char hex address")
    end
  end

  it "the wallet active flag is a boolean" do
    get "/accounts/#{ACCOUNT_ID}/wallets"
    json_response.each do |row|
      assert [true, false].include?(row[:active]), ":active must be a boolean"
    end
  end

  it "each token holding active and default are booleans" do
    get "/accounts/#{ACCOUNT_ID}/wallets"
    json_response.each do |row|
      row[:tokens].each do |holding|
        assert [true, false].include?(holding[:active]),  "holding :active must be a boolean"
        assert [true, false].include?(holding[:default]), "holding :default must be a boolean"
      end
    end
  end

  it "token decimals is a positive integer" do
    get "/accounts/#{ACCOUNT_ID}/wallets"
    json_response.each do |row|
      row[:tokens].each do |holding|
        decimals = holding[:token][:decimals]
        assert decimals.is_a?(Integer) && decimals > 0,
               "token decimals must be a positive integer"
      end
    end
  end

  it "token chain_id is a positive integer" do
    get "/accounts/#{ACCOUNT_ID}/wallets"
    json_response.each do |row|
      row[:tokens].each do |holding|
        chain_id = holding[:token][:chain_id]
        assert chain_id.is_a?(Integer) && chain_id > 0,
               "token chain_id must be a positive integer"
      end
    end
  end
end

# ── GET /accounts/:id/wallets/:wallet_id ──────────────────────────────────────

describe "GET /accounts/:id/wallets/:wallet_id" do
  it "returns 404 for a nonexistent wallet_id" do
    get "/accounts/#{ACCOUNT_ID}/wallets/#{NONEXISTENT_ID}"
    assert_equal 404, last_response.status
    assert_equal "not_found", json_response[:status]
    assert_equal "wallet", json_response[:resource]
  end

  it "returns the wallet by id" do
    get "/accounts/#{ACCOUNT_ID}/wallets"
    first = json_response.first
    skip "No wallets for seeded account" unless first

    get "/accounts/#{ACCOUNT_ID}/wallets/#{first[:id]}"
    assert_equal 200, last_response.status
    row = json_response
    assert_equal first[:id],      row[:id]
    assert_equal first[:address], row[:address]
    assert_equal first[:label],   row[:label]
  end

  it "returns 404 when the wallet belongs to a different account" do
    get "/accounts/#{ACCOUNT_ID}/wallets"
    first = json_response.first
    skip "No wallets for seeded account" unless first

    get "/accounts/#{NONEXISTENT_ID}/wallets/#{first[:id]}"
    assert_equal 404, last_response.status
    assert_equal "not_found", json_response[:status]
    assert_equal "wallet", json_response[:resource]
  end
end
