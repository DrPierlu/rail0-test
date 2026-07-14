# CLAUDE.md — rail0-test

Working instructions for Claude Code in this repository. This file carries the
project-wide rules (shared across the whole rail0 project) followed by a
test-suite-specific section. The same project-wide rules are duplicated in every per-repo CLAUDE.md; keep them consistent.

## Project structure

rail0 is a multi-repo project. All repositories prefixed with `rail0-` are part of the same project, as is `rail0` itself (the smart contract). All repos are located under the same parent directory (`/Users/pierlu/Documents/GitHub/`).

| Repo | Role |
| --- | --- |
| `rail0` | EVM smart contract (Solidity) |
| `rail0-gateway` | Backend API (Ruby/Grape) |
| `rail0-indexer` | On-chain event indexer (TypeScript/Envio) |
| `rail0-admin` | Admin UI |
| `rail0-cli` | CLI tool |
| `rail0-ruby` | Ruby SDK |
| `rail0-go` | Go SDK |
| `rail0-ts` | TypeScript SDK |
| `rail0-test` | Integration and cross-SDK tests |

> Note: `rail0-api`, `rail0-py`, and `rail0-rust` are temporarily out of scope.

When a change in one repo affects the contract, the indexer, or any SDK, flag it explicitly and propose coordinated changes across the relevant repos.

## Rules

1. **Always propose before implementing.** For any non-trivial change, present a plan of action and wait for explicit confirmation before writing any code.

2. **Follow language and framework conventions.** Respect the idioms and conventions of the language and framework used in each test suite. Match the style of surrounding code.

3. **Do not make structural changes without consent.** The architecture of each repo is intentional. Do not reorganise layers, introduce new abstractions, or change project layout without explicit approval.

4. **Avoid duplication — favour reuse and centralisation.** Before adding code, check whether the functionality already exists. Prefer extending existing helpers or fixtures over creating parallel implementations.

5. **Always work on a branch.** Never commit directly to `main`. If no branch exists for the current task, create one before making any changes using the naming convention `feature/short-desc` for new functionality or `fix/short-desc` for bug fixes.

6. **Use Conventional Commits format.** Every commit message must follow the [Conventional Commits](https://www.conventionalcommits.org/) specification: `type(scope): description`, where type is one of `feat`, `fix`, `refactor`, `docs`, `test`, `chore`.

7. **Always open a draft PR.** After the first push to a branch, open a pull request in draft status if one does not already exist. The PR title must also follow Conventional Commits format.

8. **Never log sensitive data.** Do not log or commit private keys, signatures, raw transaction payloads, HMAC secrets, JWT tokens, or any user-identifying data. Keep test secrets in environment variables / untracked files, never hard-coded in fixtures.

9. **Comment non-obvious functions.** Add a detailed comment to any helper whose logic is not immediately clear from its name alone — explaining what it does and any non-obvious invariants. Simple assertions need no comment; orchestration, signing, and multi-step flows do.

10. **Keep documentation and tests in sync.** After every change, keep the README and the test fixtures/expected shapes consistent with the current gateway and SDK behaviour. Do not consider a task complete until all are consistent.

11. **Keep all SDKs aligned when asked.** When asked to update the SDKs, check every SDK repo (`rail0-ruby`, `rail0-go`, `rail0-cli`) for alignment with the current gateway API surface. For each SDK: update client methods, README, and unit tests. Flag any SDK where alignment requires a breaking change.

12. **Align all tests when asked.** When asked to align or update tests, cover both layers: unit tests in every affected repo (gateway, indexer, all SDKs), and integration tests here in `rail0-test` (API tests, flow tests for each SDK language, and cross-SDK tests). Verify that test fixtures, helper methods, and expected response shapes are consistent with the current gateway behaviour.

## Test-suite-specific conventions

`rail0-test` is the polyglot integration and cross-SDK test suite. It exercises the live gateway and the SDKs end-to-end. Beyond the rules above, the following conventions are specific to this repo.

- **Layout:** `api/` (gateway HTTP request tests, Ruby/Minitest) and the SDK flow suites `go/` (rail0-go), `ruby/` (rail0-ruby, Minitest), and `cli/` (drives the rail0-cli binary), orchestrated by `run.sh`. There is no `cross_sdk/` directory. Add new tests to the matching directory; follow each suite's existing harness.
- **In-scope SDKs:** `rail0-go`, `rail0-ruby`, and `rail0-cli`. The `rail0-py`, `rail0-ts`, and `rail0-rust` SDKs are out of scope — do not add flow suites for them unless those repos return to scope. (The `api/` suite is Ruby, but it exercises the gateway HTTP API directly with `net/http`, not an SDK.)
- **The gateway is the source of truth.** Expected response shapes and fixtures must track the gateway's current public API; when the gateway changes, update the `api/` tests and every in-scope SDK flow (`go/`, `ruby/`, `cli/`) together.
- **Determinism.** Tests must set up and tear down their own data; no reliance on leftover state. Keep secrets in env vars.
