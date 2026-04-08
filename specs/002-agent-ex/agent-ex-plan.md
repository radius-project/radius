<!-- markdownlint-disable MD060 -->
# Agent Ex — Implementation Plan

This document describes the implementation mechanics for delivering the capabilities defined in [agent-ex-features.md](agent-ex-features.md).

---

## Current State

| Artifact | `radius/` | `dashboard/` | `docs/` | `resource-types-contrib/` | `bicep-types-aws/` |
|---|---|---|---|---|---|
| `copilot-instructions.md` | ✅ | ❌ | ❌ | ❌ | ❌ |
| `.github/instructions/` | ✅ (7) | ❌ | ❌ | ❌ | ❌ |
| `.github/skills/` | ✅ (5) | ❌ | ❌ | ❌ | ❌ |
| `.github/agents/` | ✅ (1) | ❌ | ❌ | ❌ | ❌ |
| `.github/prompts/` | ✅ (3) | ❌ | ❌ | ❌ | ❌ |
| `docs/contributing/` | ✅ (37) | ✅ (4) | ❌ | ✅ (5) | ✅ (3) |
| `.devcontainer/` | ✅ | ✅ | ✅ | ✅ | ✅ |
| `copilot-setup-steps.yml` | ❌ | ❌ | ❌ | ❌ | ❌ |

**Summary**: `radius/` is well-instrumented. The other four repos have zero agent infrastructure.

---

## Phase Validation

Each phase produces a set of **validation prompts** — questions or tasks you give to the agent that should succeed if the phase's deliverables are effective. These are committed as `radius.validate.<phase>.prompt.md` files so they're reusable, reviewable, and serve as living acceptance tests.

Validation has two layers:

- **Deterministic checks**: file exists, build passes, CI green, token count within budget. Automatable and reliable.
- **Validation prompts**: agent interactions that exercise the new infrastructure. Ask the agent to do something the phase was designed to enable; check if the answer is correct. Probabilistic, so run each prompt 2–3 times to confirm consistency.

A phase is not complete until both layers pass.

---

## Phase 0: Cloud Agent Environments

**Enables**: [1.1 Set up a development environment](agent-ex-features.md#11-set-up-a-development-environment)

Create `copilot-setup-steps.yml` for each code repo, aligned with its dev container.

**Strategy**: Version files (`go.mod`, `.node-version`, `.python-version`) are the single source of truth. Dev container features and GHA setup actions both read them. Shared post-create scripts handle project-specific tools. The dev container is a superset (includes k3d, kind, stern); the cloud agent gets the build/test subset.

| Repo | GHA setup actions | Shared script |
|---|---|---|
| `radius/` | `setup-go`, `setup-node`, `setup-python` | `.devcontainer/post-create.sh` |
| `dashboard/` | `setup-node` | `.devcontainer/postcreate.sh` |
| `docs/` | `setup-node` | `.devcontainer/post-create.sh` |
| `resource-types-contrib/` | `setup-node`, `setup-terraform` | `.devcontainer/post-create.sh` |
| `bicep-types-aws/` | `setup-node`, `setup-go` | (none) |

Action items:

- [ ] Create version files (`.node-version`, `.python-version`) where missing
- [ ] Ensure post-create scripts are idempotent and work on both dev container and GHA runner
- [ ] Create `copilot-setup-steps.yml` per repo
- [ ] Cross-reference comments between `devcontainer.json` and `copilot-setup-steps.yml`

**Validation**:

- Deterministic: Assign a test issue to Copilot cloud agent in each repo. The agent must successfully check out, install dependencies, and run `make build` (or equivalent) without manual tool installation.
- Prompt: "Build this project and run the unit tests." — agent should succeed in cloud agent environment without trial-and-error tool installation.

---

## Phase 1: Contributing Docs

**Enables**: All [Build and Test](agent-ex-features.md#1-build-and-test-features-and-bug-fixes) sub-scenarios (including [1.4 CLI commands](agent-ex-features.md#14-add-or-update-cli-commands)), [Explain Architecture](agent-ex-features.md#4-explain-architecture-and-design)

Contributing docs are the foundation that skills reference. Fix the foundation first.

- [ ] **`radius/`**: Audit all 37 docs for accuracy. Standardize format (Purpose → Prerequisites → Steps → Verification → Troubleshooting). Add missing docs for TypeSpec pipeline, full test matrix, dev environment setup.
- [ ] **`dashboard/`**: Expand from 4 to cover prerequisites, building, testing, plugin development.
- [ ] **`docs/`**: Create local contributor guide (currently external link only).
- [ ] **`resource-types-contrib/`**: Review 5 docs for accuracy. Add prerequisites. Create missing `CONTRIBUTING.md`.
- [ ] **`bicep-types-aws/`**: Review 3 docs. Add prerequisites, test documentation, type generation pipeline overview.

**Gate**: After completing Phase 1, re-evaluate the skills identified in the Gaps section and the backlog. The doc audit will reveal which workflows are truly non-obvious vs. well-documented enough for agents to follow unaided. Adjust the Phase 2 skill list before proceeding.

**Validation**:

- Deterministic: Each doc follows the standard format (Purpose → Prerequisites → Steps → Verification → Troubleshooting). Links resolve. Code snippets are syntactically valid.
- Prompts: "How do I run the control plane locally?" / "How do I add a resource type?" / "How do I build the dashboard?" — agent should find and follow the contributing doc to produce a correct, step-by-step answer for each repo.

---

## Phase 2: Skills

**Enables**: [1.3 Schema changes](agent-ex-features.md#13-modify-api-type-definitions-schema-changes), [1.4 CLI commands](agent-ex-features.md#14-add-or-update-cli-commands) (backlog), [1.9 Resource type definitions](agent-ex-features.md#19-add-or-update-resource-type-definitions-contrib), [1.13 AWS Bicep types](agent-ex-features.md#113-generate-aws-bicep-types) (backlog), [4. Explain architecture](agent-ex-features.md#4-explain-architecture-and-design)

Create 4 new skills (see Gaps section). Update 5 existing `radius/` skills:

- [ ] `radius-build-cli` — Add contrib doc link. Evaluate if a full skill is justified or if a doc reference in `copilot-instructions.md` suffices.
- [ ] `radius-build-images` — Verify accuracy. Retain (multi-step, Radius-specific).
- [ ] `radius-install-custom` — Verify accuracy. Retain.
- [ ] `architecture-documenter` — Retain (specialized Mermaid workflow).
- [ ] `contributing-docs-updater` — Update for multi-repo doc structure.

**Validation**:

- Deterministic: Each skill file < 500 lines. Each skill references ≥1 contributing doc. Skill folder follows naming convention.
- Prompts: For each skill, invoke it and verify the agent follows the correct multi-step workflow. E.g., invoke `radius-schema-changes` and confirm it runs TypeSpec → Swagger → Go in the right order.

---

## Phase 3: Instructions

**Enables**: [1.2 Go code](agent-ex-features.md#12-write-and-modify-go-code), [1.5 GitHub workflows](agent-ex-features.md#15-edit-github-actions-workflows), [1.6 Dockerfiles](agent-ex-features.md#16-write-and-modify-dockerfiles), [1.7 Bicep](agent-ex-features.md#17-write-and-modify-bicep-files), [1.8 Shell/Make](agent-ex-features.md#18-write-shell-scripts-and-makefiles), [1.10 Dashboard plugins](agent-ex-features.md#110-develop-dashboard-plugins), [1.11 Documentation](agent-ex-features.md#111-author-and-edit-documentation), [2. Code review](agent-ex-features.md#2-review-code)

Create instructions listed in the Gaps section. Each file < 2K tokens. Only Radius-specific conventions — don't repeat what linters enforce or models know.

**Validation**:

- Deterministic: Each instruction file < 200 lines. `applyTo` patterns match intended file types (test with `glob` matching). No overlap with linter-enforced rules.
- Prompts: Open a file matching the `applyTo` pattern and ask the agent to write new code in that file. Verify the output follows the conventions in the instruction (e.g., TypeSpec naming, YAML schema structure).

---

## Phase 4: Per-Repo `copilot-instructions.md`

**Enables**: All capabilities — this is the agent's entry point to each repo.

Each repo gets a `copilot-instructions.md` covering: repo purpose, tech stack, available skills/instructions/agents/prompts, link to `CONTRIBUTING.md`.

- [ ] New: `dashboard/`, `docs/`, `resource-types-contrib/`, `bicep-types-aws/`
- [ ] Update: `radius/` (reflect new skill inventory)

**Validation**:

- Deterministic: Each file < 2 pages. Lists all skills, instructions, agents, and prompts in the repo.
- Prompt: Start a fresh chat in each repo and ask "What can you help me with in this project?" — agent should reference the repo's purpose, available skills, and link to contributing docs.

---

## Phase 5: Agents and Prompts

**Enables**: [1.12 Create PRs](agent-ex-features.md#112-create-pull-requests-and-manage-contributions), [2. Review code](agent-ex-features.md#2-review-code), [3. Investigate issues](agent-ex-features.md#3-investigate-issues), [1.4 CLI commands](agent-ex-features.md#14-add-or-update-cli-commands) (prompt), [1.9 Resource types](agent-ex-features.md#19-add-or-update-resource-type-definitions-contrib)

- [ ] Replicate `issue-investigator` across repos (or make workspace-level)
- [ ] Create: `resource-type-contributor` (`resource-types-contrib/`), `dashboard-developer` (`dashboard/`), `docs-contributor` (`docs/`)
- [ ] Adapt `code-review` and `create-pr` prompts for all repos
- [ ] Add repo-specific prompts: `add-resource-type`, `add-recipe` (`resource-types-contrib/`), `add-cli-command` (`radius/`)

**Validation**:

- Deterministic: Agent and prompt files follow naming conventions. Agents are invocable (`@radius-*` resolves).
- Prompts: Invoke each agent with a representative task. E.g., `@radius-resource-type-contributor` should walk through creating a complete resource type. Run each workflow prompt end-to-end.

---

## Phase 6: Skill Lifecycle Agent

**Enables**: [5. Continuously improve agent effectiveness](agent-ex-features.md#5-continuously-improve-agent-effectiveness)

Create an automated agent that runs weekly, analyzes Copilot Agent session logs, and recommends skill additions, edits, or removals based on real usage patterns. This closes the feedback loop by replacing manual quarterly reviews with data-driven, continuous curation.

**How it works**:

1. **Log collection**: A scheduled GitHub Actions workflow gathers Copilot Agent session logs. Sources include:
   - VS Code debug logs (`$VSCODE_TARGET_SESSION_LOG` paths) committed/uploaded as CI artifacts from cloud agent runs
   - GitHub Copilot audit log API (organization-level, if available)
   - Copilot coding agent run logs from GitHub Issues/PRs (visible in PR comments and check runs)
   - Any manually captured session logs stored in `.copilot-tracking/logs/`

2. **Analysis agent**: The workflow invokes an LLM-backed analysis step (via GitHub Models or a Copilot agent prompt) that reviews the collected logs and evaluates:
   - **Add signals**: Repeated agent failures on the same workflow, trial-and-error tool installation, questions the agent couldn't answer, multi-step tasks where the agent lost its way
   - **Edit signals**: Skills that were invoked but produced incorrect or outdated guidance, instructions the agent ignored or misapplied, workflows where the agent succeeded but took unnecessary detours
   - **Remove signals**: Skills that were never invoked over the review window, instructions with zero file-pattern matches in recent sessions, context that duplicates what the model already knows (wasted token budget)

3. **Output**: The agent files a GitHub Issue per repo with a structured recommendation report:
   - Skills to add (with justification from log evidence)
   - Skills to edit (with specific problems observed)
   - Skills to remove (with usage metrics showing disuse)
   - Instructions to adjust (coverage gaps or overlaps)
   - Context budget impact estimate (tokens saved/added)

4. **Human-in-the-loop**: Recommendations are issues, not auto-applied changes. A maintainer reviews, triages, and either implements or closes with rationale. No automated changes to skills without review.

**Implementation**:

- [ ] Define log format and storage convention (`.copilot-tracking/logs/` per repo, or a central location in `design-notes/`)
- [ ] Create a GitHub Actions workflow (`skill-lifecycle-review.yml`) that runs on `schedule: cron` (weekly)
- [ ] Build the analysis prompt (`radius.skill-lifecycle-review.prompt.md`) that encodes the add/edit/remove signal heuristics
- [ ] Create the `radius-skill-lifecycle-reviewer` agent definition for local/manual runs outside CI
- [ ] Establish a log retention policy (e.g., rolling 4-week window) to bound storage and analysis scope

**Dependencies**: Phases 2–5 must be complete so there is a meaningful set of skills, instructions, and agents generating logs to analyze. Log collection infrastructure may require coordination with GitHub org admins for audit log API access.

**Validation**:

- Deterministic: Workflow runs on schedule without failure. Issue is created with valid markdown and all required sections. Log retention policy is enforced (no logs older than retention window).
- Prompt: Seed `.copilot-tracking/logs/` with 3–4 synthetic session logs (one with repeated failures, one with an unused skill, one with a successful but roundabout workflow). Run the analysis agent manually and verify it produces correct add/edit/remove recommendations matching the synthetic evidence.

---

## Execution Order

Each phase is implemented through Spec Kit. Before starting a phase, use `@radius-spec-kit-prompt-agent` to generate a `/speckit.specify` prompt scoped to that phase's deliverables. This keeps each specification focused and right-sized.

1. **Phase 0** — `copilot-setup-steps.yml` (independent; start immediately; begin with `radius/`)
2. **Phase 1** — Contributing docs (start with `radius/`, then satellite repos)
3. **Phases 2–4** — Skills, instructions, `copilot-instructions.md` (can parallelize across repos after Phase 1)
4. **Phase 5** — Agents and prompts
5. **Phase 6** — Skill lifecycle agent (after Phases 2–5 are live and generating logs; allow 2–4 weeks of log accumulation before first meaningful run)

---

## Naming Conventions

All skills, agents, and prompts use a `radius.` prefix so they are visually distinct from built-in and third-party items in VS Code chat completions.

| Artifact | Pattern | Example | Appears in chat as |
|---|---|---|---|
| Skill | `radius-<repo>-<verb>-<noun>/` | `radius-core-schema-changes/` | Listed by description in skill picker |
| Instruction | `<technology>.instructions.md` | `typescript.instructions.md` | N/A (auto-applied, not user-facing) |
| Agent | `radius-<name>.agent.md` | `radius-resource-type-contributor.agent.md` | `@radius-resource-type-contributor` |
| Prompt | `radius.<repo>.<action>.prompt.md` | `radius.contrib.add-resource-type.prompt.md` | `/radius.contrib.add-resource-type` |
| Lifecycle workflow | `skill-lifecycle-review.yml` | `.github/workflows/skill-lifecycle-review.yml` | N/A (scheduled, not user-facing) |

**Why `radius.` / `radius-`**: Chat UIs show agents as `@name` and prompts as `/name`. Without a namespace prefix, `@docs-contributor` or `/add-resource-type` could collide with extensions or other projects. The prefix groups all Radius items together in alphabetical lists and makes them immediately recognizable.

**Repo short names**: `core` (radius/), `dash` (dashboard/), `contrib` (resource-types-contrib/), `docs` (docs/), `bicep-aws` (bicep-types-aws/).

---
