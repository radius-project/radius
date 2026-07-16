# Implicit Resource-ID Dependencies in the Application Graph

Status: Design exploration — no implementation planned yet
Author: (fill in)
Date: 2026-07-16

## Problem

Both graphs derive edges from **one authored source** for `Radius.Core/applications`: `properties.connections[*].source`. (Applications.Core gateways add `properties.routes[*].destination`; out of scope here.) The wire type ([typespec/Radius.Core/applications.tsp](../../../typespec/Radius.Core/applications.tsp)) has no notion of edge kind — every edge is uniformly a "Radius connection."

Authors regularly create real dependencies **without** a `connections` block by inlining another resource's `.id`:

```bicep
resource frontend 'Radius.Compute/containers@2025-08-01-preview' = {
  properties: {
    container: { env: { DB_HOST: { value: db.id } } }   // implicit reference — no connections block
  }
}
```

That edge does not surface in the graph today. The goal is to surface it, distinguished from author-declared connections (proposed: dotted line in renderers).

## Where the signal lives

**Static graph.** Every `.id` reference — direct, symbolic, `resourceId(...)`, or nested inside `format(...)` — causes Bicep to emit the resource ID into `dependsOn` in the compiled ARM JSON. Already parsed by [`resolveDependsOn`](../../../pkg/cli/graph/modeled.go) for `DiffHash`. **Zero false positives.** Free.

**Runtime graph.** Bicep's `dependsOn` is build-time only; it does not survive into stored properties. The only signal is stored resource-ID strings inside `properties`. This requires either scanning strings and matching against the resource list (Option B) or consulting per-type schema for reference-typed fields (Option C).

## Wire change (shared across all options)

```typespec
enum ConnectionKind {
  Connection,   // author-declared via properties.connections
  Dependency,   // implicit ID reference
}

model ApplicationGraphConnection {
  id: string;
  direction: Direction;   // existing
  kind: ConnectionKind;   // new
}
```

Existing edges become `Kind: Connection`; new dependency edges are `Kind: Dependency`. The `Applications.Core` handler continues to emit `Kind: Connection` unchanged.

## Algorithm

Common rules across both graphs:

- **Resource-list membership**: every candidate target ID must resolve to a node in the graph's resource list; unresolvable IDs are dropped.
- **De-dup**: if the same target ID appears as both a `Connection` and a `Dependency`, keep the `Connection` (stronger signal wins).
- **Reciprocal edges**: for every outbound edge emitted, mirror an inbound edge on the destination with the same `kind`.

### Static graph — from Bicep `dependsOn`

Runs in [pkg/cli/graph/modeled.go](../../../pkg/cli/graph/modeled.go):

```text
for each resource R in template:
    connIDs      := resolve(R.properties.connections)   // Connection edges
    dependsOnIDs := resolve(R.dependsOn)                // candidate Dependency edges

    for each c in connIDs:
        emit outbound edge {R -> c, kind=Connection}

    for each d in dependsOnIDs:
        if d in connIDs: continue                       // dedup: already a Connection
        if d not in resource list: continue
        emit outbound edge {R -> d, kind=Dependency}

mirror all outbound edges as inbound on the target with the same kind
```

No new IO. No schema consultation. Zero false positives.

### Runtime graph — from stored `properties`

Runs in [pkg/corerp/frontend/controller/applications/graph_util.go](../../../pkg/corerp/frontend/controller/applications/graph_util.go), on Option B or C:

```text
resourceIDs := set of R.ID for every R in resource list

for each resource R in resource list:
    connIDs := resolve(R.properties.connections)         // Connection edges (existing)

    for each c in connIDs:
        emit outbound edge {R -> c, kind=Connection}

    authored := R.properties with the exclusion list applied (see below)

    for each string s reachable from authored (recursively into maps, arrays):
        (Option C only) if s is not stored under a schema-declared reference field: continue
        candidateID := resources.Parse(s)
        if candidateID is invalid: continue
        if candidateID not in resourceIDs: continue      // must resolve to a graph node
        if candidateID in connIDs: continue              // dedup
        emit outbound edge {R -> candidateID, kind=Dependency}

mirror all outbound edges as inbound on the target with the same kind
```

**Exclusion list applied to `R.properties` before scanning** (author-injected fields only):

| Field                          | Reason for exclusion                                             |
| ------------------------------ | ---------------------------------------------------------------- |
| `application`, `environment`   | Containment scope, not dependency                                |
| `connections`, `routes`        | Already surfaced as first-class edges — do not double-count      |
| `provisioningState`, `status`  | Server-computed, not author input                                |
| Any output-resource back-ref   | Runtime-populated, and points outside the Radius node set anyway |
| Any property marked `readOnly` | By definition not author input                                   |

Options B and C differ only in the marked line: Option B scans every author-injected string; Option C restricts to schema-declared reference fields.

## Options

| Option | Static graph    | Runtime graph                            | Cost             | False-positive risk |
| ------ | --------------- | ---------------------------------------- | ---------------- | ------------------- |
| **A**  | dependsOn edges | none (`Connection`-only, no change)      | small            | none                |
| **B**  | dependsOn edges | scan authored properties, exclusion list | A + medium       | narrow, enumerable  |
| **C**  | dependsOn edges | scan schema-declared reference fields    | large (schema)   | none                |

**Recommendation: Option A now; A + (B or C) as a stacked follow-up.** Static graph is the highest-value visualization surface (runs in CI before deploy), and `dependsOn` gives us the signal for free. The `kind` wire field is designed once and accepts B or C later without further schema churn.

## Narrow false-positive scenarios (Option B)

Kept for design awareness; most are addressed by the exclusion list or the resource-list membership check:

| Scenario                              | Concrete example                                                                                       | Handling                                                             |
| ------------------------------------- | ------------------------------------------------------------------------------------------------------ | -------------------------------------------------------------------- |
| Structural / containment IDs          | `properties.application`, `properties.environment` on every resource                                   | **Addressed** — in the exclusion list                                |
| Historical / audit references         | `MyCompany.Observability/incidentReport.affectedResources: [ids...]` — record of past interactions     | Corner case — the property is typically `readOnly` and thus excluded |
| Cross-application namespace collision | Two apps in the same resource group, same-named resource, one stores the other's ID                    | Does not occur — Radius uses resource groups as RBAC boundaries      |
| Output-resource back-reference        | `properties.kubernetesServiceRef: "kubernetes:core/Service/frontend"`                                  | **Addressed** — parses/list-membership rules out non-Radius nodes    |

## Non-goals

- Not deprecating `properties.connections` — author-declared connections carry semantics (`iam`, `disableDefaultEnvVars`, etc.) that implicit dependencies do not.
- No `why` description per edge. `kind` is a discriminator, not documentation.
- No transitive-dependency reasoning at build time. All edges are direct.

## Follow-up work (after Option A lands)

1. Runtime-graph symmetry (Option B or C) — new issue.
2. Downstream renderer polish (dashboards, IDE extensions, docs generators).
3. Update [docs/architecture/application-graph.md](../../../docs/architecture/application-graph.md) with an explanation of `kind`.

## Files that would change under Option A

- [typespec/Radius.Core/applications.tsp](../../../typespec/Radius.Core/applications.tsp) — add `kind` field + `ConnectionKind` enum.
- Generated Go models under [pkg/corerp/api/v20250801preview/](../../../pkg/corerp/api/v20250801preview/).
- [pkg/cli/graph/modeled.go](../../../pkg/cli/graph/modeled.go) + `modeled_test.go` — new dependency-edge extraction, de-dup, reciprocal mirroring.
- [pkg/corerp/frontend/controller/applications/graph_util.go](../../../pkg/corerp/frontend/controller/applications/graph_util.go) — set `Kind: Connection` on emitted edges (tag-only). Shared with Applications.Core; safe there.
- One CLI renderer file — new glyph + legend.

No changes to server deployment code, `dynamicrp`, or API-generation tooling. Applications.Core and older API versions continue producing `Connection`-only edges.
