# frozen_string_literal: true

require_relative "../test_helper"

# Tests for POST /sync/transactions — the HMAC-authenticated endpoint used by
# the indexer sweeper and event handlers.
#
# These tests cover authentication and input validation only. State-transition
# tests (confirm/fail side effects) require a real payment in submitted state
# and are covered by the SDK end-to-end flows in rail0-test/ruby/.

describe "POST /sync/transactions — authentication" do
  it "returns 401 when HMAC headers are absent" do
    body = { transaction_hash: "0x" + "dc" * 32, chain_id: 1,
             operation: "confirm", payment_id: "0x" + "ab" * 32,
             event_type: "authorized", block_number: 1 }
    post_json "/sync/transactions", body
    assert_equal 401, last_response.status
    assert_equal "unauthorized", json_response[:code]
  end

  it "returns 401 with a tampered signature" do
    body_str = { transaction_hash: "0x" + "dc" * 32, chain_id: 1,
                 operation: "confirm", payment_id: "0x" + "ab" * 32,
                 event_type: "authorized", block_number: 1 }.to_json
    ts  = Time.now.to_i.to_s
    http_request(:Post, "/sync/transactions",
                 body: body_str,
                 headers: {
                   "Content-Type"       => "application/json",
                   "X-Rail0-Timestamp"  => ts,
                   "X-Rail0-Signature"  => "0" * 64
                 })
    assert_equal 401, last_response.status
  end

  it "returns 401 for a stale timestamp outside the replay window" do
    body_str = { transaction_hash: "0x" + "dc" * 32, chain_id: 1,
                 operation: "confirm", payment_id: "0x" + "ab" * 32,
                 event_type: "authorized", block_number: 1 }.to_json
    stale_ts = (Time.now.to_i - 301).to_s
    sig = OpenSSL::HMAC.hexdigest("SHA256", HMAC_SECRET, "#{stale_ts}.#{body_str}")
    http_request(:Post, "/sync/transactions",
                 body: body_str,
                 headers: {
                   "Content-Type"       => "application/json",
                   "X-Rail0-Timestamp"  => stale_ts,
                   "X-Rail0-Signature"  => sig
                 })
    assert_equal 401, last_response.status
  end
end

describe "POST /sync/transactions — validation" do
  it "returns 422 for an unknown operation" do
    post_sync(transaction_hash: "0x" + "dc" * 32, chain_id: 1,
              operation: "bogus", payment_id: "0x" + "ab" * 32, block_number: 1)
    assert_equal 422, last_response.status
    assert_equal "invalid_operation", json_response[:code]
  end

  it "returns 400 when transaction_hash is missing" do
    post_sync(chain_id: 1, operation: "confirm",
              payment_id: "0x" + "ab" * 32, event_type: "authorized", block_number: 1)
    assert_equal 400, last_response.status
    assert_equal "missing_transaction_hash", json_response[:code]
  end

  it "returns 400 when payment_id is missing" do
    post_sync(transaction_hash: "0x" + "dc" * 32, chain_id: 1,
              operation: "confirm", event_type: "authorized", block_number: 1)
    assert_equal 400, last_response.status
    assert_equal "missing_payment_id", json_response[:code]
  end

  it "returns 400 when block_number is missing" do
    post_sync(transaction_hash: "0x" + "dc" * 32, chain_id: 1,
              operation: "confirm", payment_id: "0x" + "ab" * 32, event_type: "authorized")
    assert_equal 400, last_response.status
    assert_equal "missing_block_number", json_response[:code]
  end

  it "returns 422 for an unknown event_type on confirm" do
    post_sync(transaction_hash: "0x" + "dc" * 32, chain_id: 1,
              operation: "confirm", payment_id: "0x" + "ab" * 32,
              event_type: "bogus", block_number: 1)
    assert_equal 422, last_response.status
    assert_equal "invalid_event_type", json_response[:code]
  end

  it "returns 404 when the payment does not exist" do
    post_sync(transaction_hash: "0x" + "dc" * 32, chain_id: 1,
              operation: "confirm", payment_id: "0x" + "00" * 32,
              event_type: "authorized", block_number: 1)
    assert_equal 404, last_response.status
    assert_equal "payment_not_found", json_response[:code]
  end
end
