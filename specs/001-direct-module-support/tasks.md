
# Tasks: Direct Module Support

**Input**: Design documents from `specs/001-direct-module-support/`
**Prerequisites**: plan.md (required), spec.md (required for user stories), research.md, data-model.md, contracts/

**Prototype Status**: Branch `001-direct-module-support` (3 commits ahead of main) already implements source classification (`pkg/recipes/source/`), TF direct output mapping, TF config generation (`AddAllOutputs`, `AddDirectModuleContext`), reachability validation, and RecipePack controller wiring. Tasks below cover **remaining work only**.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies on incomplete tasks)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3)
- Include exact file paths in descriptions

---

## Phase 1: Setup

**Purpose**: Verify prototype state and prepare for remaining implementation

- [ ] T001 Rebase branch `001-direct-module-support` onto latest main and verify `make build` succeeds
- [ ] T002 Run existing unit tests (`go test ./pkg/recipes/source/... ./pkg/recipes/driver/terraform/... ./pkg/recipes/terraform/...`) and confirm green baseline

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Core utilities shared by ALL P1 user stories (US1, US2, US3) — MUST complete before any story work begins. The expression resolver (including `context.resource.properties.*` paths and single-level ternary evaluation) is the foundational engine for all three P1 stories.

**⚠️ CRITICAL**: No user story work can begin until this phase is complete

- [ ] T003 Implement `ResolveParameterExpressions()` and `buildContextLookup()` in `pkg/recipes/paramresolver/resolver.go` — regex-based `{{context.*}}` replacement using flat context map built from `recipecontext.Context` struct; include `context.resource.properties.*` dynamic key enumeration so any user-defined property is accessible (US3 property resolution); unrecognized expressions left as-is per R-011
- [ ] T004 Implement single-level ternary expression evaluation in `pkg/recipes/paramresolver/resolver.go` — parse `{{expr == "val" ? "trueResult" : "falseResult"}}` syntax; single-level only (nested/chained ternaries are out of scope for V1); string comparison is exact-match, case-sensitive; ternary with unresolvable condition path left as-is
- [ ] T005 Write table-driven unit tests for `ResolveParameterExpressions()` in `pkg/recipes/paramresolver/resolver_test.go` — cover: single expression, multiple expressions in one value, mixed literal+expression, unrecognized expression left as-is, empty map, nil values, nested map traversal, `context.resource.properties.*` resolution (existing property resolves, missing property left as-is, property with special characters, multiple property expressions in one string)
- [ ] T006 Write table-driven unit tests for ternary evaluation in `pkg/recipes/paramresolver/resolver_test.go` — cover: simple ternary true/false, context property lookup in condition, unresolvable condition path (left as-is), mixed ternary + literal text, nested/chained ternary explicitly out of scope (verify left as-is)
- [ ] T007 [P] Implement output mapping utility function `ApplyOutputsMapping(values map[string]any, secrets map[string]any, outputsMap map[string]string) (map[string]any, map[string]any)` in `pkg/recipes/util/outputs.go` — map module output names to the resource type's read-only property names per `outputs` map (keys = resource property names, values = module output names); when `outputs` is nil/empty, pass through all outputs unchanged
- [ ] T008 [P] Write table-driven unit tests for `ApplyOutputsMapping()` in `pkg/recipes/util/outputs_test.go` — cover: output-to-property mapping, pass-through when nil, missing output key in values (skip silently), sensitive output mapping, empty maps
- [ ] T009 [P] Implement shallow merge utility `ShallowMergeParameters(base map[string]any, override map[string]any) map[string]any` in `pkg/recipes/util/merge.go` — top-level key precedence from override per FR-004; nested objects replaced entirely (not deep-merged)
- [ ] T010 [P] Write table-driven unit tests for `ShallowMergeParameters()` in `pkg/recipes/util/merge_test.go` — cover: disjoint keys, overlapping keys (override wins), nested object replaced not merged, nil base, nil override, both nil
- [ ] T011 Add `Outputs map[string]string` field to `EnvironmentDefinition` in `pkg/recipes/types.go` and propagate from `RecipeDefinition.Outputs` through the config loader path

**Checkpoint**: Foundation ready — expression resolver (with `context.resource.properties.*` and single-level ternary), output mapping, shallow merge, and EnvironmentDefinition.Outputs all available for driver integration. All three P1 stories (US1, US2, US3) can now proceed.

---

## Phase 3: User Story 2 — Terraform Module Support (Priority: P1) 🎯 MVP

**Goal**: Platform engineers set `recipeLocation` to a Terraform registry path, Git URL, or HTTP archive. The system downloads the module, resolves `{{context.*}}` expressions (including `context.resource.properties.*` and single-level ternary) in parameters, executes it, and maps outputs to the resource type's read-only properties.

**Independent Test**: Link a recipe to a public Terraform registry module (e.g., `ballj/postgresql/kubernetes`), deploy a resource with `recipeParameters` including `{{context.resource.name}}`, verify module executes and outputs are accessible as resource properties.

### Implementation for User Story 2

- [ ] T012 [US2] Replace `AddDirectModuleContext` auto-injection with `ResolveParameterExpressions()` call in `pkg/recipes/terraform/execute.go` — in the direct module code path, resolve expressions in `EnvironmentDefinition.Parameters` before writing them to the generated config; remove the old well-known variable name matching per R-011
- [ ] T013 [US2] Wire `ShallowMergeParameters()` into the Terraform execute path in `pkg/recipes/terraform/execute.go` — merge RecipePack-level `recipeParameters` with environment-level parameters (environment wins) before expression resolution
- [ ] T014 [US2] Wire `ApplyOutputsMapping()` into `prepareRecipeResponse()` in `pkg/recipes/driver/terraform/terraform.go` — after flat output collection for direct modules, apply `EnvironmentDefinition.Outputs` to map module output names to the resource type's read-only properties in `RecipeOutput.Values` and `RecipeOutput.Secrets`
- [ ] T015 [US2] Implement `result` vs `outputs` precedence logic in `pkg/recipes/driver/terraform/terraform.go` — per FR-015: if module has a `result` output AND no `outputs` mapping is configured, treat as wrapped recipe; if `outputs` mapping exists, it takes precedence
- [ ] T016 [US2] Update unit tests in `pkg/recipes/driver/terraform/terraform_test.go` — add test cases for: direct module with outputs mapping to read-only properties, direct module without outputs mapping (pass-through), direct module with `result` output and no `outputs` mapping (wrapped behavior), direct module with both `result` and `outputs` (outputs wins)
- [ ] T017 [US2] Update unit tests in `pkg/recipes/terraform/execute_test.go` — add test cases for: expression resolution in direct module path (including `context.resource.properties.*` and ternary), shallow merge of parameters, context lookup populated from recipe context

**Checkpoint**: Terraform direct module path fully wired — expression resolution (with property paths and ternary), parameter merge, output mapping to read-only properties, result/outputs precedence

---

## Phase 4: User Story 1 — Bicep Module Support (Priority: P1)

**Goal**: Platform engineers set `recipeLocation` to a Bicep module OCI reference (e.g., `br:mcr.microsoft.com/bicep/avm/res/storage/storage-account:0.14.3`). The system deploys it via ARM, passes resolved parameters (including `context.resource.properties.*` and ternary expressions), and maps ARM outputs to the resource type's read-only properties.

**Independent Test**: Link a recipe to a public AVM Bicep module from MCR, deploy a resource with `recipeParameters`, verify ARM deployment succeeds and module outputs are accessible as resource properties.

### Implementation for User Story 1

- [ ] T018 [US1] Implement direct Bicep module detection in `pkg/recipes/driver/bicep/bicep.go` — detect when `recipeLocation` is a standard Bicep module (no Radius `result` output convention); use a flag or heuristic (e.g., absence of `result` in module outputs after ARM template inspection, or explicit `outputs` mapping presence)
- [ ] T019 [US1] Implement direct Bicep deployment path in `pkg/recipes/driver/bicep/bicep.go` — resolve `{{context.*}}` expressions in parameters via `ResolveParameterExpressions()`, pass resolved parameters as ARM deployment parameters, invoke ARM deployment
- [ ] T020 [US1] Wire `ShallowMergeParameters()` into the Bicep driver path in `pkg/recipes/driver/bicep/bicep.go` — merge RecipePack-level and environment-level parameters before expression resolution
- [ ] T021 [US1] Map ARM deployment outputs to `RecipeOutput` in `pkg/recipes/driver/bicep/bicep.go` — apply `ApplyOutputsMapping()` to map module output names to the resource type's read-only property names per `outputs` field
- [ ] T022 [US1] Ensure ARM deployment cleanup for direct Bicep modules in `pkg/recipes/driver/bicep/bicep.go` — verify ARM deployment deletion works for direct module deployments on resource delete
- [ ] T023 [US1] Write unit tests for Bicep direct module path in `pkg/recipes/driver/bicep/bicep_test.go` — cover: direct module detection, parameter resolution, output mapping to read-only properties, ARM deployment creation, ARM deployment deletion/cleanup

**Checkpoint**: Bicep direct module path complete — all three P1 user stories (Terraform + Bicep + Property Resolution) are now functional

---

## Phase 5: User Story 4 — Private Module Authentication (Priority: P2)

**Goal**: Platform engineers use modules from private registries, Git repos, or OCI registries by configuring credentials through the existing secret store.

**Independent Test**: Link a recipe to a private Terraform registry module, configure credentials via existing secret mechanism, deploy, and verify module is fetched successfully.

### Implementation for User Story 4

- [ ] T024 [US4] Implement credential passthrough for private Terraform registry modules in `pkg/recipes/terraform/execute.go` — read credentials from the existing secret store and pass as Terraform CLI config (`.terraformrc` or `TF_TOKEN_*` environment variables) before `terraform init`
- [ ] T025 [US4] Implement credential passthrough for private Git repositories in `pkg/recipes/terraform/execute.go` — configure Git credentials (`GIT_ASKPASS` or credential helper) from secret store before module download
- [ ] T026 [US4] Write unit tests for private module credential injection in `pkg/recipes/terraform/execute_test.go` — cover: registry token injection, Git credential configuration, missing credentials (fall through gracefully), credential scoping (credentials only applied when source matches)

**Checkpoint**: Private module authentication works for Terraform registry and Git sources

---

## Phase 6: User Story 5 — Source Reachability Validation at Link Time (Priority: P2)

**Goal**: The system performs best-effort validation that a `recipeLocation` pointing to a direct module source is reachable at recipe link time. Per SC-009: definitive failures (404, authentication denied) reject the operation; transient failures log warnings but allow linking.

**Independent Test**: Link a recipe with a `recipeLocation` pointing to a non-existent module and verify a validation error is returned. Link with a transiently unreachable source and verify a warning is logged but the operation succeeds.

### Implementation for User Story 5

- [ ] T027 [US5] Wire `ValidateReachability()` into `CreateOrUpdateRecipePack` controller for Bicep OCI references in `pkg/corerp/frontend/controller/recipepacks/createorupdaterecipepack.go` — extend existing Terraform reachability validation to also cover `br:` Bicep module references (HEAD request to OCI manifest); per SC-009: definitive failures (404, auth denied) reject, transient failures log warning but allow linking
- [ ] T028 [US5] Write unit tests for Bicep OCI reachability validation in `pkg/corerp/frontend/controller/recipepacks/createorupdaterecipepack_test.go` — cover: valid Bicep OCI reference passes, non-existent reference returns rejection error (definitive failure), transient network failure logs warning but allows creation, authentication denied returns rejection error

**Checkpoint**: Reachability validation covers both Terraform and Bicep module sources with best-effort semantics (SC-009)

---

## Phase 7: TypeSpec Model Alignment (Priority: P1)

**Goal**: Align the TypeSpec API model with the implementation — add `outputs` and `recipeParameters` fields. A RecipePack is a collection of recipe configurations keyed by resource type.

- [ ] T029 Add `outputs` field (type `Record<string>`) and `recipeParameters` field (type `Record<unknown>`) to the RecipePack recipe definition model in `typespec/Radius.Core/recipePacks.tsp` — RecipePack is a collection keyed by resource type; no `templateVersion` field (version is part of `recipeLocation` per A-009)
- [ ] T030 Regenerate API client code from updated TypeSpec model (`make generate`) and fix any compilation errors in generated code
- [ ] T031 Update `RecipeDefinition` struct in `pkg/corerp/datamodel/recipepack.go` to align JSON tags with generated API model — ensure `recipeParameters` and `outputs` fields serialize correctly for round-trip API calls

**Checkpoint**: TypeSpec model, generated code, and internal data model are aligned

---

## Phase 8: Functional Tests (Priority: P1)

**Goal**: End-to-end validation with real modules — must cover `{{context.resource.properties.*}}` resolution and ternary expressions

- [ ] T032 [P] Create functional test for Terraform registry module deployment in `test/functional-portable/recipes/` — use a lightweight public registry module (e.g., `ballj/postgresql/kubernetes`), configure `recipeParameters` with `{{context.*}}` expressions including `{{context.resource.properties.*}}` and ternary, verify deployment succeeds and outputs are mapped to resource read-only properties
- [ ] T033 [P] Create functional test for Git-hosted module deployment in `test/functional-portable/recipes/` — use `git::https://` source with `?ref=` version pin, verify module download, execution, and output mapping to read-only properties
- [ ] T034 [P] Create functional test for Bicep AVM module deployment in `test/functional-portable/recipes/` — use a public AVM Bicep module from MCR (e.g., `br:mcr.microsoft.com/bicep/avm/res/storage/storage-account:0.14.3`), configure `recipeParameters`, verify ARM deployment succeeds and outputs are mapped to resource read-only properties
- [ ] T035 Create functional test for backward compatibility in `test/functional-portable/recipes/` — deploy a wrapped recipe (with `context` variable and `result` output) and verify zero behavioral changes

**Checkpoint**: End-to-end tests validate the complete direct module flow for Terraform and Bicep, including property resolution and ternary expressions

---

## Phase 9: Polish & Cross-Cutting Concerns

**Purpose**: Improvements that affect multiple user stories

- [ ] T036 [P] Run full linting pass (`make lint && make format-check`) and fix any issues across all modified packages
- [ ] T037 [P] Update inline code documentation (GoDoc comments) for all new exported functions and types in `pkg/recipes/paramresolver/`, `pkg/recipes/util/`, and modified driver files
- [ ] T038 Verify `make build` succeeds and run full unit test suite (`make test`) — confirm no regressions across the codebase
- [ ] T039 Run `specs/001-direct-module-support/quickstart.md` scenarios manually against a local Radius environment to validate end-to-end UX

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — start immediately
- **Foundational (Phase 2)**: Depends on Setup — BLOCKS all user stories
- **US2 Terraform (Phase 3)**: Depends on Foundational — first MVP increment
- **US1 Bicep (Phase 4)**: Depends on Foundational — can run in parallel with Phase 3
- **US4 Private Auth (Phase 5)**: Depends on Foundational — can run in parallel with Phases 3–4
- **US5 Reachability (Phase 6)**: Depends on Foundational — can run in parallel with Phases 3–5
- **TypeSpec (Phase 7)**: No story dependency — can run in parallel with any phase after Setup
- **Functional Tests (Phase 8)**: Depends on Phases 3 + 4 (Terraform + Bicep paths wired with expression resolver)
- **Polish (Phase 9)**: Depends on all other phases

### User Story Dependencies

- **US1 (P1 Bicep)**: Can start after Foundational — no dependencies on other stories
- **US2 (P1 Terraform)**: Can start after Foundational — no dependencies on other stories
- **US3 (P1 Property Resolution)**: Implemented in Foundational phase — expression resolver includes `context.resource.properties.*` and ternary; validated through US1/US2 driver wiring and functional tests
- **US4 (P2 Private Auth)**: Independent of all other stories
- **US5 (P2 Reachability)**: Independent of all other stories (extends existing validation)

### Within Each User Story

- Core implementation before integration wiring
- Wiring before unit tests
- All tasks within a story are sequential unless marked [P]

### Parallel Opportunities

```
After Foundational completes (which includes US3 expression engine),
all of these can run simultaneously:

  ┌─ Phase 3: US2 Terraform ─────────────┐
  ├─ Phase 4: US1 Bicep ─────────────────┤
  ├─ Phase 5: US4 Private Auth ──────────┤──▶ Phase 8: Functional Tests
  ├─ Phase 6: US5 Reachability ──────────┤
  └─ Phase 7: TypeSpec ──────────────────┘
```

Within Foundational: T007+T008, T009+T010 can all run in parallel with each other (and with the T003→T004→T005→T006 sequential chain).

---

## Parallel Example: Foundational Phase

```bash
# Sequential (has dependency — same file, expression resolver):
Task T003: "Implement ResolveParameterExpressions() with context.resource.properties.* in pkg/recipes/paramresolver/resolver.go"
Task T004: "Implement single-level ternary evaluation in pkg/recipes/paramresolver/resolver.go"
Task T005: "Write tests for expression resolver (basic + properties) in pkg/recipes/paramresolver/resolver_test.go"
Task T006: "Write tests for ternary evaluation in pkg/recipes/paramresolver/resolver_test.go"

# Parallel with T003–T006 (different files, no dependencies):
Task T007: "Implement ApplyOutputsMapping() in pkg/recipes/util/outputs.go"
Task T008: "Write tests for ApplyOutputsMapping() in pkg/recipes/util/outputs_test.go"
Task T009: "Implement ShallowMergeParameters() in pkg/recipes/util/merge.go"
Task T010: "Write tests for ShallowMergeParameters() in pkg/recipes/util/merge_test.go"
```

---

## Implementation Strategy

### MVP First (User Story 2 — Terraform Only)

1. Complete Phase 1: Setup (verify prototype baseline)
2. Complete Phase 2: Foundational (expression resolver with properties + ternary, output mapping, shallow merge)
3. Complete Phase 3: User Story 2 (Terraform direct module path)
4. **STOP and VALIDATE**: Test with a real Terraform registry module end-to-end (including `context.resource.properties.*` and ternary)
5. Deploy/demo if ready — this alone unlocks the Terraform module ecosystem with property resolution

### Incremental Delivery

1. Setup + Foundational → Foundation ready (US3 expression engine included)
2. Add US2 (Terraform) → Test independently → Demo (**MVP!** — includes property resolution + ternary)
3. Add US1 (Bicep) → Test independently → Demo (full P1 delivery — all 3 P1 stories complete)
4. Add US4 + US5 → Test independently → Demo (enterprise features)
5. TypeSpec + Functional Tests + Polish → Release ready

### Parallel Team Strategy

With multiple developers after Foundational completes:

- **Developer A**: US2 Terraform (Phase 3) then Functional Tests (Phase 8)
- **Developer B**: US1 Bicep (Phase 4) then US5 Reachability (Phase 6)
- **Developer C**: US4 Private Auth (Phase 5) then Polish (Phase 9)
- **Developer D**: TypeSpec (Phase 7) then Polish (Phase 9)

---

## Summary

| Metric | Value |
|--------|-------|
| **Total tasks** | 39 |
| **Setup** | 2 tasks (Phase 1) |
| **Foundational (incl. US3 engine)** | 9 tasks (Phase 2) |
| **US2 Terraform (P1)** | 6 tasks (Phase 3) |
| **US1 Bicep (P1)** | 6 tasks (Phase 4) |
| **US4 Private Auth (P2)** | 3 tasks (Phase 5) |
| **US5 Reachability (P2)** | 2 tasks (Phase 6) |
| **TypeSpec Alignment (P1)** | 3 tasks (Phase 7) |
| **Functional Tests (P1)** | 4 tasks (Phase 8) |
| **Polish** | 4 tasks (Phase 9) |
| **Parallel opportunities** | 5 phases can run simultaneously after Foundational |
| **Suggested MVP** | Phases 1–3 (US2 Terraform: 17 tasks, includes US3 property resolution) |
| **P1 stories** | US1, US2, US3 — all delivered by end of Phase 4 |

---

## Notes

- [P] tasks = different files, no dependencies on incomplete tasks
- [Story] label maps task to specific user story for traceability
- **US3 (Property Resolution) is P1** — its expression engine (ternary + `context.resource.properties.*`) is built in Foundational and ships with US1/US2
- Prototype code (source classifier, TF output mapping, config generation) is already implemented — not re-tasked
- RecipePack is a collection of recipe configurations keyed by resource type
- Module version is part of `recipeLocation` (OCI tag, Git ref, registry syntax) — no separate `templateVersion` field (A-009)
- Output mapping maps module outputs to the resource type's read-only properties (not rename/filter)
- Reachability validation is best-effort: definitive failures reject, transient failures warn (SC-009)
- Each user story is independently completable and testable
- Commit after each task or logical group
- Stop at any checkpoint to validate story independently
