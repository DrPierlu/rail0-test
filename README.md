# rail0-test

End-to-end integration tests for the RAIL0 payment API, covering every supported SDK.

## Structure

```
rail0-test/
├── run.sh                 # orchestrator — runs all suites
├── .env.example           # required environment variables
│
├── ruby/                  # Minitest — rail0-ruby SDK
│   ├── Gemfile
│   ├── support/helpers.rb
│   └── flows/
│       ├── authorize_capture_test.rb
│       ├── authorize_void_test.rb
│       ├── charge_test.rb
│       └── partial_capture_refund_release_test.rb
│
├── go/                    # Go testing — rail0-go SDK
│   ├── go.mod
│   └── flows/
│       ├── helpers_test.go
│       ├── authorize_capture_test.go
│       └── charge_test.go
│
├── python/                # pytest — rail0-py SDK
│   ├── requirements.txt
│   └── flows/
│       ├── conftest.py
│       └── test_authorize_capture.py
│
├── rust/                  # cargo test — rail0-rust SDK
│   ├── Cargo.toml
│   └── tests/
│       ├── helpers.rs
│       ├── authorize_capture.rs
│       └── charge.rs
│
├── typescript/            # Jest + ts-jest — @rail0/sdk + ethers v6
│   ├── package.json
│   ├── tsconfig.json
│   └── src/
│       ├── helpers.ts
│       └── flows/
│           ├── authorizeCapture.test.ts
│           └── charge.test.ts
│
└── cross_sdk/             # signature compatibility across SDKs
    ├── go.mod
    └── ruby_sign_go_submit_test.go   # Ruby signs → Go submits
```

## Prerequisites

- Ruby ≥ 3.2 + Bundler
- Go ≥ 1.22
- Python ≥ 3.11
- Rust ≥ 1.75 (stable)
- Node.js ≥ 20
- Test wallets with USDC on the target chain (Arc Testnet by default)
- The API running at `RAIL0_API_URL` with the test account registered

All SDK repos are expected as siblings of `rail0-test`:

```
Documents/GitHub/
├── rail0-api
├── rail0-ruby
├── rail0-go
├── rail0-py
├── rail0-rust
├── rail0-ts
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
./run.sh ruby
./run.sh go
./run.sh python
./run.sh rust
./run.sh typescript
./run.sh cross
```

## Flows covered

| Flow | Ruby | Go | Python | Rust | TypeScript |
|---|---|---|---|---|---|
| authorize → capture → refund | ✓ | ✓ | ✓ | ✓ | ✓ |
| authorize → void | ✓ | — | ✓ | — | ✓ |
| charge | ✓ | ✓ | ✓ | ✓ | ✓ |
| partial capture × 2 → partial refund × 2 → release | ✓ | — | — | — | — |

## Cross-SDK tests

The `cross_sdk/` suite verifies that signatures produced by different SDKs are
interchangeable — the EIP-3009 digest must be identical regardless of which SDK
computed it.

| Test | Signer | Submitter |
|---|---|---|
| `ruby_sign_go_submit_test.go` | Ruby SDK | Go SDK |

## Release flow note

The `partial_capture_refund_release_test.rb` test requires `authorization_expiry`
to have passed before calling `release()`. Start the API with a short TTL:

```bash
POLICY_AUTHORIZATION_TTL=30 bundle exec puma -C config/puma.rb
```

Then run the test with the same variable set so the poller knows how long to wait:

```bash
POLICY_AUTHORIZATION_TTL=30 ./run.sh ruby
```
