# Running Radius tests

## Purpose

This is the authoritative overview of the Radius test suite. It names every test tier, gives the exact command to run it, and states who each tier is for, so you can pick the right tests for a change and find the detailed how-to. Radius applies the [testing pyramid](https://martinfowler.com/articles/practical-test-pyramid.html): many fast unit tests, fewer integration tests, and a small number of functional (end-to-end) tests. Every PR is expected to add or update tests.

## Prerequisites

- The [basic prerequisites](../contributing-code-prerequisites/) (Go toolchain, `make`, Git). These cover the unit, integration, and CLI integration tiers. The Helm chart unit tests run as part of `make test`, so **Helm** must also be installed (listed under [additional tools](../contributing-code-prerequisites/#additional-tools)).
- A Kubernetes cluster and extra setup are required only for the functional and local-iteration tiers — those prerequisites are listed on their own pages, linked below. The Bicep validation tier additionally needs Bicep downloaded (`rad bicep download`).

## Steps

### Test matrix

Each row is one tier. Run the unit tier on every change; run the higher tiers when your change affects their area.

| Tier                    | Command                                                     | Audience and scope                                                                                                                                                                              | Learn more                                                                                                      |
|-------------------------|-------------------------------------------------------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|-----------------------------------------------------------------------------------------------------------------|
| Unit                    | `make test`                                                 | Every contributor, every PR. Runs the Go unit tests under `./pkg/...` (excluding the Kubernetes controller tests) plus the Helm chart unit tests, so it needs the basic prerequisites and Helm. | This page; [first-commit walkthrough](../contributing-code-first-commit/first-commit-05-running-tests/index.md) |
| Integration             | `make test`                                                 | Contributors changing stateful subsystems. Integration tests live alongside the unit tests under `./pkg/...` and run as part of `make test` (see [Integration tests](#integration-tests)).      | [Integration tests](#integration-tests)                                                                         |
| CLI integration         | `make test-validate-cli`                                    | Contributors changing `rad` CLI commands. Runs `./pkg/cli/cmd/...` and `./cmd/rad/...`.                                                                                                         | [`pkg/cli/cmd/README.md`](../../../../pkg/cli/cmd/README.md)                                                    |
| Helm chart unit         | `make test-helm`                                            | Contributors changing the Helm chart under `deploy/Chart`.                                                                                                                                      | —                                                                                                               |
| Bicep validation        | `make test-validate-bicep`                                  | Contributors changing `.bicep` files; validates they compile cleanly.                                                                                                                           | —                                                                                                               |
| Functional (end-to-end) | `make test-functional-all-noncloud` (and per-group targets) | Contributors validating realistic user scenarios on a real cluster. Deploys applications and asserts on their state.                                                                            | [Running functional tests](./running-functional-tests.md)                                                       |
| Local iteration         | n/a (build, push, redeploy loop)                            | Contributors iterating quickly on control-plane images against a running cluster.                                                                                                               | [Accelerating local verification](./testing-local.md)                                                           |

### Unit tests

Run the unit tests with:

```sh
make test
```

`make test` runs the Go unit tests under `./pkg/...` (excluding the Kubernetes controller tests) and the Helm chart unit tests. We require unit tests for new code and for fixes or refactors of existing code; as a rule, every PR should add or change some tests. Unit tests must run with only the [basic prerequisites](../contributing-code-prerequisites/) installed — do not add external dependencies for a unit test; write an integration test instead.

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

Functional tests (also called end-to-end tests) use Radius to deploy an application to a real hosting environment (Kubernetes) and then assert on its state. They have their own dedicated set of [instructions](./running-functional-tests.md), including prerequisites, the `make test-functional-*` targets, and cleanup modes. The fastest entry point is the non-cloud suite:

```sh
make test-functional-all-noncloud
```

### Local-iteration loop

When you are iterating on control-plane images (for example the applications resource provider) against a running cluster, the build/push/redeploy loop is faster than the full functional suite. See [Accelerating local verification](./testing-local.md).

### Helm and Bicep validation

- `make test-helm` runs the Helm chart unit tests in `deploy/Chart` using the `helm-unittest` plugin.
- `make test-validate-bicep` validates that every `.bicep` file compiles cleanly.

### Test conventions

- We write unit tests in a straightforward style and use [testify](https://github.com/stretchr/testify) for assertions.
- We favor [subtests](https://go.dev/blog/subtests) and [table-driven tests](https://dave.cheney.net/2019/05/07/prefer-table-driven-tests) and apply them where appropriate.
- For functional tests specifically, see [writing functional tests](./writing-functional-tests.md), the [naming conventions](./tests-naming-conventions.md), [test logging](./tests-logging.md), and [using standard images in tests](./tests-images-pushtoghcr.md).

## Verification

- A passing run prints `ok` for every package that has tests and `[no test files]` for those that don't; `go test` exits non-zero if any test fails, so a failure is obvious in the output.
- We measure code coverage as part of the PR process because it shows whether the right tests are being added.

## Troubleshooting

- **`make test` fails setting up `KUBEBUILDER_ASSETS` / envtest.** The target uses `setup-envtest` (managed as a Go tool via the `tool` directive in `go.mod`) to download Kubernetes test binaries. Ensure you have network access on the first run; the assets are cached under `./bin` afterward.
- **`make test-helm` cannot find the unittest plugin.** The target installs the `helm-unittest` plugin automatically; if installation is blocked, install it manually with `helm plugin install https://github.com/helm-unittest/helm-unittest.git`.
- **A unit test needs an external dependency.** It belongs in the functional tier, not in `make test`. Move it so the unit suite keeps running with only the basic prerequisites.
- **A functional test fails or leaves resources behind.** Follow the [functional test instructions](./running-functional-tests.md), which cover prerequisites, namespaces, and cleanup modes.
