# Feature Specification: Reconcile hydrated state against reality on `rad startup`

**Feature Branch**: `state-restoration`
**Created**: 2026-08-28
**Status**: Draft - awaiting approval
**Input**: Design notes from the sync with Will Tsai and Nicole James on the "Delete workflow 409 loops forever when a resource is stranded in a non-terminal state" bug (recording 2026-08-28).

## Purpose

Repo Radius (the ephemeral k3d control plane the GitHub workflows spin up on every run) restores durable state from an OCI archive at the start of each run via [`rad startup`](../../pkg/cli/cmd/startup/startup.go). The restore is currently a one-way load: PostgreSQL dumps and Terraform state Secrets are put back into the fresh control plane exactly as they were persisted at the end of the previous run.

That is not sufficient when the previous run was interrupted while a resource was mid-operation. The archive can preserve a resource in a non-terminal state — for example `provisioningState: "Updating"` — that never actually completed. On the next run, the control plane accepts that state as authoritative, so every subsequent operation against the resource is blocked with `409 Conflict / target resource is in progress`. The delete workflow loops on that 409 forever and the application becomes undeletable through Radius.

This feature adds a reconciliation pass at the end of `rad startup`: for every resource the archive restored in a non-terminal `provisioningState`, query the resource's actual current state (through the resource provider that owns it) and update the state store to match reality — including marking the resource as gone when the underlying resource does not exist.

The scope is deliberately narrow: reconcile hydrated state so operations that follow see reality. It does not change the delete command, does not add a `--force` flag, and does not touch the control-plane runtime for regular (non-Repo) Radius, which already reconciles asynchronously through its own resource providers.

## Non-goals

- **A `rad app delete --force` flag was considered and explicitly rejected.** With two concurrent deletes (for example, a user re-runs `rad app delete` after their terminal died), a force option that bypasses state can convert an in-progress happy-path delete into a broken one by overwriting the state store while the first delete is still driving to a terminal state. Fixing hydration removes the need for the flag.
- **No user-facing message when a resource is genuinely still updating.** If reconciliation finds the resource actually is in `Updating` state, the hydrated state is accurate — leave it, do not warn.
- **Regular Radius (persistent control plane) is out of scope.** Its resource providers reconcile continuously; this bug is specific to the ephemeral archive-hydrated topology.
- **The concurrent-`rad app delete` behavior is out of scope** — tracked as a separate follow-up (see [Follow-up](#follow-up)).

## Decisions

### Reconciliation runs at the end of `rad startup`, before it returns success

`rad startup` today performs three stages, in order:

1. [`ScaleDown`](../../pkg/cli/cmd/startup/stateclient.go) the resource-provider deployments so no live pgx connections hold the databases open.
2. [`RestoreDatabases`](../../pkg/cli/cmd/startup/stateclient.go) — load the PostgreSQL dumps.
3. [`RestoreTerraform`](../../pkg/cli/cmd/startup/stateclient.go) — recreate the Terraform recipe state Secrets.
4. [`ScaleUp`](../../pkg/cli/cmd/startup/stateclient.go) — bring the resource providers back online.

Reconciliation adds a fifth stage that runs after ScaleUp and after the resource providers have finished their readiness probes: enumerate every resource in the restored state store whose `provisioningState` is non-terminal (`Accepted`, `Provisioning`, `Updating`, `Deleting`, or any other state that is neither `Succeeded` nor `Failed`), query its owning resource provider for the resource's current state, and rewrite the state-store entry accordingly. The whole reconciliation completes before `rad startup` exits, so downstream steps in the workflow (deploy, delete, or plain `rad` commands) observe an accurate state store from their first request.

Running after ScaleUp is required because the reconciliation queries flow through the same UCP/RP path that regular clients use — the resource providers must be ready to answer. Reconciliation must not talk directly to the database; that would re-introduce the class of bug Nicole flagged, where a database write bypasses the resource provider's state machine.

### What "reality" is for each resource

For each hydrated non-terminal resource, `rad startup` issues a GET against the resource-provider surface (UCP → RP) and interprets the response:

| Hydrated `provisioningState` | RP response          | Action                                                                                             |
| ---------------------------- | -------------------- | -------------------------------------------------------------------------------------------------- |
| any non-terminal             | `200`, terminal      | Rewrite the state-store entry with the RP-reported terminal state (`Succeeded` / `Failed` / etc.). |
| any non-terminal             | `200`, still non-terminal | Leave as-is. The hydrated state is accurate.                                                       |
| any non-terminal             | `404`                | Delete the state-store entry. The resource does not exist.                                         |
| any non-terminal             | error (network, 5xx) | Leave as-is; log a warning. Reconciliation is best-effort and never blocks startup.                |

Terminal-state entries (`Succeeded`, `Failed`) are not reconciled. The archive captured a settled state; if it drifts, the next operation the user issues will refresh it through the normal path.

### Reconciliation is best-effort and does not fail `rad startup`

`rad startup` today already treats a failed archive open as fatal, because without the archive the control plane has nothing to serve. Reconciliation is different: a reconciliation failure on any single resource, or on the reconciliation pass as a whole, must not fail startup. The workflow's subsequent commands then run against the un-reconciled state, which is no worse than today's behavior. Every reconciliation outcome (skipped, unchanged, updated, deleted, failed to query) is written to the `rad startup` log so it is visible in the workflow log.

This preserves the guarantee that a run can always at least *try* to make progress. It also means the fix is safe to ship without a fallback flag: at worst the reconciliation pass is a no-op.

### Scope of resources reconciled

The reconciler enumerates every resource in every restored resource-provider database. It does not filter by resource type, by application, or by environment. Filtering would require the reconciler to know which resource types can transition non-terminal states on their own (they all can) and would create carve-outs to keep in sync as new resource types land.

Applications and environments are themselves tracked resources and are included. If an application is hydrated in `Updating`, its record is reconciled the same way as any container or database inside it.

### Reconciliation semantics for children

An application's child resources (containers, databases, gateways, etc.) each have their own `provisioningState` and are reconciled independently. The reconciler does not need to walk the application graph; it enumerates directly from the resource store. If an application is hydrated in `Succeeded` but a container inside it was hydrated in `Updating`, only the container is reconciled.

## System Context

### Where the bug manifests today

- The GitHub delete workflow ([.github/extension/delete-azure.yml](../../.github/extension/delete-azure.yml), [.github/extension/delete-aws.yml](../../.github/extension/delete-aws.yml)) runs [`restore-state`](../../.github/extension/actions/restore-state/action.yml) which shells out to `rad startup`, then [`delete-resource`](../../.github/extension/actions/delete-resource/action.yml) which shells out to `rad app delete <name> --yes --preview`.
- The failure mode: `rad app delete` retries a `409 Conflict / target resource is in progress` indefinitely because the hydrated state store reports a resource in a non-terminal state that never actually existed (or that has since settled underneath). The specific transcript case was an application whose deployment had failed, leaving nothing in the cloud, while the state store insisted the resource was `Updating`.

### Where the change lives

- The `rad startup` command in [pkg/cli/cmd/startup/startup.go](../../pkg/cli/cmd/startup/startup.go) and its state client in [pkg/cli/cmd/startup/stateclient.go](../../pkg/cli/cmd/startup/stateclient.go). A new stage (call it `ReconcileHydrated`) is added there.
- The reconciler enumerates state through the standard UCP list endpoints and queries individual resources via the standard RP GET endpoints. It does not import the resource providers' internal packages.
- No workflow-level change. [restore-state/action.yml](../../.github/extension/actions/restore-state/action.yml) already runs `rad startup`; the reconciliation is transparent.
- No CLI change to `rad app delete`. Once reconciliation runs, the delete sees an accurate state store and proceeds normally.

### Why regular Radius is unaffected

A persistently running Radius control plane has resource providers that reconcile their world continuously — the async operation controller polls, the health controller polls, and stuck non-terminal states resolve within the RP's own polling interval. The archive-hydrate topology short-circuits that: the state is loaded from disk and immediately trusted. Reconciliation on hydrate closes that gap only for the ephemeral topology.

## Acceptance

- Deleting an application whose state archive contains at least one child resource in a non-terminal `provisioningState` succeeds when the underlying cloud resource does not exist. Today it loops on `409`.
- Deleting an application whose state archive contains a child resource in `Updating` and whose underlying cloud resource genuinely is still updating waits normally and does not falsely succeed. The reconciliation must observe the RP-reported state and leave the store unchanged.
- `rad startup` never fails because reconciliation could not reach a resource provider. The workflow log records the failure and startup returns success.
- The reconciliation pass runs no direct SQL against the resource-provider databases. Every state change goes through the RP, so state machines stay intact.
- No `--force` flag is added to `rad app delete` (or to any other command) as part of this feature.

## Follow-up

A separate issue is filed to verify that concurrent `rad app delete` against the same application from two terminals is handled correctly by a regular (persistent) Radius control plane. That case is not affected by hydration — a persistent control plane already tracks in-flight operations — but it needs an explicit test so a future change cannot regress it. See [radius-project/radius#12870](https://github.com/radius-project/radius/issues/12870).
