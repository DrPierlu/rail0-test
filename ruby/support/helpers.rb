# frozen_string_literal: true
#
# Shared helpers for all Ruby integration tests.
# Each test class includes this module.

require "eth"
require "rail0"
require "rail0/signing"

module IntegrationHelpers
  POLL_TIMEOUT  = 120
  POLL_INTERVAL = 2

  # ── Client ──────────────────────────────────────────────────────────────────

  def account_jwt
    @account_jwt ||= begin
      base_url = ENV.fetch("RAIL0_API_URL", "http://localhost:4567")
      domain   = URI.parse(base_url).host
      unauthenticated_client = Rail0::Client.new(base_url: base_url)
      resp = unauthenticated_client.auth.login(
        private_key: ENV.fetch("ACCOUNT_PRIVATE_KEY"),
        domain:      domain
      )
      resp[:token]
    end
  end

  def client
    @client ||= Rail0::Client.new(
      base_url: ENV.fetch("RAIL0_API_URL", "http://localhost:4567"),
      headers:  { "Authorization" => "Bearer #{account_jwt}" }
    )
  end

  def buyer_key
    @buyer_key ||= Eth::Key.new(priv: ENV.fetch("BUYER_PRIVATE_KEY"))
  end

  def account_key
    @account_key ||= Eth::Key.new(priv: ENV.fetch("ACCOUNT_PRIVATE_KEY"))
  end

  def buyer_address  = ENV.fetch("BUYER_ADDRESS")
  def account_id     = ENV.fetch("ACCOUNT_ID")
  def chain_id       = ENV.fetch("CHAIN_ID", "5042002").to_i
  def chain_slug     = ENV.fetch("CHAIN_SLUG", "arc-testnet")
  def token_symbol   = ENV.fetch("TOKEN_SYMBOL", "USDC")
  def amount         = ENV.fetch("AMOUNT", "1000000")

  # ── Payment method discovery ─────────────────────────────────────────────────

  def discover_payment_method
    tokens = client.accounts.wallets(account_id)
    pm = tokens.find { |m| m[:chain_slug] == chain_slug && m[:token_symbol] == token_symbol }
    raise "No #{token_symbol} payment method found on #{chain_slug} for account #{account_id}" unless pm
    assert_equal chain_id, pm[:chain_id]
    assert_equal account_key.address.to_s.downcase, pm[:address].downcase,
                 "ACCOUNT_PRIVATE_KEY does not match wallet address from API"
    pm
  end

  # ── EIP-3009 payer signature ─────────────────────────────────────────────────

  def sign_payment(private_key, signing_payload)
    sig = Rail0::Signing.sign_payload(private_key, signing_payload)
    "0x#{sig.r[2..]}#{sig.s[2..]}#{sig.v.to_s(16).rjust(2, '0')}"
  end

  # Returns { signature: } as a 0x-prefixed hex string, suitable for refund_prepare phase 2.
  def sign_payload_vrs(private_key, signing_payload)
    sig = Rail0::Signing.sign_payload(private_key, signing_payload)
    { signature: sig.to_hex }
  end

  # ── EIP-1559 tx signing ──────────────────────────────────────────────────────

  def sign_eip1559(unsigned_hex, key)
    tx = Eth::Tx::Eip1559.decode(unsigned_hex)
    tx.sign(key)
    "0x" + tx.encoded.unpack1("H*")
  end

  # ── Common payment setup ──────────────────────────────────────────────────────
  #
  # Creates a payment, signs it as payer, and submits the signature.
  # Returns [payment_id, payment_method] ready for the payee to act on.

  def create_and_sign_payment(mode: "authorize", payment_amount: amount)
    pm = discover_payment_method

    create_resp = client.payments.create(
      payment:  { payer: buyer_address, payee: pm[:address],
                  token: pm[:token_address], amount: payment_amount },
      chain_id: chain_id,
      mode:     mode
    )

    payment_id = create_resp[:rail0_id]
    assert_match(/\A0x[0-9a-f]{64}\z/, payment_id, "rail0_id must be a bytes32 hex string")

    signature  = sign_payment(ENV.fetch("BUYER_PRIVATE_KEY"), create_resp[:signing_payload])
    sign_resp  = client.payments.sign(payment_id, signature: signature)
    assert_equal "signature_stored", sign_resp[:status]
    assert_equal buyer_address.downcase, sign_resp[:recovered_payer].downcase

    [payment_id, pm]
  end

  # ── Polling ──────────────────────────────────────────────────────────────────

  def poll_until_status(payment_id, *expected, waiting_for:, timeout: POLL_TIMEOUT, interval: POLL_INTERVAL)
    deadline = Time.now + timeout
    loop do
      state  = client.payments.get(payment_id)
      status = state[:status]
      txs    = Array(state[:transactions]).map { |t| "#{t[:operation]}:#{t[:status]}" }.join(", ")
      step "  [poll] #{waiting_for}: status=#{status}#{txs.empty? ? '' : " tx=[#{txs}]"}"
      return state if expected.include?(status)
      if status == "failed"
        raise "Payment #{payment_id} failed — code=#{state[:last_error_code]} msg=#{state[:last_error_message]}"
      end
      raise "Timed out after #{timeout}s waiting for #{expected.join('/')} (last: #{status})" if Time.now >= deadline
      sleep interval
    end
  end

  # ── Output helpers ───────────────────────────────────────────────────────────

  def desc(title, *lines)
    $stdout.puts "\n#{"─" * 70}\n  #{title}\n#{"─" * 70}"
    lines.each { |l| $stdout.puts "  #{l}" }
    $stdout.puts "─" * 70
  end

  def step(msg) = $stdout.puts("  → #{msg}")
  def ok(msg)   = $stdout.puts("    ✓ #{msg}")
end
