# Implementation Plan: Reconcile hydrated state against reality on `rad startup`

**Branch**: `state-restoration` | **Date**: 2026-08-28 | **Spec**: [spec.md](spec.md)

## Summary

Add a `reconcile` custom action to `Radius.Core/applications/{name}` (mirror of [`getGraph`](../../pkg/corerp/frontend/controller/applications/v20250801preview/getgraph.go)) and a per-resource `reconcile` handler in [dynamic-rp](../../pkg/dynamicrp/) served for every dynamic resource type. Legacy `Applications.*` types are out of scope. `rad startup` gains a fifth stage, `ReconcileHydratedState`, that lists applications after `ScaleUp` and POSTs the app-scoped action for each. The corerp application handler fans out through the UCP proxy to dynamic-rp, which runs the CLI-equivalent reality check for each resource — read `properties.status.outputResources`, GET each Kubernetes object, PATCH `provisioningState` to match reality. Terraform-backed cloud outputs record `skipped` and are follow-ups. Best-effort throughout: individual failures never fail `rad startup`.

## Technical Context

**Language/Version**: Go 1.26.5 (per `go.mod`)
**Primary Dependencies**: no new external dependencies. Reuses `github.com/Azure/azure-sdk-for-go/sdk/azcore` (async operation), `k8s.io/client-go` (dynamic-rp's per-output reality check), and dynamic-rp's existing routing scaffold plus the internal `pkg/armrpc/builder` custom-action mechanism (for the app-scoped orchestrator on corerp).
**Storage**: no schema changes. Reconciliation writes go through the RPs' existing state-store paths.
**Testing**: `go test` with `stretchr/testify`; table-driven unit tests for the corerp orchestrator and the dynamic-rp `reconcile` handler; a `httptest`-backed integration test that mounts the whole custom-action flow end to end. Existing `rad startup` tests get a new fake for `ReconcileHydratedState`.
**Target Platform**: Radius control plane (Linux server binary) and `rad` CLI (macOS/Linux/Windows).
**Project Type**: Single Go module `github.com/radius-project/radius`.
**Performance Goals**: Reconciliation for an application with ≤50 resources must complete within 30 s on the k3d control plane. No hot-path allocations in the dynamic-rp handler (a k8s GET per output resource is the dominant cost).
**Constraints**: no direct SQL against RP databases; no boot-time reconciliation in the persistent control plane; legacy `Applications.*` types out of scope.
**Scale/Scope**: prototype covers dynamic-rp resources with Kubernetes `outputResources` (the transcript's failing case). Terraform-backed cloud outputs record `skipped` and are follow-ups.

## Constitution Check

*GATE: Passed at plan authoring time.*

| Principle                                            | Verdict | Note                                                                                                                                                                                                              |
| ---------------------------------------------------- | ------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| I. API-First Design                                  | ✅       | Wire change authored in TypeSpec on `typespec/Radius.Core/applications.tsp` (the app-scoped action). Dynamic types expose `reconcile` through dynamic-rp's routing without per-type TypeSpec. Go models regenerated via `make generate`.                                                |
| II. Idiomatic Code Standards                         | ✅       | `gofmt`, small exported surface, godoc on every exported symbol, table-driven tests.                                                                                                                              |
| III. Multi-Cloud Neutrality                          | ✅       | The application-scoped action is cloud-agnostic. Dynamic-rp iterates each resource's `outputResources` and queries the recorded provider per output (Kubernetes for the prototype; TF/Azure/AWS branches for follow-ups). No provider carve-out in the orchestrator. |
| IV. Testing Pyramid Discipline                       | ✅       | Unit tests for the corerp orchestrator and the dynamic-rp `reconcile` handler; a `httptest`-backed integration test that mounts the whole custom-action flow end to end.                                                             |
| V. Collaboration-Centric Design                      | ✅       | Fixes an operator-visible failure (delete workflow loops forever) without new user-facing surface — the flag path deliberately not taken.                                                                         |
| VI. Open Source and Community-First                  | ✅       | Spec and plan authored in the public repo; commits will carry `Signed-off-by`.                                                                                                                                    |
| VII. Simplicity Over Cleverness                      | ✅       | Reuses the existing `Custom` action mechanism, the existing `getGraph` traversal, and the existing async-operation pattern. No new framework code.                                                                 |
| VIII. Separation of Concerns                         | ✅       | Orchestrator in corerp, reality-check logic in dynamic-rp (one handler, all dynamic types), transport through UCP. Each layer owns what it already owns.                                                                                 |
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
# TypeSpec — additive app-scoped custom action
typespec/Radius.Core/applications.tsp        # add `reconcile` action on applications/{name}

# Regenerated Go models — via `make generate`
pkg/corerp/api/v20250801preview/zz_generated_*.go

# New — application-scoped orchestrator (mirror of getgraph.go)
pkg/corerp/frontend/controller/applications/v20250801preview/
├── reconcile.go
└── reconcile_test.go

# Edited — register the app-scoped orchestrator
pkg/corerp/setup/setup.go

# New — per-resource reality-check handler for every dynamic type
pkg/dynamicrp/frontend/
├── reconcile.go
└── reconcile_test.go

# Edited — wire the reconcile route into dynamic-rp's router
pkg/dynamicrp/frontend/routes.go              # or the equivalent registration site

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

### Phase 1 — Implement the dynamic-rp `reconcile` handler

Goal: reality-check every dynamic resource in the app against its `outputResources` and rewrite the state store.

- Register a `reconcile` route in dynamic-rp for every resource type it serves. One handler, no per-type registration.
- Implement `pkg/dynamicrp/frontend/reconcile.go`:
    - Look up the resource record.
    - If `provisioningState` is terminal, return unchanged.
    - Read `properties.status.outputResources`.
    - For each output:
        - Kubernetes object → GET via the target-cluster Kubernetes client the RP already holds. 404 → gone; terminal → settled; non-terminal → still updating.
        - Non-Kubernetes (Terraform-backed cloud output) → skip; record `skipped: cloud output not yet reality-checked` in the per-output report.
    - Aggregate per the reality table: if every output is gone → PATCH `provisioningState=Failed`; if every output is settled OK → leave `Succeeded`; if any output is still transitioning → leave `provisioningState` unchanged.
    - Write the outcome through dynamic-rp's normal PATCH path.
    - Return the new state and the per-output report.
- Update the corerp orchestrator from Phase 0 to actually walk children (the same list-per-registered-type walk `getGraph` uses, restricted to dynamic resource providers), filter to non-terminal, POST `reconcile` to each resource in parallel (bounded), collect responses, reconcile the application record from the aggregated child states, return a populated report.
- Unit tests: dynamic-rp handler with a fake k8s client (three cases from the reality table plus a `skipped` case for a cloud output); orchestrator with a fake UCP connection and multiple child responses.
- Integration test: mount the whole thing behind `httptest` — application with two `Radius.Compute/containers`, one 404 in k8s and one healthy — verify the app-scoped POST returns a report and the state store reflects reality.

**Exit criterion**: the [Acceptance](spec.md#acceptance) prototype criterion holds — an application whose dynamic-rp resources were hydrated in `Updating` but whose Kubernetes `outputResources` do not exist is deletable after `rad startup`.

### Phase 2 — Integration and functional coverage

Goal: prove the end-to-end delete flow against a k3d cluster.

- Extend `test/functional/` with a case that:
    1. Seeds a state archive with an application whose `Radius.Compute/containers` resource is `Updating` and whose k8s Deployment does not exist.
    2. Runs `rad startup` against a fresh k3d cluster loaded with that archive.
    3. Runs `rad app delete <name> --yes --preview` and asserts it succeeds (no 409 loop).
- Verify no direct SQL calls in the reconciler code path via a `grep_search`-style CI check (informational).

**Exit criterion**: functional test passes in CI on every PR.

## Rollout

- Ship as one PR that lands Phase 0 + Phase 1 together (the two are cheap and separating them leaves a dead endpoint in the tree). Phase 2 is a follow-up PR because functional-test infra changes deserve their own review.
- No feature flag. The action is dormant unless called; only `rad startup` calls it; only Repo Radius runs `rad startup`. The persistent control plane never invokes it and does not care.
- No release-note user impact (behavior change is invisible to `rad app delete` callers — they just stop looping on 409).

## Risks and open questions

- **Child enumeration source of truth.** `getGraph` walks children by scanning UCP's `System.Resources/resourceProviders` and listing each type. Corerp's `reconcile` orchestrator does the same walk but restricts to dynamic-rp types (the only ones whose reality-check handler is implemented in Phase 1). If a resource type is registered but its RP is unresponsive, the orchestrator must not hang. Timebox each per-child call to a bounded deadline and record the failure in the report.
- **What to write when the underlying resource is gone.** When the reality check returns 404 from Kubernetes, the reconcile handler moves `provisioningState` from `Updating` to `Failed`. It does **not** delete the state-store row. Rationale: once the row is in a terminal state, it no longer blocks the delete path with `409`, and the user's next `rad app delete` runs the normal delete workflow — which will call k8s, receive its own 404, treat it as "already gone", and remove the row. This keeps all cleanup on one code path. The alternative (removing the row here in the reconciler) requires a new "delete without running the delete workflow" bypass on the RP, which is exactly the kind of side-door around the RP state machine we agreed to avoid. Trade-off: between `rad startup` and the next `rad app delete`, `rad app show` will list the container as `Failed` even though there is no k8s object. If that shows up as a real UX problem we revisit; the k8s-based prototype is unlikely to hit it.
- **Preview-API sensitivity.** Adding a `Custom` action on the preview surface is additive, but downstream consumers of the generated Go client have to regenerate. There is no public preview SDK release cadence to worry about; internal callers regenerate on the next `make generate`.
