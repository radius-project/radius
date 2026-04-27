
# Agent Ex — Asset Inventory

This document catalogs the skills, instructions, repo infrastructure, and alignment fixes needed to deliver the capabilities defined in [agent-ex-features.md](agent-ex-features.md). It serves as the planning checklist for what to build; [agent-ex-plan.md](agent-ex-plan.md) describes when and how to build it.

---

## Gaps

The items below are illustrative examples based on the current-state assessment. The actual scope of work will be determined within each phase, as Spec Kit specifications are developed and implementations are tested.

### Missing skills (create now)

| Skill | Repo | Backing doc | Why |
|---|---|---|---|
| `radius-schema-changes` | `radius/` | `contributing-code-schema-changes/` | Multi-tool pipeline (TypeSpec → Swagger → Go); error-prone |
| `radius-run-controlplane` | `radius/` | `running-controlplane-locally.md` | Orchestrates multiple processes; Radius-specific |
| `radius-debug-components` | `radius/` | `radius-os-processes-debugging.md` | Non-obvious process topology |
| `radius-contrib-add-resource-type` | `resource-types-contrib/` | `contributing-resource-types-recipes.md` | Custom YAML + recipe scaffold; entirely project-specific |

### Skills backlog (defer until friction emerges)

`radius-run-tests`, `radius-add-cli-command`, `dashboard-build`, `dashboard-develop-plugin`, `docs-author-content`, `docs-build-preview`, `bicep-aws-type-generation`, and others. Standard workflows where contributing docs suffice.

### Missing instructions

| Instruction | Repo | Notes |
|---|---|---|
| `typespec.instructions.md` | `radius/` | TypeSpec API definitions; project-specific, high value |
| `typescript.instructions.md` | `dashboard/` | Radius-specific TS conventions only (not standard TS/React) |
| `yaml-schema.instructions.md` | `resource-types-contrib/` | Resource type YAML schema; entirely project-specific |
| `terraform.instructions.md` | `resource-types-contrib/` | Radius recipe conventions only |
| `markdown-docs.instructions.md` | `docs/` | Frontmatter, linking patterns, Hugo shortcodes |
| `code-review.instructions.md` | All 4 missing repos | Adapted from `radius/` |

Deferred: `python.instructions.md` (few scripts), `csharp.instructions.md` (low-frequency), `bicep.instructions.md` (contrib — may share `radius/`'s).

### Missing repo infrastructure

`dashboard/`, `docs/`, `resource-types-contrib/`, `bicep-types-aws/` each need: `.github/copilot-instructions.md`, instructions, and `copilot-setup-steps.yml`.

### Alignment issues

- Skills don't consistently link to contributing docs
- Contributing docs don't mention agent tooling
- Constitution `TODO(REPO_ADDENDUMS)` unresolved
- `code-review.instructions.md` is `radius/`-only
- No `copilot-setup-steps.yml` anywhere

---
