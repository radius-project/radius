# Running Radius tests

## Purpose

This is the authoritative overview of the Radius test suite. It names every test tier, gives the exact command to run it, and states who each tier is for, so you can pick the right tests for a change and find the detailed how-to. Radius applies the [testing pyramid](https://martinfowler.com/articles/practical-test-pyramid.html): many fast unit tests, fewer integration tests, and a small number of functional (end-to-end) tests. Every PR is expected to add or update tests.

## Prerequisites

- The [basic prerequisites](../contributing-code-prerequisites/) (Go toolchain, `make`, Git). These cover the unit, integration, and CLI integration tiers. The Helm chart unit tests run as part of `make test`, so **Helm** must also be installed (listed under [additional tools](../contributing-code-prerequisites/#additional-tools)).
- A Kubernetes cluster and extra setup are required only for the functional and local-iteration tiers — those prerequisites are listed on their own pages, linked below. The Bicep validation tier additionally needs Bicep downloaded (`rad bicep download`).

## Steps

### Test matrix

Each row is one tier. Run the unit tier on every change; run the higher tiers when your change affects their area.

| Tier                    | Command                               | Audience and scope                                                                                                                                                                         | Learn more                                                                                                      |
|-------------------------|---------------------------------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|-----------------------------------------------------------------------------------------------------------------|
| Unit                    | `make test`                           | Every contributor, every PR. Runs the Go unit tests under `./pkg/...` plus the Helm chart unit tests, so it needs the basic prerequisites and Helm.                                        | This page; [first-commit walkthrough](../contributing-code-first-commit/first-commit-05-running-tests/index.md) |
| Integration             | `make test`                           | Contributors changing stateful subsystems. Integration tests live alongside the unit tests under `./pkg/...` and run as part of `make test` (see [Integration tests](#integration-tests)). | [Integration tests](#integration-tests)                                                                         |
| CLI integration         | `make test-validate-cli`              | Contributors changing `rad` CLI commands. Runs `./pkg/cli/cmd/...` and `./cmd/rad/...`.                                                                                                    | [`pkg/cli/cmd/README.md`](../../../../pkg/cli/cmd/README.md)                                                    |
| Helm chart unit         | `make test-helm`                      | Contributors changing the Helm chart under `deploy/Chart`.                                                                                                                                 | —                                                                                                               |
| Bicep validation        | `make test-validate-bicep`            | Contributors changing `.bicep` files; validates they compile cleanly.                                                                                                                      | —                                                                                                               |
| Functional (end-to-end) | `make test-functional-local-noncloud` | Contributors validating realistic user scenarios in isolated local clusters built from their checkout, including groups that need separate clusters.                                       | [Running functional tests](./running-functional-tests.md)                                                       |
| Local iteration         | n/a (build, push, redeploy loop)      | Contributors iterating quickly on control-plane images against a running cluster.                                                                                                          | [Accelerating local verification](./testing-local.md)                                                           |

### Unit tests

Run the unit tests with:

```sh
make test
```

`make test` runs the Go unit tests under `./pkg/...` and the Helm chart unit tests. We require unit tests for new code and for fixes or refactors of existing code; as a rule, every PR should add or change some tests. Unit tests must run with only the [basic prerequisites](../contributing-code-prerequisites/) installed — do not add external dependencies for a unit test; write an integration test instead.

To compile every test without running them, use `make test-compile`.

### Integration tests

Integration tests exercise a feature together with its dependencies. In Radius they live alongside the unit tests under `./pkg/...` rather than in a separate tree, and they run as part of `make test`. They use in-memory servers and data stores instead of a deployed control plane, which keeps them fast and self-contained. Examples:

- [`pkg/ucp/integrationtests`](../../../../pkg/ucp/integrationtests) runs UCP against an in-memory server and in-memory data stores.
- [`pkg/dynamicrp/integrationtest`](../../../../pkg/dynamicrp/integrationtest) exercises the dynamic resource provider end to end in-process.
- CLI integration tests under `./pkg/cli/cmd/...` and `./cmd/rad/...` run via `make test-validate-cli`.

There is no separate `make test-integration` target today: because these tests need no external dependencies, they are part of the standard `make test` run. When a test needs an optional or external dependency, prefer the functional tier so that `make test` stays runnable with only the basic prerequisites.

### CLI integration tests

Run the CLI command tests with:

```sh
make test-validate-cli
```

This runs `./pkg/cli/cmd/...` and `./cmd/rad/...`. See [`pkg/cli/cmd/README.md`](../../../../pkg/cli/cmd/README.md) for the command and test layout.

### Functional tests

Functional tests (also called end-to-end tests) use Radius to deploy an application to a real hosting environment (Kubernetes) and then assert on its state. They have their own dedicated set of [instructions](./running-functional-tests.md), including prerequisites, the `make test-functional-*` targets, and cleanup modes. The recommended local entry point builds from your checkout, provisions isolated infrastructure, and runs all non-cloud groups:

```sh
make test-functional-local-noncloud
```

This runner includes Dynamic RP, multicluster, database, state-store lifecycle, and upgrade tests in separate clusters. To run fewer tests or clean up afterward, follow the [local functional-test guide](./running-functional-tests.md#steps).

For an already configured environment, use `make test-functional-all-noncloud` or a per-group target instead. That existing-environment aggregate excludes groups with additional setup or isolation requirements; see [special test groups](./running-functional-tests.md#run-a-special-test-group).

### Local-iteration loop

When you are iterating on control-plane images (for example the applications resource provider) against a running cluster, the build/push/redeploy loop is faster than the full functional suite. See [Accelerating local verification](./testing-local.md).

### Helm and Bicep validation

- `make test-helm` runs the Helm chart unit tests in `deploy/Chart` using the `helm-unittest` plugin.
- `make test-validate-bicep` validates that every `.bicep` file compiles cleanly.

### Test conventions

- We write unit tests in a straightforward style and use [testify](https://github.com/stretchr/testify) for assertions.
- We favor [subtests](https://go.dev/blog/subtests) and [table-driven tests](https://dave.cheney.net/2019/05/07/prefer-table-driven-tests) and apply them where appropriate.
- For functional tests specifically, see [writing functional tests](./writing-functional-tests.md), the [naming conventions](./tests-naming-conventions.md), [test logging](./tests-logging.md), and [using standard images in tests](./tests-images-pushtoghcr.md).
- For the scheduled OCI state archive end-to-end test, see [Repo Radius GHCR state end-to-end test](./repo-radius-state-e2e.md).
- For investigating a test reported as flaky, see [Investigating flaky tests](./investigating-flaky-tests.md).

#### RBAC policy storage

Run fast policy checks with `go test -race ./pkg/ucp/authorization ./pkg/components/database/inmemory -short -count=1`. For the full 10,000-assignment/1,000-role capacity checks, run:

```bash
go test -race ./pkg/ucp/authorization -run '^(TestPolicyStoreManifestCapacity|TestPolicyCacheRateLimitedCapacity)$' -count=1 -v
```

Assignment metadata regressions cover description and principal display-label updates, clearing optional metadata, history and generation changes, identical-PUT idempotency, and preservation of identity and ownership. Only metadata is mutable; changing the assignment's role or principal identity still requires a different assignment.

History regressions cover updating and deleting maximum-sized initialized assignments, metadata at its byte limit across generation digit changes, interrupted chunk preparation/publication, delayed recovery after deletion, and missing or corrupted audit chunks. Events up to 128 KiB retain the inline format; larger events use immutable 64 KiB binary chunks, encoded below the 128 KiB per-object limit, with ordered references, an exact byte count and a SHA-256 digest. Reassembly returns the same public access-change resource. Variable actor/request/target/provenance metadata has a separate 128 KiB bound; generated envelope fields have reserved space, so switching from `after` to `before` or adding generation digits cannot exhaust it. Each snapshot can hold the maximum packed policy plus a 128 KiB wrapper reserve, and the derived total bound reserves both snapshots and the envelope. Excessive request metadata fails explicitly before claiming a generation; audit contents are never truncated. Run the full snapshot-boundary and maximum Bootstrap batch checks with:

```bash
go test -race ./pkg/ucp/authorization -run '^(TestPolicyStoreMaximumHistorySnapshots|TestPolicyStoreBootstrapHistoryCapacity)$' -count=1
```

The manifest reports 86 index pages and an anchor smaller than 1 KiB. Cold-cache and changed-generation refreshes each use 174 GETs. The fake-clock rate limiter checks the deterministic 870 ms throttling cost at 200 QPS/200 burst with the burst exhausted; it does not gate race-instrumented validation on wall-clock time. Separate fake-clock tests enforce the unchanged thirty-second freshness boundary. Immutable payload chunks split at 128 entries or 128 KiB; manifests validate their IDs, kinds, entry counts and generation. A root holds at most 128 chunks, so large entries can exhaust this bound before reaching the role/assignment count limits; oversized data fails explicitly with `PolicyCapacityExceeded`.

Policy-load regressions retain unregistered permissions for inspection and remediation but exclude them from grants; role writes still reject unknown permissions. Bootstrap tests start with no assignments, require the trusted Kubernetes settings guard outside `Enforce`, and publish guard-provided Bootstrap administrators atomically with settings. The same generation's audit includes each newly added full assignment resource in `after.roleAssignments`, including its ID, principal and Bootstrap provenance; it is a delta, not the policy inventory. Repeated guard identities do not create duplicate deltas, and same-mode additions and recovery preserve them. Standalone assignment changes retain singular `roleAssignment` snapshots. In `Enforce`, the Radius settings permission remains mandatory. Bootstrap-only policies allow additive changes and authorized same-tuple promotion to permanent ownership; once a permanent administrator exists, its last grant cannot be removed.

**Required before production policy handlers:** each mutation currently rewrites the complete immutable policy and retains old or abandoned preparations. Implement unchanged-chunk reuse and a safe reclamation/retention protocol before exposing production handlers. Validate write counts and bytes at capacity, bound retained storage across repeated writes, and prove that concurrent readers, losing writers, and delayed recovery cannot lose reachable objects or resurrect deleted grants. Garbage collection is deliberately not implemented in this storage layer. Disposable-Postgres CI coverage for concurrent initialization, conflicting changes, and interrupted-write recovery is also required before production enforcement depends on this store.

For policy runtime lifecycle and UCP wiring, run `go test -race ./pkg/ucp/authorization ./pkg/ucp ./pkg/ucp/server -short -count=1`. The `TestPolicyRuntime` tests use `testing/synctest` to verify startup recovery, missing-anchor failures, repeated failures/recovery, generation polling, the exact thirty-second freshness boundary, immediate local-write refresh, and cancellation without wall-clock sleeps. See [UCP policy lifecycle](../../../architecture/ucp.md#user-authorization-policy-lifecycle) for the opt-in configuration, preinitialized-anchor prerequisite, and authorization-specific readiness contract.

For real backend recovery checks, omit `-short` and select `TestPolicyStoreAPIServer` (envtest setup below) or `TestPolicyStorePostgres` (requires a disposable database via `TEST_POSTGRES_URL`, as shown by `make db-init`). Missing Postgres configuration skips those checks rather than proving concurrency. These tests cover storage and runtime lifecycle/readiness interfaces, not management APIs or runtime request enforcement.

## Verification

- A passing run prints `ok` for every package that has tests and `[no test files]` for those that don't; `go test` exits non-zero if any test fails, so a failure is obvious in the output.
- We measure code coverage as part of the PR process because it shows whether the right tests are being added.

## Troubleshooting

- **`make test` fails setting up `KUBEBUILDER_ASSETS` / envtest.** The target uses `setup-envtest` (managed as a Go tool via the `tool` directive in `go.mod`) to download Kubernetes test binaries. Ensure you have network access on the first run; the assets are cached under `./bin` afterward.
- **`make test-helm` cannot find the unittest plugin.** The target installs the `helm-unittest` plugin automatically; if installation is blocked, install it manually with `helm plugin install oci://ghcr.io/helm-unittest/helm-unittest/unittest:1.2.1 --verify=false`.
- **A unit test needs an external dependency.** It belongs in the functional tier, not in `make test`. Move it so the unit suite keeps running with only the basic prerequisites.
- **A functional test fails or leaves resources behind.** Follow the [functional test instructions](./running-functional-tests.md), which cover prerequisites, namespaces, and cleanup modes.
