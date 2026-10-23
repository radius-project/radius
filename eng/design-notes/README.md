# Design Notes

This directory contains design documents that describe the architecture, features, and technical decisions behind the Radius project.

## Directory Structure

Design notes are organized by topic area:

| Directory        | Description                                                           |
|------------------|-----------------------------------------------------------------------|
| `architecture/`  | System-level architecture decisions                                   |
| `cli/`           | CLI-specific designs                                                  |
| `extensibility/` | Resource extensibility, user-defined types, and compute extensibility |
| `gitops/`        | GitOps integration designs                                            |
| `guides/`        | Living design guidelines (e.g., API design)                           |
| `recipes/`       | Recipe engine and providers                                           |
| `security/`      | Threat models and security designs                                    |
| `templates/`     | Design document templates                                             |
| `tools/`         | Engineering tools and workflows                                       |
| `ucp/`           | Universal Control Plane designs                                       |

## Retained Specifications

These specifications and implementation plans preserve the design history of implemented features. Their original draft statuses, planning checklists, and follow-up work are historical context, not current implementation status.

- [Long-running tests use the current release](lrt-current-release.md)
- [Remove the Bicep types submodule](remove-bicep-types-submodule.md)
- [Agent Ex features](agent-ex-features.md)
- [Agent Ex implementation plan](agent-ex-plan.md)
- [Agent Ex contributing-docs implementation plan](agent-ex-phase-3-plan.md)
- [Resource type icons](resource-type-icons.md)
- [Application graph dependency edges](graph-dependency-edges.md)
- [Live deployment graph progress](live-deploy-graph.md)

## Adding a New Design Note

1. Place the document in the appropriate topic directory
2. Use the naming convention `YYYY-MM-description.md` (e.g., `2024-06-private-bicep-registries.md`)
3. Update this README if adding a new topic directory

## Migration Plan

See [migration-plan.md](migration-plan.md) for details on which documents were migrated from the [radius-project/design-notes](https://github.com/radius-project/design-notes) repository and the rationale for the directory structure.
