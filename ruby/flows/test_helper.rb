# frozen_string_literal: true

require "minitest/autorun"
require "minitest/spec"
require "uri"
require "rail0"
require "rail0/signing"

# End-to-end flows that drive the rail0-ruby SDK against a live gateway. Each
# flow creates a payment, signs the payer's EIP-3009 authorization, then runs
# each operation (authorize/capture/refund/charge) by preparing the unsigned tx,
# signing it locally, submitting it, and polling GET /payments/:id for the
# expected status.
#
# Payee operations (authorize/capture/refund prepare+submit) are SIWE-gated, so
# the flows authenticate as the payee via the SDK's auth.login and drive those
# calls through a JWT-bearing client. Payment creation and the payer signature
# are public.
#
# Required env (see ../../.env.example):
#   RAIL0_API_URL         gateway base URL (default http://localhost:9292)
#   RAIL0_ACCOUNT_ID      merchant account UUID (ACCOUNT_ID also accepted)
#   ACCOUNT_PRIVATE_KEY   payee (merchant) key — authorizes/captures/refunds
#   BUYER_PRIVATE_KEY     payer (buyer) key — signs the EIP-3009 authorization
#   CHAIN_ID              chain id (default 5042002, Arc Testnet)
#   TOKEN_SYMBOL          token to pay with (default USDC)
#   AMOUNT                human-decimal amount (default "1.00")
# Optional:
#   SIWE_DOMAIN           SIWE login domain; defaults to the API URL host. Must
#                         match the gateway's SIWE_DOMAIN policy ("localhost" in
#                         dev).
module FlowHelpers
  POLL_TIMEOUT  = 120 # seconds
  POLL_INTERVAL = 2   # seconds

  # Fetch a required env var, failing the test if absent.
  def env(key)
    v = ENV[key]
    flunk "required env var #{key} is not set" if v.nil? || v.empty?
    v
  end

  def env_or(key, default)
    v = ENV[key]
    v.nil? || v.empty? ? default : v
  end

  def api_url = env_or("RAIL0_API_URL", "http://localhost:9292")

  # Accept ACCOUNT_ID (go/cli suites) or RAIL0_ACCOUNT_ID (api suite / .env).
  def account_id = ENV["ACCOUNT_ID"] || env("RAIL0_ACCOUNT_ID")

  def chain_id = Integer(env_or("CHAIN_ID", "5042002"))
  def token_symbol = env_or("TOKEN_SYMBOL", "USDC")
  def amount = env_or("AMOUNT", "1.00")

  # SIWE login domain — the gateway binds the message to its SIWE_DOMAIN policy
  # (a bare host). Default to the API URL host so localhost dev works out of the box.
  def siwe_domain = ENV["SIWE_DOMAIN"] || URI(api_url).host

  # SIWE login chain id — must match the gateway's SIWE_CHAIN_ID policy (default 1).
  # Set SIWE_CHAIN_ID to match how the target gateway was booted (the api suite,
  # for instance, boots it with 5042002).
  def siwe_chain_id = Integer(env_or("SIWE_CHAIN_ID", "1"))

  # Unauthenticated client — for public calls (payment_methods, create, sign, get).
  def new_client = Rail0::Client.new(base_url: api_url)

  # Payee client authenticated via SIWE, for the JWT-gated payee operations.
  # Memoised so we log in once per flow.
  def payee_client
    @payee_client ||= begin
      auth = new_client.auth.login(private_key: env("ACCOUNT_PRIVATE_KEY"), domain: siwe_domain, chain_id: siwe_chain_id)
      Rail0::Client.new(base_url: api_url, headers: { "Authorization" => "Bearer #{auth[:token]}" })
    end
  end

  # Payer (buyer) EVM address, derived from BUYER_PRIVATE_KEY.
  def payer_address
    @payer_address ||= begin
      hex = env("BUYER_PRIVATE_KEY").sub(/\A0x/, "")
      Eth::Key.new(priv: hex).address.to_s
    end
  end

  # Payment method (payee wallet address + token) matching the configured chain
  # and token symbol, discovered via the public GET /payment_methods endpoint
  # scoped to the merchant account.
  def discover_payment_method(client)
    wallets = client.payment_methods.list(account_id: account_id)
    wallets.each do |w|
      (w[:tokens] || []).each do |h|
        t = h[:token]
        return { address: w[:address], token: t } if t[:symbol] == token_symbol && t[:chain_id] == chain_id
      end
    end
    flunk "no #{token_symbol} payment method on chain #{chain_id} for account #{account_id}"
  end

  # Create a payment and submit the payer's EIP-3009 signature; returns the
  # rail0_id. Reads the signing payload the gateway returns while the payment is
  # unsigned, signs it with the buyer key, and stores it via PUT /payments/:id/sign.
  def create_and_sign(client, payment_method, mode)
    created = client.payments.create(
      chain_id: chain_id,
      mode:     mode,
      amount:   amount,
      token:    payment_method[:token][:address],
      payer:    payer_address,
      payee:    payment_method[:address]
    )
    refute_nil created[:signing_payload], "create must return signing_payload while unsigned"

    sig = Rail0::Signing.sign_payload(env("BUYER_PRIVATE_KEY"), created[:signing_payload])
    signed = client.payments.sign(created[:rail0_id], { signature: sig.to_hex })
    assert_equal "signed", signed[:status], "expected status 'signed' after depositing the signature"
    created[:rail0_id]
  end

  # Sign a prepare step's unsigned EIP-1559 transaction with the given key.
  def sign_tx(unsigned_transaction, private_key)
    Rail0::Signing.sign_transaction(unsigned_transaction, private_key)
  end

  # Poll GET /payments/:id until the status matches one of `expected`, failing on
  # timeout or a "failed" status (surfacing the decoded on-chain error).
  def poll_until_status(client, payment_id, *expected)
    deadline = Time.now + POLL_TIMEOUT
    loop do
      state = client.payments.get(payment_id)
      puts "  [poll] status=#{state[:status]}"
      return state if expected.include?(state[:status])

      if state[:status] == "failed"
        flunk "payment failed: code=#{state[:last_error_code]} msg=#{state[:last_error_message]}"
      end
      flunk "timed out waiting for #{expected.inspect} (last: #{state[:status]})" if Time.now > deadline
      sleep POLL_INTERVAL
    end
  end
end

class Minitest::Spec
  include FlowHelpers
end
