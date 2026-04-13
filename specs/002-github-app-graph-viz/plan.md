# Implementation Plan: GitHub App Graph Visualization

**Branch**: `002-github-app-graph-viz` | **Date**: 2026-04-12 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/002-github-app-graph-viz/spec.md`

**Note**: This template is filled in by the `/speckit.plan` command. See `.specify/templates/plan-template.md` for the execution workflow.

## Summary

Build interactive application graph visualization delivered via a Chrome/Edge browser extension (Manifest V3, TypeScript) that renders Radius application topology diagrams in GitHub PRs (with diff coloring), repository root pages (as an injected tab), and dedicated application pages. Radius ships graph generation as a new `rad graph build` CLI command so the core build logic remains testable locally and distributable to consumer repositories. A thin consumer GitHub Actions workflow, or a Radius-hosted reusable workflow, installs a released `rad` binary, generates a single static artifact, and commits it to `{source-branch}/app.json` on the `radius-graph` orphan branch. The browser extension fetches this artifact via the GitHub Contents API from the base repo/base ref and the head repo/head ref for forked PRs. A new authorable `codeReference` property is added to shared resource property bases in TypeSpec and propagated into the graph read model together with `appDefinitionLine` and `diffHash` metadata.

## Technical Context

**Language/Version**: TypeScript 5.4 (browser extension), Go 1.22+ (schema/graph construction), TypeSpec (API definitions)
**Primary Dependencies**: Cytoscape.js (graph rendering), esbuild (extension bundling), Chrome Extension Manifest V3 APIs, GitHub Contents API, tweetnacl (crypto), @anthropic-ai/sdk (AI features)
**Storage**: Static JSON artifact at `{source-branch}/app.json` on the `radius-graph` orphan branch; no backend database
**Testing**: Jest/Vitest (TypeScript unit tests), Go `testing` package (schema tests), manual browser testing, Playwright (future E2E)
**Target Platform**: Chrome/Edge browsers (Manifest V3), GitHub Actions CI (Linux runners)
**Project Type**: Browser extension + CI workflow + schema extension (multi-component)
**Performance Goals**: Graph visualization renders within 5 seconds (SC-001); static graph construction <2 seconds for 5–15 resources (SC-005)
**Constraints**: Entirely client-side for static/modeled graphs (SC-007); no backend server; GitHub API rate limits; Primer design system compliance (SC-008)
**Scale/Scope**: Applications with 3–20+ resources; single app definition file (`app.bicep` at repo root)

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Notes |
|-----------|--------|-------|
| I. API-First Design | ✅ PASS | `codeReference` property added via TypeSpec; graph output follows existing `ApplicationGraphResponse` schema |
| II. Idiomatic Code Standards | ✅ PASS | TypeScript strict mode; Go follows `gofmt`/`golangci-lint`; Bicep follows conventions |
| III. Multi-Cloud Neutrality | ✅ PASS | Static graph is cloud-agnostic (built from Bicep source, not live infra); deployed graph (P3) uses existing provider abstractions |
| IV. Testing Pyramid Discipline | ✅ PASS | Plan includes unit tests (TS + Go), integration tests (CI workflow), manual browser testing; E2E via Playwright planned |
| V. Collaboration-Centric Design | ✅ PASS | PR diff visualization aids developer-reviewer collaboration; repo root tab benefits platform engineers and developers |
| VI. Open Source & Community-First | ✅ PASS | Design spec in public design-notes repo; implementation in public radius repo |
| VII. Simplicity Over Cleverness | ✅ PASS | CI pre-builds graph (no client-side Bicep compilation); single fixed file path (`app.bicep`); Cytoscape.js directly (abstraction is a design goal, not hard requirement) |
| VIII. Separation of Concerns | ✅ PASS | Clear separation: CI workflow (build), extension content script (render), TypeSpec/Go (schema); graph data prep separated from rendering |
| IX. Incremental Adoption | ✅ PASS | Browser extension is opt-in; `codeReference` is optional; features degrade gracefully when artifacts missing |
| X. TypeScript Standards | ✅ PASS | Extension uses TypeScript strict mode; follows Chrome extension Manifest V3 patterns |
| XVI. Repository-Specific Standards | ✅ PASS | Follows existing `web/browser-extension/` patterns, `typespec/` conventions, `.github/workflows/` CI patterns |
| XVII. Polyglot Coherence | ✅ PASS | Consistent `ApplicationGraphResponse` schema across TypeSpec → Go → JSON → TypeScript consumption chain |

**Gate result: PASS** — No violations detected.

### Post-Design Re-evaluation (after Phase 1)

All principles re-verified after Phase 1 design decisions. No new violations:
- `codeReference` schema follows API-First via TypeSpec (Principle I)
- Static graph artifact contract is documented with schema versioning (Principle VIII)
- Validation enforced at rendering boundary only — simplicity over redundant checks (Principle VII)
- `codeReference` is optional with `omitempty`; graceful degradation on missing artifacts (Principle IX)

**Post-design gate result: PASS**

## Project Structure

### Documentation (this feature)

```text
specs/002-github-app-graph-viz/
├── plan.md              # This file (/speckit.plan command output)
├── research.md          # Phase 0 output (/speckit.plan command)
├── data-model.md        # Phase 1 output (/speckit.plan command)
├── quickstart.md        # Phase 1 output (/speckit.plan command)
├── contracts/           # Phase 1 output (/speckit.plan command)
└── tasks.md             # Phase 2 output (/speckit.tasks command - NOT created by /speckit.plan)
```

### Source Code (repository root)

```text
# Authorable schema extension + graph read model
typespec/
├── radius/
│   └── v1/
│       └── resources.tsp         # Add authorable codeReference to ApplicationScopedResource and GlobalScopedResource
├── Applications.Core/
│   └── applications.tsp          # Extend ApplicationGraphResource with codeReference, appDefinitionLine, diffHash
├── Applications.Dapr/
├── Applications.Datastores/
└── Applications.Messaging/

# Generated Go models (after TypeSpec codegen)
pkg/corerp/api/v20231001preview/
├── zz_generated_models.go        # Updated ApplicationGraphResource read model
└── zz_generated_models_serde.go  # Serialization support

# Static graph construction (reused through rad CLI)
pkg/cli/graph/
├── build.go                      # app.bicep + ARM JSON -> static graph artifact
├── build_test.go                 # Unit tests for graph construction and source mapping
├── diffhash.go                   # Canonical diff-hash generation from normalized resource properties
└── diffhash_test.go              # Unit tests for diff hashing

cmd/rad/cmd/
└── graph.go                      # NEW: rad graph build subcommand

# Browser extension
web/browser-extension/
├── esbuild.config.mjs            # NEW: bundle content/background scripts and npm deps for MV3
├── src/
│   ├── content/
│   │   ├── inject.ts             # Existing — extend with graph injection points
│   │   ├── graph-renderer.ts     # NEW: Cytoscape.js graph rendering
│   │   ├── graph-diff.ts         # NEW: Client-side diff computation
│   │   ├── graph-navigation.ts   # NEW: Popup/navigation link generation
│   │   ├── coderef-validator.ts  # NEW: codeReference validation and safe URL construction
│   │   ├── pr-graph.ts           # NEW: PR page graph injection
│   │   ├── repo-tab.ts           # NEW: Repo root tab injection
│   │   └── app-page.ts           # NEW: Dedicated application page
│   ├── shared/
│   │   ├── github-api.ts         # NEW: GitHub Contents API client for graph artifacts
│   │   ├── radius-api.ts         # NEW: Radius control plane API client for live deployment state (US7)
│   │   └── types.ts              # NEW: TypeScript types for ApplicationGraphResponse
│   └── styles/
│       └── graph.css             # NEW: Primer-compatible graph styles
├── package.json                  # Add cytoscape + esbuild dependencies and replace plain tsc-only build
└── tsconfig.json

# Reusable workflow shipped by Radius
.github/workflows/
└── __build-app-graph.yml         # NEW: install released rad, run rad graph build, commit to radius-graph orphan branch

# Graph artifact output (committed by CI in consumer repos)
radius-graph orphan branch
└── {source-branch}/app.json      # Static graph JSON for the single supported app.bicep
```

**Structure Decision**: Multi-component structure following existing repository conventions. Authorable schema changes live in `typespec/radius/v1/resources.tsp`, while the graph read model stays in `typespec/Applications.Core/applications.tsp` with generated Go output in `pkg/corerp/api/`. Static graph logic is implemented once in the existing `rad` CLI so it can be executed both locally and in consumer CI. The browser extension build is upgraded from plain `tsc` output to bundled MV3 assets so Cytoscape can be shipped safely. Radius provides a reusable workflow for consumer repositories instead of requiring them to compile Go from the Radius source tree.

## Complexity Tracking

> No constitution violations. No complexity justifications required.
