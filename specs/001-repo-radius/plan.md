# Implementation Plan: Repo Radius

**Branch**: `001-repo-radius` | **Date**: 2026-02-02 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/001-repo-radius/spec.md`

**Note**: This template is filled in by the `/speckit.plan` command. See `.specify/templates/commands/plan.md` for the execution workflow.

## Summary

Repo Radius is a Git-centric deployment mode that runs without a centralized control plane. It treats a Git repository as the system of record and is optimized for CI/CD workflows. The implementation uses hashicorp/terraform-exec for Terraform operations, Cobra + Bubble Tea for CLI, and stores all state in the repository's `.radius/` directory.

**Key Commands**: `rad init`, `rad plan`, `rad deploy`, `rad diff`, `rad app delete`

## Technical Context

**Language/Version**: Go 1.23+
**Primary Dependencies**:
- spf13/cobra v1.10.2 (CLI framework)
- charmbracelet/bubbletea v1.3.10 (interactive prompts)
- hashicorp/terraform-exec (Terraform execution)
- joho/godotenv v1.5.x (.env parsing)
- go-git/go-git v5 (Git operations - limited use)
- Git CLI via os/exec (sparse-checkout, primary Git operations)

**Storage**: Git repository (`.radius/` directory structure), local filesystem
**Testing**: Go testing with testify, mock interfaces for external dependencies
**Target Platform**: macOS, Linux, Windows (cross-platform CLI)
**Project Type**: CLI tool extending existing `cmd/rad` codebase
**Performance Goals**: CLI responsiveness (<2s for init, plan preview within Terraform limits)
**Constraints**: Must work offline after initial resource type fetch; no control plane dependency
**Scale/Scope**: Single repository, single developer or CI/CD pipeline

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

The project constitution is a template placeholder. No specific gates defined.

**Applicable Principles**:
- Follow existing Radius codebase patterns
- Use TDD where practical
- Keep CLI interface simple and scriptable

## Project Structure

### Documentation (this feature)

```text
specs/001-repo-radius/
├── plan.md              # This file (/speckit.plan command output)
├── research.md          # Phase 0 output - COMPLETE
├── data-model.md        # Phase 1 output - COMPLETE
├── quickstart.md        # Phase 1 output - COMPLETE
├── contracts/           # Phase 1 output - COMPLETE
│   └── cli-commands.md  # CLI interface contracts
└── tasks.md             # Phase 2 output (/speckit.tasks command - NOT created by /speckit.plan)
```

### Source Code (repository root)

```text
pkg/cli/
├── cmd/
│   └── rad/
│       ├── init.go          # rad init command (modified for git workspace)
│       ├── plan.go          # rad plan command (new)
│       ├── deploy.go        # rad deploy command (new)
│       ├── diff.go          # rad diff command (new)
│       └── workspace.go     # rad workspace commands (modified)
│
├── reporadius/              # NEW: Repo Radius core package
│   ├── config/
│   │   ├── environment.go   # .env file handling
│   │   ├── recipepack.go    # Recipe Pack parsing
│   │   ├── workspace.go     # Workspace configuration
│   │   └── types.go         # Resource Type loading
│   │
│   ├── plan/
│   │   ├── planner.go       # Plan generation orchestration
│   │   ├── terraform.go     # Terraform artifact generation
│   │   ├── bicep.go         # Bicep artifact generation
│   │   └── manifest.go      # plan.yaml handling
│   │
│   ├── deploy/
│   │   ├── deployer.go      # Deployment orchestration
│   │   ├── executor.go      # Step execution (terraform-exec wrapper)
│   │   └── record.go        # Deployment record handling
│   │
│   ├── diff/
│   │   └── differ.go        # State comparison
│   │
│   └── git/
│       ├── sparse.go        # Sparse checkout for resource types
│       └── state.go         # Git state detection

tests/
├── unit/
│   └── reporadius/          # Unit tests for each package
└── integration/
    └── reporadius/          # E2E CLI tests
```

**Structure Decision**: Follows existing Radius `pkg/cli/` patterns. New `reporadius/` package contains Git workspace-specific logic, separate from control plane code. Enables code sharing with control plane via interfaces.

## Complexity Tracking

No violations identified. Implementation follows existing Radius patterns.
