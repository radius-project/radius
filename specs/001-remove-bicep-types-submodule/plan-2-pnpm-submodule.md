# Implementation Plan 2: Migrate to pnpm + Remove bicep-types Submodule

**Branch**: `001-remove-bicep-types-submodule-pnpm` | **Date**: 2026-01-22 (Updated: 2026-01-31) | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `/specs/001-remove-bicep-types-submodule/spec.md`
**Depends On**: [Plan 1: Go Modules Migration](./plan-1-go-modules.md)
**Prototype**: [brooke-hamilton/radius:pnpm-direct-ref](https://github.com/brooke-hamilton/radius/tree/pnpm-direct-ref)

## Summary

Migrate JavaScript/TypeScript tooling in `hack/bicep-types-radius/` from npm to pnpm, update bicep-types npm dependencies to use pnpm git references with postinstall build scripts, remove the bicep-types git submodule, update all CI/CD workflows, and update documentation. This is the second and final phase of the submodule removal migration.

**Key Insight from Prototype:** pnpm's subdirectory reference syntax (`#path:/`) does NOT work for TypeScript packages that require compilation. The solution is to reference the full repository in package.json (pnpm fetches it automatically), build the TypeScript package via a `postinstall` script, and create a symlink for module resolution.

## Technical Context

**Language/Version**: Node.js (per .node-version), TypeScript
**Primary Dependencies**:

- `bicep-types` npm package (currently via `file:` reference to submodule)
- Various npm packages in `typespec/`, `hack/bicep-types-radius/`
**Package Manager**: npm → pnpm migration (for `hack/bicep-types-radius/` and `typespec/`)
**Storage**: N/A
**Testing**: npm/pnpm scripts, `make test`
**Target Platform**: Linux (CI), macOS/Windows (developer machines)
**Project Type**: Monorepo with TypeScript tooling, Go services
**Performance Goals**: Build time should not regress; pnpm typically improves it
**Constraints**: Must maintain reproducible builds via lockfiles with commit SHA pinning; TypeScript packages require local build
**Scale/Scope**:
- 3 npm package directories requiring pnpm migration (`autorest.bicep/`, `generator/`, `typespec/`)
- 8 CI workflow files with 15 `submodules:` occurrences to remove
- Multiple Makefile targets using npm commands

**Critical Technical Constraint (discovered in prototype):**
The `bicep-types` package is TypeScript source code that must be compiled. The compiled `lib/` directory is `.gitignore`d in the upstream repository. Therefore:
1. pnpm's `#path:/` subdirectory syntax does NOT work
2. The full repository must be referenced as a git dependency (pnpm fetches it)
3. A `postinstall` script must run `npm install && npm run build` inside the package
4. A symlink must be created so `import from "bicep-types"` resolves correctly

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Notes |
| --------- | ------ | ----- |
| **I. API-First Design** | ✅ PASS | No API changes - tooling/build system only |
| **II. Idiomatic Code Standards** | ✅ PASS | pnpm is modern, widely adopted package manager |
| **III. Multi-Cloud Neutrality** | ✅ PASS | No cloud-specific changes |
| **IV. Testing Pyramid Discipline** | ✅ PASS | Existing tests validate tooling; CI validates migration |
| **V. Collaboration-Centric Design** | ✅ PASS | Significantly improves contributor experience |
| **VI. Open Source and Community-First** | ✅ PASS | pnpm is open source, standard tooling |
| **VII. Simplicity Over Cleverness** | ✅ PASS | Replacing submodules with standard dependency management |
| **VIII. Separation of Concerns** | ✅ PASS | Clean dependency boundaries via pnpm |
| **IX. Incremental Adoption** | ✅ PASS | Migration includes contributor guide for existing clones |
| **XVI. Repository-Specific Standards** | ⚠️ CHECK | Dev container needs pnpm; devcontainer.json update required |
| **XVII. Polyglot Project Coherence** | ✅ PASS | Consistent with Node.js ecosystem patterns |

**Gate Result**: ✅ All gates pass (XVI addressed in implementation)

## Requirements Addressed

| Requirement | Coverage |
| ----------- | -------- |
| FR-001 | bicep-types git submodule completely removed |
| FR-002 | .gitmodules configuration removed |
| FR-003 | No `git submodule` commands required for build/test |
| FR-007 | All JS/TS tooling migrated to pnpm |
| FR-008 | hack/bicep-types-radius/ uses pnpm git references |
| FR-009 | Lockfiles updated for pnpm git subdirectory references |
| FR-010 | pnpm dependencies pinned to specific commit SHA |
| FR-011 | Makefiles function without submodule commands |
| FR-012 | Build scripts use pnpm |
| FR-013 | Workflow files have no submodule steps |
| FR-014 | All CI/CD workflows pass without submodule operations |
| FR-015 | Codegen workflows complete with new dependency sources |
| FR-016 | All regression tests pass |
| FR-017 | Dependabot configured for pnpm |
| FR-018 | Security scanning covers bicep-types dependencies |
| FR-019 | Dev container includes pnpm |
| FR-020 | CONTRIBUTING.md updated |
| FR-021 | One-time migration guide for existing clones |
| FR-022 | Go and pnpm setup steps documented |
| FR-023 | All READMEs updated |

## Project Structure

### Documentation (this feature)

```text
specs/001-remove-bicep-types-submodule/
├── spec.md                      # Feature specification
├── plan-1-go-modules.md         # Plan 1 (Go modules)
├── plan-2-pnpm-submodule.md     # This file (Plan 2)
├── research-2-pnpm.md           # Phase 0 research for Plan 2
├── data-model.md                # N/A for this feature
├── contracts/                   # N/A for this feature
├── quickstart.md                # Combined quickstart
└── tasks-2-pnpm-submodule.md    # Phase 2 tasks for Plan 2
```

### Source Code Changes (radius repository)

```text
radius/
├── .gitmodules                     # DELETE: Remove entirely
├── bicep-types/                    # DELETE: Remove submodule directory from git index
│
├── typespec/
│   ├── package.json                # NO CHANGE (no bicep-types dependency)
│   ├── package-lock.json           # DELETE
│   └── pnpm-lock.yaml              # CREATE: pnpm lockfile
│
├── hack/bicep-types-radius/
│   └── src/
│       ├── autorest.bicep/
│       │   ├── package.json        # MODIFY: Add pnpm config, postinstall, bicep-types-repo
│       │   ├── .npmrc              # CREATE: pnpm configuration
│       │   ├── package-lock.json   # DELETE
│       │   └── pnpm-lock.yaml      # CREATE: pnpm lockfile
│       └── generator/
│           ├── package.json        # MODIFY: Add pnpm config, postinstall, bicep-types-repo
│           ├── .npmrc              # CREATE: pnpm configuration
│           ├── package-lock.json   # DELETE
│           └── pnpm-lock.yaml      # CREATE: pnpm lockfile
│
├── build/
│   └── generate.mk                 # MODIFY: npm → pnpm, add pnpm-installed check, remove submodule commands
│
├── .github/
│   ├── dependabot.yml              # MODIFY: Remove gitsubmodule, add autorest.bicep and generator directories
│   └── workflows/
│       ├── build.yaml              # MODIFY: Remove submodules: recursive
│       ├── codeql.yml              # MODIFY: Remove submodules: recursive
│       ├── lint.yaml               # MODIFY: Remove submodules: recursive
│       ├── validate-bicep.yaml     # MODIFY: Remove submodules: true
│       ├── publish-docs.yaml       # MODIFY: Remove submodules: recursive
│       ├── long-running-azure.yaml # MODIFY: Remove submodules: recursive
│       ├── functional-test-noncloud.yaml  # MODIFY: Remove submodules: recursive
│       └── functional-test-cloud.yaml     # MODIFY: Remove submodules: recursive
│
├── .devcontainer/
│   ├── devcontainer.json           # MODIFY: Add pnpmVersion to node feature configuration
│   └── post-create.sh              # MODIFY: Change npm ci to pnpm install for typespec
│
├── CONTRIBUTING.md                 # MODIFY: Update setup instructions
└── docs/contributing/
    └── migration-guide.md          # CREATE: One-time migration guide for existing clones
```

## Technical Approach

### Current State

**Package References** (in `hack/bicep-types-radius/src/*/package.json`):

```json
{
  "devDependencies": {
    "bicep-types": "file:../../../../bicep-types/src/bicep-types"
  }
}
```

**Makefile** (`build/generate.mk`):

```makefile
generate-bicep-types:
	git submodule update --init --recursive; \
	npm --prefix bicep-types/src/bicep-types install; \
	npm --prefix bicep-types/src/bicep-types ci && npm --prefix bicep-types/src/bicep-types run build; \
	npm --prefix hack/bicep-types-radius/src/autorest.bicep ci && ...
```

**Workflows** (multiple files):

```yaml
- uses: actions/checkout@<sha>
  with:
    submodules: recursive
```

**Dependabot**:

```yaml
- package-ecosystem: gitsubmodule
  directory: /
  schedule:
    interval: weekly
```

### Target State (Validated in Prototype)

**Package References** (pnpm git reference with postinstall build):

```json
{
  "pnpm": {
    "onlyBuiltDependencies": ["autorest"]
  },
  "scripts": {
    "build": "tsc -p .",
    "test": "jest",
    "lint": "eslint src --ext ts",
    "lint:fix": "eslint src --ext ts --fix",
    "postinstall": "cd node_modules/bicep-types-repo/src/bicep-types && npm install && npm run build && cd ../../../.. && rm -rf node_modules/bicep-types && ln -sf bicep-types-repo/src/bicep-types node_modules/bicep-types"
  },
  "devDependencies": {
    "bicep-types-repo": "git+https://github.com/Azure/bicep-types.git#556bf5edad58e47ca57c6ddb1af155c3bcfdc5c7"
  }
}
```

**Key elements of package.json changes:**

| Element | Purpose |
| ------- | ------- |
| `bicep-types-repo` package name | pnpm fetches full repo to `node_modules/bicep-types-repo/`; name differs from symlink |
| `git+https://` URL format | Required by pnpm (not `github:` shorthand which defaults to SSH) |
| Commit SHA after `#` | Pins to specific version for reproducibility |
| `postinstall` script | Builds TypeScript and creates symlink for `bicep-types` imports |
| `pnpm.onlyBuiltDependencies` | Allows autorest lifecycle scripts (matches npm behavior) |

**.npmrc** (new file in each package directory):

```properties
# Allow pnpm to install packages that need to run postinstall scripts
side-effects-cache = false
```

**Makefile** (updated):

```makefile
.PHONY: generate-pnpm-installed
generate-pnpm-installed:
	@echo "$(ARROW) Detecting pnpm..."
	@which pnpm > /dev/null || { echo "pnpm is a required dependency. Run 'npm install -g pnpm' to install."; exit 1; }
	@echo "$(ARROW) OK"

.PHONY: generate-bicep-types
generate-bicep-types: generate-node-installed generate-pnpm-installed ## Generate Bicep extensibility types
	@echo "$(ARROW) Generating Bicep extensibility types from OpenAPI specs..."
	@echo "$(ARROW) Installing autorest.bicep dependencies (postinstall builds bicep-types)..."
	cd hack/bicep-types-radius/src/autorest.bicep && pnpm install
	@echo "$(ARROW) Building autorest.bicep..."
	pnpm --prefix hack/bicep-types-radius/src/autorest.bicep run build
	@echo "$(ARROW) Installing generator dependencies (postinstall builds bicep-types)..."
	cd hack/bicep-types-radius/src/generator && pnpm install
	@echo "$(ARROW) Running generator..."
	cd hack/bicep-types-radius/src/generator && pnpm run generate \
		--specs-dir ../../../../swagger --release-version ${VERSION} --verbose
```

**Workflows**:

```yaml
- uses: actions/checkout@<sha>
  # No submodules property needed
```

**Dependabot** (remove gitsubmodule, add package directories):

```yaml
# ADD: New directories for pnpm packages
- package-ecosystem: npm
  directory: /hack/bicep-types-radius/src/autorest.bicep
  schedule:
    interval: weekly
  groups:
    autorest-bicep:
      patterns:
        - "*"

- package-ecosystem: npm
  directory: /hack/bicep-types-radius/src/generator
  schedule:
    interval: weekly
  groups:
    bicep-generator:
      patterns:
        - "*"

# KEEP: Already exists in current config
- package-ecosystem: npm
  directory: /typespec
  ...

# REMOVE: No longer needed
# - package-ecosystem: gitsubmodule
```

### Build Flow (After Migration)

```
make generate-bicep-types
    │
    ├─▶ pnpm install in autorest.bicep
    │   ├─▶ Fetches Azure/bicep-types to node_modules/bicep-types-repo/
    │   └─▶ postinstall: npm install → npm run build → creates symlink
    │
    ├─▶ pnpm run build in autorest.bicep
    │
    ├─▶ pnpm install in generator
    │   ├─▶ Fetches Azure/bicep-types to node_modules/bicep-types-repo/
    │   └─▶ postinstall: npm install → npm run build → creates symlink
    │
    └─▶ pnpm run generate
```

### Migration Steps

1. **Install pnpm**: Update dev container, document installation for contributors
2. **Update autorest.bicep/package.json**: Add pnpm config, postinstall script, bicep-types-repo reference
3. **Create autorest.bicep/.npmrc**: Add `side-effects-cache = false`
4. **Update generator/package.json**: Add pnpm config, postinstall script, bicep-types-repo reference
5. **Create generator/.npmrc**: Add `side-effects-cache = false`
6. **Generate pnpm lockfiles**: Run `pnpm install` in both directories
7. **Delete npm lockfiles**: Remove `package-lock.json` from both directories
8. **Update Makefile**: Add `generate-pnpm-installed` target, update `generate-bicep-types` to use pnpm
9. **Update CI workflows**: Remove `submodules: recursive/true` from checkout steps
10. **Remove submodule**: `git submodule deinit -f bicep-types`, `git rm bicep-types`, `rm .gitmodules`
11. **Update Dependabot**: Remove `gitsubmodule`, add autorest.bicep and generator directories
12. **Update dev container**: Add `pnpmVersion` to node feature in devcontainer.json
13. **Update documentation**: CONTRIBUTING.md, create migration guide
14. **Verify**: Full CI pipeline passes, `make generate-bicep-types` succeeds

### Submodule Removal Commands

```bash
# 1. Deinitialize the submodule
git submodule deinit -f bicep-types

# 2. Remove from .git/modules
rm -rf .git/modules/bicep-types

# 3. Remove the submodule entry and directory
git rm -f bicep-types

# 4. Remove .gitmodules file (only submodule)
git rm .gitmodules
```

### Rollback Strategy

Standard git revert of the PR. Since this is atomic, reverting:

- Restores `.gitmodules` and submodule reference
- Restores npm lockfiles and commands
- Restores workflow submodule settings

Contributors would need to re-initialize submodule after revert:

```bash
git submodule add https://github.com/Azure/bicep-types.git bicep-types
```

### Why pnpm Subdirectory References Don't Work (Prototype Learning)

The original plan proposed using pnpm's subdirectory reference syntax:

```json
// ❌ This does NOT work for TypeScript packages
"bicep-types": "github:Azure/bicep-types#<sha>&path:/src/bicep-types"
```

**Why it fails:**

1. **TypeScript Compilation Required**: The `bicep-types` package is TypeScript source code. The compiled `lib/` directory is `.gitignore`d in the upstream repository.

2. **pnpm Doesn't Build**: When pnpm installs a git package with `#path:/`, it extracts the subdirectory but does NOT run `npm install` or `npm run build`. The TypeScript remains uncompiled.

3. **No `lib/` Directory**: Without compilation, `import from "bicep-types"` fails because the package's `main` field points to `lib/index.js` which doesn't exist.

**The Solution (Validated in Prototype):**

1. Reference the **full repository** in package.json as `bicep-types-repo` (pnpm fetches it automatically)
2. Use a `postinstall` script to:
   - `cd` into the package directory
   - Run `npm install && npm run build`
   - Create a symlink for `bicep-types` → the built package
3. Add `.npmrc` with `side-effects-cache = false` to ensure postinstall runs

## Research Required (Phase 0)

> **Status**: ✅ Complete and Validated via Prototype - See [research-2-pnpm.md](./research-2-pnpm.md) for findings

1. **pnpm Git Reference Syntax**: ✅ Use `git+https://github.com/Azure/bicep-types.git#<sha>` (NOT `github:...#path:/`)
2. **TypeScript Build Strategy**: ✅ `postinstall` script runs `npm install && npm run build` inside `node_modules/bicep-types-repo/src/bicep-types`
3. **Symlink Strategy**: ✅ Create symlink `node_modules/bicep-types` → `bicep-types-repo/src/bicep-types`
4. **pnpm Configuration**: ✅ `.npmrc` with `side-effects-cache = false`; `pnpm.onlyBuiltDependencies: ["autorest"]` in package.json
5. **pnpm + Dependabot**: ✅ Use `package-ecosystem: npm`; git deps require manual updates
6. **Workflow Caching**: ✅ Use `pnpm/action-setup@v4` with `actions/setup-node@v4` cache
7. **Dev Container pnpm**: ✅ Use `pnpmVersion` option in node devcontainer feature (built-in support, no extra scripts)

## Design Artifacts (Phase 1)

### data-model.md

Not applicable - this is a build/tooling change with no data model changes.

### contracts/

Not applicable - no API changes.

### quickstart.md

Developer quickstart after migration (combined for both plans):

```bash
# Clone repository - no submodules needed!
git clone https://github.com/radius-project/radius
cd radius

# Install pnpm
npm install -g pnpm@10

# Install TypeScript dependencies (postinstall scripts build bicep-types)
pnpm --prefix hack/bicep-types-radius/src/autorest.bicep install
pnpm --prefix hack/bicep-types-radius/src/generator install

# Optional: Install typespec dependencies (if needed)
npm --prefix typespec install

# Verify Go dependencies
go mod download

# Build everything
make build

# Run tests
make test

# Generate Bicep types (if needed)
make generate-bicep-types
```

### Migration Guide for Existing Contributors

```bash
# If you have an existing clone with the submodule
cd radius

# Remove the submodule artifacts
rm -rf bicep-types
rm -rf .git/modules/bicep-types

# Remove stale npm artifacts
rm -rf hack/bicep-types-radius/src/*/node_modules

# Fetch latest changes
git fetch origin
git checkout main
git pull

# Install pnpm
npm install -g pnpm

# Install dependencies (postinstall scripts build bicep-types)
cd hack/bicep-types-radius/src/autorest.bicep && pnpm install && cd ../../../..
cd hack/bicep-types-radius/src/generator && pnpm install && cd ../../../..

# Verify build works
make generate-bicep-types
```

### Version Update Guide

To update to a new version of bicep-types:

```bash
# 1. Get the new commit SHA from Azure/bicep-types
git ls-remote https://github.com/Azure/bicep-types HEAD

# 2. Update both package.json files with the new SHA
# Edit: hack/bicep-types-radius/src/autorest.bicep/package.json
# Edit: hack/bicep-types-radius/src/generator/package.json
# Change: "bicep-types-repo": "git+https://github.com/Azure/bicep-types.git#<NEW_SHA>"

# 3. Update lockfiles
cd hack/bicep-types-radius/src/autorest.bicep && pnpm install && cd ../../../..
cd hack/bicep-types-radius/src/generator && pnpm install && cd ../../../..

# 4. Test
make generate-bicep-types
```

## Dependencies

- **Upstream**: Plan 1 (Go modules) must be merged first
- **Upstream**: Azure/bicep-types repository must be accessible (public GitHub repo)
- **Prototype**: Validated on branch `brooke-hamilton/radius:pnpm-direct-ref`

## Success Criteria

| Criterion | Validation |
| --------- | ---------- |
| SC-001 | New contributors complete setup in <10 minutes |
| SC-002 | Zero submodule-related build failures |
| SC-003 | 100% of workflows have no git submodule commands |
| SC-004 | Dependabot creates PRs for npm registry packages in pnpm directories |
| SC-005 | All regression tests pass |
| SC-006 | Git worktrees work without conflicts |
| SC-007 | Documentation has no submodule references |
| SC-008 | `make generate-bicep-types` succeeds with pnpm |

## Risks and Mitigations

| Risk | Impact | Mitigation |
| ---- | ------ | ---------- |
| ~~pnpm git subdirectory references not stable~~ | N/A | ✅ Resolved: Using git+https:// reference + postinstall build instead |
| TypeScript build failure in postinstall | MEDIUM | Validated in prototype; postinstall is straightforward |
| Contributor confusion during transition | MEDIUM | Clear migration guide, announcement in release notes |
| CI cache invalidation causing slow builds | LOW | Configure pnpm caching properly in workflows |
| Dependabot not supporting pnpm git refs | LOW | Document manual update process; git deps require manual SHA updates |
| Duplicate repo fetches in node_modules | LOW | Both packages fetch same repo independently; acceptable tradeoff for simplicity |
| postinstall script portability | LOW | Script uses POSIX commands; works on Linux/macOS; Windows may need WSL or Git Bash |

## Known Limitations

| Limitation | Description | Workaround |
| ---------- | ----------- | ---------- |
| No Dependabot for git deps | `bicep-types-repo` commit SHA must be updated manually | Consider scheduled GitHub Action to check for updates |
| Requires pnpm | New dependency for contributors | Document installation in CONTRIBUTING.md |
| Duplicate fetches | Both packages fetch the same repo | Acceptable tradeoff; pnpm's content-addressable store mitigates |
| Postinstall complexity | Build logic in package.json scripts | Well-documented and validated in prototype |

## Workflow Files Requiring Updates

| File | Current Setting | Target Setting |
| ---- | --------------- | -------------- |
| `.github/workflows/build.yaml` | `submodules: recursive` (4 occurrences, lines 110, 212, 369, 436) | Remove property |
| `.github/workflows/codeql.yml` | `submodules: recursive` (line 95) | Remove property |
| `.github/workflows/lint.yaml` | `submodules: recursive` (line 58) | Remove property |
| `.github/workflows/validate-bicep.yaml` | `submodules: true` (line 64) | Remove property |
| `.github/workflows/publish-docs.yaml` | `submodules: recursive` (line 52) | Remove property |
| `.github/workflows/long-running-azure.yaml` | `submodules: recursive` (line 136) | Remove property |
| `.github/workflows/functional-test-noncloud.yaml` | `submodules: recursive` (line 208) | Remove property |
| `.github/workflows/functional-test-cloud.yaml` | `submodules: recursive` (4 occurrences, lines 172, 328, 336, 626) | Remove property |

**Total**: 15 occurrences across 8 workflow files

## Complexity Tracking

> No complexity violations identified. This plan simplifies the build system.

## Files Changed (from Prototype)

| File | Change |
| ---- | ------ |
| `.gitmodules` | DELETE |
| `bicep-types/` | DELETE (submodule removed) |
| `build/generate.mk` | Uses pnpm, added pnpm-installed check |
| `.github/dependabot.yml` | Removed gitsubmodule, added npm entries |
| `hack/.../autorest.bicep/package.json` | git+https ref, postinstall, pnpm config |
| `hack/.../autorest.bicep/.npmrc` | NEW: side-effects-cache = false |
| `hack/.../autorest.bicep/package-lock.json` | DELETE |
| `hack/.../autorest.bicep/pnpm-lock.yaml` | NEW |
| `hack/.../generator/package.json` | git+https ref, postinstall, pnpm config |
| `hack/.../generator/.npmrc` | NEW: side-effects-cache = false |
| `hack/.../generator/package-lock.json` | DELETE |
| `hack/.../generator/pnpm-lock.yaml` | NEW |
| `.devcontainer/devcontainer.json` | Add pnpmVersion to node feature (not in prototype) |
| `CONTRIBUTING.md` | Update setup instructions (not in prototype) |
| `docs/contributing/migration-guide.md` | NEW (not in prototype) |

---

## Phase Summary

| Phase | Output | Status |
| ----- | ------ | ------ |
| Phase 0 | [research-2-pnpm.md](./research-2-pnpm.md) | ✅ COMPLETE (Validated via prototype) |
| Phase 1 | quickstart.md (above), migration-guide.md (above) | ✅ INCLUDED |
| Phase 2 | tasks-2-pnpm-submodule.md | NOT STARTED (via /speckit.tasks) |

## Prototype Reference

The approach in this plan has been validated via prototype:

- **Branch**: [brooke-hamilton/radius:pnpm-direct-ref](https://github.com/brooke-hamilton/radius/tree/pnpm-direct-ref)
- **Diff**: [radius-project/radius/compare/main...brooke-hamilton:radius:pnpm-direct-ref](https://github.com/radius-project/radius/compare/main...brooke-hamilton:radius:pnpm-direct-ref)
- **Commit**: `5f73e0361fd83bbd56504731c0072a8175906ee9`
- **bicep-types pinned to**: `556bf5edad58e47ca57c6ddb1af155c3bcfdc5c7`
