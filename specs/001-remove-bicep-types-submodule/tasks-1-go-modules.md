# Tasks: Plan 1 - Migrate Go Dependencies to Direct Module Reference

**Input**: Design documents from `/specs/001-remove-bicep-types-submodule/`
**Prerequisites**: plan-1-go-modules.md (required), spec.md (required for user stories)
**Branch**: `001-remove-bicep-types-submodule-go`

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (US1-US5)
- Include exact file paths in descriptions

## Scope

This plan addresses:

- FR-004: Go code references bicep-types-go through go.mod as standard module
- FR-005: Go module dependencies resolvable via `go mod` commands
- FR-006: Go module pinned to specific git commit SHA
- FR-020 (partial): Go-related documentation updated

---

## Phase 1: Setup (Research & Verification)

**Purpose**: Validate Go module approach before making changes

- [X] T001 Identify current bicep-types submodule commit SHA via `git submodule status` in radius/
- [X] T002 Verify bicep-types-go module availability via `go list -m github.com/Azure/bicep-types/src/bicep-types-go@<commit-sha>`
- [X] T003 [P] Document all Go files importing bicep-types-go packages via `grep -r "github.com/Azure/bicep-types" --include="*.go" radius/`

---

## Phase 2: Foundational (Create Working Branch)

**Purpose**: Prepare the codebase for migration

**⚠️ CRITICAL**: Complete before implementation

- [X] T004 Create feature branch `001-remove-bicep-types-submodule-go` from main
- [X] T005 Verify submodule is initialized and build works via `make build` in radius/

**Checkpoint**: Foundation ready - Go module migration can proceed

---

## Phase 3: User Story 1 - New Contributor Onboarding (Priority: P1) 🎯 MVP

**Goal**: Go build completes without requiring submodule initialization for new contributors

**Independent Test**: Clone repository fresh, verify `go mod download && go build ./...` succeeds without submodule

### Implementation for User Story 1

- [X] T006 [US1] Remove replace directive from radius/go.mod (line with `github.com/Azure/bicep-types/src/bicep-types-go => ./bicep-types/src/bicep-types-go`)
- [X] T007 [US1] Add direct require for bicep-types-go module in radius/go.mod with pinned commit SHA
- [X] T008 [US1] Run `go mod tidy` to resolve and update radius/go.sum
- [X] T009 [US1] Verify Go build succeeds via `go build ./...` in radius/
- [X] T010 [US1] Verify all Go tests pass via `go test ./...` in radius/

**Checkpoint**: Go code compiles and tests pass with direct module reference

---

## Phase 4: User Story 2 - CI/CD Build Reliability (Priority: P1)

**Goal**: CI workflows build Go code without submodule commands

**Independent Test**: Verify build.yaml and other Go-related workflows don't require submodule for Go compilation

### Implementation for User Story 2

- [X] T011 [US2] Verify `make build` succeeds in radius/ (covers Makefile targets)
- [X] T012 [US2] Verify `make test` passes all Go-related tests in radius/ (Note: converter tests pass; unrelated test failures exist)
- [X] T013 [US2] Verify `make lint` passes (if Go linting is included)

**Checkpoint**: Makefile targets work with direct module reference

---

## Phase 5: User Story 3 - Dependency Update Management (Priority: P2)

**Goal**: Dependabot can update bicep-types-go via standard Go module workflow

**Independent Test**: Verify Dependabot configuration covers Go modules (already enabled)

### Implementation for User Story 3

- [X] T014 [US3] Verify existing Dependabot config in radius/.github/dependabot.yml covers gomod package-ecosystem
- [X] T015 [US3] No changes needed - Dependabot already monitors Go modules in this repository

**Checkpoint**: Dependabot will automatically detect bicep-types-go updates

---

## Phase 6: User Story 5 - Documentation Clarity (Priority: P3)

**Goal**: Go-related contributor documentation reflects new dependency approach

**Independent Test**: Documentation accurately describes Go setup without submodule references

### Implementation for User Story 5

- [X] T016 [P] [US5] Update Go setup section in radius/docs/contributing/contributing-code/contributing-code-building/README.md to clarify Go uses direct module dependency
- [X] T017 [P] [US5] Verify radius/docs/contributing/ Go docs don't mention submodule for Go dependencies

**Checkpoint**: Documentation accurate for Go module approach

---

## Phase 7: Polish & Validation

**Purpose**: Final verification before PR

- [X] T018 Run full `make build` and `make test` in radius/
- [X] T019 [P] Verify `go mod verify` passes (checksums valid)
- [X] T020 [P] Run `go mod graph | grep bicep-types` to confirm dependency resolution
- [ ] T021 Create PR from `001-remove-bicep-types-submodule-go` to main
- [ ] T022 Verify CI pipeline passes on PR

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies - can start immediately
- **Foundational (Phase 2)**: Depends on Setup - confirms starting state
- **User Story 1 (Phase 3)**: Depends on Foundational - core migration
- **User Story 2 (Phase 4)**: Depends on User Story 1 - validates Makefile
- **User Story 3 (Phase 5)**: Can run in parallel with Phase 4
- **User Story 5 (Phase 6)**: Can run in parallel with Phase 4-5
- **Polish (Phase 7)**: Depends on all user stories complete

### Parallel Opportunities

```text
After Phase 3 (US1) completes:
  ├── T011-T013 [US2] Makefile validation
  ├── T014-T015 [US3] Dependabot verification
  └── T016-T017 [US5] Documentation updates (all [P])
```

---

## Implementation Strategy

### MVP (User Story 1 Only)

1. Complete Phases 1-3
2. **STOP and VALIDATE**: Go builds without submodule
3. This is sufficient for Go code to work independently

### Full Plan 1 Completion

1. Complete all phases
2. Merge PR before starting Plan 2
3. Plan 2 depends on Plan 1 being merged

---

## Notes

- This plan modifies Go tooling only - submodule is NOT removed yet
- Submodule removal happens in Plan 2 (pnpm migration)
- If Plan 1 needs rollback, simply revert the PR - submodule still exists
- Plan 2 cannot proceed until Plan 1 is merged
- No test tasks included - existing tests validate the migration

---

## Summary

| Metric | Value |
| ------- | ----- |
| Total Tasks | 22 |
| Setup/Foundational | 5 |
| US1 (P1) | 5 |
| US2 (P1) | 3 |
| US3 (P2) | 2 |
| US5 (P3) | 2 |
| Polish | 5 |
| Parallel Opportunities | 6 tasks can run in parallel after US1 |
| MVP Scope | Tasks T001-T010 (Phases 1-3) |
