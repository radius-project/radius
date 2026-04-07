# Tasks: Long-Running Tests Use Current Release

**Input**: Design documents from `/specs/001-lrt-current-release/`
**Prerequisites**: plan.md ✅, spec.md ✅, research.md ✅, quickstart.md ✅

**Tests**: Not requested in the feature specification. Workflow validates itself via functional test execution.

**Organization**: Tasks are grouped by user story to enable independent implementation and testing of each story.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3)
- Include exact file paths in descriptions

## Path Conventions

- **Workflow file**: `radius/.github/workflows/long-running-azure.yaml`
- **Helper script**: `radius/.github/scripts/manage-radius-installation.sh`

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Create helper script and prepare for workflow changes

- [X] T001 Create manage-radius-installation.sh script in radius/.github/scripts/manage-radius-installation.sh
- [X] T002 Implement version detection logic (parse rad version output for CLI and control plane versions)
- [X] T003 Implement conditional install/upgrade logic (not installed → install, same version → skip, different → upgrade)
- [X] T004 Add error handling for upgrade failures with clear error messages

**Checkpoint**: Helper script ready for integration into workflow

---

## Phase 2: User Story 4 - Simplified Workflow (Priority: P2)

**Goal**: Remove all build-related logic from the workflow

**Independent Test**: Review workflow file to confirm no build steps, caching logic, or skip-build conditions remain

**Why first**: Removing the build job clears the way for the new CLI installation approach. This is foundational for other stories.

### Implementation for User Story 4

- [X] T005 [US4] Remove entire `build` job from radius/.github/workflows/long-running-azure.yaml
- [X] T006 [P] [US4] Remove VALID_RADIUS_BUILD_WINDOW environment variable from radius/.github/workflows/long-running-azure.yaml
- [X] T007 [P] [US4] Remove CONTAINER_REGISTRY environment variable from radius/.github/workflows/long-running-azure.yaml
- [X] T008 [US4] Remove `needs: build` dependency from tests job in radius/.github/workflows/long-running-azure.yaml
- [X] T009 [US4] Remove build job outputs references from tests job env section in radius/.github/workflows/long-running-azure.yaml
- [X] T010 [US4] Update workflow file header comments to reflect new behavior (no build from source)

**Checkpoint**: Workflow has no build logic; tests job has no build dependencies

---

## Phase 3: User Story 1 - Run Tests Against Current Release (Priority: P1) 🎯 MVP

**Goal**: Install CLI from official release instead of building from source

**Independent Test**: Trigger workflow and verify `rad version` shows current release version (not a build from main)

### Implementation for User Story 1

- [X] T011 [US1] Add step to install Radius CLI via official installer script in tests job in radius/.github/workflows/long-running-azure.yaml
- [X] T012 [US1] Add step to verify CLI installation (run `rad version` and check output) in radius/.github/workflows/long-running-azure.yaml
- [X] T013 [US1] Update PATH configuration to include installed CLI location in radius/.github/workflows/long-running-azure.yaml
- [X] T014 [US1] Move recipe publishing steps to tests job after CLI installation in radius/.github/workflows/long-running-azure.yaml

**Checkpoint**: CLI is installed from release; workflow can proceed to control plane management

---

## Phase 4: User Story 2 - Smart Control Plane Installation (Priority: P1)

**Goal**: Intelligently manage control plane based on version comparison

**Independent Test**: Run workflow against clusters in various states (no Radius, same version, different version)

### Implementation for User Story 2

- [X] T015 [US2] Add step to invoke manage-radius-installation.sh script in tests job in radius/.github/workflows/long-running-azure.yaml
- [X] T016 [US2] Ensure script has execute permissions and is callable from workflow
- [X] T017 [US2] Add logging output for version detection results in manage-radius-installation.sh

**Checkpoint**: Control plane is installed/upgraded/skipped based on version comparison

---

## Phase 5: User Story 3 - Graceful Upgrade Failure Handling (Priority: P2)

**Goal**: Stop workflow gracefully with clear error when upgrade is not possible

**Independent Test**: Simulate upgrade failure and verify workflow stops with appropriate error message

### Implementation for User Story 3

- [X] T018 [US3] Add error capture and display when rad upgrade command fails in manage-radius-installation.sh
- [X] T019 [US3] Ensure non-zero exit code propagates to workflow when upgrade fails
- [X] T020 [US3] Add descriptive error message explaining upgrade failure reason

**Checkpoint**: Upgrade failures stop workflow with clear error message

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Final validation and cleanup

- [X] T021 [P] Remove skip-build workflow_dispatch input option from radius/.github/workflows/long-running-azure.yaml
- [X] T022 [P] Verify BICEP_RECIPE_REGISTRY and TEST_BICEP_TYPES_REGISTRY env vars are retained
- [X] T023 [P] Update workflow file header documentation block to describe new behavior
- [ ] T024 Run workflow validation per quickstart.md success verification checklist
- [ ] T025 Verify functional tests execute successfully with installed CLI

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies - can start immediately
- **User Story 4 (Phase 2)**: Can start immediately; clears build logic (foundational cleanup)
- **User Story 1 (Phase 3)**: Depends on US4 completion (no build job to conflict)
- **User Story 2 (Phase 4)**: Depends on Setup (Phase 1) for script, US1 for CLI installation
- **User Story 3 (Phase 5)**: Depends on Setup (Phase 1) for script structure
- **Polish (Phase 6)**: Depends on all user stories being complete

### User Story Dependencies

- **User Story 4 (P2)**: Foundational cleanup - can start immediately
- **User Story 1 (P1)**: Depends on US4 (build job removal clears path)
- **User Story 2 (P1)**: Depends on Phase 1 setup + US1 (needs CLI installed)
- **User Story 3 (P2)**: Depends on Phase 1 setup (extends script with error handling)

### Within Each Phase

- Tasks marked [P] can run in parallel (different files/sections)
- Non-parallel tasks must complete sequentially
- Complete each phase before moving to the next

### Parallel Opportunities

**Phase 1 (Setup)**:

- T002, T003, T004 can run in parallel after T001 creates the script file

**Phase 2 (US4)**:

- T006, T007 can run in parallel (different env var removals)

**Phase 6 (Polish)**:

- T021, T022, T023 can run in parallel (different file sections)

---

## Implementation Strategy

### MVP First (User Story 1 Focus)

1. Complete Phase 1: Setup (create helper script)
2. Complete Phase 2: User Story 4 (remove build logic)
3. Complete Phase 3: User Story 1 (install CLI from release) 🎯 MVP
4. **STOP and VALIDATE**: Test workflow installs correct CLI version
5. Continue with remaining stories

### Incremental Delivery

1. Setup + US4 → Build logic removed
2. Add US1 → CLI installed from release → Validate (MVP!)
3. Add US2 → Smart control plane management → Validate
4. Add US3 → Error handling → Validate
5. Polish → Complete workflow ready

---

## Notes

- [P] tasks = different files/sections, no dependencies
- [Story] label maps task to specific user story for traceability
- US4 is implemented before US1 (despite P2 vs P1) because it's foundational cleanup
- Tests are not included (workflow validates itself via functional test execution)
- Commit after each task or logical group
- Stop at any checkpoint to validate story independently
