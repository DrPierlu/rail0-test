# rail0-test

Integration tests for the RAIL0 payment gateway — direct HTTP endpoint tests and end-to-end flows via the supported clients (`rail0-go`; `rail0-ruby`; `rail0-cli`).

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
│       ├── accounts_test.rb        # GET /accounts/:id/wallets(/:id) — SIWE-gated, own account only
│       ├── payment_methods_test.rb # GET /payment_methods — public discovery (by account_id | address)
│       ├── payments_test.rb        # GET /health, POST /payments, PUT /sign
│       └── indexer_test.rb         # PUT /sync/chains/:chain_id/transactions/:tx_hash auth + validation
│
├── go/                    # Go testing — rail0-go SDK
│   ├── go.mod
│   └── flows/
│       ├── helpers_test.go
│       ├── authorize_capture_test.go
│       └── charge_test.go
│
├── ruby/                  # Minitest — rail0-ruby SDK
│   ├── Gemfile            # rail0 as a sibling path gem + eth/siwe-rb
│   └── flows/
│       ├── test_helper.rb         # SDK client, SIWE login, discover/create/sign/poll helpers
│       ├── authorize_capture_test.rb
│       └── charge_test.rb
│
└── cli/                   # Go testing — drives the rail0-cli binary end-to-end
    ├── go.mod
    └── flows/
        ├── helpers_test.go        # builds the rail0 binary; runCLI/pollStatus helpers
        ├── authorize_capture_test.go
        └── charge_test.go
```

> In-scope clients: `rail0-go`, `rail0-ruby`, and `rail0-cli`. The `python`,
> `rust`, `typescript` and `cross_sdk` suites were removed.

## Prerequisites

- Ruby ≥ 3.2 + Bundler (for the `api` and `ruby` suites)
- Go ≥ 1.22 (for the `go` suite)
- Test wallets with USDC on the target chain (Arc Testnet by default)
- The gateway running at `RAIL0_API_URL` with the test account registered

The client repos are expected as siblings of `rail0-test`:

```
Documents/GitHub/
├── rail0-gateway
├── rail0-go
├── rail0-ruby     ← used by the ruby suite (sibling path gem)
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
./run.sh ruby         # rail0-ruby SDK flows
./run.sh cli          # drives the rail0 CLI binary
```

### Picking a CLI flow with `bin/test`

`bin/test` runs the rail0-cli integration flows with a selection menu (loads
`.env` automatically; the gateway + indexer must already be running):

```bash
./bin/test                 # interactive: pick environment, then a flow
./bin/test 1               # run flow #1 (env defaults to development)
./bin/test TestCharge      # run flows matching a Go test name
./bin/test all             # run every CLI flow
./bin/test -e staging 3    # run flow #3 against staging
```

**Environment selection.** Pass `-e|--env <development|test|staging|production>`
(or pick it from the interactive menu when `-e` is omitted) to choose which
gateway + indexer the flows target. The environment also picks how the indexer
is health-checked — Hasura GraphQL locally, the Hono API's `/health` on
staging/production (where Hasura is disabled). Endpoints have built-in defaults,
each overridable from `.env` via `RAIL0_API_URL_<ENV>` / `RAIL0_INDEXER_URL_<ENV>`
(e.g. `RAIL0_API_URL_STAGING`). Wallet keys, `ACCOUNT_ID` and `CHAIN_ID` still
come from `.env` — set them to match the target environment.

Selecting `production` targets the live **mainnet** gateway with real funds and
keys, so it requires confirmation: type `yes` at the prompt, or pass `-y/--yes`
(or set `RAIL0_CONFIRM_PRODUCTION=1`) for non-interactive runs.

## API suite

`api/` tests the HTTP API directly without an SDK, using Ruby's standard `net/http`. They require a running rail0-gateway instance and the seeded test account. They cover:

| File | Endpoints tested |
|---|---|
| `auth_test.rb` | `POST /auth/nonces`, `POST /auth`, `GET /payments` (auth enforcement) |
| `accounts_test.rb` | `GET /accounts/:id/wallets`, `GET /accounts/:id/wallets/:id` (SIWE-gated — 401 without a JWT, 403 for another account) |
| `payment_methods_test.rb` | `GET /payment_methods?account_id=…\|address=…` (public buyer-facing discovery) |
| `payments_test.rb` | `GET /health`, `GET /payments/:id`, `POST /payments`, `PUT /payments/:id/sign` |
| `indexer_test.rb` | `PUT /sync/chains/:chain_id/transactions/:tx_hash` (HMAC auth and input validation) |

Required env vars for the api suite (in addition to the common ones):

| Variable | Description |
|---|---|
| `RAIL0_ACCOUNT_ID` | UUID of the seeded test account (fixed: `019e748b-da9a-7c3f-ba32-50572ffd5388`) |
| `RAIL0_SYNC_HMAC_SECRET` | HMAC secret used to sign `PUT /sync/chains/:chain_id/transactions/:tx_hash` requests |

## Flows covered

| Flow | Go (`rail0-go`) | Ruby (`rail0-ruby`) | CLI (`rail0-cli`) |
|---|---|---|---|
| authorize → capture (settle) | — | — | ✓ |
| authorize → capture → refund | ✓ | ✓ | ✓ |
| charge | ✓ | ✓ | ✓ |
| authorize → void | — | — | ✓ |
| partial capture ×2 → partial refund ×2 → release | — | — | ✓ |
| charge → dispute → close dispute | — | — | ✓ |

> **Authorization TTL.** The gateway reads `AUTHORIZATION_TTL` (seconds). The
> capture/refund flows must complete within that window, so it must be long
> enough to cover on-chain + indexer confirmation latency (e.g.
> `AUTHORIZATION_TTL=300`). The release flow only calls `release()` *after*
> `authorizationExpiry` — run it on its own with a short TTL
> (`AUTHORIZATION_TTL=30`) so it doesn't wait minutes for expiry.

> **Disputes** are payer-driven and signal-only: the CLI/SDK only prepares the
> transaction; the test signs it with the payer key and broadcasts it directly
> to the chain. The payer (buyer) wallet must hold native gas on the target chain.
