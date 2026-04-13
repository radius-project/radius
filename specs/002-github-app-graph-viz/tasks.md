# Tasks: GitHub App Graph Visualization

**Input**: Design documents from `/specs/002-github-app-graph-viz/`
**Prerequisites**: plan.md (required), spec.md (required for user stories), research.md, data-model.md, contracts/

**Organization**: Tasks are grouped by user story to enable independent implementation and testing of each story.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3)
- Include exact file paths in descriptions

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Browser extension build system upgrade and Go project scaffolding required before any feature work

- [X] T001 Create esbuild bundler configuration for Manifest V3 content/background script bundling in web/browser-extension/esbuild.config.mjs
- [X] T002 Add cytoscape, cytoscape-dagre, and esbuild dependencies to web/browser-extension/package.json and replace plain tsc-only build/watch/clean scripts with esbuild-based equivalents
- [X] T003 [P] Create pkg/cli/graph/ directory with placeholder package declaration for static graph construction module

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Core shared infrastructure that MUST be complete before ANY user story can be implemented

**⚠️ CRITICAL**: No user story work can begin until this phase is complete

- [X] T004 Create TypeScript type definitions for ApplicationGraphResponse, ApplicationGraphResource, ApplicationGraphConnection, StaticGraphArtifact, GraphDiff, ModifiedResource, DiffStatus, and Direction in web/browser-extension/src/shared/types.ts
- [X] T005 [P] Create GitHub Contents API client with getFileContents (raw file fetch by owner/repo/path/ref), checkFileExists (HEAD check for app.bicep), and fetchPRDetails (base/head repo+ref extraction) methods in web/browser-extension/src/shared/github-api.ts
- [X] T006 [P] Create Primer-compatible graph styles including node shapes, diff color borders (green added, yellow modified, red removed), popup overlay styling, loading indicator, and tab styles in web/browser-extension/src/styles/graph.css
- [X] T007 [P] Update web/browser-extension/manifest.json with content script matches and permissions needed for GitHub Contents API access on PR and repo root pages

**Checkpoint**: Foundation ready — user story implementation can now begin

---

## Phase 3: User Story 5 — `codeReference` Property on Resource Schemas (Priority: P1)

**Goal**: Add an optional `codeReference` string property to all Radius resource types via shared TypeSpec bases, and extend the `ApplicationGraphResource` read model with `codeReference`, `appDefinitionLine`, and `diffHash` metadata fields.

**Independent Test**: Define a resource with `codeReference: 'src/cache/redis.ts#L10'` in a `.bicep` file, compile it with `bicep build`, and verify the property appears in the compiled ARM JSON output. Run `grep -n "CodeReference" pkg/corerp/api/v20231001preview/zz_generated_models.go` to confirm Go model generation.

### Implementation for User Story 5

- [X] T008 [US5] Add optional `codeReference` string property with doc annotation to ApplicationScopedResource and GlobalScopedResource shared authorable bases in typespec/radius/v1/resources.tsp
- [X] T009 [US5] Add `codeReference` (string), `appDefinitionLine` (int32), and `diffHash` (string) optional properties to ApplicationGraphResource model in typespec/Applications.Core/applications.tsp
- [X] T010 [US5] Run `make generate` to regenerate Go models and serialization code, then verify `CodeReference`, `AppDefinitionLine`, and `DiffHash` fields appear in pkg/corerp/api/v20231001preview/zz_generated_models.go and zz_generated_models_serde.go

**Checkpoint**: Schema extension complete — `codeReference` available on all Radius resources, graph read model extended with metadata fields

---

## Phase 4: User Story 3 — Static Application Graph Construction (Priority: P1)

**Goal**: Implement `rad graph build` CLI command that compiles a Bicep app definition to ARM JSON, parses resources/connections/dependencies, maps source line numbers, computes diff hashes, and emits a static graph JSON artifact either to a local file or to `{source-branch}/app.json` on the `radius-graph` orphan branch.

**Independent Test**: Run `go build -o bin/rad ./cmd/rad && bin/rad graph build --bicep app.bicep --output /tmp/graph.json` with a sample Bicep file and verify the output JSON contains correct resources, connections, `codeReference`, `appDefinitionLine`, and `diffHash` values per the static-graph-artifact contract.

### Implementation for User Story 3

- [X] T011 [US3] Implement ARM JSON parser and static graph builder in pkg/cli/graph/build.go: parse languageVersion 1.9 resources map, extract type/name/properties.connections/dependsOn, resolve resourceId() expressions to Radius-style resource IDs, parse app.bicep source text to map symbolic names to declaration line numbers, copy authorable codeReference into the read model, and emit StaticGraphArtifact JSON. Include unit tests in pkg/cli/graph/build_test.go covering: valid multi-resource Bicep, dependsOn edge extraction, codeReference pass-through, source line mapping, and compilation failure error handling.
- [X] T012 [P] [US3] Implement canonical diff hash generator in pkg/cli/graph/diffhash.go: extract review-relevant properties (connections, container image, ports, routes), canonicalize to sorted JSON, compute SHA-256 hash, return hex-encoded diffHash string. Include unit tests in pkg/cli/graph/diffhash_test.go covering: deterministic output for identical inputs, different hashes for changed properties, stable ordering across Go map iteration.
- [X] T013 [US3] Implement `rad graph build` CLI subcommand with `--bicep` (input file path, default `app.bicep`) and `--output` (output file path, default `.radius/static/app.json`) for local file mode, plus orphan-branch publishing flags in cmd/rad/cmd/graph.go; wire into existing rad command tree
- [X] T014 [P] [US3] Create reusable GitHub Actions workflow in .github/workflows/__build-app-graph.yml that installs a released rad binary, runs `rad graph build`, and commits `{source-branch}/app.json` to the `radius-graph` orphan branch on push events that modify app.bicep

**Checkpoint**: Static graph construction works locally via CLI and in consumer CI via reusable workflow — graph artifacts are committed to the orphan branch

---

## Phase 5: User Story 2 — Interactive Resource Navigation from Graph (Priority: P1)

**Goal**: Implement the popup navigation component that appears when a user clicks a graph resource node, showing "Source code" (from `codeReference`) and "App definition" (from `appDefinitionLine`) links with security validation.

**Independent Test**: Render a graph with resources that have `codeReference` and `appDefinitionLine` values, click a node, and verify the popup appears with correct links. Click a resource with an invalid `codeReference` (path traversal, URL scheme) and verify the "Source code" link is omitted. Verify links construct correct GitHub URLs for diff view (PR modified resources) and blob view (unchanged resources).

### Implementation for User Story 2

- [X] T015 [US2] Implement graph navigation popup component in web/browser-extension/src/content/graph-navigation.ts: popup DOM creation with "Source code" link (when valid codeReference), "App definition" link (using appDefinitionLine or file-level fallback), and diff-view vs blob-view URL logic based on resource diff status. Imports validation and URL construction utilities from coderef-validator.ts.
- [X] T016 [US2] Implement codeReference security validation utilities in web/browser-extension/src/content/coderef-validator.ts (separate file from graph-navigation.ts): exported isValidCodeReference() enforcing strict allowlist regex (`^[a-zA-Z0-9_\-./]+(?:#L\d+)?$`), no `..` path traversal, no absolute paths, no URL schemes, no query strings, no backslashes; and exported buildGitHubFileUrl() for safe programmatic URL construction from validated path components (FR-009a, FR-009b). Include unit tests in web/browser-extension/src/content/coderef-validator.test.ts covering valid paths, line anchors, path traversal rejection, URL scheme rejection, and backslash rejection.

**Checkpoint**: Navigation popup ready — clicking graph nodes provides secure, context-aware navigation to source code and app definition

---

## Phase 6: User Story 1 — PR Diff Visualization with Change Highlighting (Priority: P1) 🎯 MVP

**Goal**: Detect PR pages that modify a Radius app definition, fetch pre-built graph artifacts for base and head branches, compute the diff, and render an interactive color-coded application graph (green=added, yellow=modified, red=removed) in the PR description area.

**Independent Test**: Load the browser extension, navigate to a GitHub PR that modifies `app.bicep` with CI-generated artifacts on the `radius-graph` orphan branch for both source branches, and verify: loading indicator appears, graph renders below PR description with correct diff coloring, clicking nodes shows navigation popups, PR without app definition changes shows no graph.

### Implementation for User Story 1

- [X] T017 [P] [US1] Implement client-side graph diff computation in web/browser-extension/src/content/graph-diff.ts: build resource ID maps from base and head ApplicationGraphResponse, classify each resource as added (in head only), removed (in base only), modified (in both but diffHash changed), or unchanged (in both with same diffHash), return GraphDiff object. Include unit tests in web/browser-extension/src/content/graph-diff.test.ts covering: added-only diff, removed-only diff, modified detection via diffHash, unchanged pass-through, and empty graph handling.
- [X] T018 [US1] Implement Cytoscape.js graph renderer with cytoscape-dagre DAG layout in web/browser-extension/src/content/graph-renderer.ts: create Cytoscape instance on a container element, convert ApplicationGraphResource array to Cytoscape nodes/edges, apply diff status as node data attributes, style nodes with Primer diff colors (green/yellow/red borders), display resource name + type + optional image tag, bind node tap events to graph-navigation popup, support deterministic layout (FR-036), handle 3-20+ resources (FR-035)
- [X] T019 [US1] Implement PR page graph injection orchestrator in web/browser-extension/src/content/pr-graph.ts: detect app.bicep in PR changed files, extract base/head repo+ref from PR metadata via github-api.ts, fetch graph artifacts for both refs from the `radius-graph` orphan branch (handling 404 with "waiting for CI" message), compute diff via graph-diff.ts, inject graph container below PR description (#discussion_bucket), render via graph-renderer.ts, show loading indicator during fetch/render
- [X] T020 [US1] Update web/browser-extension/src/content/inject.ts with PR page URL detection (/:owner/:repo/pull/:number pattern), turbo:load and MutationObserver listeners for SPA navigation, debounced re-injection on navigation, duplicate injection prevention via element ID check, and pr-graph.ts initialization trigger

**Checkpoint**: Full PR diff visualization working — reviewers see interactive color-coded application graph when viewing PRs that modify the Radius app definition. MVP complete.

---

## Phase 7: User Story 4 — Repository Root "Application Graph" Tab (Priority: P2)

**Goal**: Inject an "Application graph" tab on repository root pages alongside README/License tabs that renders the current-state graph for the default branch without diff coloring.

**Independent Test**: Navigate to a GitHub repo root that contains `app.bicep` with a CI-generated graph artifact on `radius-graph`, verify the "Application graph" tab appears, click it, and confirm the graph renders without diff coloring with interactive navigation popups on node click. Navigate to a repo without `app.bicep` and verify no tab is injected.

### Implementation for User Story 4

- [X] T021 [US4] Implement repo root tab injection in web/browser-extension/src/content/repo-tab.ts: check for app.bicep existence via github-api.ts checkFileExists, inject "Application graph" tab element into the repo root tab bar, fetch the default branch artifact from the `radius-graph` orphan branch on tab click, render graph via graph-renderer.ts without diff coloring (all resources as unchanged status), show loading state during fetch, handle missing artifact gracefully
- [X] T022 [US4] Update web/browser-extension/src/content/inject.ts with repo root page URL detection (/:owner/:repo with no sub-path or /tree/ pattern), repo-tab.ts initialization trigger, and cleanup on SPA navigation away from repo root

**Checkpoint**: Repo root tab provides a living architecture reference — the graph always reflects the current default-branch topology

---

## Phase 8: User Story 6 — Dedicated Modeled App Graph Page (Priority: P2)

**Goal**: Add an "Applications" section to the repository sidebar with a link to a dedicated page that renders the full modeled application graph with interactive navigation.

**Independent Test**: Navigate to a repo root, verify the "Applications" sidebar section appears with an application link, click the link, and verify the dedicated page renders the full graph with working node popups. Navigate to a repo without `app.bicep` and verify no sidebar section is injected.

### Implementation for User Story 6

- [X] T023 [US6] Implement dedicated modeled app graph page in web/browser-extension/src/content/app-page.ts: create full-page graph container, fetch the current branch artifact from the `radius-graph` orphan branch, render via graph-renderer.ts with full-size layout, support interactive navigation popups, handle missing artifact with informative message
- [X] T024 [US6] Update web/browser-extension/src/content/inject.ts with sidebar "Applications" section injection on repo root pages (when app.bicep detected), dedicated page route handling (/:owner/:repo/radius/app/:name URL pattern), and app-page.ts initialization trigger

**Checkpoint**: Dedicated modeled graph page provides full-screen interactive exploration of application topology

---

## Phase 9: User Story 7 — Dedicated Deployed App Graph Page (Priority: P3)

**Goal**: Show actual infrastructure resources alongside modeled resources on a deployed application graph page with live status indicators (grey=queued, yellow=in-progress, green=success, red=failed), cloud portal links for successful resources, and error modals for failed resources.

**Independent Test**: Deploy a Radius application, navigate to the deployed app graph page, verify modeled resources appear alongside infrastructure resources with correct status color indicators, click a successful resource to navigate to cloud portal, click a failed resource to see the error modal with deployment error message.

### Implementation for User Story 7

- [X] T025 [US7] Create Radius control plane API client for fetching live deployment state (resource provisioning status, output resources, deployment errors) in web/browser-extension/src/shared/radius-api.ts (separate from github-api.ts to maintain separation of concerns)
- [X] T026 [US7] Extend web/browser-extension/src/content/app-page.ts with deployed graph mode: render modeled and infrastructure (outputResources) nodes, apply DeploymentStatus color indicators (grey/yellow/green/red), add click handler for successful resources linking to cloud provider portal URL, add error modal popup for failed resources displaying deployment error message
- [X] T027 [US7] Update web/browser-extension/src/content/inject.ts with deployed app graph page route detection and initialization

**Checkpoint**: Deployed graph page enables post-deployment troubleshooting — developers can see live infrastructure status and drill into failures

---

## Phase 10: Polish & Cross-Cutting Concerns

**Purpose**: Documentation, validation, and improvements that affect multiple user stories

- [X] T028 [P] Add documentation for graph visualization feature covering CLI usage, browser extension setup, and CI workflow integration in docs/contributing/
- [ ] T029 Run end-to-end quickstart validation per specs/002-github-app-graph-viz/quickstart.md (schema extension, rad graph build, browser extension load, PR diff, repo root tab, codeReference navigation)
- [ ] T030 [P] Code cleanup: review cross-story integration points in inject.ts, verify SPA navigation handling across all page types, ensure no duplicate injections
- [ ] T031 [P] Performance benchmarks: time `rad graph build` execution on a 15-resource Bicep file and verify <2s (SC-005); measure browser extension graph render time with performance.now() on a 15-resource graph and verify <5s (SC-001); validate WCAG 2.1 AA contrast ratios (≥3:1) for diff colors against GitHub light and dark theme backgrounds (SC-002)

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — can start immediately
- **Foundational (Phase 2)**: Depends on Setup completion — BLOCKS all user stories
- **US5 — codeReference Schema (Phase 3)**: Depends on Foundational; BLOCKS US3 (graph needs schema fields) and US2 (navigation needs codeReference)
- **US3 — Static Graph Construction (Phase 4)**: Depends on US5; BLOCKS US1 (PR diff needs artifacts), US4 (tab needs artifacts), US6 (page needs artifacts)
- **US2 — Interactive Navigation (Phase 5)**: Depends on US5; BLOCKS US1 (PR diff uses navigation popups)
- **US1 — PR Diff Visualization (Phase 6)**: Depends on US3 + US2; delivers MVP
- **US4 — Repo Root Tab (Phase 7)**: Depends on US3 + US2; can run in parallel with US1 if staffed
- **US6 — Dedicated Modeled Page (Phase 8)**: Depends on US3 + US2; can run in parallel with US4
- **US7 — Deployed Graph (Phase 9)**: Depends on all previous stories + live Radius environment
- **Polish (Phase 10)**: Depends on all desired user stories being complete

### User Story Dependencies

```
US5 (codeReference Schema)
 ├──→ US3 (Static Graph Construction)
 │     ├──→ US1 (PR Diff Visualization) ← also depends on US2
 │     ├──→ US4 (Repo Root Tab) ← also depends on US2
 │     └──→ US6 (Dedicated Modeled Page) ← also depends on US2
 └──→ US2 (Interactive Navigation)
       ├──→ US1 (PR Diff Visualization)
       ├──→ US4 (Repo Root Tab)
       └──→ US6 (Dedicated Modeled Page)

US7 (Deployed Graph) ← depends on US3, US2, US6
```

### Within Each User Story

- Models/schemas before services/builders
- Core implementation before integration points
- Story complete before moving to next priority

### Parallel Opportunities

- **Phase 1**: T003 can run in parallel with T001–T002 (different directories)
- **Phase 2**: T005, T006, T007 can all run in parallel (different files)
- **Phase 4**: T012 (diffhash.go) and T014 (CI workflow) can run in parallel with T011 (build.go)
- **Phase 5**: T016 (coderef-validator.ts) targets a different file from T015 (graph-navigation.ts) and can run in parallel
- **Phase 6**: T017 (graph-diff.ts) can run in parallel with T018 (graph-renderer.ts)
- **After Phase 3**: US3 and US2 can run in parallel (different languages/directories)
- **After Phase 4+5**: US1, US4, US6 can start in parallel if team capacity allows

---

## Parallel Example: User Story 1 (PR Diff Visualization)

```bash
# After US3 and US2 are complete, launch parallel US1 tasks:
Task T017: "Implement graph diff computation in web/browser-extension/src/content/graph-diff.ts"
Task T018: "Implement Cytoscape graph renderer in web/browser-extension/src/content/graph-renderer.ts"

# Then sequential (depends on T017 + T018):
Task T019: "Implement PR page graph injection in web/browser-extension/src/content/pr-graph.ts"
Task T020: "Update inject.ts with PR page detection"
```

---

## Implementation Strategy

### MVP First (P1 Stories: US5 → US3 → US2 → US1)

1. Complete Phase 1: Setup
2. Complete Phase 2: Foundational (CRITICAL — blocks all stories)
3. Complete Phase 3: US5 (schema extension)
4. Complete Phase 4: US3 (static graph construction) — can overlap with Phase 5
5. Complete Phase 5: US2 (interactive navigation) — can overlap with Phase 4
6. Complete Phase 6: US1 (PR diff visualization)
7. **STOP and VALIDATE**: Test US1 end-to-end per quickstart.md
8. Deploy/demo if ready — MVP delivered

### Incremental Delivery

1. Setup + Foundational → Foundation ready
2. Add US5 → Schema testable independently (compile Bicep, check property)
3. Add US3 → Graph construction testable independently (CLI + CI)
4. Add US2 → Navigation testable independently (popup rendering)
5. Add US1 → Full PR diff visualization → **Deploy/Demo (MVP!)**
6. Add US4 → Repo root tab → Deploy/Demo
7. Add US6 → Dedicated modeled page → Deploy/Demo
8. Add US7 → Deployed graph (requires live Radius) → Deploy/Demo

### Parallel Team Strategy

With multiple developers:

1. Team completes Setup + Foundational together
2. Developer A: US5 (schema, TypeSpec/Go)
3. Once US5 done:
   - Developer A: US3 (static graph, Go CLI)
   - Developer B: US2 (navigation, TypeScript)
4. Once US3 + US2 done:
   - Developer A: US1 (PR diff, TypeScript)
   - Developer B: US4 (repo root tab, TypeScript)
   - Developer C: US6 (dedicated page, TypeScript)
5. US7 when all P1+P2 stories are stable

---

## Notes

- [P] tasks = different files, no dependencies on incomplete tasks in the same phase
- [Story] label maps task to specific user story for traceability
- Each user story is independently testable at its checkpoint
- Commit after each task or logical group
- Stop at any checkpoint to validate the story independently
- The browser extension build system upgrade (Phase 1) is a prerequisite because the current plain `tsc` build cannot bundle npm dependencies like Cytoscape.js for Manifest V3
- Security: codeReference validation is enforced at the rendering boundary (browser extension) per FR-009a/FR-009b — never interpolate raw values into href/innerHTML
