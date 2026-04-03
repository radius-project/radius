# Implementation Plan 1: Migrate Go Dependencies to Direct Module Reference

**Branch**: `001-remove-bicep-types-submodule-go` | **Date**: 2026-01-22 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `/specs/001-remove-bicep-types-submodule/spec.md`

## Summary

Migrate Go code from using the local `bicep-types` submodule via `replace` directive to referencing `github.com/Azure/bicep-types/src/bicep-types-go` as a standard Go module dependency pinned to a specific commit SHA. This is the first phase of removing the bicep-types submodule, enabling the second phase (pnpm migration + submodule removal) to proceed safely.

## Technical Context

**Language/Version**: Go 1.25.5 (per go.mod)
**Primary Dependencies**: `github.com/Azure/bicep-types/src/bicep-types-go` (currently via replace directive to local submodule)
**Storage**: N/A
**Testing**: `go test`, `make test`
**Target Platform**: Linux (CI), macOS/Windows (developer machines)
**Project Type**: Monorepo with Go services, TypeScript tooling
**Performance Goals**: Build time should not regress significantly
**Constraints**: Must maintain reproducible builds via commit SHA pinning
**Scale/Scope**: ~20 Go files importing bicep-types-go packages

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Notes |
| ----------- | -------- | ------- |
| **I. API-First Design** | ✅ PASS | No API changes - dependency management only |
| **II. Idiomatic Code Standards** | ✅ PASS | Using standard Go modules is idiomatic |
| **III. Multi-Cloud Neutrality** | ✅ PASS | No cloud-specific changes |
| **IV. Testing Pyramid Discipline** | ✅ PASS | Existing tests validate bicep-types-go integration |
| **V. Collaboration-Centric Design** | ✅ PASS | Improves contributor experience |
| **VI. Open Source and Community-First** | ✅ PASS | Following standard open source dependency practices |
| **VII. Simplicity Over Cleverness** | ✅ PASS | Replacing complex submodule with standard Go modules |
| **VIII. Separation of Concerns** | ✅ PASS | Clean dependency boundary via module reference |
| **IX. Incremental Adoption** | ✅ PASS | First phase of two-phase migration; Go code works independently |
| **XVI. Repository-Specific Standards** | ✅ PASS | Following radius repo Go conventions |
| **XVII. Polyglot Project Coherence** | ✅ PASS | Consistent with Go ecosystem patterns |

**Gate Result**: ✅ All gates pass

## Requirements Addressed

| Requirement | Coverage |
| ------------- | ---------- |
| FR-004 | Go code references bicep-types-go through go.mod as standard module |
| FR-005 | Go module dependencies resolvable via `go mod` commands |
| FR-006 | Go module pinned to specific git commit SHA |
| FR-020 (partial) | Go-related documentation updated |

## Project Structure

### Documentation (this feature)

```text
specs/001-remove-bicep-types-submodule/
├── spec.md                      # Feature specification
├── plan-1-go-modules.md         # This file (Plan 1)
├── plan-2-pnpm-submodule.md     # Plan 2 (pnpm + submodule removal)
├── research-1-go-modules.md     # Phase 0 research for Plan 1
└── tasks-1-go-modules.md        # Phase 2 tasks for Plan 1
```

### Source Code Changes (radius repository)

```text
radius/
├── go.mod                       # MODIFY: Replace directive → direct dependency
├── go.sum                       # MODIFY: Add bicep-types-go checksums
├── CONTRIBUTING.md              # MODIFY: Update Go dependency instructions
└── docs/contributing/           # MODIFY: Update relevant contributor docs
```

## Technical Approach

### Current State

The `go.mod` file contains:

```go
replace github.com/Azure/bicep-types/src/bicep-types-go => ./bicep-types/src/bicep-types-go
```

This points to the local submodule directory. Go imports throughout the codebase use:

```go
import "github.com/Azure/bicep-types/src/bicep-types-go/types"
import "github.com/Azure/bicep-types/src/bicep-types-go/factory"
import "github.com/Azure/bicep-types/src/bicep-types-go/index"
import "github.com/Azure/bicep-types/src/bicep-types-go/writers"
```

### Target State

Replace the `replace` directive with a direct module dependency:

```go
require (
    github.com/Azure/bicep-types/src/bicep-types-go v0.0.0-20260115000000-abc123def456
)
```

The import paths remain unchanged since they already use the canonical module path.

### Migration Steps

1. **Identify target commit SHA** in Azure/bicep-types repository
2. **Update go.mod**: Remove `replace` directive, add direct `require`
3. **Run `go mod tidy`**: Resolve and verify dependencies
4. **Run `go build ./...`**: Verify compilation
5. **Run `make test`**: Verify all tests pass
6. **Update documentation**: Remove submodule references from Go setup docs

> **Note**: Dependabot is already configured for Go module updates in the repository, so no additional configuration is needed.

### Rollback Strategy

Standard git revert of the PR. The submodule is still present (removed in Plan 2), so reverting restores the `replace` directive and everything works as before.

## Research Required (Phase 0)

1. **Commit SHA Selection**: Identify the current bicep-types commit the submodule points to, verify it's available as a Go module
2. **Go Module Availability**: Verify `github.com/Azure/bicep-types/src/bicep-types-go` can be fetched as a Go module from the Azure/bicep-types repository
3. **API Compatibility**: Confirm no breaking changes between submodule version and direct module version

## Design Artifacts (Phase 1)

### data-model.md

Not applicable - this is a dependency management change with no data model changes.

### contracts/

Not applicable - no API changes.

### quickstart.md

Developer quickstart for verifying Go module migration:

```bash
# Clone repository (no --recurse-submodules needed for Go after this change)
git clone https://github.com/radius-project/radius
cd radius

# Verify Go dependencies resolve
go mod download

# Build
make build

# Run tests
make test
```

## Dependencies

- **Upstream**: Azure/bicep-types repository must have the target commit available
- **Downstream**: Plan 2 (pnpm + submodule removal) depends on this plan completing successfully

## Success Criteria

| Criterion | Validation |
| ----------- | ------------ |
| SC-001 (partial) | Go build completes without submodule initialization |
| SC-003 (partial) | Go-related build/test workflows have no git submodule commands |
| SC-005 | All existing Go tests pass without modification |

## Risks and Mitigations

| Risk | Impact | Mitigation |
| ------ | -------- | ------------ |
| bicep-types-go not available as standalone module | HIGH | Research in Phase 0; may need to work with Azure/bicep-types maintainers |
| Version mismatch causing build failures | MEDIUM | Pin to exact commit SHA matching current submodule |

## Complexity Tracking

> No complexity violations identified. This plan simplifies the dependency graph.

---

## Phase Summary

| Phase | Output | Status |
| ------- | -------- | -------- |
| Phase 0 | research-1-go-modules.md | NOT STARTED |
| Phase 1 | quickstart.md (above) | INCLUDED |
| Phase 2 | tasks-1-go-modules.md | NOT STARTED (via /speckit.tasks) |
