# Implementation Plan: Automatic Application Discovery

**Branch**: `001-auto-app-discovery` | **Date**: February 2, 2026 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `/specs/001-auto-app-discovery/spec.md`

**Note**: This template is filled in by the `/speckit.plan` command. See `.specify/templates/commands/plan.md` for the execution workflow.

## Summary

Enable developers to adopt Radius with zero manual Resource Type or Recipe authoring by automatically discovering infrastructure dependencies from codebase analysis, detecting team practices from existing IaC, matching to proven Recipes, and generating complete Radius application definitions. The feature is implemented as composable skills exposed via MCP for AI agents, CLI commands, and programmatic API.

## Technical Context

**Language/Version**: Go 1.22+ (aligned with radius repo go.mod)
**Primary Dependencies**: 
- Radius CLI framework (`pkg/cli/`)
- MCP SDK for Go (Model Context Protocol server)
- LLM integration for codebase analysis (Azure OpenAI or compatible)
- Bicep generation utilities (`bicep-tools/`)

**Storage**: Local filesystem only (discovery.md, app.bicep outputs)
**Testing**: 
- Unit tests via `go test` / `make test`
- Integration tests in `test/functional-portable/`
- Functional tests using `magpiego` framework

**Target Platform**: Linux, macOS, Windows (CLI tool)
**Project Type**: Extension to existing Radius CLI (`cmd/rad/`)
**Performance Goals**: Discovery of ≤100 source files in <30 seconds (NFR-01)
**Constraints**: 
- No code execution during discovery (static analysis only)
- No external transmission of code
- Deterministic output (same input → identical output)

**Scale/Scope**: 
- 5 supported languages (Python, JS/TS, Go, Java, C#)
- 7 composable skills
- 2 CLI commands (`rad app discover`, `rad app generate`)
- 1 MCP server command (`rad mcp serve`)

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Notes |
|-----------|--------|-------|
| **I. API-First Design** | ✅ PASS | Skills define JSON input/output schemas; MCP exposes structured APIs |
| **II. Idiomatic Code Standards** | ✅ PASS | Go code in `pkg/`, follows Effective Go, godoc for exports |
| **III. Multi-Cloud Neutrality** | ✅ PASS | Recipe sources support Azure, AWS, on-prem; Resource Types are cloud-agnostic |
| **IV. Testing Pyramid Discipline** | ✅ PASS | Unit tests for skills, integration tests for CLI, functional tests for E2E |
| **V. Collaboration-Centric Design** | ✅ PASS | Developers use discovery; Platform engineers configure Recipe sources and team practices |
| **VI. Open Source and Community-First** | ✅ PASS | Spec authored in public repo; community can contribute Resource Types catalog |
| **VII. Simplicity Over Cleverness** | ✅ PASS | Skills are single-purpose; LLM handles complexity of language analysis |
| **VIII. Separation of Concerns** | ✅ PASS | Skills layer → Core engine → Language analyzers; clear module boundaries |
| **IX. Incremental Adoption** | ✅ PASS | Feature is additive; existing Radius workflows unaffected |
| **XII. Resource Type Schema Quality** | ✅ PASS | Pre-defined catalog with validated schemas; fallback generation prompts contribution |
| **XIII. Recipe Development Standards** | ✅ PASS | Integrates with existing Recipe sources (AVM, Terraform, Bicep repos) |

**Gate Result**: ✅ PASS - No constitution violations identified.

## Project Structure

### Documentation (this feature)

```text
specs/001-auto-app-discovery/
├── plan.md              # This file (/speckit.plan command output)
├── research.md          # Phase 0 output (/speckit.plan command)
├── data-model.md        # Phase 1 output (/speckit.plan command)
├── quickstart.md        # Phase 1 output (/speckit.plan command)
├── contracts/           # Phase 1 output (/speckit.plan command)
│   └── skills.json      # MCP skill schemas
└── tasks.md             # Phase 2 output (/speckit.tasks command)
```

### Source Code (repository root)

```text
# Skills and Core Engine
pkg/
├── cli/
│   └── cmd/
│       └── radinit/
│           └── app/
│               ├── discover.go      # rad app discover command
│               └── generate.go      # rad app generate command
├── discovery/                       # NEW: Core discovery engine
│   ├── skills/                      # Composable skills layer
│   │   ├── discover_dependencies.go
│   │   ├── discover_services.go
│   │   ├── discover_team_practices.go
│   │   ├── generate_resource_types.go
│   │   ├── discover_recipes.go
│   │   ├── generate_app_definition.go
│   │   └── validate_app_definition.go
│   ├── engine/                      # Core analysis engine
│   │   ├── analyzer.go              # LLM-based codebase analyzer
│   │   ├── practices.go             # Team practices detection
│   │   └── bicep_generator.go       # Bicep output generation
│   └── types/                       # Shared types
│       ├── dependency.go
│       ├── service.go
│       ├── practice.go
│       └── recipe.go
└── mcp/                             # NEW: MCP server implementation
    ├── server.go                    # MCP server (stdio + HTTP)
    ├── handlers.go                  # Skill invocation handlers
    └── transport.go                 # Transport layer (stdio, HTTP)

# CLI Commands
cmd/
└── rad/
    └── cmd/
        ├── app_discover.go          # Wire discover command
        ├── app_generate.go          # Wire generate command
        └── mcp_serve.go             # Wire MCP server command

# Tests
test/
├── functional-portable/
│   └── discovery/                   # NEW: Functional tests
│       ├── discover_test.go
│       ├── generate_test.go
│       └── testdata/                # Sample codebases
│           ├── nodejs-ecommerce/
│           ├── python-api/
│           └── go-microservice/
└── unit/
    └── discovery/                   # NEW: Unit tests
        ├── skills_test.go
        └── engine_test.go
```

**Structure Decision**: The feature extends the existing Radius CLI with new commands under `rad app` and adds a new `pkg/discovery/` package for the core engine. MCP server is implemented in `pkg/mcp/` following the skills-first architecture where CLI commands are thin wrappers around skill implementations.

## Complexity Tracking

> No constitution violations requiring justification.

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| [e.g., 4th project] | [current need] | [why 3 projects insufficient] |
| [e.g., Repository pattern] | [specific problem] | [why direct DB access insufficient] |
