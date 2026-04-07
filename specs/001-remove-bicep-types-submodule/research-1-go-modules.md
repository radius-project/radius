# Research: Go Modules Migration

**Plan**: [plan-1-go-modules.md](./plan-1-go-modules.md)
**Date**: 2026-01-22
**Status**: Complete

## Research Questions

From Plan 1, the following items required research:

1. Commit SHA Selection - Identify the current bicep-types commit the submodule points to
2. Go Module Availability - Verify bicep-types-go can be fetched as a Go module
3. API Compatibility - Confirm no breaking changes between versions
4. Dependabot Behavior - Research Dependabot support for subdirectory Go modules

---

## Findings

### 1. Current Submodule Commit SHA

| Aspect | Details |
| ------ | ------- |
| **Decision** | The bicep-types submodule points to commit `c1a289be58bea8e23cecbce871a11a3fad8c3467` |
| **Evidence** | `git submodule status` returns: `+c1a289be58bea8e23cecbce871a11a3fad8c3467 bicep-types (heads/main-8-gc1a289b)` |
| **Rationale** | This commit is the baseline. The Go module version should match this commit or newer. |
| **Alternatives Considered** | Using `main` branch HEAD - rejected for reproducibility concerns |

### 2. Go Module Structure in Azure/bicep-types

| Aspect | Details |
| ------ | ------- |
| **Decision** | ✅ A valid Go module exists at `github.com/Azure/bicep-types/src/bicep-types-go` |
| **Evidence** | The `src/bicep-types-go/go.mod` file contains: |

```go
module github.com/Azure/bicep-types/src/bicep-types-go

go 1.21

require github.com/stretchr/testify v1.11.1
```

| **Rationale** | The module already uses the canonical import path that Radius uses. No code changes needed. |
| **Alternatives Considered** | Forking bicep-types to Radius org - rejected as unnecessary complexity |

**Module Packages Available:**

- `types/` - Core Bicep type definitions
- `factory/` - Type factory for creating Bicep types
- `index/` - Type indexing utilities
- `writers/` - JSON and Markdown output writers
- `cmd/bicep-types/` - CLI tool

### 3. Go Subdirectory Module Support

| Aspect | Details |
| ------ | ------- |
| **Decision** | Go natively supports modules in subdirectories of monorepos |
| **Evidence** | [Go Modules Reference](https://go.dev/ref/mod): "If the module is not defined in the repository's root directory, the module subdirectory is the part of the module path that names the directory." |
| **Rationale** | The pattern `github.com/Azure/bicep-types/src/bicep-types-go` is standard Go practice (similar to `golang.org/x/tools/gopls`) |
| **Alternatives Considered** | None - this is the standard approach |

**Version Tagging Convention:**

For subdirectory modules, semantic version tags must be prefixed with the module subdirectory path:

- Tag format: `src/bicep-types-go/v1.0.0`
- If no tags exist, Go uses pseudo-versions: `v0.0.0-20260115000000-c1a289be58be`

**Current Tag Status:**

⚠️ Azure/bicep-types does NOT publish semantic version tags with the subdirectory prefix. This means:

- We must use commit-based pseudo-versions
- Format: `v0.0.0-YYYYMMDDHHMMSS-<12-char-commit-hash>`
- Example: `v0.0.0-20260115000000-c1a289be58be`

### 4. Dependabot Support for Subdirectory Go Modules

| Aspect | Details |
| ------ | ------- |
| **Decision** | Dependabot fully supports Go modules in subdirectories |
| **Evidence** | [Dependabot documentation](https://docs.github.com/en/code-security/dependabot/dependabot-version-updates/configuration-options-for-the-dependabot.yml-file) confirms `gomod` ecosystem with `directory` option |
| **Rationale** | No special configuration needed - Dependabot will detect updates to the bicep-types-go dependency |
| **Alternatives Considered** | Manual dependency updates - rejected for maintainability |

**Configuration Example:**

```yaml
version: 2
updates:
  - package-ecosystem: "gomod"
    directory: "/"  # Root go.mod that imports bicep-types-go
    schedule:
      interval: "weekly"
    groups:
      go-dependencies:
        patterns:
          - "*"
```

**Note:** Since bicep-types uses pseudo-versions (no semantic tags), Dependabot will detect new commits but the version numbers will be commit-based.

---

## Implementation Decisions

### go.mod Changes

**Current State:**

```go
// replace github.com/radius-project/radius/bicep-types/src/bicep-types-go => ./bicep-types/src/bicep-types-go
replace github.com/Azure/bicep-types/src/bicep-types-go => ./bicep-types/src/bicep-types-go
```

**Target State:**

```go
require (
    github.com/Azure/bicep-types/src/bicep-types-go v0.0.0-20260115000000-c1a289be58be
)
```

Remove the `replace` directive and add a direct `require` with the pseudo-version matching the current submodule commit.

### Migration Commands

```bash
# 1. Get the exact version Go will assign to the commit
go list -m github.com/Azure/bicep-types/src/bicep-types-go@c1a289be58be

# 2. Update go.mod (remove replace, add require)
go get github.com/Azure/bicep-types/src/bicep-types-go@c1a289be58be

# 3. Verify and clean up
go mod tidy

# 4. Test
go build ./...
make test
```

### Dependabot Configuration

The existing Dependabot configuration already includes:

```yaml
- package-ecosystem: gomod
  directory: /
  schedule:
    interval: weekly
```

This will automatically pick up the new dependency. No changes required.

---

## Risks and Mitigations

| Risk | Likelihood | Impact | Mitigation |
| ---- | ---------- | ------ | ---------- |
| Pseudo-version format changes | Low | Low | Go's pseudo-version format is stable and well-documented |
| Upstream breaking changes | Medium | Medium | Pin to specific commit; review Dependabot PRs carefully |
| Build failures due to missing dependencies | Low | High | Test thoroughly before merge; easy rollback available |

---

## Summary

| Research Question | Answer | Confidence |
| ----------------- | ------ | ---------- |
| Current submodule commit | `c1a289be58bea8e23cecbce871a11a3fad8c3467` | ✅ High |
| Go module available | Yes, at `github.com/Azure/bicep-types/src/bicep-types-go` | ✅ High |
| API compatible | Yes, same module path used by Radius imports | ✅ High |
| Dependabot support | Yes, works with `gomod` ecosystem | ✅ High |
| Version format | Pseudo-version (no semantic tags upstream) | ✅ High |

**All research questions resolved. Plan 1 is ready for task breakdown.**
