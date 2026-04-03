# Implementation Plan: Long-Running Tests Use Current Release

**Branch**: `001-lrt-current-release` | **Date**: 2024-12-15 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/001-lrt-current-release/spec.md`

## Summary

Update the long-running Azure test workflow to install the current official Radius release (including release candidates when published) instead of building from the main branch. The workflow will use the official installer script to install the CLI, detect the control plane version on the cluster using `rad version`, and intelligently install/upgrade based on version comparison. All build-related logic (container image builds, caching, skip-build conditions) will be removed.

## Technical Context

**Language/Version**: YAML (GitHub Actions), Bash
**Primary Dependencies**: Radius CLI (installed via official installer), `rad version`, `rad upgrade kubernetes`, `rad install kubernetes`
**Storage**: N/A (workflow only)
**Testing**: Manual workflow execution, CI validation
**Target Platform**: GitHub Actions runners (ubuntu-24.04), AKS cluster
**Project Type**: CI/CD workflow modification, installer script enhancement
**Performance Goals**: Workflow completion time should be comparable or faster (no build step)
**Constraints**: Must work with existing AKS test infrastructure, must handle version detection edge cases
**Scale/Scope**: Single workflow file modification

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle                           | Status  | Notes                                                                                    |
|-------------------------------------|---------|------------------------------------------------------------------------------------------|
| I. API-First Design                 | ✅ Pass | N/A - workflow change, not API                                                           |
| II. Idiomatic Code Standards        | ✅ Pass | Bash scripts follow shell.instructions.md, YAML follows github-workflows.instructions.md |
| III. Multi-Cloud Neutrality         | ✅ Pass | N/A - test infrastructure only                                                           |
| IV. Testing Pyramid Discipline      | ✅ Pass | Workflow tests itself via functional test execution                                      |
| V. Collaboration-Centric Design     | ✅ Pass | Simplifies workflow for all contributors                                                 |
| VI. Open Source and Community-First | ✅ Pass | Design documented in design-notes                                                        |
| VII. Simplicity Over Cleverness     | ✅ Pass | Removes complexity (build logic), uses standard CLI commands                             |
| VIII. Separation of Concerns        | ✅ Pass | Version detection logic in shell script, workflow handles orchestration                  |
| IX. Incremental Adoption            | ✅ Pass | No breaking changes to other workflows                                                   |

## Project Structure

### Documentation (this feature)

```text
specs/001-lrt-current-release/
├── spec.md                      # Feature specification
├── plan.md                      # This file
├── technical-plan-context.md    # CLI command reference
├── research.md                  # Phase 0 output
├── checklists/
│   └── requirements.md          # Specification quality checklist
└── tasks.md                     # Phase 2 output (created by /speckit.tasks)
```

### Source Code (radius repository)

```text
radius/
├── .github/
│   ├── workflows/
│   │   └── long-running-azure.yaml    # Primary file to modify
│   └── scripts/
│       └── manage-radius-installation.sh  # New: version detection and install/upgrade logic
├── deploy/
│   └── install.sh                     # Modify: add optional --include-rc parameter
└── Makefile                           # No changes needed (tests run via existing make targets)
```

**Structure Decision**: Minimal changes - single workflow file modification, a helper script for version management logic, and an enhancement to the official installer script. The helper script follows the principle of extracting complex logic from workflow YAML into testable shell scripts. The installer script enhancement adds an optional parameter to include release candidates when determining the latest version.

## Complexity Tracking

No constitution violations. This change reduces complexity by removing:

- Build job (~200 lines)
- Skip-build logic (~50 lines)
- Caching logic (~40 lines)
- Build-related environment variables (~10 lines)

## Post-Design Constitution Re-Check

*Re-evaluated after Phase 1 design completion.*

| Principle                           | Status  | Post-Design Notes                                                              |
|-------------------------------------|---------|--------------------------------------------------------------------------------|
| I. API-First Design                 | ✅ Pass | No API changes                                                                 |
| II. Idiomatic Code Standards        | ✅ Pass | Shell script follows `set -euo pipefail`, proper quoting, clear variable names |
| III. Multi-Cloud Neutrality         | ✅ Pass | Test infrastructure only, no cloud-specific dependencies added                 |
| IV. Testing Pyramid Discipline      | ✅ Pass | Functional tests validate the workflow; manual verification steps documented   |
| V. Collaboration-Centric Design     | ✅ Pass | Simplified workflow benefits all contributors                                  |
| VI. Open Source and Community-First | ✅ Pass | Design fully documented in design-notes repository                             |
| VII. Simplicity Over Cleverness     | ✅ Pass | Net reduction of ~300 lines; uses standard CLI commands                        |
| VIII. Separation of Concerns        | ✅ Pass | Version logic in shell script, workflow handles orchestration                  |
| IX. Incremental Adoption            | ✅ Pass | No breaking changes to other workflows or user-facing behavior                 |

**Gate Status**: ✅ PASSED - Ready for task breakdown (/speckit.tasks)

## Implementation Phases

### Phase 0: Installer Script Enhancement

- Modify `deploy/install.sh` to add an optional `--include-rc` parameter
- When `--include-rc` is specified, the `getLatestRelease` function should include release candidate versions (remove `grep -v rc` filter)
- When `--include-rc` is not specified, behavior remains unchanged (release candidates are excluded)
- Add corresponding `INCLUDE_RC` environment variable support (e.g., `INCLUDE_RC=true`) as an alternative to the command-line flag
- Update script usage/help text to document the new parameter
- Ensure backward compatibility: existing installations without the flag continue to work as before

### Phase 1: Setup (Shared Infrastructure)

- Create `manage-radius-installation.sh` helper script
- Implement version detection logic (parse rad version output, including release candidate version formats like `v0.40.0-rc1`)
- Implement conditional install/upgrade logic
- Add error handling for upgrade failures

### Phase 2: Remove Build Job (User Story 4)

- Delete the entire `build` job from workflow
- Remove build-related environment variables
- Update workflow file header comments

### Phase 3: Add CLI Installation (User Story 1)

- Add step to install CLI via official installer with `--include-rc` flag to support release candidate versions
- Add step to verify CLI installation
- Update PATH configuration
- Move recipe publishing steps to tests job

### Phase 4: Smart Control Plane Installation (User Story 2)

- Add step to invoke manage-radius-installation.sh script
- Ensure script has execute permissions
- Add logging output for version detection results

### Phase 5: Graceful Upgrade Failure Handling (User Story 3)

- Add error capture and display when rad upgrade command fails
- Ensure non-zero exit code propagates to workflow
- Add descriptive error message explaining upgrade failure reason

### Phase 6: Polish & Cross-Cutting Concerns

- Remove skip-build workflow_dispatch input option
- Verify BICEP_RECIPE_REGISTRY and TEST_BICEP_TYPES_REGISTRY env vars are retained
- Update workflow file header documentation block
- Run workflow validation per quickstart.md success verification checklist
- Verify functional tests execute successfully with installed CLI

## References

- [Feature Specification](spec.md)
- [Technical Context](technical-plan-context.md)
- [Research Findings](research.md)
- [Quick Implementation Reference](quickstart.md)
- [GitHub Workflows Best Practices](../../.github/instructions/github-workflows.instructions.md)
- [Shell Script Guidelines](../../.github/instructions/shell.instructions.md)
