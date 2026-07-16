# Implicit Resource-ID Dependencies in the Application Graph

Status: Design exploration — no implementation planned yet
Author: (fill in)
Date: 2026-07-16

## Problem

Both the runtime and static application graphs derive edges from **one authored source** for `Radius.Core/applications`:

- `properties.connections[*].source` — the primary Radius connection mechanism.

(`Applications.Core/gateway` resources additionally contribute edges via `properties.routes[*].destination` — see [routesPathResolver in graph_util.go:772](../../../pkg/corerp/frontend/controller/applications/graph_util.go). This is `Applications.Core`-only; no equivalent exists under `Radius.Core`, so it does not apply to the design in this note.)

See [resolveConnections in graph_util.go:649](../../../pkg/corerp/frontend/controller/applications/graph_util.go). The wire type ([typespec/Radius.Core/applications.tsp:80](../../../typespec/Radius.Core/applications.tsp)) is:

```typespec
model ApplicationGraphConnection {
  id: string;
  direction: Direction;   // Outbound | Inbound
}
```

Authors frequently create real dependencies **without** using `properties.connections`, for example by embedding another resource's `.id` inline:

```bicep
resource frontend 'Radius.Compute/containers@2025-08-01-preview' = {
  properties: {
    container: {
      image: 'nginx'
      env: {
        DB_HOST: { value: db.id }        // implicit ID reference — real dependency, not a Radius connection
      }
    }
    // no `connections` block at all
  }
}
```

These implicit dependencies do not surface in the graph today. Every edge in the response is uniformly a "Radius connection." Two consequences:

- **Missing edges.** Users looking at `rad app graph app.bicep` see a disconnected node when the actual application has a hard dependency.
- **No way to distinguish** author-declared connections from implicit dependencies even if they were both surfaced, because `ApplicationGraphConnection` has no notion of *kind*.

The request is to visualize both, distinguishing them (proposed: dotted line for implicit dependencies) so operators can see the full dependency structure without conflating it with the semantically stronger Radius `connections` relationship.

## Where the signal lives

### Static graph (Bicep-side, offline)

Every implicit ID reference — `db.id`, `db.properties.id`, `resourceId('Type','name')`, `reference('sym').id` — causes Bicep to emit that resource ID into the entry's **`dependsOn`** array in the compiled ARM JSON. That's true whether the reference sits in a top-level property, deep inside a nested object, inside an array, or inside an ARM expression like `format(...)`.

We already parse `dependsOn` into a resolved-ID list in [`resolveDependsOn`](../../../pkg/cli/graph/modeled.go) for `DiffHash` input. **Emitting edges from it is nearly free** — the extraction is done, we just do not use the result for graph structure.

**Guarantees:**

- Zero false positives — Bicep only writes an ID into `dependsOn` when the compilation actually consumed it as a reference.
- Zero false negatives for anything Bicep resolves — includes symbolic-name codegen, `resourceId(...)` expressions, and property-value reads.

### Runtime graph (control-plane-side)

No equivalent free signal:

- The LIST endpoint returns each resource's `properties`. Bicep's `dependsOn` is build-time only; it does not survive into the persisted resource shape.
- Property values can carry other resource IDs as strings, but the graph builder has no reliable way today to tell "this string is a semantic dependency" from "this string happens to look like an ARM ID."

Any runtime symmetry must therefore either:

1. Recursively scan every string in `properties` and treat matches against known resource IDs as edges — cheap but permissive; a resource that stores another resource's ID as opaque config would flip into a false-positive edge.
2. Consult per-type schema to declare which fields are "reference-typed" — rigorous but requires an annotation on every existing type.

## Options

### Option A — Static graph only, `kind` field on the wire

Add a `kind` discriminator to `ApplicationGraphConnection` with two values:

- `Connection` — author-declared via `properties.connections` (existing behavior).
- `Dependency` — implicit ID reference. Static graph only for this milestone.

**TypeSpec** ([typespec/Radius.Core/applications.tsp](../../../typespec/Radius.Core/applications.tsp)):

```typespec
enum ConnectionKind {
  @doc("Author-declared Radius connection (properties.connections or properties.routes).")
  Connection,
  @doc("Implicit dependency inferred from a resource-ID reference used in the resource's authored properties. Static graph only.")
  Dependency,
}

model ApplicationGraphConnection {
  id: string;
  direction: Direction;
  kind: ConnectionKind;
}
```

**Static graph** ([pkg/cli/graph/modeled.go](../../../pkg/cli/graph/modeled.go)):

- `outboundConnections` continues to return `properties.connections`-derived edges, tagged `Kind: "Connection"`.
- New `outboundDependencies(properties, dependsOn, existing)` returns `Outbound` edges from `dependsOn` IDs not already in `existing`, tagged `Kind: "Dependency"`.
- De-duplication: an ID that appears in both `connections` and `dependsOn` surfaces once as `Connection` (stronger signal).
- `addInboundConnections` renamed to something kind-neutral and mirrors the reciprocal edge with the same `kind`.

**Runtime graph**: every emitted edge tagged `Kind: "Connection"` (backward-compatible; no behavior change; explicitly documented as "runtime has no dependency edges yet — tracked as follow-up").

**Renderer** ([pkg/cli/cmd/app/graph/](../../../pkg/cli/cmd/app/graph/)):

- Text output: distinct glyph for `Dependency` (e.g. `⋯>` vs `-->`), with a short legend at the top.
- JSON consumers: switch on `kind` to render dotted vs solid.

**Cost:** small.

- TypeSpec + generated model change (~5 lines).
- ~40 LOC in the static builder for the new dependency-edge path + de-dup.
- One text-renderer branch and a legend line.
- One design-note update (this file becomes the design doc, or gets superseded by an implementation doc).
- Existing tests need one field update (all edges gain `kind`); new tests for the dependency path.

**Wins:**

- Uses signal Bicep already emits.
- Zero false positives.
- Small, reviewable diff.
- Wire schema designed once to accept the runtime extension later (Options B/C don't need another schema change).

**Gaps:**

- Runtime graph has no dependency edges. Filed as a follow-up.

### Option B — Both graphs, implicit-ID scanning

Same as A plus a runtime-side scanner:

- Recursively walk every string leaf in each resource's `properties`.
- Parse candidates as Radius resource IDs (`resources.Parse`).
- Cross-reference against the resource list included in the current graph.
- Matches not already in the resource's `connections` are emitted as `Kind: "Dependency"`.

**Cost:** A + ~80 LOC of scanner and de-dup + a test suite covering false-positive edges (opaque-ID storage, near-matches, cross-namespace references).

**Wins:** Full symmetry between static and runtime graphs.

**Scope restriction — only author-injected properties.** The scanner runs only over properties an author actually writes in `app.bicep`. In practice this means excluding:

- `application` and `environment` (containment scope, not dependency).
- The `connections` block (already surfaced as first-class edges by the existing resolver — do not double-count).
- `provisioningState` and the entire `status` block (server-computed, not author input).
- Any output-resource back-references — those are runtime-populated, not authored, and address entities outside the Radius graph node set anyway.
- Any property flagged read-only in the resource-type schema (`readOnly: true` in the OpenAPI-shaped fields) — by definition not author input.

This is the same principle the existing `getResourceTypeSpecificProperties` uses on the graph today for a different purpose (drop-list `provisioningState`, `connections`, `status`): the "authored properties bag" is a well-defined subset of stored properties, and it is the correct input for a scanner whose goal is to find author-declared references.

**Residual risk after the exclusion.** With the exclusion in place, several categories become non-issues; a couple remain as scenarios worth naming even if they will not fire in normal use. Captured here because they inform the eventual scanner design:

1. **Structural / containment IDs — addressed.** `properties.application` and `properties.environment` are explicit exclusions. Without the exclusion, a 20-resource app would emit 40 spurious edges (every resource → app + every resource → env). With the exclusion, zero.

2. **Historical / audit references — corner case, retained for design awareness.** A hypothetical `MyCompany.Observability/incidentReport` resource whose properties include:

   ```json
   {
     "properties": {
       "resolvedAt": "2026-07-14T10:23:00Z",
       "affectedResources": [
         "/planes/radius/local/.../containers/frontend",
         "/planes/radius/local/.../containers/backend"
       ]
     }
   }
   ```

   would record which resources it was about — not which resources it currently depends on. Those resources may have been redeployed, replaced, or deleted since the report was written. Emitting current dotted edges to them would be actively misleading, especially when compared against a redeployed graph where the "dependency" appears and disappears based on unrelated resource churn. Radius does not ship such a type today; this scenario matters if the community authors one, at which point they can either flag the property `readOnly` (the report is populated at scan time, not authored) or the community can pick up Option C's per-property annotation.

3. **Cross-application namespace collisions — does not occur in practice.** Two apps in the same resource group with a resource of the same name would create ambiguity. Radius treats resource groups as RBAC boundaries and community practice is one application per resource group, so this configuration does not arise. Captured for design completeness only; a defensive scanner still filters strictly by resource-list membership rather than name.

4. **Output-resource back-references — addressed.** Some resource types record Kubernetes / AWS / Azure output-resource IDs in properties for observability (`kubernetesServiceRef`, etc.). Output resources are not Radius graph nodes; a defensive scanner filters candidates through `resources.Parse` and requires the target to appear in the resource list before emitting an edge, which structurally rules these out. Additionally, output-resource fields are runtime-populated (server-injected), so the "author-injected properties only" restriction excludes them at the input side.

**No way to author-suppress on a per-instance basis.** If a resource type legitimately stores an ID for a reason we do not want to see as an edge and none of the built-in exclusions apply, the only remediation is a code change to the exclusion list. Option C addresses this by making the schema itself declarative.

### Option C — `kind` field, but schema-driven

Extend the resource-type schema with a marker such as `x-radius-reference: true` on properties that carry a dependency-bearing resource ID. Both graphs consult it:

- Static graph: continues to use `dependsOn` (source of truth for Bicep).
- Runtime graph: scans only fields declared reference-bearing.

**Cost:** substantial.

- New schema annotation + validator support ([bicep-tools/pkg/manifest/manifest.go](../../../bicep-tools/pkg/manifest/manifest.go), [pkg/schema/validator.go](../../../pkg/schema/validator.go)).
- TypeSpec + generated model.
- Runtime scanner + de-dup.
- Author documentation.
- Migration: every existing resource type needs the annotation added over time.

**Wins:**

- Zero false positives on either graph.
- Explicit authoring model — a resource type's public schema declares its own dependency contract.

**Downsides:**

- Slowest option to ship.
- Requires eco-system-wide changes across resource-types-contrib.
- Adds a schema concept just for graph visualization.

## Recommendation

**Option A now; A + (B or C) as a stacked follow-up.**

Reasoning:

1. **Static graph is the highest-value visualization surface** — users run it in CI before deploy, precisely when they want to see what will happen. Missing dependencies at that stage cost the most.
2. **`dependsOn` is a perfect signal** — free, correct, already parsed. Not using it is leaving value on the floor.
3. **The `kind` field is designed to accept B or C later** — no schema churn needed if we come back to add runtime symmetry.
4. **Small enough to land in one review cycle** without competing with other in-flight graph work.

## Non-goals

- Deprecating `properties.connections` in favor of implicit dependencies. Author-declared connections carry semantics (`iam`, `disableDefaultEnvVars`, connection-specific auth) that implicit dependencies do not. Both mechanisms coexist.
- Adding a "why" description per edge. `kind` is a discriminator, not documentation. If we later want reason text, add a separate optional field.
- Reasoning about transitive dependencies at graph-build time. Every edge remains a direct-only reference.

## Follow-up work

Should Option A land:

1. **Runtime-graph symmetry.** New issue for either Option B (scanner) or Option C (schema annotation). Choice depends on how much churn the community would tolerate for a graph-visualization improvement.
2. **Renderer polish.** Dotted-line rendering in downstream consumers (dashboards, IDE extensions, docs generators).
3. **Documentation.** Update [docs/architecture/application-graph.md](../../../docs/architecture/application-graph.md) with an explanation of `kind` and when each fires.

## Files that would change under Option A

- [typespec/Radius.Core/applications.tsp](../../../typespec/Radius.Core/applications.tsp) — new `ConnectionKind` enum, add `kind` field to `ApplicationGraphConnection`.
- Generated Go models under [pkg/corerp/api/v20250801preview/](../../../pkg/corerp/api/v20250801preview/).
- [pkg/cli/graph/modeled.go](../../../pkg/cli/graph/modeled.go) — new dependency-edge extraction + de-dup.
- [pkg/cli/graph/modeled_test.go](../../../pkg/cli/graph/modeled_test.go) — tests for dependency edges, de-dup, no-double-emit.
- [pkg/corerp/frontend/controller/applications/graph_util.go](../../../pkg/corerp/frontend/controller/applications/graph_util.go) — set `Kind: "Connection"` on emitted edges (tag-only, no behavior change). Note: this file is shared with `Applications.Core`; the tag-only change is safe for the older namespace but the new `Dependency` kind is only produced by the `Radius.Core` static-graph path.
- One CLI renderer file — new glyph + legend.
- [docs/architecture/application-graph.md](../../../docs/architecture/application-graph.md) — new section explaining `kind`.

`Applications.Core/applications` and older API versions are out of scope for this design — they continue to produce `Connection`-only edges from their existing extraction paths.

No changes to server-side deployment code, `dynamicrp`, or API-generation tooling.
