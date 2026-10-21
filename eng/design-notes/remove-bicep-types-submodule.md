# Feature Specification: Remove bicep-types Submodule & Migrate to pnpm + Go Modules

**Feature Branch**: `001-remove-bicep-types-submodule`
**Created**: January 22, 2026
**Status**: Draft
**Source**: [GitHub Issue #10913](https://github.com/radius-project/radius/issues/10913)

## Overview

Migrate Radius repository from git submodule for bicep-types to direct dependency references using Go modules and pnpm. This eliminates submodule usage, improves contributor experience, streamlines dependency management, and reduces build/test failures related to submodules.

## Clarifications

### Session 2026-01-22

- Q: How should bicep-types dependency versions be pinned after migration? → A: Pin to specific git commit SHA in both go.mod and pnpm lockfiles
- Q: What is the rollback strategy if the migration causes unexpected issues? → A: No dedicated rollback needed - migration is atomic and can be reverted via standard git revert
- Q: Should pnpm migration be limited to bicep-types or apply repository-wide? → A: Migrate all JavaScript/TypeScript tooling in the repository to pnpm
- Q: How to handle transition for contributors with existing clones containing the submodule? → A: Provide a one-time migration guide as part of the contributing docs with cleanup commands
- Q: Is Dependabot support for pnpm git references a concern? → A: pnpm is fully supported by Dependabot; only configuration updates are needed (covered by FR-017)

## User Scenarios & Testing *(mandatory)*

### User Story 1 - New Contributor Onboarding (Priority: P1)

As a new contributor to the Radius project, I want to clone the repository and start developing without needing to understand or manage git submodules, so that I can focus on code changes rather than wrestling with repository setup.

**Why this priority**: Contributor friction during initial setup leads to contributor abandonment. Submodules are a known source of confusion and errors for new contributors, impacting project growth.

**Independent Test**: A new contributor can clone the repository and successfully build the project using standard commands without any submodule-specific instructions.

**Acceptance Scenarios**:

1. **Given** a fresh clone of the Radius repository, **When** a contributor runs the standard build commands, **Then** the build completes successfully without submodule initialization steps
2. **Given** a contributor has cloned the repository with `--depth 1` (shallow clone), **When** they run the build, **Then** dependencies are resolved automatically without submodule-related errors
3. **Given** a contributor follows the onboarding documentation, **When** they set up their development environment, **Then** no submodule commands are required

---

### User Story 2 - CI/CD Build Reliability (Priority: P1)

As a project maintainer, I want CI/CD pipelines to build reliably without submodule synchronization failures, so that builds don't fail due to submodule state issues.

**Why this priority**: Build failures from submodule desynchronization waste maintainer time, block PRs, and erode confidence in the CI system. This directly impacts development velocity.

**Independent Test**: CI workflows complete successfully across multiple consecutive runs without submodule-related failures.

**Acceptance Scenarios**:

1. **Given** a pull request is submitted, **When** CI workflows execute, **Then** all builds complete without submodule initialization or synchronization steps
2. **Given** the bicep-types upstream repository receives updates, **When** the Radius CI runs, **Then** dependency updates are handled through standard dependency management (Go modules/pnpm) rather than submodule updates
3. **Given** a GitHub Actions workflow runs with shallow clone settings, **When** the build executes, **Then** all dependencies resolve correctly without requiring full git history

---

### User Story 3 - Dependency Update Management (Priority: P2)

As a project maintainer, I want to update bicep-types dependencies using standard dependency management tools (Dependabot, Go modules, pnpm), so that dependency updates follow the same workflow as all other dependencies.

**Why this priority**: Consistent dependency management reduces cognitive overhead and enables automated security patching through Dependabot.

**Independent Test**: Dependabot can successfully create PRs for bicep-types dependency updates using standard Go/pnpm configuration.

**Acceptance Scenarios**:

1. **Given** a new version of bicep-types is released, **When** Dependabot scans for updates, **Then** it creates a PR to update the dependency through go.mod or pnpm lockfiles
2. **Given** a maintainer wants to update bicep-types manually, **When** they run standard Go or pnpm update commands, **Then** the dependency updates without git submodule commands
3. **Given** Dependabot is configured, **When** it monitors for security vulnerabilities, **Then** bicep-types vulnerabilities are detected and reported like other dependencies

---

### User Story 4 - Git Worktree Support (Priority: P2)

As a developer working on multiple features, I want to use git worktrees for parallel development, so that I can work on multiple branches simultaneously without conflicts.

**Why this priority**: Worktrees are a productivity tool blocked by submodule usage. Enabling worktrees improves developer experience for power users.

**Independent Test**: A developer can create and use git worktrees without submodule conflicts.

**Acceptance Scenarios**:

1. **Given** a developer has the main repository checked out, **When** they create a git worktree for a feature branch, **Then** the worktree functions correctly without submodule conflicts
2. **Given** multiple worktrees exist, **When** the developer builds in each worktree, **Then** builds complete successfully with isolated dependency states

---

### User Story 5 - Documentation Clarity (Priority: P3)

As a contributor reading the documentation, I want setup instructions that use only standard dependency tools, so that I can understand and follow the onboarding process easily.

**Why this priority**: Documentation accuracy affects contributor success. Removing submodule instructions simplifies docs and reduces maintenance burden.

**Independent Test**: Documentation accurately reflects the build process without mentioning submodules.

**Acceptance Scenarios**:

1. **Given** a contributor reads the CONTRIBUTING guide, **When** they follow the setup steps, **Then** no submodule commands are mentioned or required
2. **Given** the documentation is updated, **When** a new contributor follows it, **Then** they can successfully build and test without referring to external resources

---

### Edge Cases

- What happens when a contributor has an old clone with the submodule still present?
  - Clear migration instructions should guide cleanup of stale submodule artifacts
- How does the system handle network failures during dependency resolution?
  - Standard Go/pnpm retry and caching mechanisms apply
- What happens if bicep-types upstream changes its directory structure?
  - Go module and pnpm path references would need updating, detected by CI failures

## Requirements *(mandatory)*

### Functional Requirements

#### Submodule Removal

- **FR-001**: The bicep-types git submodule MUST be completely removed from the repository
- **FR-002**: All references to submodule paths MUST be removed from .gitmodules configuration
- **FR-003**: The repository MUST not require any `git submodule` commands for building or testing

#### Go Dependencies

- **FR-004**: Go code MUST reference bicep-types-go through go.mod as a standard module dependency (github.com/Azure/bicep-types/src/bicep-types-go)
- **FR-005**: Go module dependencies MUST be resolvable using standard `go mod` commands
- **FR-006**: Go module dependencies MUST be pinned to a specific git commit SHA for reproducibility

#### NPM/pnpm Dependencies

- **FR-007**: All JavaScript/TypeScript tooling in the repository MUST be migrated to use pnpm as the package manager
- **FR-008**: NPM packages in hack/bicep-types-radius/ MUST use pnpm git references with postinstall build scripts for bicep-types packages
- **FR-009**: Lockfiles MUST be updated to reflect pnpm git references with postinstall build scripts
- **FR-010**: pnpm dependencies MUST be pinned to a specific git commit SHA for reproducibility

#### Build System

- **FR-011**: All Makefiles MUST function without submodule-related commands
- **FR-012**: Build scripts MUST use pnpm for JavaScript/TypeScript dependency management
- **FR-013**: Workflow files MUST not contain submodule initialization or update steps

#### CI/CD

- **FR-014**: All CI/CD workflows MUST pass without submodule operations
- **FR-015**: Codegen workflows MUST complete successfully with new dependency sources
- **FR-016**: All regression tests MUST pass after migration

#### Dependency Management

- **FR-017**: Dependabot configuration MUST be updated to monitor Go and pnpm dependency sources
- **FR-018**: Security scanning MUST cover bicep-types dependencies through standard vulnerability detection

#### Development Environment

- **FR-019**: The dev container MUST be updated to include pnpm pre-installed

#### Documentation

- **FR-020**: CONTRIBUTING.md and contributor documentation MUST be updated to remove all references to the bicep-types submodule and submodule-related instructions
- **FR-021**: Contributor documentation MUST include a one-time migration guide with cleanup commands for existing clones that contain the submodule
- **FR-022**: Contributor documentation MUST include updated steps for Go and pnpm setup
- **FR-023**: Any README files referencing submodules MUST be updated

### Key Entities

- **bicep-types-go**: Go language bindings for Bicep types, currently consumed as a submodule, to be consumed as a Go module
- **bicep-types NPM packages**: JavaScript/TypeScript packages for Bicep types, to be consumed via pnpm git references
- **hack/bicep-types-radius/**: Directory containing Radius-specific Bicep type tooling with npm dependencies to migrate

## Assumptions

- The Azure/bicep-types repository structure supports direct Go module and pnpm git subdirectory references
- pnpm git references with subdirectory paths are stable and supported for production use
- No Radius features depend on submodule-specific behaviors (local modifications, specific commit pinning beyond what dependency managers support)
- CI/CD environments have pnpm available or can install it as part of workflow setup
- Migration changes are atomic and can be reverted via standard git revert if critical issues arise

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: New contributors can complete repository setup and first build in under 10 minutes following documentation
- **SC-002**: CI/CD build success rate improves by eliminating submodule-related failures (target: 0 submodule-related build failures after migration is merged)
- **SC-003**: 100% of build/test workflows execute without any git submodule commands
- **SC-004**: Dependabot successfully monitors and creates PRs for npm registry package updates in pnpm directories (Note: bicep-types-repo git dependency requires manual SHA updates as Dependabot does not support git references)
- **SC-005**: All existing regression tests pass without modification to test logic (only dependency resolution changes)
- **SC-006**: Git worktrees can be created and used without conflicts or additional configuration
- **SC-007**: Documentation requires no mentions of git submodule commands for standard development workflows
