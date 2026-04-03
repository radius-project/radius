# Tasks: Plan 2 - Migrate to pnpm + Remove bicep-types Submodule

**Input**: Design documents from `/specs/001-remove-bicep-types-submodule/`
**Prerequisites**: plan-2-pnpm-submodule.md (required), spec.md (required for user stories), research-2-pnpm.md
**Branch**: `001-remove-bicep-types-submodule-pnpm`
**Depends On**: Plan 1 (Go Modules) must be merged first
**Prototype**: [brooke-hamilton/radius:pnpm-direct-ref](https://github.com/brooke-hamilton/radius/tree/pnpm-direct-ref)

## Format: `- [ ] [TaskID] [P?] [Story?] Description with file path`

- **Checkbox**: ALWAYS start with `- [ ]` (markdown checkbox)
- **Task ID**: Sequential number (T001, T002...) in execution order
- **[P]**: Include ONLY if task is parallelizable (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (US1-US5) - REQUIRED for user story phases only
- Include exact file paths in descriptions

## Scope

This plan addresses:

- FR-001: bicep-types git submodule completely removed
- FR-002: .gitmodules configuration removed
- FR-003: No `git submodule` commands required for build/test
- FR-007 through FR-023: pnpm migration, CI/CD updates, documentation

## Critical Technical Constraint

**⚠️ pnpm subdirectory syntax does NOT work for TypeScript packages requiring compilation.**

The `bicep-types` package is TypeScript source that must be compiled. The solution validated in the prototype:

1. Reference full repo via `git+https://github.com/Azure/bicep-types.git#<sha>` (NOT `github:...#path:/`)
2. pnpm fetches it to `node_modules/bicep-types-repo/`
3. `postinstall` script runs `npm install && npm run build` inside the package
4. Symlink created: `node_modules/bicep-types` → `bicep-types-repo/src/bicep-types`
5. `.npmrc` with `side-effects-cache = false` ensures postinstall runs

---

## Phase 1: Setup (Environment Preparation)

**Purpose**: Prepare pnpm tooling and verify prerequisites

- [X] T001 Verify Plan 1 (Go modules migration) is merged to main
- [X] T002 Verify pnpm is installed locally via `pnpm --version`
- [X] T003 Identify current bicep-types submodule commit SHA for pnpm git references via `git submodule status` in radius/

---

## Phase 2: Foundational (Create Working Branch)

**Purpose**: Prepare the codebase for migration

**⚠️ CRITICAL**: Complete before any user story implementation

- [X] T004 Create feature branch `001-remove-bicep-types-submodule-pnpm` from main
- [X] T005 Pull latest main to ensure Plan 1 changes are included
- [X] T006 Verify current build works via `make build` in radius/

**Checkpoint**: Foundation ready - pnpm migration can proceed

---

## Phase 3: User Story 1 - New Contributor Onboarding (Priority: P1) 🎯 MVP

**Goal**: New contributors can clone and build without submodule initialization

**Independent Test**: Clone repository fresh (no --recurse-submodules), run pnpm install and build commands successfully

### Implementation for User Story 1

#### pnpm Migration - typespec/ (no bicep-types dependency)

- [X] T007 [US1] Delete radius/typespec/package-lock.json
- [X] T008 [US1] Generate pnpm lockfile via `pnpm install` in radius/typespec/
- [X] T009 [US1] Verify typespec builds via `pnpm run build` in radius/typespec/

#### pnpm Migration - hack/bicep-types-radius/src/generator/

- [X] T010 [US1] Update radius/hack/bicep-types-radius/src/generator/package.json:
  - Add `"pnpm": { "onlyBuiltDependencies": ["autorest"] }`
  - Add `postinstall` script for bicep-types build and symlink
  - Replace `"bicep-types": "file:..."` with `"bicep-types-repo": "git+https://github.com/Azure/bicep-types.git#<sha>"`
- [X] T011 [US1] Create radius/hack/bicep-types-radius/src/generator/.npmrc with `side-effects-cache = false`
- [X] T012 [US1] Delete radius/hack/bicep-types-radius/src/generator/package-lock.json
- [X] T013 [US1] Generate pnpm lockfile via `pnpm install` in radius/hack/bicep-types-radius/src/generator/
- [X] T014 [US1] Verify generator builds via `pnpm run build` in radius/hack/bicep-types-radius/src/generator/

#### pnpm Migration - hack/bicep-types-radius/src/autorest.bicep/

- [X] T015 [US1] Update radius/hack/bicep-types-radius/src/autorest.bicep/package.json:
  - Add `"pnpm": { "onlyBuiltDependencies": ["autorest"] }`
  - Add `postinstall` script for bicep-types build and symlink
  - Replace `"bicep-types": "file:..."` with `"bicep-types-repo": "git+https://github.com/Azure/bicep-types.git#<sha>"`
- [X] T016 [US1] Create radius/hack/bicep-types-radius/src/autorest.bicep/.npmrc with `side-effects-cache = false`
- [X] T017 [US1] Delete radius/hack/bicep-types-radius/src/autorest.bicep/package-lock.json
- [X] T018 [US1] Generate pnpm lockfile via `pnpm install` in radius/hack/bicep-types-radius/src/autorest.bicep/
- [X] T019 [US1] Verify autorest.bicep builds via `pnpm run build` in radius/hack/bicep-types-radius/src/autorest.bicep/

#### Submodule Removal

- [X] T020 [US1] Remove submodule from git index via `git rm bicep-types` in radius/
- [X] T021 [US1] Delete radius/.gitmodules file
- [X] T022 [US1] Clean up .git/modules/bicep-types directory via `rm -rf .git/modules/bicep-types`

#### Verify Build Works

- [X] T023 [US1] Verify full build via `make build` in radius/
- [ ] T024 [US1] Verify tests via `make test` in radius/

**Checkpoint**: Repository builds and tests pass without submodule

---

## Phase 4: User Story 2 - CI/CD Build Reliability (Priority: P1)

**Goal**: All CI workflows pass without submodule initialization steps

**Independent Test**: CI pipeline completes successfully on PR

### Implementation for User Story 2

#### Makefile Updates

- [X] T025 [US2] Update radius/build/generate.mk: add `generate-pnpm-installed` target to verify pnpm availability
- [X] T026 [US2] Update radius/build/generate.mk: replace `npm` commands with `pnpm` in generate-bicep-types target
- [X] T027 [US2] Update radius/build/generate.mk: remove `git submodule update --init --recursive` commands
- [X] T028 [US2] Update radius/build/generate.mk line ~33: change error message from `run 'npm ci' in typespec directory.` to `run 'pnpm install' in typespec directory.`
- [X] T029 [US2] Update radius/build/generate.mk line ~55: change error message from `run 'npm install -g autorest@3.7.2' to install autorest` to `run 'pnpm add -g autorest@3.7.2' to install autorest`
- [X] T030 [US2] Update radius/build/test.mk line ~179: change error message from `run 'npm install -g oav@4.0.2' to install oav` to `run 'pnpm add -g oav@4.0.2' to install oav`
- [X] T031 [US2] Verify `make generate-bicep-types` works in radius/

#### Workflow Updates - build.yaml (4 occurrences)

- [X] T032 [US2] Update radius/.github/workflows/build.yaml line ~110: remove `submodules: recursive` from checkout step
- [X] T033 [US2] Update radius/.github/workflows/build.yaml line ~212: remove `submodules: recursive` from checkout step
- [X] T034 [US2] Update radius/.github/workflows/build.yaml line ~369: remove `submodules: recursive` from checkout step
- [X] T035 [US2] Update radius/.github/workflows/build.yaml line ~436: remove `submodules: recursive` from checkout step
- [X] T036 [US2] Add pnpm setup step to radius/.github/workflows/build.yaml (pnpm/action-setup@v4)

#### Workflow Updates - Other Files

- [X] T037 [P] [US2] Update radius/.github/workflows/codeql.yml line ~95: remove `submodules: recursive`, add pnpm setup
- [X] T038 [P] [US2] Update radius/.github/workflows/lint.yaml line ~58: remove `submodules: recursive`, add pnpm setup
- [X] T039 [P] [US2] Update radius/.github/workflows/validate-bicep.yaml line ~64: remove `submodules: true`, add pnpm setup
- [X] T040 [P] [US2] Update radius/.github/workflows/publish-docs.yaml line ~52: remove `submodules: recursive`, add pnpm setup
- [X] T041 [P] [US2] Update radius/.github/workflows/long-running-azure.yaml line ~136: remove `submodules: recursive`, add pnpm setup

#### Workflow Updates - Functional Tests (4 occurrences in cloud.yaml)

- [X] T042 [US2] Update radius/.github/workflows/functional-test-noncloud.yaml line ~208: remove `submodules: recursive`, add pnpm setup
- [X] T043 [US2] Update radius/.github/workflows/functional-test-cloud.yaml line ~172: remove `submodules: recursive`, add pnpm setup
- [X] T044 [US2] Update radius/.github/workflows/functional-test-cloud.yaml line ~328: remove `submodules: recursive`, add pnpm setup
- [X] T045 [US2] Update radius/.github/workflows/functional-test-cloud.yaml line ~336: remove `submodules: recursive`, add pnpm setup
- [X] T046 [US2] Update radius/.github/workflows/functional-test-cloud.yaml line ~626: remove `submodules: recursive`, add pnpm setup

#### Workflow Updates - npm to pnpm Consistency

**⚠️ IMPORTANT**: Tasks T047-T055 MUST be executed AFTER T036-T046. The pnpm setup step must be added to each workflow BEFORE the workflow can use pnpm commands. When editing each workflow file, add the pnpm setup step first, then update npm commands to pnpm in the same edit.

- [X] T047 [P] [US2] Update radius/.github/workflows/lint.yaml line ~76: change `npm ci` to `pnpm install`
- [X] T048 [P] [US2] Update radius/.github/workflows/lint.yaml line ~78: change `npm install -g autorest@3.7.2` to `pnpm add -g autorest@3.7.2`
- [X] T049 [P] [US2] Update radius/.github/workflows/lint.yaml line ~79: change `npm install -g oav@4.0.2` to `pnpm add -g oav@4.0.2`
- [X] T050 [P] [US2] Update radius/.github/workflows/publish-docs.yaml line ~95: change `npm ci` to `pnpm install`
- [X] T051 [P] [US2] Update radius/.github/workflows/publish-docs.yaml line ~98: change `npm install -g autorest@3.7.2` to `pnpm add -g autorest@3.7.2`
- [X] T052 [P] [US2] Update radius/.github/workflows/publish-docs.yaml line ~99: change `npm install -g oav@4.0.2` to `pnpm add -g oav@4.0.2`
- [ ] T053 [P] [US2] Update radius/.github/workflows/devcontainer-feature-test.yaml line ~36: change `npm install -g @devcontainers/cli` to `pnpm add -g @devcontainers/cli`
- [ ] T054 [P] [US2] Update radius/.github/workflows/devcontainer-feature-test.yaml line ~53: change `npm install -g @devcontainers/cli` to `pnpm add -g @devcontainers/cli`
- [X] T055 [P] [US2] Update radius/.github/workflows/validate-bicep.yaml lines ~78-84: update npx installation fallback from `npm install -g npx` to `pnpm add -g npx` (or remove if npx is bundled with Node.js)

**Checkpoint**: All 15 submodule references removed from 8 workflow files, npm commands updated to pnpm for consistency

---

## Phase 5: User Story 3 - Dependency Update Management (Priority: P2)

**Goal**: Dependabot monitors pnpm dependencies correctly

**Independent Test**: Dependabot configuration is valid and covers all pnpm directories

**Note**: Git dependencies (`git+https://...#commit`) have NO auto-update support in Dependabot. The bicep-types-repo commit SHA must be updated manually. Consider a scheduled GitHub Action for notifications.

### Implementation for User Story 3

- [X] T056 [US3] Update radius/.github/dependabot.yml: remove `gitsubmodule` package-ecosystem section
- [X] T057 [P] [US3] Update radius/.github/dependabot.yml: add npm config for `/hack/bicep-types-radius/src/generator`
- [X] T058 [P] [US3] Update radius/.github/dependabot.yml: add npm config for `/hack/bicep-types-radius/src/autorest.bicep`
- [X] T059 [US3] Verify Dependabot config syntax via `actionlint` or manual review

**Checkpoint**: Dependabot configured for all pnpm directories

---

## Phase 6: User Story 4 - Git Worktree Support (Priority: P2)

**Goal**: Git worktrees work without submodule conflicts

**Independent Test**: Create worktree, build successfully

### Implementation for User Story 4

- [ ] T060 [US4] Test git worktree creation via `git worktree add ../radius-test feature-branch`
- [ ] T061 [US4] Verify build in worktree via `make build` in worktree directory
- [ ] T062 [US4] Clean up test worktree via `git worktree remove ../radius-test`

**Checkpoint**: Worktrees work without submodule issues

---

## Phase 7: User Story 5 - Documentation Clarity (Priority: P3)

**Goal**: All documentation reflects pnpm workflow without submodule references

**Independent Test**: Documentation accurately describes setup process

### Implementation for User Story 5

#### Dev Container

- [X] T063 [US5] Get latest pnpm version via `pnpm --version` (or check corepack) and update dev container:
  - Update radius/.devcontainer/devcontainer.json: add `"pnpmVersion": "<version>"` to the node feature configuration
- [X] T064 [US5] Update radius/.devcontainer/post-create.sh line ~6: change `npm install -g @typespec/compiler` to `pnpm add -g @typespec/compiler`
- [X] T065 [US5] Update radius/.devcontainer/post-create.sh line ~21: change `npm ci` to `pnpm install` in typespec directory
- [X] T066 [US5] Update radius/.devcontainer/post-create.sh line ~22: change `npm install -g autorest@3.7.2` to `pnpm add -g autorest@3.7.2`
- [X] T067 [US5] Update radius/.devcontainer/post-create.sh line ~23: change `npm install -g oav@4.0.2` to `pnpm add -g oav@4.0.2`

#### Contributing Documentation

- [ ] T068 [P] [US5] Update radius/CONTRIBUTING.md: replace npm with pnpm, remove all submodule references, document postinstall behavior
- [X] T069 [P] [US5] Create radius/docs/contributing/migration-guide.md with cleanup commands for existing clones
- [X] T070 [P] [US5] Update radius/docs/contributing/contributing-code/contributing-code-prerequisites/README.md lines ~183-185: change npm commands to pnpm

#### README Updates

- [ ] T071 [P] [US5] Review and update radius/README.md if it mentions submodules
- [ ] T072 [P] [US5] Review and update radius/hack/bicep-types-radius/README.md if it exists

**Checkpoint**: All documentation updated

---

## Phase 8: Polish & Validation

**Purpose**: Final verification before PR

- [X] T073 Run full `make build` in radius/
- [ ] T074 Run full `make test` in radius/
- [X] T075 [P] Run `make generate-bicep-types` to verify codegen workflow
- [ ] T076 [P] Verify pnpm lockfiles are consistent via `pnpm install --frozen-lockfile` in each directory
- [ ] T077 Fresh clone test: clone to new directory without `--recurse-submodules`, verify build
- [ ] T078 Create PR from `001-remove-bicep-types-submodule-pnpm` to main
- [ ] T079 Verify all CI workflows pass on PR

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: Depends on Plan 1 being merged
- **Foundational (Phase 2)**: Depends on Setup completion
- **User Story 1 (Phase 3)**: Depends on Foundational - core migration + submodule removal
- **User Story 2 (Phase 4)**: Depends on User Story 1 - workflows need submodule removed first
- **User Story 3 (Phase 5)**: Can start after User Story 2 (Dependabot config)
- **User Story 4 (Phase 6)**: Can run in parallel with Phase 5 (worktree testing)
- **User Story 5 (Phase 7)**: Can run in parallel with Phases 5-6
- **Polish (Phase 8)**: Depends on all user stories complete

### Within User Story 1 (Phase 3)

```text
pnpm migrations can run in parallel:
  ├── T007-T009: typespec/
  ├── T010-T014: generator/ (includes .npmrc creation)
  └── T015-T019: autorest.bicep/ (includes .npmrc creation)
Then sequentially:
  └── T020-T024: Submodule removal + verification
```

### Within User Story 2 (Phase 4)

```text
T025-T031: Makefile updates (sequential)
T032-T036: build.yaml updates (sequential - same file)
T037-T041: Other workflow files (parallel - different files)
T042-T055: Functional test workflows + npm-to-pnpm consistency (partially parallel)
```

### Parallel Opportunities After Phase 4

```text
After Phase 4 (US2) completes:
  ├── T056-T059 [US3] Dependabot config
  ├── T060-T062 [US4] Worktree testing
  └── T063-T072 [US5] Documentation updates
```

---

## Implementation Strategy

### MVP (User Stories 1-2)

1. Complete Phases 1-4
2. **STOP and VALIDATE**: Repository builds, CI passes, no submodule
3. This is sufficient for core functionality

### Full Plan 2 Completion

1. Complete all phases
2. Merge PR
3. Feature complete: submodule removed, pnpm migrated

---

## Notes

- Plan 1 MUST be merged before starting this plan
- **CRITICAL**: pnpm subdirectory syntax does NOT work for TypeScript packages requiring compilation
- Use `git+https://github.com/Azure/bicep-types.git#<sha>` format (NOT `github:...#path:/`)
- The `postinstall` script builds TypeScript and creates symlink for module resolution
- `.npmrc` with `side-effects-cache = false` ensures postinstall scripts run correctly
- Package named `bicep-types-repo` in devDependencies; symlink created as `bicep-types`
- Use same commit SHA as was in submodule for initial migration
- 15 total `submodules:` occurrences across 8 workflow files to update
- Dev container requires corepack for pnpm activation
- Existing clones need migration guide for cleanup
- Git dependencies have NO Dependabot auto-update support; manual SHA updates required

---

## Summary

| Metric | Value |
| ------- | ----- |
| Total Tasks | 79 |
| Setup/Foundational | 6 |
| US1 (P1) | 18 |
| US2 (P1) | 31 |
| US3 (P2) | 4 |
| US4 (P2) | 3 |
| US5 (P3) | 10 |
| Polish | 7 |
| Parallel Opportunities | 20+ tasks can run in parallel (pnpm dirs, workflows, docs) |
| MVP Scope | Tasks T001-T055 (Phases 1-4) |
| Workflow Files Updated | 10 |
| Submodule References Removed | 15 |
| npm→pnpm Consistency Updates | 17 |
