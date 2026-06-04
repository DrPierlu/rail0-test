"""Flow: create → sign → authorize → capture → refund (EIP-3009)"""

import pytest
from .conftest import create_and_sign, poll_until_status, sign_eip1559
from rail0.signing import sign_payload


def test_authorize_capture_refund(client, buyer_account, account_account, payment_method, amount, chain_id):
    # ── Create + sign ──────────────────────────────────────────────────────────
    payment_id = create_and_sign(client, payment_method, buyer_account, chain_id, amount, mode="authorize")
    print(f"\n  payment_id={payment_id}")

    # ── Authorize ──────────────────────────────────────────────────────────────
    print("→ authorize/payload")
    prep = client.payments.authorize_prepare(payment_id)
    assert prep["unsigned_transaction"]

    signed = sign_eip1559(prep["unsigned_transaction"], account_account)
    client.payments.authorize(payment_id, {"signed_transaction": signed})
    auth = poll_until_status(client, payment_id, "authorized", waiting_for="authorize")
    print(f"  authorized — capturable={auth['on_chain']['capturable_amount']}")

    # ── Capture ────────────────────────────────────────────────────────────────
    print("→ capture/payload")
    prep = client.payments.capture_prepare(payment_id, {"amount": amount})
    assert prep["unsigned_transaction"]

    signed = sign_eip1559(prep["unsigned_transaction"], account_account)
    client.payments.capture(payment_id, {"signed_transaction": signed})
    cap = poll_until_status(client, payment_id, "captured", "partially_captured", waiting_for="capture")
    assert cap["on_chain"]["capturable_amount"] == "0", "capturable must be 0 after full capture"
    print(f"  captured — status={cap['status']}")

    # ── Refund (EIP-3009 two-phase) ────────────────────────────────────────────
    print("→ refund/prepare phase 1")
    phase1 = client.payments.refund_prepare(payment_id, amount)
    assert "signing_payload" in phase1, "phase 1 must return signing_payload"
    assert "unsigned_transaction" not in phase1, "phase 1 must NOT return unsigned_transaction"

    print("→ signing EIP-3009 refund payload with Python SDK")
    vrs = sign_payload(account_account.key.hex(), phase1["signing_payload"])
    signature = "0x" + vrs["r"][2:] + vrs["s"][2:] + format(vrs["v"], "02x")

    print("→ refund/prepare phase 2")
    phase2 = client.payments.refund_prepare(payment_id, amount, signature=signature)
    assert "unsigned_transaction" in phase2, "phase 2 must return unsigned_transaction"

    print("→ submitting refund")
    signed = sign_eip1559(phase2["unsigned_transaction"], account_account)
    client.payments.refund(payment_id, {"signed_transaction": signed})
    final = poll_until_status(client, payment_id, "refunded", "partially_refunded", waiting_for="refund")
    assert final["on_chain"]["refundable_amount"] == "0", "refundable must be 0 after full refund"
    print(f"  refunded — status={final['status']}")


def test_authorize_void(client, buyer_account, account_account, payment_method, amount, chain_id):
    payment_id = create_and_sign(client, payment_method, buyer_account, chain_id, amount, mode="authorize")

    prep = client.payments.authorize_prepare(payment_id)
    client.payments.authorize(payment_id, {"signed_transaction": sign_eip1559(prep["unsigned_transaction"], account_account)})
    poll_until_status(client, payment_id, "authorized", waiting_for="authorize")

    prep = client.payments.void_prepare(payment_id)
    client.payments.void(payment_id, {"signed_transaction": sign_eip1559(prep["unsigned_transaction"], account_account)})
    final = poll_until_status(client, payment_id, "voided", waiting_for="void")
    assert final["on_chain"]["capturable_amount"] == "0"


def test_charge(client, buyer_account, account_account, payment_method, amount, chain_id):
    payment_id = create_and_sign(client, payment_method, buyer_account, chain_id, amount, mode="charge")

    prep = client.payments.charge_prepare(payment_id)
    client.payments.charge(payment_id, {"signed_transaction": sign_eip1559(prep["unsigned_transaction"], account_account)})
    poll_until_status(client, payment_id, "charged", waiting_for="charge")
