# rail0-test

Integration tests for the RAIL0 payment gateway — direct HTTP endpoint tests and end-to-end flows via the supported clients (`rail0-go`; `rail0-cli`).

## Structure

```
rail0-test/
├── run.sh                 # orchestrator — runs all suites
├── .env.example           # required environment variables
│
├── api/                   # Minitest — direct HTTP endpoint tests (no SDK)
│   ├── Gemfile
│   ├── test_helper.rb
│   └── tests/
│       ├── auth_test.rb            # POST /auth/nonces, POST /auth, GET /payments auth
│       ├── accounts_test.rb        # payment-methods, wallets, wallet_tokens
│       ├── payments_test.rb        # GET /health, POST /payments, PUT /sign
│       └── indexer_test.rb         # POST /sync/transactions auth + validation
│
├── go/                    # Go testing — rail0-go SDK
│   ├── go.mod
│   └── flows/
│       ├── helpers_test.go
│       ├── authorize_capture_test.go
│       └── charge_test.go
│
└── cli/                   # Go testing — drives the rail0-cli binary end-to-end
    ├── go.mod
    └── flows/
        ├── helpers_test.go        # builds the rail0 binary; runCLI/pollStatus helpers
        ├── authorize_capture_test.go
        └── charge_test.go
```

> Only the `rail0-go` and `rail0-cli` clients are in scope; the `ruby`, `python`,
> `rust`, `typescript` and `cross_sdk` suites were removed.

## Prerequisites

- Ruby ≥ 3.2 + Bundler (for the `api` suite)
- Go ≥ 1.22 (for the `go` suite)
- Test wallets with USDC on the target chain (Arc Testnet by default)
- The gateway running at `RAIL0_API_URL` with the test account registered

The client repos are expected as siblings of `rail0-test`:

```
Documents/GitHub/
├── rail0-gateway
├── rail0-go
├── rail0-cli      ← built by the cli suite
└── rail0-test      ← this repo
```

## Setup

```bash
cp .env.example .env
# edit .env with your test keys, API URL, account ID
```

## Running

```bash
# All suites
./run.sh

# Single suite
./run.sh api          # direct HTTP endpoint tests
./run.sh go           # rail0-go SDK flows
./run.sh cli          # drives the rail0 CLI binary
```

## API suite

`api/` tests the HTTP API directly without an SDK, using Ruby's standard `net/http`. They require a running rail0-gateway instance and the seeded test account. They cover:

| File | Endpoints tested |
|---|---|
| `auth_test.rb` | `POST /auth/nonces`, `POST /auth`, `GET /payments` (auth enforcement) |
| `accounts_test.rb` | `GET /accounts/:id/payment-methods`, `GET /accounts/:id/wallets`, `GET /accounts/:id/wallets/:id` |
| `payments_test.rb` | `GET /health`, `GET /payments/:id`, `POST /payments`, `PUT /payments/:id/sign` |
| `indexer_test.rb` | `POST /sync/transactions` (HMAC auth and input validation) |

Required env vars for the api suite (in addition to the common ones):

| Variable | Description |
|---|---|
| `RAIL0_ACCOUNT_ID` | UUID of the seeded test account (fixed: `019e748b-da9a-7c3f-ba32-50572ffd5388`) |
| `RAIL0_INDEXER_HMAC_SECRET` | HMAC secret used to sign `/sync/transactions` requests |

## Flows covered

| Flow | Go (`rail0-go`) | CLI (`rail0-cli`) |
|---|---|---|
| authorize → capture → refund | ✓ | ✓ |
| charge | ✓ | ✓ |
| authorize → void | — | ✓ |
| partial capture ×2 → partial refund ×2 → release | — | ✓ |
| charge → dispute → close dispute | — | ✓ |

> **Authorization TTL.** The gateway reads `AUTHORIZATION_TTL` (seconds). The
> capture/refund flows must complete within that window, so it must be long
> enough to cover on-chain + indexer confirmation latency (e.g.
> `AUTHORIZATION_TTL=300`). The release flow only calls `release()` *after*
> `authorizationExpiry` — run it on its own with a short TTL
> (`AUTHORIZATION_TTL=30`) so it doesn't wait minutes for expiry.

> **Disputes** are payer-driven and signal-only: the CLI/SDK only prepares the
> transaction; the test signs it with the payer key and broadcasts it directly
> to the chain. The payer (buyer) wallet must hold native gas on the target chain.
