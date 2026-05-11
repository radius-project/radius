# Tasks: Git App Graph Preview

**Input**: Design documents from `/specs/001-git-app-graph-preview/`
**Prerequisites**: plan.md ✓, spec.md ✓, research.md ✓, data-model.md ✓, contracts/ ✓, quickstart.md ✓

**Tests**: Included per spec.md "Testing Requirements" section - comprehensive testing across the testing pyramid.

**Organization**: Tasks grouped by user story for independent implementation and testing.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (US1, US2, US3, US4, US5, US6)
- Exact file paths for radius repository

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Project initialization and scaffolding for the static graph feature

- [ ] T001 Create package structure `pkg/cli/bicep/` with package doc in pkg/cli/bicep/doc.go
- [ ] T002 Create package structure `pkg/cli/git/` with package doc in pkg/cli/git/doc.go
- [ ] T003 [P] Add new test directories: test/unit/cli/graph/, test/integration/cli/graph/, test/functional/cli/graph/
- [ ] T004 [P] Define AppGraph types in pkg/corerp/api/v20231001preview/appgraph_static_types.go
- [ ] T005 [P] Define GraphDiff types in pkg/corerp/api/v20231001preview/appgraph_diff_types.go
- [ ] T006 Add JSON schema validation helpers in pkg/cli/output/json.go (deterministic key ordering)

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Core infrastructure required by ALL user stories

**⚠️ CRITICAL**: No user story work can begin until this phase is complete

- [ ] T007 Implement Bicep CLI executor in pkg/cli/bicep/executor.go (wraps `bicep build --stdout`)
- [ ] T008 Implement ARM JSON parser interface in pkg/cli/bicep/parser.go (extract resources array)
- [ ] T009 Implement resource extraction from ARM JSON in pkg/cli/bicep/extractor.go
- [ ] T010 Implement connection detection from resource properties in pkg/cli/bicep/connections.go
- [ ] T011 [P] Implement source hash computation (SHA256 of input files) in pkg/cli/bicep/hash.go
- [ ] T012 [P] Add Bicep file detection logic (`.bicep` extension) in pkg/cli/cmd/app/graph/detect.go
- [ ] T013 Unit tests for Bicep executor in test/unit/cli/graph/executor_test.go
- [ ] T014 Unit tests for ARM JSON parser in test/unit/cli/graph/parser_test.go

**Checkpoint**: Foundation ready - user story implementation can begin

---

## Phase 3: User Story 1 - Generate App Graph from Bicep Files (Priority: P1) 🎯 MVP

**Goal**: Enable `rad app graph app.bicep` to generate a JSON app graph from Bicep files without deployment

**Independent Test**: Run CLI against sample Bicep file, verify JSON output contains expected resources and connections

### Tests for User Story 1

- [ ] T015 [P] [US1] Unit test for graph generation with valid Bicep in test/unit/cli/graph/static_test.go
- [ ] T016 [P] [US1] Unit test for error handling with invalid Bicep in test/unit/cli/graph/static_errors_test.go
- [ ] T017 [P] [US1] Unit test for module resolution in test/unit/cli/graph/modules_test.go
- [ ] T018 [US1] Integration test with real Bicep CLI in test/integration/cli/graph/bicep_integration_test.go

### Implementation for User Story 1

- [ ] T019 [US1] Implement static graph generator in pkg/cli/cmd/app/graph/static.go
- [ ] T020 [US1] Add file input detection to existing graph.go entry point in pkg/cli/cmd/app/graph/graph.go
- [ ] T021 [US1] Implement module resolution (transitive Bicep imports) in pkg/cli/bicep/modules.go
- [ ] T022 [US1] Add parameter file support (`--parameters`) in pkg/cli/cmd/app/graph/params.go
- [ ] T023 [US1] Implement required parameter validation (fail if missing) in pkg/cli/bicep/params.go
- [ ] T024 [US1] Add Radius Bicep extension type detection in pkg/cli/bicep/radius_types.go
- [ ] T025 [US1] Implement error handling with line/column info in pkg/cli/bicep/errors.go
- [ ] T026 [US1] Add logging for graph generation operations in pkg/cli/cmd/app/graph/static.go

**Checkpoint**: `rad app graph app.bicep` generates valid JSON graph from Bicep files

---

## Phase 4: User Story 2 - Export Graph as Diff-Friendly Format (Priority: P1)

**Goal**: Output deterministic JSON to `.radius/app-graph.json` and optional Markdown with Mermaid diagrams

**Independent Test**: Generate graph twice from identical input, verify byte-identical output; verify Markdown renders in GitHub

### Tests for User Story 2

- [ ] T027 [P] [US2] Unit test for deterministic JSON output in test/unit/cli/graph/json_determinism_test.go
- [ ] T028 [P] [US2] Unit test for Mermaid diagram generation in test/unit/cli/graph/mermaid_test.go
- [ ] T029 [P] [US2] Unit test for Markdown table formatting in test/unit/cli/graph/markdown_test.go
- [ ] T030 [US2] Integration test for file output paths in test/integration/cli/graph/output_test.go

### Implementation for User Story 2

- [ ] T031 [US2] Implement deterministic JSON serializer in pkg/cli/output/deterministic_json.go
- [ ] T032 [US2] Implement default output path (`.radius/app-graph.json`) in pkg/cli/cmd/app/graph/output.go
- [ ] T033 [US2] Add `--stdout` flag for stdout output in pkg/cli/cmd/app/graph/flags.go
- [ ] T034 [US2] Add `-o` flag for custom output path in pkg/cli/cmd/app/graph/flags.go
- [ ] T035 [US2] Implement Mermaid diagram generator in pkg/cli/output/mermaid.go
- [ ] T036 [US2] Implement Markdown table formatter in pkg/cli/output/markdown.go
- [ ] T037 [US2] Add `--format markdown` flag (generates both JSON + Markdown) in pkg/cli/cmd/app/graph/flags.go
- [ ] T038 [US2] Integrate display.go with new output formatters in pkg/cli/cmd/app/graph/display.go
- [ ] T039 [US2] Add Mermaid shape mapping (containers→rectangles, gateways→diamonds, databases→cylinders) in pkg/cli/output/mermaid.go

**Checkpoint**: Deterministic JSON output + optional Markdown with Mermaid diagrams working

---

## Phase 5: User Story 3 - Git Metadata Enrichment (Priority: P2)

**Goal**: Automatically enrich each resource with git commit information (SHA, author, date, message)

**Independent Test**: Generate graph in git repo, verify each resource has git metadata; verify `--no-git` skips enrichment

### Tests for User Story 3

- [ ] T040 [P] [US3] Unit test for git blame parsing in test/unit/cli/graph/git_blame_test.go
- [ ] T041 [P] [US3] Unit test for git log parsing in test/unit/cli/graph/git_log_test.go
- [ ] T042 [P] [US3] Unit test for uncommitted file detection in test/unit/cli/graph/git_uncommitted_test.go
- [ ] T043 [US3] Integration test with real git repository in test/integration/cli/graph/git_integration_test.go

### Implementation for User Story 3

- [ ] T044 [US3] Implement git repository detection in pkg/cli/git/repo.go
- [ ] T045 [US3] Implement git blame executor in pkg/cli/git/blame.go
- [ ] T046 [US3] Implement git log metadata extraction in pkg/cli/git/log.go
- [ ] T047 [US3] Implement per-resource git info enrichment in pkg/cli/git/metadata.go
- [ ] T048 [US3] Add uncommitted changes detection in pkg/cli/git/status.go
- [ ] T049 [US3] Add `--no-git` flag for faster execution in pkg/cli/cmd/app/graph/flags.go
- [ ] T050 [US3] Handle non-git directories gracefully in pkg/cli/git/metadata.go
- [ ] T051 [US3] Handle shallow clones (mark as "history unavailable") in pkg/cli/git/blame.go
- [ ] T052 [US3] Add linked commit SHA in Markdown output in pkg/cli/output/markdown.go

**Checkpoint**: Graph resources include git metadata by default; `--no-git` works

---

## Phase 6: User Story 4 - GitHub Action for PR Graph Diff (Priority: P2)

**Goal**: GitHub Action reads committed JSON from git history and posts diff comments on PRs

**Independent Test**: Create PR with graph changes, verify Action posts comment with change table and Mermaid diagrams

### Tests for User Story 4

- [ ] T053 [P] [US4] Unit test for JSON-to-JSON diff computation in test/unit/cli/graph/diff_test.go
- [ ] T054 [P] [US4] Unit test for diff summary generation in test/unit/cli/graph/diff_summary_test.go
- [ ] T055 [US4] Unit test for diff Markdown rendering in test/unit/cli/graph/diff_render_test.go

### Implementation for User Story 4

- [ ] T056 [US4] Implement graph diff computation in pkg/cli/cmd/app/graph/diff.go
- [ ] T057 [US4] Implement resource comparison (by ID) in pkg/cli/cmd/app/graph/diff.go
- [ ] T058 [US4] Implement connection comparison (by source+target tuple) in pkg/cli/cmd/app/graph/diff.go
- [ ] T059 [US4] Implement diff summary statistics in pkg/cli/cmd/app/graph/diff.go
- [ ] T060 [US4] Implement diff Markdown renderer (change table + before/after Mermaid) in pkg/cli/output/diff_markdown.go
- [ ] T061 [US4] Create GitHub Action definition in actions/app-graph-diff/action.yml
- [ ] T062 [US4] Implement diff computation shell script in actions/app-graph-diff/diff.sh
- [ ] T063 [US4] Implement Mermaid rendering for Action in actions/app-graph-diff/render.js
- [ ] T064 [US4] Add PR comment create-or-update logic in actions/app-graph-diff/comment.sh
- [ ] T065 [US4] Add monorepo support (glob `**/.radius/app-graph.json`) in actions/app-graph-diff/detect.sh
- [ ] T066 [US4] Add staleness validation (compare committed vs regenerated) in actions/app-graph-diff/validate.sh
- [ ] T067 [US4] Add `push` trigger support for baseline tracking in actions/app-graph-diff/action.yml

**Checkpoint**: GitHub Action posts diff comments on PRs with graph changes

---

## Phase 7: User Story 5 - Historical Graph Timeline (Priority: P3)

**Goal**: Enable `rad app graph history` to show graph evolution across commits

**Independent Test**: Run history command on repo with multiple commits, verify timeline shows graph changes

### Tests for User Story 5

- [ ] T068 [P] [US5] Unit test for timeline generation in test/unit/cli/graph/history_test.go
- [ ] T069 [US5] Integration test with multi-commit git history in test/integration/cli/graph/history_integration_test.go

### Implementation for User Story 5

- [ ] T070 [US5] Implement `rad app graph history` subcommand in pkg/cli/cmd/app/graph/history.go
- [ ] T071 [US5] Add `--commits N` flag in pkg/cli/cmd/app/graph/history.go
- [ ] T072 [US5] Implement `--at <commit>` flag for graph at specific commit in pkg/cli/cmd/app/graph/flags.go
- [ ] T073 [US5] Implement `rad app graph diff --from X --to Y` subcommand in pkg/cli/cmd/app/graph/diff.go
- [ ] T074 [US5] Implement commit range iteration in pkg/cli/git/history.go
- [ ] T075 [US5] Implement change summary per commit in pkg/cli/cmd/app/graph/history.go

**Checkpoint**: Historical timeline and commit-specific graph generation working

---

## Phase 8: User Story 6 - Environment-Resolved Graph (Priority: P3)

**Goal**: Enable `rad app graph --environment` to show resolved recipe types instead of abstract Radius types

**Independent Test**: Generate graph with environment flag, verify abstract types resolve to concrete infrastructure types

### Tests for User Story 6

- [ ] T076 [P] [US6] Unit test for recipe type resolution in test/unit/cli/graph/resolve_test.go
- [ ] T077 [US6] Integration test with Radius environment in test/integration/cli/graph/environment_test.go

### Implementation for User Story 6

- [ ] T078 [US6] Add `--environment` flag in pkg/cli/cmd/app/graph/flags.go
- [ ] T079 [US6] Implement environment connection in pkg/cli/cmd/app/graph/environment.go
- [ ] T080 [US6] Implement recipe lookup from environment in pkg/cli/cmd/app/graph/recipes.go
- [ ] T081 [US6] Implement portable type → concrete type resolution in pkg/cli/cmd/app/graph/resolve.go
- [ ] T082 [US6] Add "no recipe bound" annotation for unbound types in pkg/cli/cmd/app/graph/resolve.go
- [ ] T083 [US6] Integrate resolved types into graph output in pkg/cli/cmd/app/graph/static.go

**Checkpoint**: Environment-resolved graphs show concrete infrastructure types

---

## Phase 9: Polish & Cross-Cutting Concerns

**Purpose**: Documentation, validation, and refinements

- [ ] T084 [P] Update pkg/cli/cmd/app/graph/README.md with new command documentation
- [ ] T085 [P] Add godoc comments to all exported types in pkg/cli/bicep/
- [ ] T086 [P] Add godoc comments to all exported types in pkg/cli/git/
- [ ] T087 Run golangci-lint and fix any issues in new code
- [ ] T088 [P] Create functional E2E test in test/functional/cli/graph/e2e_test.go
- [ ] T089 Validate quickstart.md scenarios work end-to-end
- [ ] T090 Update CHANGELOG.md with new feature description
- [ ] T091 [P] Create GitHub Action README in actions/app-graph-diff/README.md

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies - can start immediately
- **Foundational (Phase 2)**: Depends on Setup - BLOCKS all user stories
- **US1 (Phase 3)**: Depends on Foundational - First MVP
- **US2 (Phase 4)**: Depends on Foundational - Second MVP capability
- **US3 (Phase 5)**: Depends on US1/US2 working - Enrichment layer
- **US4 (Phase 6)**: Depends on US2 (diff-friendly output) + US3 (git metadata)
- **US5 (Phase 7)**: Depends on US3 (git integration)
- **US6 (Phase 8)**: Can start after US1 - independent branch
- **Polish (Phase 9)**: After all desired user stories complete

### User Story Dependencies

```
Foundational (Phase 2)
        │
        ├──────────────┬──────────────┐
        ▼              ▼              ▼
    US1 (P1)       US2 (P1)       US6 (P3)
        │              │         (independent)
        └──────┬───────┘
               ▼
           US3 (P2)
               │
        ┌──────┴──────┐
        ▼             ▼
    US4 (P2)      US5 (P3)
```

### Parallel Opportunities

**Phase 1 (Setup)**: T001, T002 sequential; T003-T006 all parallel
**Phase 2 (Foundational)**: T007-T010 sequential; T011-T012 parallel; T013-T014 parallel
**Each User Story**: All tests marked [P] can run in parallel; implementation tasks mostly sequential within story

---

## Parallel Example: User Story 1 Tests

```bash
# Launch all US1 tests in parallel:
Task: "T015 [P] [US1] Unit test for graph generation with valid Bicep"
Task: "T016 [P] [US1] Unit test for error handling with invalid Bicep"
Task: "T017 [P] [US1] Unit test for module resolution"

# Then run integration test:
Task: "T018 [US1] Integration test with real Bicep CLI"
```

---

## Implementation Strategy

### MVP First (US1 + US2 = P1 Stories)

1. Complete Phase 1: Setup
2. Complete Phase 2: Foundational (CRITICAL)
3. Complete Phase 3: User Story 1 (core graph generation)
4. Complete Phase 4: User Story 2 (diff-friendly output)
5. **STOP and VALIDATE**: Run quickstart.md scenarios locally
6. Deploy CLI changes for developer feedback

### Git Integration (P2 Stories)

1. Add US3: Git metadata enrichment
2. Add US4: GitHub Action for PR diffs
3. **VALIDATE**: Test in real PR workflow

### Advanced Features (P3 Stories)

1. Add US5: Historical timeline (optional)
2. Add US6: Environment resolution (optional)
3. Final polish phase

### Parallel Team Strategy

With multiple developers:
- **Developer A**: Phase 1 → Phase 2 → US1 → US3 → US5
- **Developer B**: Phase 1 (parallel) → US2 → US4 (GitHub Action)
- **Developer C**: US6 (can start after Phase 2)

---

## Notes

- Radius repository is at `../radius` relative to design-notes
- All Go code must pass `golangci-lint` (Constitution Principle II)
- All exported types need godoc comments (NFR-002)
- JSON output must be deterministic (FR-004, SC-002)
- Existing `rad app graph <appname>` behavior must not change (FR-003)
- GitHub Action is lightweight (reads JSON only, no Bicep tooling)
