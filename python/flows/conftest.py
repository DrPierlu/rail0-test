"""Shared fixtures for all Python integration tests."""

import os
import time

import pytest
from eth_account import Account as EthAccount

from rail0 import Rail0Client
from rail0.signing import sign_payload


# ── Environment helpers ──────────────────────────────────────────────────────

def get_env(key: str, default: str | None = None) -> str:
    v = os.environ.get(key, default)
    if not v:
        raise RuntimeError(f"Required env var {key} is not set")
    return v


# ── Fixtures ─────────────────────────────────────────────────────────────────

@pytest.fixture(scope="session")
def client() -> Rail0Client:
    return Rail0Client(base_url=get_env("RAIL0_API_URL", "http://localhost:4567"))


@pytest.fixture(scope="session")
def buyer_account():
    return EthAccount.from_key(get_env("BUYER_PRIVATE_KEY"))


@pytest.fixture(scope="session")
def account_account():
    return EthAccount.from_key(get_env("ACCOUNT_PRIVATE_KEY"))


@pytest.fixture(scope="session")
def payment_method(client):
    account_id  = get_env("ACCOUNT_ID")
    chain_slug  = get_env("CHAIN_SLUG", "arc-testnet")
    token_sym   = get_env("TOKEN_SYMBOL", "USDC")

    methods = client.accounts.payment_methods(account_id)
    for m in methods:
        if m.get("chainSlug") == chain_slug and m.get("tokenSymbol") == token_sym:
            return m
    raise RuntimeError(f"No {token_sym} payment method on {chain_slug}")


@pytest.fixture
def amount() -> str:
    return get_env("AMOUNT", "1000000")


@pytest.fixture
def chain_id() -> int:
    return int(get_env("CHAIN_ID", "5042002"))


# ── Helpers ───────────────────────────────────────────────────────────────────

POLL_TIMEOUT  = 120
POLL_INTERVAL = 2


def create_and_sign(client, payment_method, buyer_account, chain_id: int, amount: str, mode: str = "authorize") -> str:
    """Create a payment and submit the payer's EIP-3009 signature. Returns payment_id."""
    create_resp = client.payments.create_payment({
        "payment": {
            "payer":  buyer_account.address.lower(),
            "payee":  payment_method["wallet_address"],
            "token":  payment_method["token_address"],
            "amount": amount,
        },
        "chain_id": chain_id,
        "mode": mode,
    })
    payment_id = create_resp["rail0_id"]

    # signing_payload is returned by the authorize/prepare endpoint, not the create response
    prepare_resp = client.payments.authorize_prepare(payment_id)
    sig = sign_payload(buyer_account.key.hex(), prepare_resp["signing_payload"])
    signature = "0x" + sig["r"][2:] + sig["s"][2:] + format(sig["v"], "02x")

    sign_resp = client.payments.sign(payment_id, {"signature": signature})
    assert sign_resp["status"] == "signature_stored"
    return payment_id


def sign_eip1559(unsigned_hex: str, account) -> str:
    """Sign an unsigned EIP-1559 transaction with eth_account."""
    from eth_account._utils.typed_transactions import TypedTransaction
    unsigned_bytes = bytes.fromhex(unsigned_hex.removeprefix("0x"))
    tx = TypedTransaction.from_bytes(unsigned_bytes)
    signed = account.sign_transaction(tx.as_dict())
    return signed.rawTransaction.hex()


def poll_until_status(client, payment_id: str, *expected: str, waiting_for: str = "") -> dict:
    deadline = time.time() + POLL_TIMEOUT
    while True:
        state = client.payments.get(payment_id)
        status = state["status"]
        print(f"  [poll] {waiting_for}: status={status}")
        if status in expected:
            return state
        if status == "failed":
            raise RuntimeError(f"payment failed: {state.get('failure_code')} — {state.get('failure_message')}")
        if time.time() >= deadline:
            raise TimeoutError(f"timed out waiting for {expected} (last: {status})")
        time.sleep(POLL_INTERVAL)
