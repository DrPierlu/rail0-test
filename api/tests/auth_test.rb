# frozen_string_literal: true

require_relative "../test_helper"
require "base64"

# Tests for the authentication endpoints:
#   POST /auth/nonces  — issue a single-use nonce
#   POST /auth    — verify SIWE message + signature, return JWT
#   GET  /payments — verify JWT enforcement

describe "POST /auth/nonces" do
  it "returns 201 with a hex nonce and expires_at" do
    nonce = obtain_nonce
    assert_equal 201, last_response.status
    assert_match(/\A[0-9a-f]{32}\z/, nonce)
    assert_match(/\A\d{4}-\d{2}-\d{2}T/, json_response[:expires_at])
  end

  it "each call produces a unique nonce" do
    n1 = obtain_nonce
    n2 = obtain_nonce
    refute_equal n1, n2
  end
end

describe "POST /auth" do
  it "returns 201 with token, address, account_id, expires_at on valid SIWE" do
    token = obtain_jwt
    assert_equal 201, last_response.status
    body = json_response
    assert body[:token]
    assert_equal SEEDED_ADDRESS, body[:address].downcase
    assert body[:account_id]
    assert body[:expires_at]
  end

  it "token is a valid JWT with correct sub claim" do
    token = obtain_jwt
    # Decode without verification — we're just checking the payload shape.
    # JWT segments are base64url with padding stripped; re-pad to a multiple of
    # 4 before decoding (urlsafe_decode64 is strict about padding).
    seg = token.split(".")[1]
    seg += "=" * ((4 - seg.length % 4) % 4)
    payload = JSON.parse(Base64.urlsafe_decode64(seg), symbolize_names: true)
    assert_equal SEEDED_ADDRESS, payload[:sub].downcase
    assert payload[:exp].to_i > Time.now.to_i
  end

  it "rejects a replayed nonce" do
    nonce   = obtain_nonce
    message = siwe_message(nonce: nonce)
    sig     = SEEDED_KEY.personal_sign(message)
    post_json "/auth", { message: message, signature: sig }  # first — succeeds
    post_json "/auth", { message: message, signature: sig }  # replay

    assert last_response.status >= 400
    assert_equal "nonce_used", json_response[:status]
  end

  it "rejects an unknown nonce" do
    message = siwe_message(nonce: "deadbeefdeadbeefdeadbeefdeadbeef")
    sig     = SEEDED_KEY.personal_sign(message)
    post_json "/auth", { message: message, signature: sig }

    assert last_response.status >= 400
    assert_equal "invalid_nonce", json_response[:status]
  end

  it "rejects a wrong signature (different key)" do
    nonce   = obtain_nonce
    message = siwe_message(nonce: nonce)
    sig     = UNKNOWN_KEY.personal_sign(message)  # signed with different key
    post_json "/auth", { message: message, signature: sig }

    assert last_response.status >= 400
    assert_equal "signer_mismatch", json_response[:status]
  end

  it "rejects a request without message" do
    post_json "/auth", { signature: "0x00" }
    assert last_response.status >= 400
  end

  it "rejects a request without signature" do
    post_json "/auth", { message: "hello" }
    assert last_response.status >= 400
  end

  it "rejects an unparseable SIWE message" do
    nonce = obtain_nonce
    post_json "/auth", { message: "not a siwe message", signature: "0x00" }
    assert last_response.status >= 400
    assert_equal "invalid_siwe", json_response[:status]
  end

  it "rejects an address not in the wallets table" do
    nonce   = obtain_nonce
    message = siwe_message(nonce: nonce, address: UNKNOWN_KEY.address.to_s)
    sig     = UNKNOWN_KEY.personal_sign(message)
    post_json "/auth", { message: message, signature: sig }

    assert_equal 422, last_response.status
    assert_equal "address_not_registered", json_response[:status]
  end
end

describe "GET /payments — auth enforcement" do
  it "returns 401 without a token" do
    get "/payments"
    assert_equal 401, last_response.status
  end

  it "returns 401 with an invalid token" do
    get "/payments", headers: auth_headers("not.a.token")
    assert_equal 401, last_response.status
  end

  it "returns 200 with a valid JWT" do
    token = obtain_jwt
    get "/payments", headers: auth_headers(token)
    assert_equal 200, last_response.status
    assert_instance_of Array, json_response
  end
end
