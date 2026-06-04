"""Shared fixtures for all Python integration tests."""

import os
import time

import pytest
from eth_account import Account as EthAccount

from rail0 import Rail0Client


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
        if m.get("chain_slug") == chain_slug and m.get("token_symbol") == token_sym:
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


def sign_payload(private_key: str, signing_payload: dict) -> dict:
    """Sign an EIP-712 signing_payload and return {v, r, s}."""
    import coincurve
    from Crypto.Hash import keccak as _keccak_lib

    def keccak256(data: bytes) -> bytes:
        k = _keccak_lib.new(digest_bits=256)
        k.update(data)
        return k.digest()

    def hex_to_bytes(h: str) -> bytes:
        return bytes.fromhex(h[2:] if h.startswith("0x") else h)

    def abi_address(addr: str) -> bytes:
        return b"\x00" * 12 + hex_to_bytes(addr)

    def abi_uint256(v: int) -> bytes:
        return v.to_bytes(32, "big")

    d = signing_payload["domain"]
    m = signing_payload["message"]
    primary_type = signing_payload.get("primaryType", "TransferWithAuthorization")

    domain_type   = "EIP712Domain(string name,string version,uint256 chainId,address verifyingContract)"
    transfer_type = "TransferWithAuthorization(address from,address to,uint256 value,uint256 validAfter,uint256 validBefore,bytes32 nonce)"
    receive_type  = "ReceiveWithAuthorization(address from,address to,uint256 value,uint256 validAfter,uint256 validBefore,bytes32 nonce)"

    domain_typehash   = keccak256(domain_type.encode())
    transfer_typehash = keccak256(transfer_type.encode())
    receive_typehash  = keccak256(receive_type.encode())

    domain_hash = keccak256(
        domain_typehash
        + keccak256(d["name"].encode())
        + keccak256(d["version"].encode())
        + abi_uint256(d["chainId"])
        + abi_address(d["verifyingContract"])
    )

    typehash = receive_typehash if primary_type == "ReceiveWithAuthorization" else transfer_typehash
    struct_hash = keccak256(
        typehash
        + abi_address(m["from"])
        + abi_address(m["to"])
        + abi_uint256(int(m["value"]))
        + abi_uint256(int(m["validAfter"]))
        + abi_uint256(int(m["validBefore"]))
        + hex_to_bytes(m["nonce"])
    )

    digest = keccak256(b"\x19\x01" + domain_hash + struct_hash)

    key_hex = private_key[2:] if private_key.startswith("0x") else private_key
    key = coincurve.PrivateKey(bytes.fromhex(key_hex))
    sig = key.sign_recoverable(digest, hasher=None)

    recovery_id = sig[64]
    r = "0x" + sig[:32].hex()
    s = "0x" + sig[32:64].hex()
    v = recovery_id + 27
    return {"v": v, "r": r, "s": s}


def create_and_sign(client, payment_method, buyer_account, chain_id: int, amount: str, mode: str = "authorize") -> str:
    """Create a payment and submit the payer's EIP-3009 signature. Returns payment_id."""
    create_resp = client.payments.create({
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

    sig = sign_payload(buyer_account.key.hex(), create_resp["signing_payload"])
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
