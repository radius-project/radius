# Phase 0 Research — Application Graph Dependency Edges

**Feature**: [spec.md](spec.md) · **Plan**: [plan.md](plan.md) · **Date**: 2026-07-16

The plan raised five unknowns. All are resolved from the current codebase — no external research required. Each entry names the decision, the rationale, and the code reference.

## R-001 · Shape of `dependsOn` after `collectResources` normalization

**Decision**: Use `resolveDependsOn` as-is; it returns canonical `[resourceId('TYPE','NAME')]`-derived Radius resource IDs.

**Rationale**: [pkg/cli/graph/modeled.go](../../pkg/cli/graph/modeled.go) already normalizes ARM `languageVersion` 2.0 symbolic-name templates into the classic `[resourceId(...)]` form in `collectResources` / `rewriteSymbolicConnections` / `normalizeSymbolicEntry` before any consumer sees a resource entry. `resolveDependsOn` — the same helper `DiffHash` uses — parses those strings into canonical IDs. The dependency-edge extraction reuses that helper verbatim; no new parser is required.

**Alternatives considered**:

- Rolling a second parser inside the new `pkg/graph/edges/` package. Rejected: duplicates logic already covered by `resolveDependsOn` and doubles the maintenance surface for a well-tested transformation.

## R-002 · Existing `resolveDependsOn` reuse

**Decision**: Keep `resolveDependsOn` in `pkg/cli/graph/`. The new package `pkg/graph/edges/` accepts *already-resolved* canonical IDs and does no ARM parsing.

**Rationale**: FR-018 requires the primitives to be free of ARM template syntax so the future runtime caller (Phase 2) can use them without stubbing ARM helpers. Callers (static and runtime) do their own conversion into the pure-Go `Resource` struct. The static caller resolves `dependsOn` via `resolveDependsOn`; the runtime caller has no `dependsOn` at all in Phase 1 (empty slice).

**Alternatives considered**:

- Move `resolveDependsOn` into `pkg/graph/edges/`. Rejected: it would drag ARM-specific parsing into a neutral package and violate the "no ARM syntax in primitives" rule (FR-019).

## R-003 · Wire model for `kind`

**Decision**: Follow the exact shape of the existing `Direction` enum. Add:

```typespec
@doc("The origin of a connection: 'Connection' for author-declared entries in properties.connections, 'Dependency' for implicit entries derived from Bicep's dependsOn list.")
enum ConnectionKind {
  @doc("The edge was declared by the author in properties.connections.")
  Connection,

  @doc("The edge was inferred from a Bicep dependsOn entry (implicit dependency).")
  Dependency,
}
```

And on `ApplicationGraphConnection`:

```typespec
@doc("Discriminator identifying the origin of this edge. Connection edges are author-declared; Dependency edges are implicit dependencies surfaced from Bicep's dependsOn list.")
kind: ConnectionKind;
```

**Rationale**: `Direction` is the closest structural analogue already on this model (a required enum discriminator). The naming `ConnectionKind` follows the user's own phrasing ("connection kind") and mirrors the `Direction` pattern (`{ModelName}Kind` alongside the noun-only `Direction`; the alignment is close enough given the reference is already `ApplicationGraphConnection`).

**Alternatives considered**:

- `EdgeKind`. Rejected: the containing model is `ApplicationGraphConnection`, not `ApplicationGraphEdge`, so `EdgeKind` would introduce a noun the wire doesn't use elsewhere.
- Making `kind` optional (`kind?: ConnectionKind`). Rejected: an "unset kind" state would force every consumer to guard against null; because the runtime handler always emits `kind: Connection` and the static builder always emits either value, an unset state has no producer and would be dead API surface.

## R-004 · How to regenerate Go models from TypeSpec

**Decision**: Run `make generate` at the repository root; commit the regenerated files under `pkg/corerp/api/v20250801preview/zz_generated_models.go` and `swagger/specification/core/resource-manager/Radius.Core/preview/2025-08-01-preview/applications.json`.

**Rationale**: [CONTRIBUTING.md](../../CONTRIBUTING.md) documents `make generate` as the standard regeneration target. CI enforces that generated files are checked in (see [`.github/workflows/`](../../.github/workflows/)).

**Alternatives considered**:

- Hand-editing `zz_generated_models.go`. Rejected — the file is stamped `DO NOT EDIT` and will be overwritten on next generation.

## R-005 · Where Applications.Core vs Radius.Core preview conversion happens

**Decision**: The shared internal graph builder in [pkg/corerp/frontend/controller/applications/graph_util.go](../../pkg/corerp/frontend/controller/applications/graph_util.go) returns an internal representation. Each API version has its own conversion layer:

- Applications.Core (stable): [pkg/corerp/frontend/controller/applications/getgraph.go](../../pkg/corerp/frontend/controller/applications/getgraph.go).
- Radius.Core preview: [pkg/corerp/frontend/controller/applications/v20250801preview/getgraph.go](../../pkg/corerp/frontend/controller/applications/v20250801preview/getgraph.go).

Only the Radius.Core preview conversion is touched. It sets `kind: Connection` on every edge (the runtime has no `Dependency` edges in Phase 1). The shared internal builder is not modified.

**Rationale**: This surgical placement preserves FR-016 ("`Applications.Core` MUST NOT change"): the shared builder is unchanged, so Applications.Core responses remain byte-identical. Only the Radius.Core preview converter — a file already dedicated to the preview surface — changes.

**Alternatives considered**:

- Adding `kind` on the internal representation and having each API version's converter emit or drop it. Rejected as premature: the internal builder's representation is not part of the wire, and adding a field there just to have Applications.Core drop it later would be dead code. If Phase 2 introduces runtime `Dependency` extraction that both API versions must reason about, we revisit this then; today, only Radius.Core preview needs to know about `kind`.

## R-006 · New package location — `pkg/graph/edges/`

**Decision**: Create a new Radius-domain-neutral package at `pkg/graph/edges/`. Not under `pkg/cli/`, not under `pkg/corerp/`, and not inside the existing `pkg/algorithm/graph/` (which is a generic dependency-graph algorithm library, unrelated to Radius resource-type exclusion rules).

**Rationale**: FR-017 requires that both the CLI (`pkg/cli/graph/`) and the runtime handler (`pkg/corerp/frontend/controller/applications/v20250801preview/`) import the primitives. A neutral top-level package avoids the anti-pattern of `pkg/corerp/` importing from `pkg/cli/`. `pkg/algorithm/graph/` is generic and cloud/Radius-agnostic; overloading it with Radius-specific `Radius.Core/*` exclusion rules would violate separation of concerns (Constitution VIII).

**Alternatives considered**:

- `pkg/cli/graph/`: rejected — makes control-plane import CLI code.
- `pkg/corerp/graph/`: rejected — makes CLI import control-plane internals.
- `pkg/algorithm/graph/`: rejected — that package is domain-neutral graph algorithms; Radius resource-type exclusion belongs in a Radius-domain package.
