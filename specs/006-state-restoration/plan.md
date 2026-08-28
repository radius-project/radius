# Implementation Plan: Reconcile hydrated state against reality on `rad startup`

**Branch**: `state-restoration` | **Date**: 2026-08-28 | **Spec**: [spec.md](spec.md)

## Summary

Add a `reconcile` custom action to `Radius.Core/applications/{name}` (mirror of [`getGraph`](../../pkg/corerp/frontend/controller/applications/v20250801preview/getgraph.go)) plus a per-resource-type `reconcile` custom action registered on `Radius.Compute/containers` (prototype scope). `rad startup` gains a fifth stage, `ReconcileHydratedState`, that lists applications after `ScaleUp` and POSTs the app-scoped action for each. The corerp application handler fans out through the UCP proxy to per-resource-type handlers, which query reality (Kubernetes for containers) and rewrite the state store through the RP's normal write path. Best-effort throughout: individual failures never fail `rad startup`.

## Technical Context

**Language/Version**: Go 1.26.5 (per `go.mod`)
**Primary Dependencies**: no new external dependencies. Reuses `github.com/Azure/azure-sdk-for-go/sdk/azcore` (async operation), `k8s.io/client-go` (per-container reality check), and the internal `pkg/armrpc/builder` custom-action registration mechanism.
**Storage**: no schema changes. Reconciliation writes go through the RPs' existing state-store paths.
**Testing**: `go test` with `stretchr/testify`; table-driven unit tests for the corerp orchestrator and the containers handler; a `httptest`-backed integration test that mounts the whole custom-action flow end to end. Existing `rad startup` tests get a new fake for `ReconcileHydratedState`.
**Target Platform**: Radius control plane (Linux server binary) and `rad` CLI (macOS/Linux/Windows).
**Project Type**: Single Go module `github.com/radius-project/radius`.
**Performance Goals**: Reconciliation for an application with ≤50 containers must complete within 30 s on the k3d control plane. No hot-path allocations in the containers handler (a k8s GET per container is the dominant cost).
**Constraints**: no direct SQL against RP databases; no boot-time reconciliation in the persistent control plane; opt-in per resource type.
**Scale/Scope**: prototype covers `Radius.Compute/containers` only. Ships as one PR; other types are follow-ups.

## Constitution Check

*GATE: Passed at plan authoring time.*

| Principle                                            | Verdict | Note                                                                                                                                                                                                              |
| ---------------------------------------------------- | ------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| I. API-First Design                                  | ✅       | Wire change authored in TypeSpec (`typespec/Radius.Core/applications.tsp` and `typespec/Radius.Compute/containers.tsp`); Go models regenerated via `make generate`.                                                |
| II. Idiomatic Code Standards                         | ✅       | `gofmt`, small exported surface, godoc on every exported symbol, table-driven tests.                                                                                                                              |
| III. Multi-Cloud Neutrality                          | ✅       | The application-scoped action is cloud-agnostic. Per-resource handlers query their own underlying provider (Kubernetes for the prototype; TF/Azure/AWS for follow-ups). No provider carve-out in the orchestrator. |
| IV. Testing Pyramid Discipline                       | ✅       | Unit tests for the orchestrator and the containers handler; a `httptest`-backed integration test that mounts the whole custom-action flow end to end.                                                             |
| V. Collaboration-Centric Design                      | ✅       | Fixes an operator-visible failure (delete workflow loops forever) without new user-facing surface — the flag path deliberately not taken.                                                                         |
| VI. Open Source and Community-First                  | ✅       | Spec and plan authored in the public repo; commits will carry `Signed-off-by`.                                                                                                                                    |
| VII. Simplicity Over Cleverness                      | ✅       | Reuses the existing `Custom` action mechanism, the existing `getGraph` traversal, and the existing async-operation pattern. No new framework code.                                                                 |
| VIII. Separation of Concerns                         | ✅       | Orchestrator in corerp, reality-check logic in the per-type handler, transport through UCP. Each layer owns what it already owns.                                                                                 |
| IX. Incremental Adoption & Backward Compatibility    | ✅       | `Radius.Core/2025-08-01-preview` is preview; adding a `Custom` action is additive. `Applications.Core` is not touched.                                                                                            |
| XII / XIII (resource type / recipe standards)        | N/A     | No new resource types or recipes.                                                                                                                                                                                 |
| XVII. Polyglot Project Coherence                     | ✅       | TypeSpec is the single source of truth for the wire; Go generated code follows.                                                                                                                                    |

**No violations. Complexity Tracking section is empty.**

## Project Structure

### Documentation (this feature)

```text
specs/006-state-restoration/
├── plan.md              # This file
└── spec.md              # Feature spec
```

Additional artifacts (research/data-model/quickstart/tasks) are not required — the scope is small enough to plan directly.

### Source Code (repository root)

Additions and edits, all within the existing single Go module:

```text
# TypeSpec — additive custom actions
typespec/Radius.Core/applications.tsp        # add `reconcile` action on applications/{name}
typespec/Radius.Compute/containers.tsp       # add `reconcile` action on containers/{name}

# Regenerated Go models — via `make generate`
pkg/corerp/api/v20250801preview/zz_generated_*.go

# New — application-scoped orchestrator (mirror of getgraph.go)
pkg/corerp/frontend/controller/applications/v20250801preview/
├── reconcile.go
└── reconcile_test.go

# New — per-resource-type reality check (prototype scope)
pkg/corerp/frontend/controller/containers/
├── reconcile.go
└── reconcile_test.go

# Edited — register the two new custom actions
pkg/corerp/setup/setup.go

# Edited — new ReconcileHydratedState stage
pkg/cli/cmd/startup/
├── startup.go             # wire the stage
├── stateclient.go         # add method to the StateRestoreClient interface
└── startup_test.go        # add coverage for the new stage
```

## Phases

### Phase 0 — Wire the app-scoped action end to end with a no-op handler

Goal: prove the registration, routing, async-operation lifecycle, and `rad startup` invocation before we do any reality checking.

- Add the `reconcile` custom action on `Radius.Core/applications/{name}` in TypeSpec; regenerate.
- Implement `pkg/corerp/frontend/controller/applications/v20250801preview/reconcile.go` as a stub that returns an immediately-succeeded async operation with an empty report.
- Register the action in `pkg/corerp/setup/setup.go` beside `getGraph`.
- Add `ReconcileHydratedState(ctx, connection)` on `StateRestoreClient` in `pkg/cli/cmd/startup/stateclient.go`. Implementation lists applications, POSTs the action, polls to completion, logs the (empty) report.
- Wire the new stage in `pkg/cli/cmd/startup/startup.go` after `ScaleUp`.
- Unit tests: fake `StateRestoreClient` records the call; corerp handler test verifies the async-operation shape.

**Exit criterion**: `rad startup` on a k3d cluster with one hydrated `Radius.Core/applications` succeeds and logs `reconciled 0 resources` for it.

### Phase 1 — Implement the containers per-resource `reconcile`

Goal: reality-check containers against Kubernetes and rewrite the state store.

- Add the `reconcile` custom action on `Radius.Compute/containers/{name}` in TypeSpec; regenerate.
- Implement `pkg/corerp/frontend/controller/containers/reconcile.go`:
    - Look up the container record.
    - If `provisioningState` is terminal, return unchanged.
    - Read the underlying Kubernetes object (Deployment) via the RP's existing k8s client. Interpret 404, terminal, non-terminal per the [reality table](spec.md#what-reality-is-for-each-resource).
    - Write the outcome back through the container RP's normal PATCH path.
    - Return the new state.
- Register the action in `pkg/corerp/setup/setup.go` under the `containers` resource block.
- Update the corerp orchestrator from Phase 0 to actually walk children, filter to non-terminal, POST `reconcile` to each container in parallel (bounded), collect responses, reconcile the application record from the aggregated child states, return a populated report.
- Unit tests: containers handler with a fake k8s client (three cases from the reality table); orchestrator with a fake UCP connection and multiple child responses.
- Integration test: mount the whole thing behind `httptest` — application with two containers, one 404 in k8s and one healthy — verify the app-scoped POST returns a report and the state store reflects reality.

**Exit criterion**: the [Acceptance](spec.md#acceptance) prototype criterion holds — an application whose containers were hydrated in `Updating` but do not exist in Kubernetes is deletable after `rad startup`.

### Phase 2 — Integration and functional coverage

Goal: prove the end-to-end delete flow against a k3d cluster.

- Extend `test/functional/` with a case that:
    1. Seeds a state archive with an application whose container is `Updating` and whose k8s Deployment does not exist.
    2. Runs `rad startup` against a fresh k3d cluster loaded with that archive.
    3. Runs `rad app delete <name> --yes --preview` and asserts it succeeds (no 409 loop).
- Verify no direct SQL calls in the reconciler code path via a `grep_search`-style CI check (informational).

**Exit criterion**: functional test passes in CI on every PR.

## Rollout

- Ship as one PR that lands Phase 0 + Phase 1 together (the two are cheap and separating them leaves a dead endpoint in the tree). Phase 2 is a follow-up PR because functional-test infra changes deserve their own review.
- No feature flag. The action is dormant unless called; only `rad startup` calls it; only Repo Radius runs `rad startup`. The persistent control plane never invokes it and does not care.
- No release-note user impact (behavior change is invisible to `rad app delete` callers — they just stop looping on 409).

## Risks and open questions

- **Child enumeration source of truth.** `getGraph` walks children by scanning UCP's `System.Resources/resourceProviders` and listing each type. If a resource type is registered but its RP is unresponsive, the corerp orchestrator must not hang. Timebox each per-child call to a bounded deadline and record the failure in the report.
- **What to write when the underlying resource is gone.** When the reality check returns 404 from Kubernetes, the reconcile handler moves `provisioningState` from `Updating` to `Failed`. It does **not** delete the state-store row. Rationale: once the row is in a terminal state, it no longer blocks the delete path with `409`, and the user's next `rad app delete` runs the normal delete workflow — which will call k8s, receive its own 404, treat it as "already gone", and remove the row. This keeps all cleanup on one code path. The alternative (removing the row here in the reconciler) requires a new "delete without running the delete workflow" bypass on the RP, which is exactly the kind of side-door around the RP state machine we agreed to avoid. Trade-off: between `rad startup` and the next `rad app delete`, `rad app show` will list the container as `Failed` even though there is no k8s object. If that shows up as a real UX problem we revisit; the k8s-based prototype is unlikely to hit it.
- **Preview-API sensitivity.** Adding a `Custom` action on the preview surface is additive, but downstream consumers of the generated Go client have to regenerate. There is no public preview SDK release cadence to worry about; internal callers regenerate on the next `make generate`.
