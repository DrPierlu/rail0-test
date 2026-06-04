# frozen_string_literal: true

require_relative "../test_helper"

# Tests for the payment endpoints that work without direct DB access:
#   GET  /health
#   GET  /payments/:id
#   POST /payments
#   PUT  /payments/:id/sign

CHAIN_ID    = 5042002
FAKE_PAYER  = "0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266"
FAKE_PAYEE  = "0x70997970C51812dc3A010C7d01b50e0d17dc79C8"
FAKE_TOKEN  = "0x3600000000000000000000000000000000000000"

# ── GET /health ────────────────────────────────────────────────────────────────

describe "GET /health" do
  it "returns 200 with api_version and contract_version" do
    get "/health"
    assert_equal 200, last_response.status
    assert json_response[:api_version]
    assert json_response[:contract_version]
  end
end

# ── GET /payments/:id ──────────────────────────────────────────────────────────

describe "GET /payments/:id" do
  it "returns 422 for an unknown payment id" do
    get "/payments/0x#{"00" * 32}"
    assert_equal 422, last_response.status
    assert_equal "payment_not_found", json_response[:code]
  end
end

# ── POST /payments ─────────────────────────────────────────────────────────────

describe "POST /payments" do
  def valid_body
    {
      payment: {
        payer: FAKE_PAYER,
        payee: FAKE_PAYEE,
        token: FAKE_TOKEN,
        amount: "1.0"
      },
      chain_id: CHAIN_ID,
      mode: "authorize"
    }
  end

  it "returns 400 when the payment field is missing" do
    post_json "/payments", { chain_id: CHAIN_ID, mode: "authorize" }
    assert_equal 400, last_response.status
  end

  it "returns 400 when amount is missing" do
    body = valid_body
    body[:payment].delete(:amount)
    post_json "/payments", body
    assert_equal 400, last_response.status
  end

  it "returns 400 when chain_id is missing" do
    body = valid_body
    body.delete(:chain_id)
    post_json "/payments", body
    assert_equal 400, last_response.status
  end

  it "returns 400 for an invalid mode" do
    post_json "/payments", valid_body.merge(mode: "teleport")
    assert_equal 400, last_response.status
  end

  it "returns 400 when payer is missing" do
    body = valid_body
    body[:payment].delete(:payer)
    post_json "/payments", body
    assert_equal 400, last_response.status
  end

  it "returns 400 when payee is missing" do
    body = valid_body
    body[:payment].delete(:payee)
    post_json "/payments", body
    assert_equal 400, last_response.status
  end

  it "returns 400 when token is missing" do
    body = valid_body
    body[:payment].delete(:token)
    post_json "/payments", body
    assert_equal 400, last_response.status
  end

  it "returns 422 for an unsupported chain_id" do
    post_json "/payments", valid_body.merge(chain_id: 999999)
    assert_equal 422, last_response.status
  end

  it "returns 400 for invalid JSON" do
    http_request(:Post, "/payments",
                 body: "not json",
                 headers: { "Content-Type" => "application/json" })
    assert_equal 400, last_response.status
  end

  it "creates a payment in authorize mode and returns signing_payload" do
    post_json "/payments", valid_body
    assert_equal 201, last_response.status
    body = json_response
    assert_match(/\A0x[0-9a-f]{64}\z/, body[:rail0_id])
    assert body[:signing_payload]
    get "/payments/#{body[:rail0_id]}"
    state = json_response
    assert_equal "unsigned", state[:status]
    assert_equal "authorize", state[:mode]
  end

  it "creates a payment in charge mode" do
    post_json "/payments", valid_body.merge(mode: "charge")
    assert_equal 201, last_response.status
    body = json_response
    get "/payments/#{body[:rail0_id]}"
    assert_equal "charge", json_response[:mode]
  end
end

# ── PUT /payments/:id/sign ─────────────────────────────────────────────────────

describe "PUT /payments/:id/sign" do
  def create_payment
    post_json "/payments", {
      payment: { payer: FAKE_PAYER, payee: FAKE_PAYEE, token: FAKE_TOKEN, amount: "1.0" },
      chain_id: CHAIN_ID, mode: "authorize"
    }
    json_response[:rail0_id]
  end

  it "returns 400 when signature is missing" do
    pid = create_payment
    put_json "/payments/#{pid}/sign", {}
    assert_equal 400, last_response.status
  end

  it "returns 422 for a too-short signature" do
    pid = create_payment
    put_json "/payments/#{pid}/sign", { signature: "0x1234" }
    assert_equal 422, last_response.status
  end

  it "returns 422 for a signature without 0x prefix" do
    pid = create_payment
    put_json "/payments/#{pid}/sign", { signature: "aa" * 65 }
    assert_equal 422, last_response.status
  end

  it "returns 4xx for an unknown payment" do
    put_json "/payments/0x#{"00" * 32}/sign", { signature: "0x" + "aa" * 65 }
    assert last_response.status >= 400
  end
end
