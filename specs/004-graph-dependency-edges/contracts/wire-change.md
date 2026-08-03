# Wire-Change Contract — Radius.Core/2025-08-01-preview

**Feature**: [spec.md](../spec.md) · **Plan**: [plan.md](../plan.md) · **Date**: 2026-07-16

This is the exact TypeSpec-source-level change that lands on the `Radius.Core/2025-08-01-preview` API surface. Every downstream artifact (`swagger/`, `pkg/corerp/api/v20250801preview/zz_generated_models.go`, Bicep types) is regenerated from this source via `make generate`.

**Scope constraint (FR-016)**: `Applications.Core` TypeSpec, generated code, and clients are not touched.

## File changed

[typespec/Radius.Core/applications.tsp](../../../typespec/Radius.Core/applications.tsp)

## Diff (added lines)

```typespec
@doc("The origin of a connection: 'Connection' for author-declared entries in properties.connections, 'Dependency' for implicit entries derived from Bicep's dependsOn list.")
enum ConnectionKind {
  @doc("The edge was declared by the author in properties.connections.")
  Connection,

  @doc("The edge was inferred from a Bicep dependsOn entry (implicit dependency). Not emitted by the runtime graph in Phase 1 — runtime dependency extraction is Phase 2.")
  Dependency,
}
```

And inside `model ApplicationGraphConnection`:

```typespec
model ApplicationGraphConnection {
  @doc("The resource ID ")
  id: string;

  @doc("The direction of the connection. 'Outbound' indicates this connection specifies the ID of the destination and 'Inbound' indicates indicates this connection specifies the ID of the source.")
  direction: Direction;

  @doc("Discriminator identifying the origin of this edge. Connection edges are author-declared entries under properties.connections. Dependency edges are implicit dependencies derived from Bicep's dependsOn list (static graph only in Phase 1). Every emitted edge carries a kind.")
  kind: ConnectionKind;
}
```

## Compatibility

- `Radius.Core/2025-08-01-preview` is a **preview** version; additive changes are permitted per Constitution IX ("Incremental Adoption & Backward Compatibility") because the surface has not yet been marked stable.
- `kind` is **required** (not `kind?:`). Making it optional would introduce a producer-less "unset" state (see [research.md](../research.md#r-003--wire-model-for-kind)).
- Consumers that already parse `Direction` and treat unknown enum members leniently need no change. Consumers that key strictly off the enum shape will need to regenerate their client (dashboard, third-party SDKs).

## Regenerated artifacts

Committed alongside the TypeSpec change, produced by `make generate` at the repository root:

- `swagger/specification/core/resource-manager/Radius.Core/preview/2025-08-01-preview/applications.json` — new `ConnectionKind` schema, new `kind` property on the `ApplicationGraphConnection` schema.
- `pkg/corerp/api/v20250801preview/zz_generated_models.go` — Go structs regenerated. New `ConnectionKind` type (string alias with two constants) and new `Kind *ConnectionKind` field on `ApplicationGraphConnection`.
- Bicep type extensions (if the extension consumes this model) — regenerated via the existing Bicep tooling in `bicep-tools/`.

CI enforces that all regenerated files match the current TypeSpec source. The plan includes a task to run `make generate` and commit the diff.

## Handler behavior after regeneration

**Static graph builder** — [`pkg/cli/graph/modeled.go`](../../../pkg/cli/graph/modeled.go): sets `Kind` on every edge. `Connection` for `properties.connections` entries, `Dependency` for `dependsOn` entries. Connection-wins de-dup keeps at most one edge per `(Source, Target)` pair.

**Runtime handler (Radius.Core preview)** — [`pkg/corerp/frontend/controller/applications/v20250801preview/getgraph.go`](../../../pkg/corerp/frontend/controller/applications/v20250801preview/getgraph.go): sets `Kind: Connection` on every emitted edge, unconditionally. Runtime `Dependency` extraction is Phase 2.

**Runtime handler (Applications.Core)** — untouched. Its wire model has no `kind` field.

## Non-goals in this contract

- No changes to `ApplicationGraphResource`, `ApplicationGraphOutputResource`, `Direction`, or `ApplicationGraphResponse`.
- No new query parameters, headers, or endpoints.
- No changes to `Applications.Core` (any version).
