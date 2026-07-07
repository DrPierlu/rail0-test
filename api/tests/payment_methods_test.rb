# frozen_string_literal: true

require_relative "../test_helper"

# GET /payment_methods — public, buyer-facing discovery of a merchant's active
# wallet/token combinations. No auth. Reachable by the merchant account_id (all
# its active wallets) or by a single wallet address (that one wallet). Exactly
# one of the two is required.

NONEXISTENT_ACCOUNT = "00000000-0000-0000-0000-000000000000"

describe "GET /payment_methods?account_id=" do
  it "returns a non-empty array for the seeded merchant — no auth needed" do
    get "/payment_methods?account_id=#{ACCOUNT_ID}"
    assert_equal 200, last_response.status
    assert_instance_of Array, json_response
    refute_empty json_response, "Seeded merchant must expose at least one payment method"
  end

  it "each wallet carries its active token holdings nested under :tokens" do
    get "/payment_methods?account_id=#{ACCOUNT_ID}"
    row = json_response.first
    %i[id address label active tokens].each do |key|
      assert row.key?(key), "Missing key :#{key} in wallet row"
    end
    holding = row[:tokens].first
    skip "Seeded wallet has no token holdings" unless holding
    %i[token active default].each do |key|
      assert holding.key?(key), "Missing key :#{key} in token holding"
    end
    %i[chain_id symbol address decimals].each do |key|
      assert holding[:token].key?(key), "Missing key :#{key} in holding token"
    end
    assert holding[:token][:decimals].is_a?(Integer) && holding[:token][:decimals] > 0
    assert holding[:token][:chain_id].is_a?(Integer) && holding[:token][:chain_id] > 0
  end

  it "addresses and ids are well-formed" do
    get "/payment_methods?account_id=#{ACCOUNT_ID}"
    json_response.each do |row|
      assert_match(/\A[0-9a-f-]{36}\z/, row[:id].to_s, "id must be a UUID")
      assert_match(/\A0x[0-9a-fA-F]{40}\z/, row[:address], "address must be a 40-char hex address")
      assert [true, false].include?(row[:active]), ":active must be a boolean"
    end
  end

  it "returns an empty array for an unknown account" do
    get "/payment_methods?account_id=#{NONEXISTENT_ACCOUNT}"
    assert_equal 200, last_response.status
    assert_equal [], json_response
  end
end

describe "GET /payment_methods?address=" do
  it "returns just the wallet with that address" do
    get "/payment_methods?address=#{SEEDED_ADDRESS}"
    assert_equal 200, last_response.status
    assert_instance_of Array, json_response
    refute_empty json_response, "Seeded payee wallet must be discoverable by address"
    json_response.each do |row|
      assert_equal SEEDED_ADDRESS, row[:address].downcase
    end
  end

  it "returns an empty array for an unknown address" do
    get "/payment_methods?address=0x#{'ab' * 20}"
    assert_equal 200, last_response.status
    assert_equal [], json_response
  end
end

describe "GET /payment_methods validation" do
  it "rejects a request with neither account_id nor address (400)" do
    get "/payment_methods"
    assert_equal 400, last_response.status
  end

  it "rejects a request with both account_id and address (400)" do
    get "/payment_methods?account_id=#{ACCOUNT_ID}&address=#{SEEDED_ADDRESS}"
    assert_equal 400, last_response.status
  end
end
