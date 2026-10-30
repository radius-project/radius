# Running Radius tests

## Purpose

Run the tests relevant to your change and add coverage for new behavior or bug fixes.

## Prerequisites

- Install the [development prerequisites](../contributing-code-prerequisites/), including Helm and Node.js for `make test`.
- For functional tests, follow the [cluster and tool setup](./running-functional-tests.md). Bicep validation requires `rad bicep download`.

## Steps

### Run tests

Run commands from the repository root. During development, run the affected Go package or test:

```bash
go test ./pkg/ucp/authorization/...
go test ./pkg/ucp/authorization -run TestRoleBoundaries
```

Replace the package and test name with the ones you changed.

| Change                                          | Command                               |
|-------------------------------------------------|---------------------------------------|
| Standard test suite: Go, Helm, and script tests | `make test`                           |
| CLI commands                                    | `make test-validate-cli`              |
| Helm chart                                      | `make test-helm`                      |
| Bicep files                                     | `make test-validate-bicep`            |
| End-to-end behavior in isolated local clusters  | `make test-functional-local-noncloud` |

For functional-test selection and cleanup, use the [functional test guide](./running-functional-tests.md). For a faster build/push/redeploy loop against a running cluster, see [local verification](./testing-local.md).

### Add or update tests

- Put Go tests in `*_test.go` files beside the code. Cover success, failure, and relevant edge cases; add a regression test for each bug fix.
- Follow nearby tests: use [testify](https://github.com/stretchr/testify) assertions and table-driven subtests where useful.
- Keep unit tests self-contained. Tests requiring a deployed Radius instance or external services belong in the functional suite.
- For functional tests, follow [writing functional tests](./writing-functional-tests.md), [naming](./tests-naming-conventions.md), [logging](./tests-logging.md), and [test images](./tests-images-pushtoghcr.md).
- When adding a new script test suite, wire its target into [`build/test.mk`](../../../../build/test.mk) so CI runs it.

### Integration tests

In-process integration tests under `pkg/`, such as [`pkg/ucp/integrationtests`](../../../../pkg/ucp/integrationtests), run with `make test`; no separate integration target is needed.

#### RBAC policy storage

Run fast policy checks with `go test -race ./pkg/ucp/authorization ./pkg/components/database/inmemory -short -count=1`. For the full 10,000-assignment/1,000-role capacity checks, run:

```bash
go test -race ./pkg/ucp/authorization -run '^(TestPolicyStoreManifestCapacity|TestPolicyCacheRateLimitedCapacity)$' -count=1 -v
```

Assignment metadata regressions cover description and principal display-label updates, clearing optional metadata, history and generation changes, identical-PUT idempotency, and preservation of identity and ownership. Only metadata is mutable; changing the assignment's role or principal identity still requires a different assignment.

The manifest reports 86 index pages and an anchor smaller than 1 KiB. Cold-cache and changed-generation refreshes each use 174 GETs and must become ready within thirty seconds at 200 QPS/200 burst, including when the burst is exhausted. Immutable payload chunks split at 128 entries or 128 KiB; manifests validate their IDs, kinds, entry counts and generation. A root holds at most 128 chunks, so large entries can exhaust this bound before reaching the role/assignment count limits; oversized data fails explicitly with `PolicyCapacityExceeded`.

For real backend recovery checks, omit `-short` and select `TestPolicyStoreAPIServer` (envtest setup below) or `TestPolicyStorePostgres` (requires a disposable database via `TEST_POSTGRES_URL`, as shown by `make db-init`). Missing Postgres configuration skips those checks rather than proving concurrency. These tests cover storage/readiness interfaces, not management APIs or runtime enforcement.

## Verification

Run the affected tests and check that the command exits successfully before submitting your PR.

## Troubleshooting

- **Missing envtest binaries:** run `make test-get-envtools` and export the printed `KUBEBUILDER_ASSETS` value before running the Go test directly. The first download needs network access.
- **Missing Helm plugin:** `make test-helm` installs `helm-unittest` automatically; check network access if installation fails.
- **Functional failures or leftover resources:** use the [functional test guide](./running-functional-tests.md). For intermittent failures, see [investigating flaky tests](./investigating-flaky-tests.md).
