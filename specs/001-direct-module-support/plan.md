# Implementation Plan: Direct Module Support

**Branch**: `001-direct-module-support` | **Date**: 2026-04-30 | **Spec**: `specs/001-direct-module-support/spec.md`
**Input**: Feature specification from `specs/001-direct-module-support/spec.md`

## Summary

Enable platform engineers to use any standard Bicep or Terraform module as a Radius recipe — without writing a Radius-specific wrapper. The system classifies `recipeLocation` at creation time (registry, Git, HTTP, S3, GCS, or OCI) using pattern-matching heuristics, resolves `{{context.*}}` template expressions (including single-level ternary and `context.resource.properties.*` paths) in `recipeParameters` at deployment time, passes parameters through to the module's native inputs, and maps module outputs to the resource type's read-only properties via the `outputs` field on RecipePack. A RecipePack is a collection of recipe configurations keyed by resource type. Existing wrapped recipes continue to function identically.

The prototype (3 commits on branch `001-direct-module-support`) already implements the Terraform-side core: source classification (`pkg/recipes/source/`), direct output mapping in the Terraform driver, config generation for all outputs, and best-effort reachability validation at RecipePack creation time.

## Technical Context

**Language/Version**: Go 1.22+ (per `go.mod`)
**Primary Dependencies**: `hashicorp/terraform-exec`, `hashicorp/terraform-config-inspect`, `hashicorp/go-getter`, ARM deployment client (Bicep), `github.com/radius-project/radius` monorepo
**Storage**: Kubernetes Secrets (Terraform state), ARM deployment tracking (Bicep state)
**Testing**: `go test` with `testify`, table-driven tests; functional tests in `test/functional-portable/`
**Target Platform**: Linux containers (Kubernetes), cross-platform CLI
**Project Type**: Cloud-native platform (API server + IaC drivers)
**Performance Goals**: Direct module deployment within 10% of equivalent wrapped-recipe deployment time (SC-002); inaccessible source fails within 60s (SC-005)
**Constraints**: Zero behavioral changes to existing wrapped recipes (FR-014); no new driver or execution engine (A-001); retry delegated to IaC engine (A-012)
**Scale/Scope**: 5 user stories (3×P1, 2×P2); ~8 packages modified; ~2000 LOC new + ~200 LOC modified

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

The project constitution (`/.specify/memory/constitution.md`) is a template placeholder — no project-specific principles have been ratified. Gate passes trivially: no constraints to evaluate.

**Post-Phase 1 re-check**: Design adheres to existing Radius conventions (Go idioms per `.github/instructions/golang.instructions.md`, existing driver architecture, existing test patterns). No violations detected.

## Project Structure

### Documentation (this feature)

```text
specs/001-direct-module-support/
├── plan.md              # This file
├── research.md          # Phase 0 output — design decisions R-001 through R-011
├── data-model.md        # Phase 1 output — extended RecipeDefinition, SourceType, ResolvedSource
├── quickstart.md        # Phase 1 output — usage examples (AVM, Git, registry)
├── contracts/
│   └── source-resolver.go  # Phase 1 output — Resolver interface contract
└── tasks.md             # Phase 2 output (created by /speckit.tasks)
```

### Source Code (repository root)

```text
pkg/
├── recipes/
│   ├── source/                         # NEW — Source classification & validation
│   │   ├── types.go                    #   SourceType enum, ResolvedSource, Resolver interface
│   │   ├── resolver.go                 #   Pattern-matching classifier, reachability probes
│   │   └── resolver_test.go            #   Table-driven classification + HTTP mock tests
│   ├── driver/
│   │   ├── terraform/
│   │   │   └── terraform.go            # MODIFIED — prepareRecipeResponse: direct output mapping
│   │   └── bicep/
│   │       └── bicep.go                # MODIFIED — direct Bicep module deployment path
│   ├── terraform/
│   │   ├── config/
│   │   │   └── config.go              # MODIFIED — AddAllOutputs(), AddDirectModuleContext()
│   │   ├── execute.go                 # MODIFIED — direct module context injection + output gen
│   │   └── module.go                  # MODIFIED — ModuleOutputs collection in inspectModule()
│   ├── paramresolver/                  # NEW — {{context.*}} template expression resolver
│   │   ├── resolver.go                 #   ResolveParameterExpressions(), buildContextLookup()
│   │   └── resolver_test.go            #   Expression resolution tests
│   └── types.go                       # MODIFIED — RecipeOutput.DirectModule flag
├── corerp/
│   ├── datamodel/
│   │   └── recipepack.go              # MODIFIED — RecipeDefinition: recipeParameters, outputs
│   └── frontend/controller/recipepacks/
│       └── createorupdaterecipepack.go # MODIFIED — source validation at creation time
typespec/
└── Radius.Core/
    └── recipePacks.tsp                # MODIFIED — outputs, recipeParameters in TypeSpec model

test/
└── functional-portable/
    └── recipes/                        # EXTENDED — direct module functional tests
```

**Structure Decision**: This feature extends the existing monorepo structure. All new code lives in `pkg/recipes/source/` (classification) and `pkg/recipes/paramresolver/` (expression resolution). Modifications are surgical additions to existing driver, config, and controller packages. No new top-level directories.

## Complexity Tracking

No constitution violations to justify — the design follows existing patterns throughout.

---

## Phase 0: Outline & Research

*Status: **Complete** — see `specs/001-direct-module-support/research.md`*

All technical unknowns have been resolved. Key decisions:

| ID | Decision | Rationale |
|----|----------|-----------|
| R-001 | Pattern-matching heuristics for source classification | Deterministic, no network calls, simple ordered rules |
| R-002 | Two-phase resolution: classify → direct or OCI fallback | Avoids latency for unambiguous sources |
| R-003 | Flat output mapping for direct modules (Values + Secrets) | Simplest approach; no Radius-specific conventions imposed |
| R-004 | Best-effort reachability validation at creation time | Catches typos early; 30s timeout; transient → warning |
| R-005 | Version embedded in `recipeLocation` (OCI tag, Git ref, registry syntax) | No separate `templateVersion` field needed (A-009); version is part of the source reference |
| R-006 | Runtime detection via `IsDirectModuleSource()` classifier | No new persisted fields needed |
| R-007 | `AddAllOutputs()` generates forwarding output blocks | Terraform requires declared root outputs |
| R-010 | Sensitive outputs → `Secrets` map (via TF `sensitive` flag) | Security without module modifications |
| R-011 | Explicit `{{context.*}}` expressions (not auto-injection) | Unambiguous; platform engineer controls mapping |

---

## Phase 1: Design & Contracts

*Status: **Complete** — see `specs/001-direct-module-support/data-model.md`, `contracts/`, `quickstart.md`*

### Data Model

See `specs/001-direct-module-support/data-model.md` for full details. Summary of entities:

#### RecipeDefinition (extended)

| Field | Type | Description |
|-------|------|-------------|
| `recipeKind` | `string` | `"terraform"` or `"bicep"` |
| `recipeLocation` | `string` | Module source (registry path, Git URL, OCI ref, etc.) |
| `recipeParameters` | `map[string]any` | Input parameters; values support `{{context.*}}` expressions |
| `plainHTTP` | `bool` | Allow insecure OCI connections |
| `outputs` | `map[string]string` | Maps resource type's read-only property names → module output names |

#### SourceType (new internal enum)

| Value | Pattern | Direct? |
|-------|---------|---------|
| `TerraformRegistry` | `ns/name/provider` (3 segments, no dots in ns) | Yes |
| `Git` | `git::` prefix | Yes |
| `HTTP` | `http://` or `https://` (no `git::`) | Yes |
| `S3` | `s3::` prefix | Yes |
| `GCS` | `gcs::` prefix | Yes |
| `OCI` | `oci://` or hostname-with-dot first segment | No |
| `Unknown` | Unclassifiable | No (fallback) |

#### RecipeOutput (extended)

| Field | Change |
|-------|--------|
| `DirectModule bool` | **New** — signals driver to skip `result` parsing and use flat output mapping |

#### State Transition

```
RecipePack Create → Source Classification → Reachability Probe (best-effort)
                                                ↓
Resource Deploy → Classify source → Download module → Inspect → Generate config
                                                ↓
                  Resolve {{context.*}} expressions → terraform init+apply (or ARM deploy)
                                                ↓
                  Map outputs (flat + Secrets for sensitive) → Apply outputs mapping to resource type's read-only properties → Resource ready
```

### Interface Contracts

See `specs/001-direct-module-support/contracts/source-resolver.go`. Key contract:

```go
type Resolver interface {
    Classify(templatePath string) ResolvedSource
    ValidateReachability(ctx context.Context, templatePath string) error
}
```

**Convenience function** (primary entry point for drivers):
```go
func IsDirectModuleSource(templatePath string) bool
```

### Quickstart

See `specs/001-direct-module-support/quickstart.md` for end-to-end usage examples covering:
1. AVM PostgreSQL module with t-shirt sizing (ternary expressions)
2. Git-hosted module with ref/subdirectory
3. HTTP archive module
4. Output mapping for resource property materialization

---

## Prototype Status

The branch `001-direct-module-support` (3 commits ahead of main) implements the following:

| Component | File(s) | Status |
|-----------|---------|--------|
| Source classifier | `pkg/recipes/source/types.go`, `resolver.go` | ✅ Implemented + tested |
| Reachability validation | `pkg/recipes/source/resolver.go` | ✅ Registry + HTTP probes |
| RecipePack controller | `pkg/corerp/frontend/controller/recipepacks/` | ✅ Classification at create time |
| TF driver output mapping | `pkg/recipes/driver/terraform/terraform.go` | ✅ Direct module flat mapping |
| TF config: AddAllOutputs | `pkg/recipes/terraform/config/config.go` | ✅ Forward all module outputs |
| TF config: AddDirectModuleContext | `pkg/recipes/terraform/config/config.go` | ✅ Well-known context variable injection |
| TF execute: direct module path | `pkg/recipes/terraform/execute.go` | ✅ Conditional context + output generation |
| TF module inspection | `pkg/recipes/terraform/module.go` | ✅ ModuleOutputs collection |

### Not yet implemented (remaining work):

| Component | Description | Priority |
|-----------|-------------|----------|
| `{{context.*}}` expression resolver | `ResolveParameterExpressions()` in param resolver package, including `context.resource.properties.*` paths | P1 |
| Single-level ternary evaluation | Single-level ternary in `{{context.*}}` expressions (nested ternaries out of scope for V1) | P1 |
| Bicep driver: direct module path | ARM deployment of standard Bicep modules without `result` output | P1 |
| Output mapping application | Apply `outputs` field to map module outputs to the resource type's read-only properties | P1 |
| Shallow merge for parameters | Environment-level parameter override with shallow merge semantics | P1 |
| Private module authentication | Credential passthrough for private registries/repos | P2 |
| Functional tests | End-to-end tests with real modules (registry, Git) | P1 |
| TypeSpec model update | `outputs` and `recipeParameters` in `recipePacks.tsp` | P1 |

---

## Implementation Approach (Phase 2 Preview)

The following outlines the logical work breakdown for task generation. Tasks should be ordered by dependency.

### Layer 1: Core Infrastructure (no driver dependencies)

1. **Parameter expression resolver** — `pkg/recipes/paramresolver/`: `ResolveParameterExpressions()`, `buildContextLookup()`, regex-based `{{context.*}}` replacement (including `context.resource.properties.*` paths for application-to-recipe property resolution), single-level ternary evaluation (`{{expr == "val" ? "trueResult" : "falseResult"}}`; nested ternaries out of scope for V1). Table-driven tests. This is the foundational expression engine for US1, US2, and US3 — all P1.
2. **Output mapping utility** — Apply `outputs` map to translate module output names to the resource type's read-only properties in `RecipeOutput.Values`/`Secrets`. Shared by both drivers.
3. **Shallow merge utility** — Merge RecipePack parameters with environment-level overrides (top-level key precedence).

### Layer 2: Terraform Driver Completion

4. **Wire expression resolver into TF execute path** — Replace `AddDirectModuleContext` (auto-injection) with `ResolveParameterExpressions()` per R-011 decision.
5. **Wire output mapping** — After `prepareRecipeResponse`, apply `outputs` mapping to map module outputs to resource type's read-only properties.
6. **Handle `result` vs `outputs` precedence** — Per FR-015: `outputs` mapping takes precedence when both are present.

### Layer 3: Bicep Driver

7. **Bicep direct module detection** — Classify `recipeLocation` for Bicep (`br:` OCI references that are standard modules vs. wrapped).
8. **Bicep direct deployment** — Pass resolved parameters as ARM deployment parameters; map ARM outputs to resource type's read-only properties via `outputs`.
9. **Bicep cleanup** — Ensure ARM deployment deletion for direct Bicep modules.

### Layer 4: API & Validation

10. **TypeSpec model update** — Add `outputs` and `recipeParameters` to RecipePack model (RecipePack is a collection keyed by resource type).
11. **Reachability validation enhancement** — Wire `ValidateReachability` into create/update controller for Bicep OCI references. Best-effort: definitive failures (404, auth denied) reject; transient failures log warnings but allow linking (per SC-009).
12. **Private module auth** — Credential passthrough from secret store to module fetch.

### Layer 5: Testing & Integration

13. **Unit tests** — Expression resolver (including ternary and `context.resource.properties.*`), output mapping, shallow merge, classifier edge cases.
14. **Functional tests** — End-to-end with real Terraform registry module, Git module, Bicep AVM module. Must cover `{{context.resource.properties.*}}` resolution and ternary expressions.
15. **Backward compatibility tests** — Verify wrapped recipes remain unchanged.

---

## Generated Artifacts

| Artifact | Path | Status |
|----------|------|--------|
| Plan | `specs/001-direct-module-support/plan.md` | ✅ This file |
| Research | `specs/001-direct-module-support/research.md` | ✅ Complete (R-001 through R-011) |
| Data Model | `specs/001-direct-module-support/data-model.md` | ✅ Complete |
| Contracts | `specs/001-direct-module-support/contracts/source-resolver.go` | ✅ Complete |
| Quickstart | `specs/001-direct-module-support/quickstart.md` | ✅ Complete |
| Tasks | `specs/001-direct-module-support/tasks.md` | ⏳ Pending (`/speckit.tasks`) |
