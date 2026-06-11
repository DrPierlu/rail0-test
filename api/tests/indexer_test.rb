# frozen_string_literal: true

require_relative "../test_helper"

# Tests for PUT /sync/chains/:chain_id/transactions/:tx_hash — the
# HMAC-authenticated endpoint used by the indexer to confirm or fail
# on-chain transactions.
#
# These tests cover authentication and input validation only. State-transition
# tests (confirm/fail side effects) require a real payment in submitted state
# and are covered by the SDK end-to-end flows in rail0-test/ruby/.

TX_HASH  = ("0x" + "dc" * 32).freeze
CHAIN_ID = 1

describe "PUT /sync/chains/:chain_id/transactions/:tx_hash — authentication" do
  it "returns 401 when HMAC headers are absent" do
    body = { operation: "confirm", event_type: "authorized", block_number: 1 }
    put_json "/sync/chains/#{CHAIN_ID}/transactions/#{TX_HASH}", body
    assert_equal 401, last_response.status
    assert_equal "unauthorized", json_response[:code]
  end

  it "returns 401 with a tampered signature" do
    body_str = { operation: "confirm", event_type: "authorized", block_number: 1 }.to_json
    ts = Time.now.to_i.to_s
    http_request(:Put, "/sync/chains/#{CHAIN_ID}/transactions/#{TX_HASH}",
                 body: body_str,
                 headers: {
                   "Content-Type"      => "application/json",
                   "X-Rail0-Timestamp" => ts,
                   "X-Rail0-Signature" => "0" * 64
                 })
    assert_equal 401, last_response.status
  end

  it "returns 401 for a stale timestamp outside the replay window" do
    body_str = { operation: "confirm", event_type: "authorized", block_number: 1 }.to_json
    stale_ts = (Time.now.to_i - 301).to_s
    sig = OpenSSL::HMAC.hexdigest("SHA256", HMAC_SECRET, "#{stale_ts}.#{body_str}")
    http_request(:Put, "/sync/chains/#{CHAIN_ID}/transactions/#{TX_HASH}",
                 body: body_str,
                 headers: {
                   "Content-Type"      => "application/json",
                   "X-Rail0-Timestamp" => stale_ts,
                   "X-Rail0-Signature" => sig
                 })
    assert_equal 401, last_response.status
  end
end

describe "PUT /sync/chains/:chain_id/transactions/:tx_hash — validation" do
  it "returns 422 for an unknown operation" do
    put_sync(CHAIN_ID, TX_HASH, operation: "bogus")
    assert_equal 422, last_response.status
  end

  it "returns 422 for an unknown event_type on confirm" do
    put_sync(CHAIN_ID, TX_HASH,
             operation: "confirm", event_type: "bogus", block_number: 1)
    assert_equal 422, last_response.status
    assert_equal "invalid_event_type", json_response[:status]
  end

  it "returns 422 when block_number is missing on confirm" do
    put_sync(CHAIN_ID, TX_HASH,
             operation: "confirm", event_type: "authorized")
    assert_equal 422, last_response.status
    assert_equal "missing_param", json_response[:status]
  end

  it "returns 404 when the transaction does not exist" do
    unknown_hash = "0x" + "00" * 32
    put_sync(CHAIN_ID, unknown_hash,
             operation: "confirm", event_type: "authorized", block_number: 1)
    assert_equal 404, last_response.status
  end
end
