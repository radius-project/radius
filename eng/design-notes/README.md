# Design Notes

This directory contains design documents that describe the architecture, features, and technical decisions behind the Radius project.

## Directory Structure

Design notes are organized by topic area:

| Directory | Description |
|-----------|-------------|
| `architecture/` | System-level architecture decisions |
| `cli/` | CLI-specific designs |
| `extensibility/` | Resource extensibility, user-defined types, and compute extensibility |
| `gitops/` | GitOps integration designs |
| `guides/` | Living design guidelines (e.g., API design) |
| `recipes/` | Recipe engine and providers |
| `security/` | Threat models and security designs |
| `templates/` | Design document templates |
| `tools/` | Engineering tools and workflows |
| `ucp/` | Universal Control Plane designs |

### Related Directories

| Directory | Description |
|-----------|-------------|
| `../specs/` | Spec Kit specifications (structured project artifacts: plans, research, tasks, checklists) |

## Adding a New Design Note

1. Place the document in the appropriate topic directory
2. Use the naming convention `YYYY-MM-description.md` (e.g., `2024-06-private-bicep-registries.md`)
3. Update this README if adding a new topic directory

## Migration Plan

See [migration-plan.md](migration-plan.md) for details on which documents were migrated from the [radius-project/design-notes](https://github.com/radius-project/design-notes) repository and the rationale for the directory structure.
