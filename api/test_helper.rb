# frozen_string_literal: true

require "minitest/autorun"
require "minitest/spec"
require "net/http"
require "uri"
require "json"
require "openssl"
require "securerandom"
require "eth"

# ── HTTP client ────────────────────────────────────────────────────────────────

BASE_URL       = ENV.fetch("RAIL0_API_URL",  "http://localhost:9292")
ACCOUNT_ID     = ENV.fetch("RAIL0_ACCOUNT_ID")           # seeded TEST_ACCOUNT_ID
HMAC_SECRET    = ENV.fetch("RAIL0_SYNC_HMAC_SECRET")  # HMAC secret for /sync/*

# EVM key matching the wallet seeded in db/seed.rb (address 0xe3dd5ea618f74de4c95b0ff0ebf9921a06a694c4).
# Read from ENV (rule 8: never hardcode secrets) — same key the flow suites use.
SEEDED_KEY     = Eth::Key.new(priv: ENV.fetch("ACCOUNT_PRIVATE_KEY"))
SEEDED_ADDRESS = SEEDED_KEY.address.to_s.downcase

# A key whose address is NOT registered in wallets.
UNKNOWN_KEY    = Eth::Key.new(priv: "0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80")

Response = Struct.new(:status, :body)

module ApiHelpers
  def get(path, headers: {})
    http_request(:Get, path, headers: headers)
  end

  def post_json(path, body = {}, headers: {})
    http_request(:Post, path, body: body.to_json, headers: { "Content-Type" => "application/json" }.merge(headers))
  end

  def put_json(path, body = {}, headers: {})
    http_request(:Put, path, body: body.to_json, headers: { "Content-Type" => "application/json" }.merge(headers))
  end

  def json_response
    JSON.parse(last_response.body, symbolize_names: true)
  end

  def last_response = @last_response

  # Signs a request body (or empty string for GET) with HMAC-SHA256.
  def hmac_headers(body_str = "")
    ts  = Time.now.to_i.to_s
    sig = OpenSSL::HMAC.hexdigest("SHA256", HMAC_SECRET, "#{ts}.#{body_str}")
    { "X-Rail0-Timestamp" => ts, "X-Rail0-Signature" => sig }
  end

  def put_sync(chain_id, tx_hash, body = {})
    body_str = body.to_json
    put_json "/sync/chains/#{chain_id}/transactions/#{tx_hash}", body,
             headers: hmac_headers(body_str).merge("Content-Type" => "application/json")
  end

  # Issues a fresh nonce via POST /nonces and returns the nonce string.
  def obtain_nonce
    @last_response = post_json "/auth/nonces", {}
    json_response[:value]
  end

  # Builds a minimal EIP-4361 message that the API's siwe parser accepts.
  def siwe_message(nonce:, address: SEEDED_KEY.address.to_s, chain_id: 5042002)
    [
      "localhost wants you to sign in with your Ethereum account:",
      address,
      "",
      "Sign in to RAIL0",
      "",
      "URI: https://localhost",
      "Version: 1",
      "Chain ID: #{chain_id}",
      "Nonce: #{nonce}",
      "Issued At: #{Time.now.utc.iso8601}"
    ].join("\n")
  end

  # Full SIWE login flow — returns the JWT token string.
  def obtain_jwt(key: SEEDED_KEY)
    nonce   = obtain_nonce
    message = siwe_message(nonce: nonce, address: key.address.to_s)
    sig     = key.personal_sign(message)
    @last_response = post_json "/auth", { message: message, signature: sig }
    json_response[:token]
  end

  def auth_headers(token)
    { "Authorization" => "Bearer #{token}" }
  end

  private

  def http_request(method_name, path, body: nil, headers: {})
    uri = URI("#{BASE_URL}#{path}")
    Net::HTTP.start(uri.host, uri.port, use_ssl: uri.scheme == "https",
                    read_timeout: 15, open_timeout: 10) do |http|
      req = Net::HTTP.const_get(method_name).new(uri.request_uri)
      headers.each { |k, v| req[k] = v }
      req.body = body if body
      raw = http.request(req)
      @last_response = Response.new(raw.code.to_i, raw.body)
    end
  end
end

class Minitest::Spec
  include ApiHelpers
end
