# Tasks: Git Workspace Mode

**Input**: Design documents from `/specs/001-repo-radius/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/cli-commands.md

**Tests**: Tests are NOT explicitly requested in the feature specification. Tasks focus on implementation only.

**Organization**: Tasks are grouped by user story to enable independent implementation and testing of each story.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3)
- Include exact file paths in descriptions

## Path Conventions

Based on plan.md, the source structure is:
- `pkg/cli/git/` - New Git workspace core package
- `pkg/cli/cmd/rad/` - CLI command implementations
- `test/` - Test files (existing Radius test structure)

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Project initialization and basic structure for Git workspace

- [X] T001 Create `pkg/cli/git/` directory structure per plan.md
- [X] T002 [P] Add hashicorp/terraform-exec dependency to go.mod
- [X] T003 [P] Add joho/godotenv dependency to go.mod
- [X] T004 [P] Create exit code constants in pkg/cli/git/exitcodes.go
- [X] T005 [P] Create error format struct in pkg/cli/git/errors.go per NFR-021

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Core infrastructure that MUST be complete before ANY user story can be implemented

**⚠️ CRITICAL**: No user story work can begin until this phase is complete

### Configuration Loading

- [X] T006 Implement Environment struct and parsing in pkg/cli/git/config/environment.go
- [X] T007 [P] Implement AWSConfig, AzureConfig, KubernetesConfig structs in pkg/cli/git/config/environment.go
- [X] T008 Implement LoadEnvironment function using godotenv in pkg/cli/git/config/environment.go
- [X] T009 [P] Implement environment validation (required fields, credential detection) in pkg/cli/git/config/environment.go

### Recipe Loading

- [X] T010 Implement RecipeFile and Recipe structs in pkg/cli/git/config/recipes.go
- [X] T011 Implement LoadRecipes function in pkg/cli/git/config/recipes.go
- [X] T012 Implement recipe validation (pinned versions, valid locations) in pkg/cli/git/config/recipes.go

### Workspace Management

- [X] T013 Implement GitWorkspace struct in pkg/cli/workspaces/git.go
- [X] T014 Implement GitConnection and workspace interface methods in pkg/cli/workspaces/git.go
- [X] T015 Implement IsGitWorkspace detection function in pkg/cli/git/repo/state.go
- [ ] T016 [P] Update workspace registry to include built-in `git` workspace in pkg/cli/workspaces/workspace.go

### Git Operations

- [X] T017 Implement Git repository detection (git rev-parse) in pkg/cli/git/repo/state.go
- [X] T018 [P] Implement sparse-checkout helper for resource types in pkg/cli/git/repo/sparse.go
- [X] T019 [P] Implement uncommitted changes detection in pkg/cli/git/repo/state.go

**Checkpoint**: Foundation ready - user story implementation can now begin in parallel

---

## Phase 3: User Story 1 - Initialize Repository for Radius (Priority: P1) 🎯 MVP

**Goal**: Enable developers to initialize a Git repository for Radius Git workspace mode

**Independent Test**: Run `rad init` in a Git repository and verify directory structure is created, Resource Types are populated, and environment configuration is established.

### Implementation for User Story 1

- [ ] T020 [US1] Create rad init command skeleton in pkg/cli/cmd/rad/init_git.go
- [ ] T021 [US1] Implement Git repository verification in rad init (check .git/ or git rev-parse)
- [ ] T022 [US1] Implement directory structure creation (.radius/, .radius/config/, .radius/model/, .radius/plan/, .radius/deploy/)
- [ ] T023 [US1] Implement Resource Types population via sparse-checkout from resource-types-contrib
- [ ] T024 [US1] Implement existing .env file detection and validation
- [ ] T025 [US1] Implement interactive prompts for container platform selection (Kubernetes/ACI) using Bubble Tea
- [ ] T026 [US1] Implement interactive prompts for other resources platform selection (K8s/AWS/Azure)
- [ ] T027 [US1] Implement deployment tool detection (terraform CLI, bicep CLI)
- [ ] T028 [US1] Implement .env file creation/update with collected configuration
- [ ] T029 [US1] Implement default recipes.yaml generation in .radius/config/recipes/
- [ ] T030 [US1] Implement workspace switch to `git` in ~/.rad/config.yaml
- [ ] T031 [US1] Implement re-initialization warning when .radius/ exists
- [ ] T032 [US1] Implement success summary output with next steps
- [ ] T033 [US1] Implement error handling for network/auth failures during Resource Types fetch

**Checkpoint**: User Story 1 complete - `rad init` fully functional

---

## Phase 4: User Story 2 - Generate Deployment Artifacts (Priority: P1)

**Goal**: Enable developers to generate ready-to-deploy deployment artifacts from an application model

**Independent Test**: Run `rad plan <app.bicep> -e <env>` with a valid application model and verify deployment artifacts are generated in `.radius/plan/<app>/<env>/`

### Implementation for User Story 2

- [ ] T034 [US2] Create Plan and DeploymentStep structs in pkg/cli/git/plan/types.go
- [ ] T035 [P] [US2] Create RecipeVersion struct for change detection in pkg/cli/git/plan/types.go
- [ ] T036 [US2] Create rad plan command skeleton in pkg/cli/cmd/rad/plan.go
- [ ] T037 [US2] Implement Bicep file parsing to extract Application resource name
- [ ] T038 [US2] Implement environment auto-selection (single env) vs required flag (multiple envs)
- [ ] T039 [US2] Implement recipe lookup for each Application Resource type
- [ ] T040 [US2] Implement recipe version pinning validation (--allow-unpinned-recipes flag)
- [ ] T041 [US2] Implement Terraform artifact generation (main.tf, terraform.tfvars) in pkg/cli/git/plan/terraform.go
- [ ] T042 [US2] Implement terraform init execution using terraform-exec in pkg/cli/git/plan/terraform.go
- [ ] T043 [US2] Implement terraform plan execution and output capture in pkg/cli/git/plan/terraform.go
- [ ] T044 [US2] Implement terraform-context.log generation in pkg/cli/git/plan/terraform.go
- [ ] T045 [P] [US2] Implement Bicep artifact generation (.bicep, .bicepparam) in pkg/cli/git/plan/bicep.go
- [ ] T046 [P] [US2] Implement bicep build validation in pkg/cli/git/plan/bicep.go
- [ ] T047 [P] [US2] Implement az deployment group what-if execution in pkg/cli/git/plan/bicep.go
- [ ] T048 [P] [US2] Implement bicep-context.log generation in pkg/cli/git/plan/bicep.go
- [ ] T049 [US2] Implement plan.yaml manifest generation in pkg/cli/git/plan/manifest.go
- [ ] T050 [US2] Implement recipe version change detection and warning
- [ ] T051 [US2] Implement plan output directory structure (.radius/plan/<app>/<env>/<seq>-<name>-<tool>/)
- [ ] T052 [US2] Implement success summary output with next steps

**Checkpoint**: User Story 2 complete - `rad plan` fully functional

---

## Phase 5: User Story 3 - Deploy from Git (Priority: P1)

**Goal**: Enable developers to deploy infrastructure from their Git workspace with auditability and reproducibility

**Independent Test**: Run `rad plan`, commit changes, then run `rad deploy` and verify resources are deployed and details captured in `.radius/deploy/`

### Implementation for User Story 3

- [ ] T053 [US3] Create DeploymentRecord and related structs in pkg/cli/git/deploy/types.go
- [ ] T054 [P] [US3] Create ApplicationResourceResult and CloudResource structs in pkg/cli/git/deploy/types.go
- [ ] T055 [US3] Create rad deploy command skeleton in pkg/cli/cmd/rad/deploy_git.go
- [ ] T056 [US3] Implement environment variable validation (AWS_ACCOUNT_ID, AZURE_CLIENT_ID, etc.)
- [ ] T057 [US3] Implement Kubernetes configuration validation (kubeconfig, context, connectivity)
- [ ] T058 [US3] Implement uncommitted changes detection in .radius/plan/
- [ ] T059 [US3] Implement commit/tag argument parsing and validation
- [ ] T060 [US3] Implement GitHub Actions detection (GITHUB_ACTIONS=true, GITHUB_SHA)
- [ ] T061 [US3] Implement deployment confirmation prompt with environment details
- [ ] T062 [US3] Implement -y flag for non-interactive deployment
- [ ] T063 [US3] Implement Terraform deployment execution using terraform-exec in pkg/cli/git/deploy/executor.go
- [ ] T064 [US3] Implement Bicep deployment execution (az deployment group create) in pkg/cli/git/deploy/executor.go
- [ ] T065 [US3] Implement deployment progress display with spinner
- [ ] T066 [US3] Implement stop-on-failure semantics (no auto-rollback)
- [ ] T067 [US3] Implement Azure resource capture (az resource show) in pkg/cli/git/deploy/capture.go
- [ ] T068 [P] [US3] Implement AWS resource capture from Terraform state in pkg/cli/git/deploy/capture.go
- [ ] T069 [P] [US3] Implement Kubernetes resource capture via K8s API in pkg/cli/git/deploy/capture.go
- [ ] T070 [US3] Implement DeploymentRecord JSON generation in pkg/cli/git/deploy/record.go
- [ ] T071 [US3] Implement deployment record storage (.radius/deploy/<app>/<env>/deployment-<commit>.json)
- [ ] T072 [US3] Implement auto-staging of deployment records (git add .radius/deploy/)
- [ ] T073 [US3] Implement --commit flag for automatic commit after deployment
- [ ] T074 [US3] Implement --message/-m flag for custom commit message
- [ ] T075 [US3] Implement --quiet flag for suppressed progress output
- [ ] T076 [US3] Implement success summary output with next steps
- [ ] T077 [US3] Implement detailed error output for deployment failures

**Checkpoint**: User Story 3 complete - `rad deploy` fully functional

---

## Phase 6: User Story 4 - Perform Diffs (Priority: P1)

**Goal**: Enable developers to compare infrastructure state across the entire lifecycle and detect drift

**Independent Test**: Create multiple plan commits, deploy, then run `rad diff` with various source/target combinations and verify accurate comparisons

### Implementation for User Story 4

- [ ] T078 [US4] Create rad diff command skeleton in pkg/cli/cmd/rad/diff.go
- [ ] T079 [US4] Implement diff with no args (uncommitted changes in .radius/)
- [ ] T080 [US4] Implement single commit diff (commit vs live cloud)
- [ ] T081 [US4] Implement two commit diff (abc123...def456 syntax)
- [ ] T082 [US4] Implement commit artifact type detection (model only, plan, deployment record)
- [ ] T083 [US4] Implement model vs model comparison (Bicep file diff)
- [ ] T084 [US4] Implement plan vs plan comparison (IaC file diff)
- [ ] T085 [US4] Implement plan vs deployment comparison
- [ ] T086 [US4] Implement deployment vs live cloud comparison (drift detection) in pkg/cli/git/diff/differ.go
- [ ] T087 [US4] Implement Azure resource state query (ARM API) in pkg/cli/git/diff/azure.go
- [ ] T088 [P] [US4] Implement AWS resource state query (Terraform state) in pkg/cli/git/diff/aws.go
- [ ] T089 [P] [US4] Implement Kubernetes resource state query in pkg/cli/git/diff/kubernetes.go
- [ ] T090 [US4] Implement diff output formatting (modified/added/removed properties)
- [ ] T091 [US4] Implement --output json flag for machine-readable output
- [ ] T092 [US4] Implement --all-environments flag
- [ ] T093 [US4] Implement drift detection summary with reconciliation guidance
- [ ] T094 [US4] Implement warning for uncommitted deployment records

**Checkpoint**: User Story 4 complete - `rad diff` fully functional

---

## Phase 7: User Story 5 - Configure Multiple Environments (Priority: P2)

**Goal**: Enable developers to configure multiple deployment environments (dev, staging, production)

**Independent Test**: Create multiple `.env` files and verify `rad plan` and `rad deploy` respect the selected environment

### Implementation for User Story 5

- [ ] T095 [US5] Implement environment file discovery (scan for .env, .env.*)
- [ ] T096 [US5] Implement environment listing in rad init output
- [ ] T097 [US5] Update rad plan to support --environment flag with multiple environments
- [ ] T098 [US5] Update rad deploy to support --environment flag with multiple environments
- [ ] T099 [US5] Implement environment-specific recipe file support (RECIPES env var)
- [ ] T100 [US5] Implement TF_CLI_CONFIG_FILE support per environment
- [ ] T101 [US5] Implement TF_BACKEND_CONFIG support for Terraform backend configuration
- [ ] T102 [US5] Implement credential pattern detection warning

**Checkpoint**: User Story 5 complete - Multi-environment support functional

---

## Phase 8: User Story 7 - Workspace Management (Priority: P2)

**Goal**: Enable developers to switch between Git workspace and Control Plane workspace modes

**Independent Test**: Run `rad workspace list`, `rad workspace create`, and `rad workspace switch` and verify workspace switching works

### Implementation for User Story 7

- [ ] T103 [US7] Update rad workspace list to show git workspace as built-in
- [ ] T104 [US7] Update rad workspace switch to support git workspace
- [ ] T105 [US7] Implement workspace-aware command routing (git vs control plane)
- [ ] T106 [US7] Implement error messages for workspace-incompatible commands
- [ ] T107 [US7] Update ~/.rad/config.yaml handling for git workspace

**Checkpoint**: User Story 7 complete - Workspace switching functional

---

## Phase 9: User Story 8 - Run in GitHub Actions (Priority: P2)

**Goal**: Enable CI/CD engineers to run Git workspace in GitHub Actions workflows

**Independent Test**: Create a GitHub Actions workflow that runs `rad plan` and `rad deploy` and verify successful execution

### Implementation for User Story 8

- [ ] T108 [US8] Implement GITHUB_ACTIONS environment detection
- [ ] T109 [US8] Implement GITHUB_SHA automatic commit reference
- [ ] T110 [US8] Implement non-interactive mode (skip prompts when GITHUB_ACTIONS=true)
- [ ] T111 [US8] Implement exit code propagation for workflow step handling
- [ ] T112 [P] [US8] Create GitHub Action setup-rad for CLI installation (radius-project/setup-rad)
- [ ] T113 [P] [US8] Document GitHub Actions workflow examples in quickstart.md

**Checkpoint**: User Story 8 complete - GitHub Actions integration functional

---

## Phase 10: User Story 10 - Migrate Configuration to Control Plane (Priority: P3)

**Goal**: Enable platform engineers to migrate Git workspace configuration to Control Plane Radius

**Independent Test**: Run `rad migrate --workspace <name>` and verify configuration is migrated to Control Plane

### Implementation for User Story 10

- [ ] T114 [US10] Create rad migrate command skeleton in pkg/cli/cmd/rad/migrate.go
- [ ] T115 [US10] Implement Control Plane workspace validation
- [ ] T116 [US10] Implement migration preview display
- [ ] T117 [US10] Implement Resource Types migration
- [ ] T118 [US10] Implement recipe files migration
- [ ] T119 [US10] Implement environment configuration migration
- [ ] T120 [US10] Implement --dry-run flag
- [ ] T121 [US10] Implement conflict detection and resolution prompts
- [ ] T122 [US10] Implement migration success summary

**Checkpoint**: User Story 10 complete - Migration to Control Plane functional

---

## Phase 11: Polish & Cross-Cutting Concerns

**Purpose**: Improvements that affect multiple user stories

- [ ] T123 [P] Add comprehensive CLI help text for all commands
- [ ] T124 [P] Implement --quiet flag consistently across all commands
- [ ] T125 Code cleanup and Go linting fixes
- [ ] T126 [P] Add verbose logging (respecting TF_LOG forwarding per research.md)
- [ ] T127 Run quickstart.md validation end-to-end
- [ ] T128 Update existing rad command documentation
- [ ] T129 [P] Create CHANGELOG entries for breaking changes (rad init replacement)

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies - can start immediately
- **Foundational (Phase 2)**: Depends on Setup completion - BLOCKS all user stories
- **User Stories (Phases 3-10)**: All depend on Foundational phase completion
  - P1 stories (US1, US2, US3, US4) are critical path
  - P2 stories (US5, US7, US8) can proceed in parallel after P1
  - P3 story (US10) can proceed after P2
- **Polish (Phase 11)**: Depends on all desired user stories being complete

### User Story Dependencies

- **User Story 1 (P1) - rad init**: First entry point - no dependencies on other stories
- **User Story 2 (P1) - rad plan**: Depends on `rad init` creating directory structure
- **User Story 3 (P1) - rad deploy**: Depends on `rad plan` generating artifacts
- **User Story 4 (P1) - rad diff**: Depends on deployment records from `rad deploy`
- **User Story 5 (P2) - Multi-env**: Enhances US1-US4, can be implemented in parallel
- **User Story 7 (P2) - Workspaces**: Independent, enhances workspace switching
- **User Story 8 (P2) - GitHub Actions**: Depends on core commands (US1-US3)
- **User Story 10 (P3) - Migrate**: Depends on full feature set

### Within Each User Story

- Models/types before services
- Services before CLI commands
- Core implementation before integration
- Story complete before moving to next priority

### Parallel Opportunities

Within Phase 2 (Foundational):
- T007, T009 can run in parallel (config structs)
- T016, T17, T18, T19 can run in parallel (different packages)

Within User Story 2 (rad plan):
- T035 can run in parallel with T034
- T045, T046, T047, T048 can run in parallel with T041-T044 (Bicep vs Terraform)

Within User Story 3 (rad deploy):
- T054 can run in parallel with T053
- T067, T068, T069 can run in parallel (different capture targets)

Within User Story 4 (rad diff):
- T087, T088, T089 can run in parallel (different cloud targets)

---

## Parallel Example: User Story 2

```bash
# Launch Terraform and Bicep artifact generation in parallel:
Task: T041 "Implement Terraform artifact generation in pkg/cli/git/plan/terraform.go"
Task: T045 "Implement Bicep artifact generation in pkg/cli/git/plan/bicep.go"

# Launch context log generation in parallel:
Task: T044 "Implement terraform-context.log generation"
Task: T048 "Implement bicep-context.log generation"
```

---

## Implementation Strategy

### MVP First (User Stories 1-3)

1. Complete Phase 1: Setup
2. Complete Phase 2: Foundational (CRITICAL - blocks all stories)
3. Complete Phase 3: User Story 1 (rad init)
4. Complete Phase 4: User Story 2 (rad plan)
5. Complete Phase 5: User Story 3 (rad deploy)
6. **STOP and VALIDATE**: Test full workflow end-to-end
7. Deploy/demo if ready

### Incremental Delivery

1. Setup + Foundational → Foundation ready
2. Add User Story 1 (rad init) → Test independently → First milestone!
3. Add User Story 2 (rad plan) → Test plan generation
4. Add User Story 3 (rad deploy) → Full deployment workflow functional (MVP!)
5. Add User Story 4 (rad diff) → Drift detection capability
6. Add P2 stories → Enhanced workflows
7. Add P3 story → Migration path complete

### Task Count Summary

| Phase | User Story | Task Count |
|-------|------------|------------|
| Phase 1 | Setup | 5 |
| Phase 2 | Foundational | 14 |
| Phase 3 | US1 - rad init | 14 |
| Phase 4 | US2 - rad plan | 19 |
| Phase 5 | US3 - rad deploy | 25 |
| Phase 6 | US4 - rad diff | 17 |
| Phase 7 | US5 - Multi-env | 8 |
| Phase 8 | US7 - Workspaces | 5 |
| Phase 9 | US8 - GitHub Actions | 6 |
| Phase 10 | US10 - Migrate | 9 |
| Phase 11 | Polish | 7 |
| **Total** | | **129** |

---

## Notes

- [P] tasks = different files, no dependencies
- [Story] label maps task to specific user story for traceability
- Each user story should be independently completable and testable
- Commit after each task or logical group
- Stop at any checkpoint to validate story independently
- terraform-exec automatically inherits user's AWS/Azure credentials (see research.md)
- TF_LOG must be explicitly forwarded via tf.SetLog() (see research.md)
