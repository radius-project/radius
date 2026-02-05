# Tasks: Automatic Application Discovery

**Input**: Design documents from `/specs/001-auto-app-discovery/`  
**Prerequisites**: plan.md ✓, spec.md ✓, research.md ✓, data-model.md ✓, contracts/ ✓, quickstart.md ✓

## Format: `[ID] [P?] [Story?] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2)
- Include exact file paths in descriptions

> **Note**: Phases are ordered by **priority** (P1 → P2 → P3), not by user story number.
> Phase 3 = US1, Phase 4 = US2, Phase 5 = US4, Phase 6 = US6 (all P1),
> then Phase 7 = US3, Phase 8 = US7 (P2), Phase 9 = US5 (P3).

---

## Phase 1: Setup

**Purpose**: Project initialization and test infrastructure

- [X] T001 Create pkg/discovery/ directory structure per plan.md
- [X] T002 [P] Create pkg/discovery/types.go with core type definitions from data-model.md (DiscoveryResult, Service, DetectedDependency, Evidence, etc.)
- [X] T003 [P] Create pkg/discovery/errors.go with discovery-specific error types
- [X] T004 [P] Create pkg/mcp/ directory structure for MCP server
- [X] T005 [P] Create test/functional-portable/discovery/ directory with testdata folders (nodejs-app/, python-app/, go-app/)
- [X] T006 Create test/functional-portable/discovery/testdata/nodejs-app/ sample project with package.json containing pg, ioredis dependencies
- [X] T007 [P] Create test/functional-portable/discovery/testdata/python-app/ sample project with requirements.txt containing psycopg2, redis dependencies
- [X] T008 [P] Create test/functional-portable/discovery/testdata/go-app/ sample project with go.mod containing lib/pq, go-redis dependencies

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Core infrastructure that MUST be complete before ANY user story implementation

**⚠️ CRITICAL**: No user story work can begin until this phase is complete

- [X] T009 Create pkg/discovery/analyzers/analyzer.go with Analyzer interface (Analyze(ctx, path) → []DetectedDependency)
- [X] T010 Create pkg/discovery/analyzers/registry.go with analyzer registry pattern for language detection
- [X] T011 [P] Create pkg/discovery/catalog/catalog.go with infrastructure library catalog loader (YAML → map[library]DependencyType)
- [X] T012 [P] Create pkg/discovery/catalog/libraries.yaml with initial library-to-dependency mappings from research.md
- [X] T013 [P] Create pkg/discovery/skills/skill.go with Skill interface (Execute(ctx, input) → output, error)
- [X] T014 Create pkg/discovery/output/markdown.go with Markdown output generator for discovery.md
- [X] T015 [P] Create pkg/discovery/resourcetypes/catalog.go with pre-defined Resource Type catalog (OQ-1 Option A)
- [X] T016 [P] Create pkg/discovery/resourcetypes/types.yaml with Resource Type definitions for PostgreSQL, MySQL, Redis, MongoDB, RabbitMQ, Kafka, Azure Blob, S3

**Checkpoint**: Foundation ready - user story implementation can begin

---

## Phase 3: User Story 1 - Analyze Existing Codebase (Priority: P1) 🎯 MVP

**Goal**: Developers can run `rad app discover` to detect infrastructure dependencies from their codebase

**Independent Test**: Point Radius at sample codebase with known dependencies and verify correct detection

### Implementation for User Story 1

- [X] T017 [P] [US1] Create pkg/discovery/analyzers/manifest/package_json.go to parse package.json for JS/TS dependencies
- [X] T018 [P] [US1] Create pkg/discovery/analyzers/manifest/requirements.go to parse requirements.txt for Python dependencies
- [X] T019 [P] [US1] Create pkg/discovery/analyzers/manifest/go_mod.go to parse go.mod for Go dependencies
- [X] T020 [P] [US1] Create pkg/discovery/analyzers/manifest/pom.go to parse pom.xml for Java dependencies
- [X] T021 [P] [US1] Create pkg/discovery/analyzers/manifest/csproj.go to parse .csproj for C# dependencies
- [X] T022 [US1] Create pkg/discovery/analyzers/javascript.go implementing Analyzer for JavaScript/TypeScript (uses package_json.go)
- [X] T023 [P] [US1] Create pkg/discovery/analyzers/python.go implementing Analyzer for Python (uses requirements.go)
- [X] T024 [P] [US1] Create pkg/discovery/analyzers/golang.go implementing Analyzer for Go (uses go_mod.go)
- [X] T025 [P] [US1] Create pkg/discovery/analyzers/java.go implementing Analyzer for Java (uses pom.go)
- [X] T026 [P] [US1] Create pkg/discovery/analyzers/csharp.go implementing Analyzer for C# (uses csproj.go)
- [X] T027 [US1] Create pkg/discovery/skills/discover_dependencies.go implementing discover_dependencies skill
- [X] T028 [US1] Create pkg/discovery/skills/discover_services.go implementing discover_services skill with entrypoint detection
- [X] T029 [US1] Create pkg/discovery/engine.go orchestrating discovery workflow (call analyzers → aggregate results)
- [X] T030 [US1] Create pkg/cli/cmd/app/discover.go implementing `rad app discover` command using Cobra
- [X] T031 [US1] Add unit tests for manifest parsers in pkg/discovery/analyzers/manifest/*_test.go
- [X] T032 [US1] Add unit tests for analyzers in pkg/discovery/analyzers/*_test.go
- [X] T033 [US1] Create test/functional-portable/discovery/discover_test.go functional test with sample projects

**Checkpoint**: `rad app discover .` works and outputs ./radius/discovery.md with detected dependencies

---

## Phase 4: User Story 2 - Map to Resource Types (Priority: P1)

**Goal**: Detected dependencies are automatically mapped to valid Radius Resource Types

**Independent Test**: After dependency detection, verify valid Resource Type definitions are produced for each detected dependency

### Implementation for User Story 2

- [X] T034 [P] [US2] Create pkg/discovery/resourcetypes/matcher.go to match DetectedDependency → ResourceType from catalog
- [X] T035 [US2] Create pkg/discovery/skills/generate_resource_types.go implementing generate_resource_types skill
- [X] T036 [US2] Update pkg/discovery/engine.go to include Resource Type mapping in discovery workflow
- [X] T037 [US2] Update pkg/discovery/output/markdown.go to include Resource Type mappings in discovery.md
- [X] T038 [US2] Add unit tests for matcher in pkg/discovery/resourcetypes/matcher_test.go
- [X] T039 [US2] Add unit tests for generate_resource_types skill in pkg/discovery/skills/generate_resource_types_test.go

**Checkpoint**: Discovery output includes mapped Resource Types for each detected dependency

---

## Phase 5: User Story 4 - Generate Application Definition (Priority: P1)

**Goal**: Developers can run `rad app generate` to create a deployable app.bicep

**Independent Test**: Generate app.bicep from sample discovery output and verify it deploys with Radius

### Implementation for User Story 4

- [X] T040 [P] [US4] Create pkg/discovery/output/bicep.go with Bicep template generation for app.bicep
- [X] T041 [P] [US4] Create pkg/discovery/output/templates/ directory with Bicep template fragments
- [X] T042 [US4] Create pkg/discovery/output/templates/application.bicep.tmpl for application resource
- [X] T043 [P] [US4] Create pkg/discovery/output/templates/container.bicep.tmpl for container resources
- [X] T044 [P] [US4] Create pkg/discovery/output/templates/resource.bicep.tmpl for infrastructure resources
- [X] T045 [US4] Create pkg/discovery/skills/generate_app_definition.go implementing generate_app_definition skill
- [X] T046 [US4] Create pkg/discovery/skills/validate_app_definition.go implementing validate_app_definition skill (calls bicep build)
- [X] T047 [US4] Create pkg/cli/cmd/app/generate/generate.go implementing `rad app generate` command using Cobra
- [X] T048 [US4] Add unit tests for bicep generator in pkg/discovery/output/bicep_test.go
- [X] T049 [US4] Add unit tests for generate_app_definition skill in pkg/discovery/skills/generate_app_definition_test.go
- [X] T050 [US4] Create test/functional-portable/discovery/generate_test.go functional test for generate workflow

**Checkpoint**: `rad app generate` creates valid ./radius/app.bicep from discovery results

---

## Phase 6: User Story 6 - AI Coding Agent Integration (Priority: P1)

**Goal**: All skills exposed via MCP server for AI agent integration

**Independent Test**: Invoke each skill via MCP and verify structured JSON output

### Implementation for User Story 6

- [X] T051 [P] [US6] Create pkg/mcp/server.go with MCP server core implementation
- [X] T052 [P] [US6] Create pkg/mcp/protocol.go with JSON-RPC 2.0 message types
- [X] T053 [US6] Create pkg/mcp/handlers.go mapping skills to MCP tools
- [X] T054 [P] [US6] Create pkg/mcp/stdio.go for VS Code extension integration
- [X] T055 [P] [US6] Create pkg/mcp/transports/http.go for remote agent integration
- [X] T056 [US6] Create pkg/mcp/tools.go with MCP tool definitions from contracts/mcp-tools.json
- [X] T057 [US6] Create pkg/cli/cmd/mcp/serve.go implementing `rad mcp serve` command
- [X] T058 [US6] Add unit tests for MCP server in pkg/mcp/server_test.go
- [X] T059 [US6] Add unit tests for handlers in pkg/mcp/handlers_test.go
- [X] T059.1 [US6] Add concurrency test for concurrent skill invocations (NFR-06) in pkg/mcp/server_test.go
- [X] T060 [US6] Create test/functional-portable/discovery/mcp_test.go functional test for MCP tools

**Checkpoint**: `rad mcp serve` starts MCP server; AI agents can invoke skills via MCP protocol

---

## Phase 7: User Story 3 - Match Recipes from Sources (Priority: P2)

**Goal**: Platform engineers can configure recipe sources and get recipe suggestions for detected dependencies

**Independent Test**: Configure AVM and internal sources, verify detected dependencies are matched

### Implementation for User Story 3

- [X] T061 [P] [US3] Create pkg/discovery/recipes/source.go with RecipeSource interface
- [X] T062 [P] [US3] Create pkg/discovery/recipes/avm.go implementing AVM recipe source
- [X] T063 [P] [US3] Create pkg/discovery/recipes/terraform.go implementing Terraform registry source
- [X] T064 [P] [US3] Create pkg/discovery/recipes/git.go implementing Git repository source
- [X] T065 [US3] Create pkg/discovery/recipes/matcher.go with recipe matching logic
- [X] T066 [US3] Create pkg/discovery/skills/discover_recipes.go implementing discover_recipes skill
- [X] T067 [US3] Create pkg/discovery/config/sources.go with recipe source configuration loading (including auth via env vars, credential helpers, tokens per FR-37)
- [X] T067.1 [P] [US3] Add --recipe-profile flag to select environment-specific recipe sets (FR-33)
- [X] T068 [US3] Create pkg/cli/cmd/recipe/source.go implementing `rad recipe source add` command
- [X] T069 [US3] Update pkg/cli/cmd/app/generate.go to include interactive recipe selection
- [X] T070 [US3] Add unit tests for recipe sources in pkg/discovery/recipes/*_test.go
- [X] T071 [US3] Add unit tests for discover_recipes skill in pkg/discovery/skills/discover_recipes_test.go
- [X] T071.1 [US3] Create test/functional-portable/discovery/recipes_test.go functional test

**Checkpoint**: Recipe sources configurable; detected dependencies matched to recipes from AVM/internal sources

---

## Phase 8: User Story 7 - Apply Team Practices (Priority: P2)

**Goal**: Team infrastructure practices (naming, tags, sizing) automatically applied to generated output

**Independent Test**: Configure team practices, verify generated Resource Types incorporate those practices

### Implementation for User Story 7

- [X] T072 [P] [US7] Create pkg/discovery/practices/types.go with TeamPractices, NamingPattern, PracticeSource types
- [X] T073 [P] [US7] Create pkg/discovery/practices/config.go to load .radius/team-practices.yaml
- [X] T074 [US7] Create pkg/discovery/practices/terraform.go to extract practices from Terraform files
- [X] T075 [P] [US7] Create pkg/discovery/practices/bicep.go to extract practices from Bicep files
- [X] T076 [US7] Create pkg/discovery/skills/discover_team_practices.go implementing discover_team_practices skill
- [X] T077 [US7] Update pkg/discovery/skills/generate_resource_types.go to apply team practices to Resource Type defaults
- [X] T078 [US7] Update pkg/discovery/output/bicep.go to apply naming conventions in generated output
- [X] T079 [US7] Add unit tests for practices parsers in pkg/discovery/practices/*_test.go
- [X] T080 [US7] Add unit tests for discover_team_practices skill in pkg/discovery/skills/discover_team_practices_test.go
- [X] T081 [US7] Create test/functional-portable/discovery/practices_test.go functional test with sample IaC

**Checkpoint**: Team practices detected from IaC and config; applied to generated Resource Types and app.bicep

---

## Phase 9: User Story 5 - New Application Scaffolding (Priority: P3)

**Goal**: Developers can scaffold new applications with `rad app scaffold`

**Independent Test**: Run scaffold command with infrastructure selections, verify deployable app.bicep created

### Implementation for User Story 5

- [X] T082 [US5] Create pkg/cli/cmd/app/scaffold.go implementing `rad app scaffold` command
- [X] T083 [US5] Create pkg/discovery/scaffold/templates.go with scaffolding templates for new apps
- [X] T084 [US5] Update scaffold command to support --add-dependency flag for manual specification (FR-24)
- [X] T084.1 [US5] Add --add-dependency flag to rad app generate command (FR-24) in pkg/cli/cmd/app/generate.go
- [X] T085 [US5] Add unit tests for scaffold command in pkg/cli/cmd/app/scaffold_test.go

**Checkpoint**: `rad app scaffold` creates new application structure with selected infrastructure

---

## Phase 10: Polish & Cross-Cutting Concerns

**Purpose**: Improvements that affect multiple user stories

- [X] T086 [P] Add progress indicators for long-running operations (NFR-02) in pkg/discovery/progress.go
- [X] T087 [P] Add structured JSON logging (NFR-05) to all discovery operations
- [X] T088 [P] Add --accept-defaults flag handling (FR-32) to all interactive commands
- [X] T089 [P] Add --output flag (FR-34) to customize output paths
- [X] T090 Implement --update flag (FR-20) for diff/patch mode in generate command
- [X] T091 Implement existing app.bicep conflict handling (FR-21) with overwrite/merge/diff options
- [X] T092 Add confidence threshold filtering (FR-05) with visual distinction for high/medium/low
- [X] T093 [P] Update docs/ with discovery feature documentation
- [X] T094 Run quickstart.md validation with sample projects
- [X] T095 Performance optimization: ensure ≤100 files analyzed in <30s (NFR-01)

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies - start immediately
- **Foundational (Phase 2)**: Depends on Setup - BLOCKS all user stories
- **US1 (Phase 3)**: Depends on Foundational - core discovery capability
- **US2 (Phase 4)**: Depends on Foundational + catalog from T015-T016
- **US4 (Phase 5)**: Depends on US1 + US2 (needs discovery results and Resource Types)
- **US6 (Phase 6)**: Depends on Foundational + at least some skills implemented
- **US3 (Phase 7)**: Depends on Foundational - can parallel with US1/US2
- **US7 (Phase 8)**: Depends on US2 (needs Resource Type generation)
- **US5 (Phase 9)**: Depends on US4 (needs generation capability)
- **Polish (Phase 10)**: Depends on core user stories complete

### User Story Dependencies

- **US1 (Discover)**: No story dependencies - can start after Foundational
- **US2 (Resource Types)**: Can parallel with US1 (same phase dependency)
- **US4 (Generate)**: Depends on US1 + US2 outputs
- **US6 (MCP)**: Depends on skills existing but can parallel with UI polish
- **US3 (Recipes)**: Independent of US1/US2; integrates with US4
- **US7 (Practices)**: Integrates with US2; can parallel with US3
- **US5 (Scaffold)**: Depends on US4

### Parallel Opportunities

**Phase 1 (Setup)**:
```
T002, T003, T004, T005 in parallel
T006, T007, T008 in parallel
```

**Phase 2 (Foundational)**:
```
T011, T012, T013 in parallel
T015, T016 in parallel
```

**Phase 3 (US1 - Discover)**:
```
T017, T018, T019, T020, T021 in parallel (manifest parsers)
T022, T023, T024, T025, T026 in parallel (after parsers complete)
```

**Phase 6 (US6 - MCP)**:
```
T051, T052 in parallel
T054, T055 in parallel
```

---

## Implementation Strategy

### MVP First (User Stories 1 + 2 + 4)

1. Complete Phase 1: Setup
2. Complete Phase 2: Foundational
3. Complete Phase 3: US1 - Discover
4. Complete Phase 4: US2 - Resource Types
5. Complete Phase 5: US4 - Generate
6. **VALIDATE**: Test discover → generate workflow end-to-end
7. Deploy MVP - developers can go from codebase to app.bicep

### Add AI Integration

8. Complete Phase 6: US6 - MCP Server
9. **VALIDATE**: AI agents can invoke skills via MCP

### Add Platform Engineer Features

10. Complete Phase 7: US3 - Recipe Sources
11. Complete Phase 8: US7 - Team Practices
12. **VALIDATE**: Full enterprise workflow with custom recipes and practices

### Final Polish

13. Complete Phase 9: US5 - Scaffold
14. Complete Phase 10: Polish
15. Full release

---

## Task Summary

| Phase | User Story | Task Count | Priority |
|-------|------------|------------|----------|
| 1 | Setup | 8 | - |
| 2 | Foundational | 8 | - |
| 3 | US1 - Discover | 17 | P1 |
| 4 | US2 - Resource Types | 6 | P1 |
| 5 | US4 - Generate | 11 | P1 |
| 6 | US6 - MCP | 10 | P1 |
| 7 | US3 - Recipes | 11 | P2 |
| 8 | US7 - Practices | 10 | P2 |
| 9 | US5 - Scaffold | 4 | P3 |
| 10 | Polish | 10 | - |
| **Total** | | **99** | |

### Tasks by Priority

- **P1 (MVP)**: 45 tasks (US1 + US2 + US4 + US6)
- **P2**: 23 tasks (US3 + US7)
- **P3**: 5 tasks (US5)
- **Infrastructure**: 26 tasks (Setup + Foundational + Polish)

### Parallel Opportunities

- 12 parallel groups identified
- Maximum parallelism: 5 tasks simultaneously (manifest parsers)
- Each user story phase has internal parallel opportunities

### Independent Test Criteria

| Story | Independent Test |
|-------|------------------|
| US1 | `rad app discover .` on sample codebase produces discovery.md with correct dependencies |
| US2 | Discovery output includes valid Resource Type mappings for detected dependencies |
| US4 | `rad app generate` produces valid app.bicep that passes `bicep build` |
| US6 | `rad mcp serve` responds to MCP tool invocations with correct JSON output |
| US3 | Recipe sources configured; dependencies matched to available recipes |
| US7 | Team practices in config/IaC reflected in generated Resource Type defaults |
| US5 | `rad app scaffold` creates deployable app structure without existing code |

### Format Validation

✅ All 95 tasks follow checklist format: `- [ ] [TaskID] [P?] [Story?] Description with file path`
