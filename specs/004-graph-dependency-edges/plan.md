# Implementation Plan: Application Graph Dependency Edges

**Branch**: `edges` (working name; will rename to `004-graph-dependency-edges` when moved to a feature branch) | **Date**: 2026-07-16 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `specs/004-graph-dependency-edges/spec.md`

**Note**: `.specify/scripts/bash/setup-plan.sh` requires a `NNN-feature-name` branch and refuses to run on `edges`. Artifacts in this directory are created manually against the feature directory recorded in `.specify/feature.json` (`specs/004-graph-dependency-edges`).

## Summary

Phase 1 of a two-phase change to the Radius application graph, scoped to the `Radius.Core/2025-08-01-preview` API surface.

- **Static graph builder** (`pkg/cli/graph/modeled.go`) gains a second edge source: Bicep's compiled `dependsOn` array. Edges are tagged `kind: Connection` (from `properties.connections`) or `kind: Dependency` (from `dependsOn`). Connection wins when both sources signal the same source-target pair.
- **Exclusion list**: `Radius.Core/applications`, `Radius.Core/environments`, `Radius.Core/recipePacks`, `Radius.Compute/containerImages`, `Applications.Core/applications`, `Applications.Core/environments`. Members are never graph nodes and never edge targets.
- **Wire model**: `ApplicationGraphConnection` on `Radius.Core/2025-08-01-preview` gains a `kind` field. `Applications.Core` TypeSpec, generated code, handlers, and tests are untouched.
- **Runtime handler** (Radius.Core preview): sets `kind: Connection` on every edge it emits. `Dependency` edges at runtime are Phase 2.
- **Shared primitives**: extraction, resolution, exclusion, mirroring, and Connection-wins de-dup live in a new neutral package `pkg/graph/edges/` so Phase 2's runtime dependency scan is a wiring change, not a re-implementation.

## Technical Context

**Language/Version**: Go 1.26.5 (per `go.mod`)
**Primary Dependencies**: standard library only for the new package (`sort`, `strings`, `strconv`). Consumers already depend on TypeSpec-generated preview models under `pkg/corerp/api/v20250801preview/`.
**Storage**: N/A (build-time and in-memory operations)
**Testing**: `go test` with `stretchr/testify` (existing repo convention); table-driven tests in every new/modified file. Regenerated code kept in sync via `make generate`.
**Target Platform**: Radius control plane (Linux server binary) and `rad` CLI (macOS/Linux/Windows).
**Project Type**: Single Go module `github.com/radius-project/radius`.
**Performance Goals**: Static graph must build the `rabbitmq-app` fixture in under 50 ms (informational; not a hard SLO). No new hot-path allocations on the runtime handler.
**Constraints**: No wire changes to `Applications.Core`. `kind` is additive on `Radius.Core/2025-08-01-preview`, which is still `-preview` so additive change is safe. No new external dependencies.
**Scale/Scope**: A typical Radius application graph is <100 resources; extraction is O(N × M) where N is resource count and M is average `dependsOn` length. No scaling concerns.

## Constitution Check

*GATE: Passed at plan authoring time. Re-verified after research and data-model artifacts.*

| Principle | Verdict | Note |
|-----------|---------|------|
| I. API-First Design | ✅ | Wire change authored in TypeSpec (`typespec/Radius.Core/applications.tsp`); Go models regenerated via `make generate`. |
| II. Idiomatic Code Standards | ✅ | New package `pkg/graph/edges/` follows `gofmt`, small exported surface, godoc on every exported symbol, table-driven tests. |
| III. Multi-Cloud Neutrality | N/A | Graph representation is cloud-agnostic; no cloud-specific logic added. |
| IV. Testing Pyramid Discipline | ✅ | Unit tests for the new package cover: connection extraction, `dependsOn` resolution, exclusion, mirroring, Connection-wins de-dup, symbolic + `resourceId()` forms. Integration coverage via existing `pkg/cli/graph/modeled_test.go` (extended) and preview handler tests. |
| V. Collaboration-Centric Design | ✅ | Developers see accurate dependency edges; platform engineers get a troubleshooting discriminator (`kind`) they can filter on. |
| VI. Open Source and Community-First | ✅ | Spec and plan authored in the public repo; commits will carry `Signed-off-by`. |
| VII. Simplicity Over Cleverness | ✅ | Reuses the existing `resolveDependsOn` helper; new package is a small pure-Go primitive layer with no reflection, no code-gen, no interfaces exceeding the callers' needs. |
| VIII. Separation of Concerns | ✅ | `pkg/graph/edges/` has no CLI or control-plane imports; callers convert their own inputs into its pure-Go shape. |
| IX. Incremental Adoption & Backward Compatibility | ✅ | `Applications.Core` is frozen. `Radius.Core/2025-08-01-preview` is not yet stable, so adding a required field on `ApplicationGraphConnection` is safe. |
| XII / XIII (resource type / recipe standards) | N/A | No resource types or recipes are affected. |
| XVII. Polyglot Project Coherence | ✅ | TypeSpec is the single source of truth for the wire; Go generated code follows. Dashboard consumers will pick up `kind` when they regenerate their preview model. |

**No violations. Complexity Tracking section is empty.**

## Project Structure

### Documentation (this feature)

```text
specs/004-graph-dependency-edges/
├── plan.md              # This file (/speckit.plan output)
├── spec.md              # /speckit.specify + /speckit.clarify output
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/           # Phase 1 output
│   └── wire-change.md
└── checklists/
    └── requirements.md  # /speckit.specify + /speckit.clarify output
```

### Source Code (repository root)

New package + edited files, all within the existing single Go module:

```text
# New — shared extraction primitives
pkg/graph/edges/
├── edges.go             # Resource, Edge, ExtractEdges, exclusion list type
├── edges_test.go        # Table-driven unit tests
└── doc.go               # Package-level godoc

# Edited — static graph builder consumes the new primitives
pkg/cli/graph/
├── modeled.go
├── modeled_test.go

# Edited — TypeSpec wire change (source of truth)
typespec/Radius.Core/
└── applications.tsp     # Add ConnectionKind enum + kind field on ApplicationGraphConnection

# Regenerated — Go models + OpenAPI (via `make generate`)
pkg/corerp/api/v20250801preview/
└── zz_generated_models.go   # DO-NOT-EDIT; regenerated

swagger/specification/core/resource-manager/Radius.Core/preview/2025-08-01-preview/
└── applications.json        # Regenerated from TypeSpec

# Edited — runtime handler for Radius.Core preview sets kind: Connection
pkg/corerp/frontend/controller/applications/v20250801preview/
└── getgraph.go              # response conversion sets kind on every edge
└── getgraph_test.go         # golden updates

# Untouched (explicit non-goal, enforced by FR-016)
pkg/corerp/frontend/controller/applications/    # Applications.Core internal graph builder
typespec/Applications.Core/                     # Applications.Core TypeSpec

# Edited — architecture docs
docs/architecture/application-graph.md
```

**Structure Decision**: Single Go module; add the new package under `pkg/graph/edges/`. This avoids two anti-patterns: (a) importing CLI code from the control plane if the primitives lived in `pkg/cli/graph/`, and (b) overloading the generic `pkg/algorithm/graph/` package (which is a domain-neutral dependency-graph algorithm library) with Radius-specific exclusion rules.

## Phase 0 — Research

See [research.md](research.md). Summary of resolved unknowns:

1. **`dependsOn` shape after `collectResources` normalization** — always canonical `[resourceId('TYPE','NAME')]` strings after normalization; symbolic-name entries from languageVersion 2.0 are rewritten upstream. Reuse `resolveDependsOn` verbatim.
2. **Existing `resolveDependsOn` signature and behavior** — returns canonical Radius resource IDs; already used by `DiffHash`. No change needed.
3. **Wire model shape** — `ApplicationGraphConnection` today has `id` (string) and `direction` (`enum Direction { Outbound, Inbound }`). Adding a required `kind: ConnectionKind` field with `Connection` and `Dependency` members mirrors the `Direction` enum exactly.
4. **How to regenerate Go from TypeSpec** — `make generate` at the repository root (see `CONTRIBUTING.md`, section "Generated code"). CI enforces that generated files are checked in.
5. **Where the Applications.Core / Radius.Core preview conversion happens** — the shared internal builder in `pkg/corerp/frontend/controller/applications/graph_util.go` returns an internal representation; each API version's handler (Applications.Core in the parent directory, Radius.Core preview under `v20250801preview/`) converts it to its own wire model. Radius.Core preview conversion is where we set `kind: Connection`. The shared internal builder does not need to change in Phase 1.

## Phase 1 — Design Artifacts

### Data Model — [data-model.md](data-model.md)

Defines the three exported Go entities in `pkg/graph/edges/`:

- `Resource`: `{ ID string; Type string; Properties map[string]any; DependsOn []string }` — pure Go input, no ARM syntax, no HTTP.
- `Edge`: `{ Source string; Target string; Direction string; Kind string }` — pure Go output.
- `ExtractEdges(resources []Resource, excluded map[string]struct{}) []Edge` — the public entry point. Plain parameter for the exclusion set today (Constitution VII); promote to a struct if a second knob emerges.

### Contracts — [contracts/wire-change.md](contracts/wire-change.md)

TypeSpec diff on `Radius.Core/2025-08-01-preview`:

- Add `enum ConnectionKind { Connection, Dependency }`.
- Add `kind: ConnectionKind` to `ApplicationGraphConnection`.

No other API surface is touched. Regenerated Go models and OpenAPI are committed alongside the TypeSpec change.

### Quickstart — [quickstart.md](quickstart.md)

End-to-end verification recipe against the `rabbitmq-app` fixture: `bicep build` → `rad app graph -f rabbitmq-app.json` → assertions on node count, edge count, and `kind` per edge.

### Agent context

No agent-context update required for this feature (no new agent capabilities are introduced).

## Complexity Tracking

Not applicable — no principle violations.
