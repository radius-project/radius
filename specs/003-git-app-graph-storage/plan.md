# Implementation Plan: Git App Graph Preview

**Branch**: `001-git-app-graph-preview` | **Date**: February 4, 2026 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `/specs/001-git-app-graph-preview/spec.md`
**User Prompt**: /speckit.plan Leverage the existing Radius technology stack for CLI, etc. as well as the application and environment, and resource definitions (https://github.com/radius-project/radius, https://docs.radapp.io/). Data structures and general diff constructs should use Git principles while specific integrations and renderings in the GitHub UI should use their technologies (e.g. GitHub Actions, GitHub Apps, etc.) such that if we were to build similar integrations in the future (for example, with GitLab), our design choices today won't limit us in the future. 

## Summary

Enable **static app graph generation** from Bicep files without deployment, enriched with **git changelog metadata**, to support visualization and diffing in GitHub PR workflows. The implementation extends the existing `rad app graph` command to accept Bicep file input, generates deterministic JSON output suitable for version control, and provides a lightweight GitHub Action for PR diff visualization.

**Technical Approach** (from [research.md](./research.md)):
- Use official Bicep CLI to compile `.bicep` → ARM JSON, then extract resources and connections
- Extend existing CLI patterns in `pkg/cli/cmd/app/graph/` with file-based input detection
- Add git metadata via shell exec (`git blame`, `git log`) for source tracking
- Output to `.radius/app-graph.json` (committed artifact model)
- GitHub Action reads JSON from git history, computes diff, posts Mermaid-enhanced PR comments

## Technical Context

**Language/Version**: Go 1.21+ (matches existing Radius codebase)
**Primary Dependencies**: 
- Cobra (CLI framework, existing)
- Bicep CLI (external, managed via `rad bicep download`)
- Git (external, system requirement)
**Storage**: File-based (`.radius/app-graph.json` committed to git)
**Testing**: `make test` (Go unit tests), `make functional-test` (E2E)
**Target Platform**: Linux, macOS, Windows (cross-platform CLI)
**Project Type**: CLI extension (single project, existing radius repository)
**Performance Goals**: 
- Graph generation < 5s for 50 resources (per SC-001)
- Git enrichment adds < 2s for 1000 commits (per SC-006)
**Constraints**: 
- Must work without Radius environment (static analysis only)
- Must handle Bicep files up to 5000 lines (per SC-007)
**Scale/Scope**: Applications with up to 100+ resources, graphs committed across all Radius users

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Notes |
|-----------|--------|-------|
| **I. API-First Design** | ✅ PASS | JSON schema defined in [contracts/](./contracts/), OpenAPI spec provided |
| **II. Idiomatic Code Standards** | ✅ PASS | Go implementation follows Effective Go; uses existing `pkg/cli/` patterns |
| **III. Multi-Cloud Neutrality** | ✅ PASS | Works with any cloud's Bicep resources; cloud-specific types rendered generically |
| **IV. Testing Pyramid Discipline** | ✅ PASS | Unit tests for parsing/diffing, integration tests for git/Bicep CLI, functional E2E tests |
| **V. Collaboration-Centric Design** | ✅ PASS | Developers preview graphs locally; platform engineers review in PRs |
| **VI. Open Source & Community-First** | ✅ PASS | Design spec in public repo; GitHub Action works in forks |
| **VII. Simplicity Over Cleverness** | ✅ PASS | Uses Bicep CLI (not custom parser); shell exec for git (not libgit2) |
| **VIII. Separation of Concerns** | ✅ PASS | Core diff logic platform-agnostic; GitHub rendering layer separate |
| **IX. Incremental Adoption** | ✅ PASS | New capability; doesn't change existing `rad app graph <appname>` behavior |
| **XIV. Documentation Quality** | ✅ PASS | [quickstart.md](./quickstart.md) follows Diátaxis tutorial pattern |
| **XVII. Polyglot Coherence** | ✅ PASS | Cross-repo impact documented in spec; coordinated with `radius` and `docs` repos |

**Post-Design Re-Check**: All principles remain satisfied. Platform abstraction in architecture supports future GitLab integration per user requirement.

## Project Structure

### Documentation (this feature)

```text
specs/001-git-app-graph-preview/
├── spec.md              # Feature specification (input)
├── plan.md              # This file
├── research.md          # Phase 0 output (technology decisions)
├── data-model.md        # Phase 1 output (entity definitions)
├── quickstart.md        # Phase 1 output (user guide)
├── contracts/           # Phase 1 output (JSON schema)
│   └── app-graph-schema.yaml
└── tasks.md             # Phase 2 output (implementation tasks)
```

### Source Code (radius repository)

```text
# radius repository (../radius)
pkg/
├── cli/
│   ├── cmd/
│   │   └── app/
│   │       └── graph/
│   │           ├── graph.go           # Extended: file input detection
│   │           ├── static.go          # NEW: static graph generation
│   │           ├── diff.go            # NEW: graph diff computation
│   │           ├── display.go         # Extended: Markdown/Mermaid output
│   │           └── graph_test.go      # Extended: new test cases
│   ├── bicep/
│   │   └── parser.go                  # NEW: ARM JSON → AppGraph extraction
│   └── git/
│       └── metadata.go                # NEW: git blame/log integration
├── corerp/
│   └── api/
│       └── v20231001preview/
│           └── appgraph_types.go      # Extended: new types for static graphs

test/
├── unit/
│   └── cli/
│       └── graph/                     # NEW: unit tests for graph generation
├── integration/
│   └── cli/
│       └── graph/                     # NEW: integration tests with git/Bicep
└── functional/
    └── cli/
        └── graph/                     # NEW: E2E test scenarios

# GitHub Action (separate repo or actions/ folder)
actions/
└── app-graph-diff/
    ├── action.yml                     # Action definition
    ├── diff.sh                        # Diff computation script
    └── render.js                      # Markdown/Mermaid rendering
```

**Structure Decision**: Extends existing `radius` repository structure. New code follows established patterns in `pkg/cli/`. GitHub Action is a separate deliverable, potentially in its own repo (`radius-project/app-graph-diff-action`).

## Architecture

### Component Diagram

```
┌─────────────────────────────────────────────────────────────────────────┐
│                              rad CLI                                     │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                          │
│  ┌──────────────────┐    ┌──────────────────┐    ┌──────────────────┐  │
│  │  graph.go        │    │  static.go       │    │  diff.go         │  │
│  │  (entry point)   │───▶│  (file input)    │───▶│  (comparison)    │  │
│  │                  │    │                  │    │                  │  │
│  └──────────────────┘    └──────────────────┘    └──────────────────┘  │
│           │                       │                       │             │
│           ▼                       ▼                       ▼             │
│  ┌──────────────────┐    ┌──────────────────┐    ┌──────────────────┐  │
│  │  Radius API      │    │  Bicep CLI       │    │  Git CLI         │  │
│  │  (deployed apps) │    │  (compilation)   │    │  (metadata)      │  │
│  └──────────────────┘    └──────────────────┘    └──────────────────┘  │
│                                   │                       │             │
│                                   ▼                       ▼             │
│                          ┌──────────────────┐    ┌──────────────────┐  │
│                          │  parser.go       │    │  metadata.go     │  │
│                          │  (ARM→AppGraph)  │    │  (blame/log)     │  │
│                          └──────────────────┘    └──────────────────┘  │
│                                   │                       │             │
│                                   └───────────┬───────────┘             │
│                                               ▼                          │
│                                   ┌──────────────────────┐              │
│                                   │  AppGraph (JSON)     │              │
│                                   │  .radius/app-graph.json            │
│                                   └──────────────────────┘              │
└─────────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────────┐
│                         GitHub Action                                    │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                          │
│  ┌──────────────────┐    ┌──────────────────┐    ┌──────────────────┐  │
│  │  Checkout        │───▶│  diff.sh         │───▶│  render.js       │  │
│  │  (base + head)   │    │  (JSON compare)  │    │  (Mermaid)       │  │
│  └──────────────────┘    └──────────────────┘    └──────────────────┘  │
│                                                           │             │
│                                                           ▼             │
│                                               ┌──────────────────────┐  │
│                                               │  PR Comment          │  │
│                                               │  (create-or-update)  │  │
│                                               └──────────────────────┘  │
└─────────────────────────────────────────────────────────────────────────┘
```

### Platform Abstraction (per user requirement)

```
┌─────────────────────────────────────────────────────────────────────────┐
│                     Platform-Agnostic Core                               │
├─────────────────────────────────────────────────────────────────────────┤
│  • AppGraph data model (JSON schema)                                     │
│  • Bicep → ARM → AppGraph parsing                                        │
│  • Git metadata extraction                                               │
│  • Diff computation (JSON semantic comparison)                           │
│  • Core rendering (Markdown tables, Mermaid diagrams)                    │
└─────────────────────────────────────────────────────────────────────────┘
                                    │
          ┌─────────────────────────┼─────────────────────────┐
          ▼                         ▼                         ▼
┌──────────────────┐    ┌──────────────────┐    ┌──────────────────┐
│  GitHub Layer    │    │  GitLab Layer    │    │  CLI Layer       │
│  (Action)        │    │  (Future)        │    │  (Terminal)      │
├──────────────────┤    ├──────────────────┤    ├──────────────────┤
│  • PR comments   │    │  • MR notes      │    │  • stdout        │
│  • Workflow YAML │    │  • CI YAML       │    │  • --format      │
│  • Mermaid render│    │  • Mermaid render│    │  • diff command  │
└──────────────────┘    └──────────────────┘    └──────────────────┘
```

## Implementation Phases

### Phase 1 (P1) - Core Graph Generation
- US1: Generate app graph from Bicep files
- US2: Export graph as diff-friendly JSON/Markdown

### Phase 2 (P2) - Git & GitHub Integration
- US3: Git metadata enrichment
- US4: GitHub Action for PR diff comments

### Phase 3 (P3) - Advanced Features
- US5: Historical graph timeline
- US6: Environment-resolved graphs

## Complexity Tracking

> No complexity violations identified.

| Principle | Evaluation | Status |
|-----------|------------|--------|
| Simplicity Over Cleverness | Using Bicep CLI (not custom parser) | ✅ Simple |
| Simplicity Over Cleverness | Shell exec for git (not libgit2) | ✅ Simple |
| Incremental Adoption | Additive to existing CLI; no breaking changes | ✅ Non-disruptive |

## Generated Artifacts

- [research.md](./research.md) - Technology decisions and alternatives
- [data-model.md](./data-model.md) - Entity definitions and relationships
- [quickstart.md](./quickstart.md) - User-facing tutorial
- [contracts/app-graph-schema.yaml](./contracts/app-graph-schema.yaml) - OpenAPI JSON schema

## Next Steps

Run `/speckit.tasks` to generate actionable implementation tasks from this plan.
