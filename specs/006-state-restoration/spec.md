# Feature Specification: Reconcile hydrated state against reality on `rad startup`

**Feature Branch**: `state-restoration`
**Created**: 2026-08-28
**Status**: Draft - awaiting approval
**Input**: Design notes from the sync with Will Tsai and Nicole James on the "Delete workflow 409 loops forever when a resource is stranded in a non-terminal state" bug (recording 2026-08-28).

## Purpose

Repo Radius (the ephemeral k3d control plane the GitHub workflows spin up on every run) restores durable state from an OCI archive at the start of each run via [`rad startup`](../../pkg/cli/cmd/startup/startup.go). The restore is currently a one-way load: PostgreSQL dumps and Terraform state Secrets are put back into the fresh control plane exactly as they were persisted at the end of the previous run.

That is not sufficient when the previous run was interrupted while a resource was mid-operation. The archive can preserve a resource in a non-terminal state — for example `provisioningState: "Updating"` — that never actually completed. On the next run, the control plane accepts that state as authoritative, so every subsequent operation against the resource is blocked with `409 Conflict / target resource is in progress`. The delete workflow loops on that 409 forever and the application becomes undeletable through Radius.

This feature adds a reconciliation pass triggered by `rad startup` and executed against the running control plane: for every application in the plane, an application-scoped `reconcile` action asks each resource's owning resource provider to check its actual current state and rewrite the state store to match reality — including removing entries when the underlying resource does not exist.

The scope is deliberately narrow: reconcile hydrated state so operations that follow see reality.

## Non-goals

- **A `rad app delete --force` flag was considered and explicitly rejected.** A force option that bypasses state can convert an in-progress happy-path delete into a broken one by overwriting the state store while the first delete is still driving to a terminal state. Fixing hydration is the right approach.
- **No user-facing message when a resource is genuinely still updating.** If reconciliation finds the resource actually is in `Updating` state, the hydrated state is accurate — leave it. The users will continue to see the rror message we see today:
```
RESPONSE 409: 409 Conflict
ERROR CODE: Conflict
{
  "error": {
    "code": "Conflict",
    "message": "The target resource is in progress state: Updating."
  }
}
```
- **The persistent Radius control plane's async controllers are not changed.** They already reconcile continuously; the new action is dormant unless `rad startup` (or a test) invokes it.
- **The concurrent-`rad app delete` behavior evaluation as part of control plane is out of scope** — tracked as a separate follow-up (see [Follow-up](#follow-up)).

## Decisions

### The client-facing endpoint is an application-scoped custom action

Reconciliation is a per-application operation: walk the application's children, check each one's reality, roll the results back into the state store. That is the same shape as [`getGraph`](../../pkg/corerp/frontend/controller/applications/v20250801preview/getgraph.go) — an application-scoped custom action registered on `Radius.Core/applications` that walks children across resource providers. `reconcile` therefore reuses the exact pattern, up to and including the corerp orchestrator that already knows how to fan out across RPs through the UCP proxy.

```text
POST /planes/radius/local/resourceGroups/{rg}/providers/Radius.Core/applications/{app}/reconcile?api-version=2025-08-01-preview
Content-Type: application/json
{}
```

- Registered in [pkg/corerp/setup/setup.go](../../pkg/corerp/setup/setup.go) under the `Custom` map on the application resource, next to `getGraph`.
- Handler in `pkg/corerp/frontend/controller/applications/v20250801preview/reconcile.go`.
- Response is the standard ARM-RPC async pattern: `202 Accepted` with a `Location` header. The client polls to completion. This matches every other write-shaped action in Radius and keeps `rad startup` from holding a synchronous connection open while UCP proxies to many RPs.

Naming: `reconcile`, lowercase, matches the codebase convention (`getGraph`, `join`, `getmetadata`). Not `refresh`, not `reconcileStatus` — the action is exactly analogous to the RP-internal reconciliation the persistent control plane already does asynchronously.

### The corerp handler orchestrates; UCP is the proxy

corerp's [`GetGraphv20250801preview`](../../pkg/corerp/frontend/controller/applications/v20250801preview/getgraph.go) already receives a `sdk.Connection` at construction, enumerates an application's children by walking resource types across resource providers, and issues per-resource GETs that UCP proxies to the owning RP. `reconcile` reuses that walk and issues a per-resource `reconcile` POST to each non-terminal child instead of a GET.

Concretely, the handler:

1. Loads the application record.
2. Traverses the same resource-type registration list `getGraph` uses (via UCP's `System.Resources/resourceProviders`) to build the child set.
3. Filters to children whose current `provisioningState` is non-terminal — anything other than `Succeeded` or `Failed`. Terminal-state children are left alone; the archive captured a settled state and the next user operation will refresh it through the normal path.
4. For each such child, issues:

    ```text
    POST /planes/…/providers/{Namespace}/{resourceType}/{name}/reconcile?api-version=…
    ```

    in parallel (bounded fan-out), through the UCP-fronted connection the handler already has.
5. After every child response returns, reconciles the application record itself: if all children are now terminal, transition the application accordingly; if any child remains non-terminal, leave the application in its hydrated state.
6. Aggregates per-child outcomes into a report and completes the async operation.

UCP is not a smart orchestrator here — it is the proxy layer that already routes `/planes/…/providers/{ns}/…` to the owning RP. That is enough. "UCP asks every RP" is satisfied by construction because every per-child call goes through UCP.

### Per-resource `reconcile` handled by dynamic-rp

Legacy `Applications.Core/*` / `Applications.Datastores/*` / `Applications.Dapr/*` / `Applications.Messaging/*` types are out of scope. Only the dynamic types the modern Radius application model uses are reconciled — `Radius.Compute/containers`, `Radius.Compute/gateways`, and every community-contributed type served by [dynamic-rp](../../pkg/dynamicrp/).

Dynamic-rp implements `reconcile` once and serves it for every dynamic type — no per-type registration, no per-type TypeSpec. The handler runs the algorithm you would otherwise run from the CLI (`rad resource list -a <app> --preview`, then check each resource): list the app's resources, check each one's underlying provider, PATCH state to match.

For a single resource, the dynamic-rp handler:

1. Loads the resource from dynamic-rp's own store.
2. If `provisioningState` is terminal, returns unchanged.
3. Reads the resource's `properties.status.outputResources` — the concrete backing objects the recipe engine recorded when the resource was deployed.
4. For each output resource, queries its underlying provider:
    - Kubernetes objects → GET via the target-cluster Kubernetes client the RP already holds.
    - Terraform-backed cloud outputs (Azure/AWS resource IDs) → **out of scope for the prototype**; record `skipped: cloud output not yet reality-checked` in the per-output report. Follow-up work adds the cloud-SDK branches inside the same handler.
5. Aggregates outcomes per the table in [What "reality" is for each resource](#what-reality-is-for-each-resource) and writes the result back through dynamic-rp's normal `PATCH` code path. **No direct SQL.**
6. Returns the new state, or an error the corerp orchestrator will record in the report.

### What "reality" is for each resource

| Hydrated `provisioningState` | Reality query result       | Action                                                                                             |
| ---------------------------- | -------------------------- | -------------------------------------------------------------------------------------------------- |
| any non-terminal             | terminal (settled)         | Rewrite the state-store entry with the observed terminal state (`Succeeded` / `Failed` / etc.).    |
| any non-terminal             | still non-terminal         | Leave as-is. The hydrated state is accurate.                                                       |
| any non-terminal             | not found                  | Delete the state-store entry. The resource does not exist.                                         |
| any non-terminal             | error (network, 5xx, etc.) | Leave as-is; record the error in the response. Reconciliation is best-effort and never fails.      |

Terminal-state entries are never reconciled by this action.

### Reconciliation is best-effort and does not fail `rad startup`

A failed per-resource `reconcile` does not fail the application's `reconcile`. A failed application `reconcile` does not fail `rad startup`. Every outcome (skipped, unchanged, updated, deleted, failed to query) is written to the `rad startup` log so it is visible in the workflow log.

This preserves the guarantee that a run can always at least *try* to make progress. It also means the change is safe to ship without a fallback flag: at worst the pass is a no-op.

### The client-side stage

`rad startup` today performs four stages ([pkg/cli/cmd/startup/stateclient.go](../../pkg/cli/cmd/startup/stateclient.go)): `ScaleDown` → `RestoreDatabases` → `RestoreTerraform` → `ScaleUp`. A fifth stage, `ReconcileHydratedState`, is added after `ScaleUp` and after the resource-provider deployments are ready to serve. It:

1. Lists applications in the plane through UCP.
2. For each application, POSTs `.../applications/{app}/reconcile` and polls the async operation to completion (with a bounded timeout).
3. Logs the per-application report.
4. Always returns success.

No workflow-level change. [restore-state/action.yml](../../.github/extension/actions/restore-state/action.yml) already runs `rad startup`; the reconciliation is transparent.

### Why the persistent control plane is not disrupted

The `reconcile` action is dormant unless something calls it. Regular Radius never does — the persistent control plane's async operation controller and health controller reconcile continuously through their own polling loops, and stuck non-terminal states resolve within the RP's own interval. The archive-hydrate topology short-circuits that: state is loaded from disk and immediately trusted. `rad startup` calling `reconcile` closes that gap only for the ephemeral topology, without changing the runtime for the persistent one.

## System Context

### Where the bug manifests today

- The GitHub delete workflow ([.github/extension/delete-azure.yml](../../.github/extension/delete-azure.yml), [.github/extension/delete-aws.yml](../../.github/extension/delete-aws.yml)) runs [`restore-state`](../../.github/extension/actions/restore-state/action.yml) which shells out to `rad startup`, then [`delete-resource`](../../.github/extension/actions/delete-resource/action.yml) which shells out to `rad app delete <name> --yes --preview`.
- The failure mode: `rad app delete` retries `409 Conflict / target resource is in progress` indefinitely because the hydrated state store reports a resource in a non-terminal state that never actually existed (or that has since settled underneath). The transcript's specific case was an application whose deployment had failed, leaving nothing in the cloud, while the state store insisted the resource was `Updating`.

### Where the change lives

- Client-side stage: [pkg/cli/cmd/startup/startup.go](../../pkg/cli/cmd/startup/startup.go) and [pkg/cli/cmd/startup/stateclient.go](../../pkg/cli/cmd/startup/stateclient.go).
- Application-scoped orchestrator: `pkg/corerp/frontend/controller/applications/v20250801preview/reconcile.go` (new), registered in [pkg/corerp/setup/setup.go](../../pkg/corerp/setup/setup.go) beside `getGraph`.
- Per-resource handler for every dynamic type: `pkg/dynamicrp/frontend/reconcile.go` (new), wired into dynamic-rp's existing router. One implementation serves every registered dynamic type.
- API surface: additive `reconcile` custom action on `Radius.Core/applications/{name}`, authored in [typespec/](../../typespec/) and regenerated via `make generate`. Dynamic types do not need per-type TypeSpec — dynamic-rp exposes the action once for every type it serves.

Nothing outside these files needs to change.

## Acceptance

- Deleting an application whose state archive contains at least one child resource in a non-terminal `provisioningState` succeeds when the underlying cloud/Kubernetes resource does not exist. Today it loops on `409`.
- Deleting an application whose state archive contains a child resource in `Updating` and whose underlying resource genuinely is still updating waits normally and does not falsely succeed. The reconciler must observe the reality-reported state and leave the store unchanged.
- `rad startup` never fails because reconciliation could not reach a resource provider. The workflow log records the failure and startup returns success.
- The reconciler runs no direct SQL against any resource-provider database. Every state change goes through the RP's normal write path, so state machines stay intact.
- The prototype covers dynamic-rp resources with Kubernetes `outputResources` end-to-end (the transcript's failing case): an application whose containers/gateways/secretstores were hydrated in `Updating` is deletable after `rad startup`, whether or not the underlying Kubernetes objects still exist. Terraform-backed cloud outputs record `skipped` and are follow-ups.

## Follow-up

- A separate issue tracks verifying that concurrent `rad app delete` against the same application from two terminals is handled correctly by a regular (persistent) Radius control plane. That case is not affected by hydration — a persistent control plane already tracks in-flight operations — but it needs an explicit test so a future change cannot regress it. See [radius-project/radius#12870](https://github.com/radius-project/radius/issues/12870).
- Reality-checking Terraform-backed cloud `outputResources` (Azure/AWS resources managed via recipes) is follow-up work. Adds cloud-SDK branches inside the same dynamic-rp `reconcile` handler; no framework changes.
