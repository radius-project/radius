<!-- markdownlint-disable MD060 -->
# Agent Ex — Implementation Plan

This plan describes **how** to deliver the capabilities defined in [agent-ex-features.md](agent-ex-features.md).

The plan has one north star: **a single, durable knowledge base that humans read directly and every supported agent (GitHub Copilot in VS Code, GitHub Copilot CLI, GitHub Copilot Cloud Agent, and Claude Code) can also read.** Tool-specific UX layers are thin conveniences that link back to that knowledge base. We never hide a capability behind a single tool, and adding support for a new tool is itself a documented capability (see [agent-ex-features.md, Section 5.3](agent-ex-features.md#53-add-a-new-capability-to-the-agent-ex-system)).

---

## Summary

The plan, in order:

1. [**Phase 0 — Meta-tooling**](#phase-0--meta-tooling-foundation). Build the factory first: templates, naming conventions, authoring skills, an "add a capability" agent mode, docs-drift code-review instructions.
2. [**Phase 1 — `AGENTS.md`**](#phase-1--single-entry-point-for-every-agent). One entry point per repo. `.github/copilot-instructions.md` is a symlink to it.
3. [**Phase 2 — Cloud Agent bootstrap**](#phase-2--cloud-agent-bootstrap). `copilot-setup-steps.yml` + shared dev-container post-create script.
4. [**Phase 3 — Contributing docs**](#phase-3--contributing-docs) · [**Phase 4 — Architecture docs**](#phase-4--architecture-docs-grounded-in-code) · [**Phase 5 — Coding instructions**](#phase-5--coding-instructions-project-specific-only). Run in parallel. Audit, fill gaps, trim to project-specific only.
5. [**Phase 6 — Per-workflow conveniences**](#phase-6--per-workflow-copilot-conveniences). Skills, prompts, custom agents — only where justified.
6. [**Phase 7 — Continuous improvement**](#phase-7--continuous-improvement-loop). Weekly log-signal analysis + weekly docs-drift review.

Cross-cutting:

- [**Multi-repo rollout**](#4-multi-repo-rollout): `radius/` lands each phase first; satellites follow with a slimmer version.
- [**CI gates**](#6-ci-gates-deterministic-run-on-every-pr): deterministic only (size budgets, symlink, link check, `actionlint`, capability-index). Docs drift is not a blocking gate.

---

## 1. Tool landscape

The four supported AI tools read different files. The plan must work for all of them. The matrix also includes two GitHub.com surfaces (Chat and Code Review) that read a subset of the same files — they aren't in our supported-tools list, but our knowledge reaches them for free.

| File / location | Copilot (VS Code) | Copilot (Cloud Agent) | Copilot (CLI) | Copilot (GitHub.com Chat) | Copilot (Code Review) | Claude Code |
|---|:-:|:-:|:-:|:-:|:-:|:-:|
| `AGENTS.md` (root, canonical) | ✅ | ✅ | ✅ | — | — | ✅ (fallback) |
| `.github/copilot-instructions.md` → `AGENTS.md` (symlink) | ✅ | ✅ | ✅ | ✅ | ✅ | — |
| `CONTRIBUTING.md`, `docs/**` (linked from above) | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `.github/instructions/*.instructions.md` (path-scoped instructions) | ✅ | ✅ | ✅ | — | ✅ | via `.claude/rules/` |
| `.github/skills/*/SKILL.md` (Agent Skills open standard) | ✅ | ✅ | ✅ | — | — | via `.claude/skills/` |
| `.github/agents/*.agent.md` (custom agents) | ✅ | ✅ | ✅ | — | — | via `.claude/agents/` |
| `.github/prompts/*.prompt.md` | ✅ | — | — | — | — | — |
| `copilot-setup-steps.yml` | — | ✅ | — | — | — | — |

Sources:

- GitHub's [Support for different types of custom instructions](https://docs.github.com/en/copilot/reference/custom-instructions-support).
- GitHub's [About agent skills](https://docs.github.com/en/copilot/concepts/agents/about-agent-skills) — the Agent Skills open standard supports `.github/skills`, `.claude/skills`, or `.agents/skills` and works with Copilot Cloud Agent, Copilot CLI, and agent mode in VS Code.
- GitHub's [About custom agents](https://docs.github.com/en/copilot/concepts/agents/cloud-agent/about-custom-agents) — custom agents at `.github/agents/<name>.agent.md` work with Copilot Cloud Agent, Copilot CLI, and VS Code.
- Anthropic's [Create custom subagents](https://code.claude.com/docs/en/sub-agents) — Claude Code custom agents ("subagents") live at `.claude/agents/<name>.md`. Same Markdown-with-YAML-frontmatter shape as Copilot's, but with partially incompatible frontmatter fields, so a symlink doesn't work.
- Anthropic's [How Claude remembers your project](https://code.claude.com/docs/en/memory) — Claude Code falls back to `AGENTS.md` when no `CLAUDE.md` is present.

**Implication**:

- Repo-wide guidance lives **once** in `AGENTS.md`. `.github/copilot-instructions.md` is a real OS symlink to it so the entire Copilot family (including the GitHub.com surfaces) resolves to the same content; Claude Code reads `AGENTS.md` via fallback.
- Path-scoped instructions, skills, and custom agents are read by every Copilot agent surface (VS Code, Cloud Agent, CLI). Claude Code has its own equivalents at parallel paths (`.claude/rules/`, `.claude/skills/`, `.claude/agents/`) with partially incompatible frontmatter, so symlinks don't work. We treat the `.github/...` versions as canonical for our Copilot-first workflow and defer the Claude mirrors until we adopt each surface broadly enough to justify a generator.
- Prompts remain VS Code-only chrome.
- Adding support for a new AI tool follows the [agent-ex-features.md, Section 5.3](agent-ex-features.md#53-add-a-new-capability-to-the-agent-ex-system) "add a capability" workflow.

---

## 2. File strategy

A single rule governs every file we add:

> **Capability lives in docs. Tool-specific UX is just a wrapper. Wrappers link to docs.**

And a corollary that governs the [agent-ex-features.md capability list](agent-ex-features.md):

> **Every leaf capability is backed by one primary contributor doc under `CONTRIBUTING.md` or `docs/contributing/`.** That doc is the contributor's entry point for the capability; it may link out to architecture docs or sibling contributor docs for depth. A skill, prompt, or custom agent is optional; the primary doc is not. Parent (umbrella) capabilities are the union of their sub-capability rows. The mapping lives in the capability index in `docs/contributing/README.md` (built in Phase 3).

Concretely:

- **`AGENTS.md`** (≤ 2 pages) is the canonical orientation file. It points to everything else.
- **`.github/copilot-instructions.md`** is a real OS symlink to `AGENTS.md` so the Copilot family (which prefers that path) sees the same bytes. Claude Code reads `AGENTS.md` directly via its fallback behavior — no separate symlink needed.
- **`CONTRIBUTING.md` + `docs/contributing/`** hold all how-to knowledge: setup, build, test, debug, schema changes, releases.
- **`docs/architecture/`** holds code-grounded explanations of every major subsystem.
- **`.github/instructions/*.instructions.md`** are path-scoped Radius conventions, in Copilot's `applyTo:` format (≤ 200 lines each). They hold *only* what linters can't enforce. Claude Code reads its own equivalents at `.claude/rules/`.
- **`.github/skills/*/SKILL.md`** wrap multi-step Radius-specific workflows (≤ 500 lines each). Every skill MUST link to a contributing doc that contains the same steps in prose.
- **`.github/prompts/*.prompt.md`** are slash-command shortcuts (VS Code only). Every prompt MUST be reproducible by a non-VS-Code agent that reads the same backing doc.
- **`.github/agents/*.agent.md`** are custom agents — read by the Copilot agent surfaces (VS Code, Cloud Agent, CLI) per [GitHub's Custom Agents docs](https://docs.github.com/en/copilot/concepts/agents/cloud-agent/about-custom-agents). Claude Code reads its own equivalents at `.claude/agents/`. Body is backed by a doc per the same rule as skills.
- **`copilot-setup-steps.yml`** mirrors the dev container post-create script so the Cloud Agent gets the same environment.

---

## 3. Phases

Each phase unlocks visible capability and ships with a verification step. Phase 0 builds the meta-tooling that every later phase uses (and that capability #5 in [agent-ex-features.md](agent-ex-features.md) calls out as foundational: the system maintains itself). Phase 1 uses Phase 0's `AGENTS.md` template; Phase 2 is independent of Phase 0 and could even run first. Phases 3, 4, and 5 each depend only on Phase 0 and can run concurrently. Phase 6 depends on Phase 3 (so skills, prompts, and custom agents have backing docs to link to). Phase 7 depends on 3–6 living long enough to generate signal.

All five repos in scope (`radius/`, `dashboard/`, `docs/`, `resource-types-contrib/`, `bicep-types-aws/`) move through each phase in parallel; the work in satellite repos is smaller because there is less existing surface area.

---

### Phase 0 — Meta-tooling foundation

**Unlocks**: A working factory for everything that follows. Templates, conventions, code-review instructions that flag missing doc updates, and the agents that draft new docs and add new capabilities all exist before the rest of the system is built. Every later artifact — including `AGENTS.md` itself — is produced through the meta-tooling rather than reverse-engineered into shape.

**Why first**: this is the embodiment of the "system maintains itself" principle. The conventions and templates are cheap to write (just Markdown), they bootstrap from existing well-formed examples in `radius/` (e.g., [docs/contributing/contributing-code/contributing-code-control-plane/running-controlplane-locally.md](../../docs/contributing/contributing-code/contributing-code-control-plane/running-controlplane-locally.md), [docs/architecture/ucp.md](../../docs/architecture/ucp.md)), and they make every later phase consistent. Catching drift from day one means the freshly cleaned docs from Phase 3 cannot rot silently while the rest of the plan executes.

**Deliverables (in `radius/` only; satellite repos pick up each Phase 0 artifact when they reach the phase that needs it: the `AGENTS.md` template in Phase 1, the contributing- and architecture-doc format templates and the capability-index pattern in Phase 3, the docs-drift addition to `code-review.instructions.md` in Phase 5, and any skills or custom agents that prove useful in Phase 6)**:

- **Convention and template docs** under `docs/contributing/`:
  - `contributing-agent-assets.md` — file-strategy rule ([Section 2](#2-file-strategy)), file-size budgets, CI gates ([Section 6](#6-ci-gates-deterministic-run-on-every-pr)), naming conventions ([Section 5](#5-naming-conventions)), and templates for `AGENTS.md`, instructions, skills, prompts, and agent modes.
  - `authoring-contributing-docs.md` — standard format for contributing docs (Purpose → Prerequisites → Steps → Verification → Troubleshooting) and architecture docs (Entry points → Packages → Flow → Change-safety), with one annotated example of each.
  - `extending-agent-ex.md` — the "add a new capability" decision tree (doc only? instruction? skill? prompt? custom agent?), the live files to update (the primary contributing doc, the capability index in `docs/contributing/README.md`, `AGENTS.md` if a new top-level link is needed, and any optional wrappers), validation steps, and a repo-onboarding checklist. The planning docs `agent-ex-features.md` and `agent-ex-plan.md` are not in this list — they describe the original buildout and are not edited by ongoing capability work.
- **Skills and a custom agent** that automate the above (Copilot agent surfaces — VS Code, Cloud Agent, CLI; the docs above let any other tool do the same thing manually):
  - `radius-author-doc` skill — given a topic and a starting code reference, drafts a contributing or architecture doc using the template.
  - `radius-update-doc` skill — given a PR diff and an affected doc, proposes a targeted patch. Invoked manually by contributors and by the code-review instructions below.
  - `radius-add-capability` agent mode — walks a contributor through `docs/contributing/extending-agent-ex.md`: choosing the right asset type, authoring or extending the primary contributing doc, scaffolding any optional wrappers, and updating the capability index and `AGENTS.md`.
  - `/radius-author-doc` and `/radius-add-capability` prompts as VS Code shortcuts.
- **Docs-drift code-review instructions**: extend `.github/instructions/code-review.instructions.md` (and replicate to satellites in Phase 5) with a doc-impact assessment step. The reviewer (Copilot Code Review on GitHub.com, plus any agent surface running a review) inspects the PR diff and, when changes touch code paths whose behavior is documented in `CONTRIBUTING.md`, `docs/contributing/`, or `docs/architecture/`, suggests the specific doc(s) that likely need updating and what to change. Backed by a per-repo path map (`<code-glob>` ↔ `<doc-path>`) checked into `docs/contributing/contributing-agent-assets.md` so the suggestion is concrete rather than vague. Land the map empty; it grows as Phase 3 fills out the docs. This is advisory, not a blocking gate — drift detection at scale is handled by the Phase 7 lifecycle workflow.

**Verification**:

- Deterministic: every template and convention doc exists and renders. The code-review instructions reference the path map and the `radius-update-doc` skill.
- Prompt: invoke `/radius-author-doc` with a known topic; the produced draft matches the template (correct headings, links to a real code reference, no hallucinated paths). Invoke `@radius-add-capability` with a fake new capability; the agent produces a coherent set of edits that pass the [Section 6 CI gates](#6-ci-gates-deterministic-run-on-every-pr) without manual cleanup. Open a synthetic PR that changes a mapped code path without touching the mapped doc; Copilot Code Review (or an agent following the same instructions) flags the missing doc update and proposes a concrete patch.

---

### Phase 1 — Single entry point for every agent

**Unlocks**: Any agent in any tool, in any repo, gets the same orientation on first read. Uses the `AGENTS.md` template from Phase 0.

**Deliverables (per repo)**:

- New `AGENTS.md` at repo root, ≤ 2 pages, generated from the Phase 0 template (or by invoking `/radius-author-doc` with the AGENTS.md template). Sections:
  1. What this repo is and what it ships
  2. Tech stack and repo layout (1 paragraph each)
  3. How to build and test → link to `CONTRIBUTING.md`
  4. How the system works → link to `docs/architecture/README.md`
  5. Conventions → link to `.github/instructions/`
  6. Copilot agent surface users (VS Code, Cloud Agent, CLI): link to `.github/skills/`, `.github/agents/`. VS Code users additionally: link to `.github/prompts/`.
  7. How to contribute → link to `CONTRIBUTING.md` and to `docs/contributing/extending-agent-ex.md`
- `.github/copilot-instructions.md` replaced by an OS symlink to `../AGENTS.md`.
- Old hand-written `.github/copilot-instructions.md` content folded into `AGENTS.md` (drop the file inventory; AGENTS.md points to directories, not individual files).
- `CONTRIBUTING.md` reviewed for an "agent users" line linking to `AGENTS.md`.

**Verification**:

- Deterministic: `readlink .github/copilot-instructions.md` resolves to `AGENTS.md`. AGENTS.md word count ≤ 1500. All links resolve.
- Prompt: in each of the four supported tools (Copilot in VS Code, Copilot Cloud Agent, Copilot CLI, Claude Code), open a fresh session and ask *"What is this repo and how do I build it?"* — answer must reference the right doc and be correct.

---

### Phase 2 — Cloud Agent bootstrap

**Unlocks**: Assigning an issue to Copilot Cloud Agent results in a working environment without trial-and-error tool installation. Independent of Phase 1; can run in parallel.

**Deliverables (per repo)**:

- Pin tool versions in version files (`.node-version`, `.python-version`, `go.mod` already has Go). These become the single source of truth for both the dev container and the Cloud Agent.
- Make `.devcontainer/post-create.sh` (or equivalent) idempotent and safe to run on a GHA runner.
- Add `.github/copilot-setup-steps.yml` per repo. Each one uses the same `setup-go`/`setup-node`/`setup-python` actions that read the version files, then calls the shared post-create script.
- Cross-reference comments between `devcontainer.json` and `copilot-setup-steps.yml` so a change in one prompts a check of the other.

**Verification**:

- Deterministic: `copilot-setup-steps.yml` validates against `actionlint`. CI runs the post-create script in an Ubuntu container and succeeds.
- Prompt: assign a "build and run unit tests" test issue to Cloud Agent in each repo. Agent succeeds without manual intervention.

---

### Phase 3 — Contributing docs

**Unlocks**: Agents (and humans) get correct, current step-by-step answers to "how do I do X?" — which is what every other capability rests on. Phase 0's `radius-author-doc` and `radius-update-doc` skills do most of the writing; humans review.

**Deliverables (per repo)**:

- Audit every doc under `CONTRIBUTING.md` and `docs/contributing/`. Every doc follows one format: **Purpose → Prerequisites → Steps → Verification → Troubleshooting**.
- Verify each doc against current code: build commands, file paths, command flags.
- Fill the gaps surfaced by the [agent-ex-features.md capability list](agent-ex-features.md):
  - `radius/`: TypeSpec → Swagger → Go pipeline; full test matrix; local dev environment setup.
  - `dashboard/`: prerequisites, build, test, plugin development.
  - `docs/`: local contributor guide (currently external link only).
  - `resource-types-contrib/`: prerequisites, `CONTRIBUTING.md`.
  - `bicep-types-aws/`: prerequisites, test docs, type generation pipeline overview.
- Add a docs index under each repo's `docs/contributing/README.md` that `AGENTS.md` links to.
- Add a **capability index** to `docs/contributing/README.md` (in `radius/` first; satellites add only the rows for capabilities they own) that maps each leaf row in [agent-ex-features.md](agent-ex-features.md) to its single primary backing contributor doc. Every leaf capability has exactly one primary doc; a capability with no primary doc is a Phase 3 gap, not a row to omit. Parent capability rows (e.g., capability 1, "Build and test") are the union of their sub-capability rows. The same table seeds the per-repo code↔doc path map used by Phase 0's docs-drift code-review instructions and Phase 7's drift workflow.

**Gate**: after the audit, re-evaluate the skills, custom-agent, and instruction backlog in [Phase 5](#phase-5--coding-instructions-project-specific-only) and [Phase 6](#phase-6--per-workflow-copilot-conveniences). Many "missing skills" turn into "existing doc is enough."

**Verification**:

- Deterministic: every doc matches the format. Markdown link check passes. Code snippets parse.
- Prompt: in each repo, ask the agent five canonical questions ("how do I run the control plane locally?", "how do I add a resource type?", "how do I build the dashboard?"). All answers must cite the doc and be correct.

---

### Phase 4 — Architecture docs grounded in code

**Unlocks**: Agents and humans can answer "how does X work?" with specific file/function references and Mermaid diagrams instead of guessing. Drafted with Phase 0's `radius-author-doc` skill using the architecture-doc template.

**Deliverables**:

- **Subsystem audit (first step, gates the rest of the phase)**: inventory every major subsystem per repo (control plane, UCP, dynamic RP, deployment engine, CLI, dashboard plugins, AWS type pipeline, recipes, etc.) and compare against the existing `docs/architecture/` contents. The audit produces the concrete list of architecture docs to create, expand, or leave alone — without it, the rest of Phase 4 has no defined scope. Capture the inventory in the PR that opens Phase 4.
- Each subsystem identified by the audit gets a doc under `docs/architecture/<name>.md` with:
  - Entry points (file + symbol)
  - Key packages and their responsibilities
  - One representative end-to-end flow (sequence diagram in Mermaid)
  - Change-safety notes (what tests to run, what other components are affected)
- `docs/architecture/README.md` is a current index used by both humans and the `architecture-documenter` skill.
- Add a CI check that the index lists every `*.md` file in the directory.

**Verification**:

- Deterministic: every linked file path exists in the codebase. Mermaid blocks render. Link check passes.
- Prompt: for each repo, ask the agent five "how does X work?" questions. Answers cite real files and line ranges; diagrams reflect current structure.

---

### Phase 5 — Coding instructions (project-specific only)

**Unlocks**: Agent-authored code automatically follows Radius conventions in every Copilot agent surface (VS Code, Cloud Agent, CLI) and in GitHub.com Code Review. Claude Code reads the generated `.claude/rules/*.md` mirrors.

**Deliverables**:

- Audit existing `.github/instructions/*.instructions.md`. Cut anything that:
  - Duplicates what a linter (`gofmt`, `golangci-lint`, `actionlint`, `shellcheck`) already enforces.
  - Repeats general language knowledge the model already has.
- Each file ≤ 200 lines, project-specific only. `applyTo` patterns precise.
- Add the missing path-scoped instructions: TypeSpec for `radius/`, TypeScript/React conventions for `dashboard/`, resource-type YAML schema and recipe Terraform conventions for `resource-types-contrib/`, and Hugo Markdown for `docs/`. Skip anything that an existing linter or doc covers adequately.
- Replicate `code-review.instructions.md` to the four satellite repos, adapting examples per stack.

**Verification**:

- Deterministic: each instruction file ≤ 200 lines. `applyTo` globs validated against repo paths. No overlap with linter rules (spot check).
- Prompt: open a file matching each `applyTo` pattern, ask the agent to add a function/component. Output follows the documented convention.

---

### Phase 6 — Per-workflow Copilot conveniences

**Unlocks**: Copilot agent surfaces (VS Code, Cloud Agent, CLI) get one-click multi-step workflows for the remaining repo-specific tasks via skills and custom agents. VS Code additionally gets prompt-file shortcuts. Claude Code (and any tool that doesn't pick up these surfaces) loses nothing because the same knowledge already lives in the docs from Phases 3 and 4. (The cross-cutting authoring/extension conveniences from Phase 0 are already live.)

**Rules**:

- A new skill, prompt, or custom agent is only justified if it satisfies at least two of: project-specific, multi-step/non-obvious, frequently repeated, error-prone without guidance. Matches Design Principle 6 in [agent-ex-features.md](agent-ex-features.md#design-principles).
- Every skill/prompt/agent file links to its backing contributing or architecture doc, and contains no information that isn't also in that doc.

**Deliverables**:

- `radius/`:
  - Update `radius-build-cli`, `radius-build-images`, `radius-install-custom`, `architecture-documenter`, `contributing-docs-updater` for accuracy and doc links.
  - Add `radius-schema-changes` (TypeSpec → Swagger → Go).
  - Add `radius-run-controlplane` and `radius-debug-components` if the Phase 3 audit confirms the docs alone aren't sufficient.
- `resource-types-contrib/`:
  - `radius-contrib-add-resource-type` skill (YAML + recipe scaffold).
  - `radius.contrib.add-recipe` prompt.
  - `radius-resource-type-contributor` agent mode.
- `dashboard/`: `radius-dashboard-developer` agent mode. No skills until friction is observed.
- `docs/`: `radius-docs-contributor` agent mode. No skills until friction is observed.
- `bicep-types-aws/`: nothing in Phase 6 unless friction is observed.
- All repos: replicate the `radius.code-review.prompt.md` and `radius.create-pr.prompt.md` prompts; replicate or workspace-promote the `issue-investigator` agent.

**Verification**:

- Deterministic: every skill/prompt/agent is listed in the repo's `AGENTS.md` and contains a link to a backing doc. File size budgets respected (skill ≤ 500 lines, prompt ≤ 300 lines, agent ≤ 200 lines).
- Prompt: invoke each one and confirm it produces the same outcome as a non-VS-Code agent given only the backing doc.

---

### Phase 7 — Continuous improvement loop

**Unlocks**: Drift detection. Skills, instructions, and docs are pruned or extended based on real usage instead of guesswork. Code↔doc drift that slips past per-PR code review is caught here in batch.

**Deliverables (in `radius/` only; reviews logs from all repos)**:

- A scheduled GHA workflow (`skill-lifecycle-review.yml`, weekly) that:
  1. Collects **only sanitized, structured signals** (task type, invoked skill, failure category, retry count, success/failure outcome) from Copilot session logs and audit logs.
  2. Runs a redaction step that fails the workflow if any secret pattern survives.
  3. Invokes an LLM analysis step (GitHub Models or a Copilot prompt) using the sanitized signals.
  4. Files a GitHub Issue per repo with structured recommendations (skills to add/edit/remove, instruction gaps, context-budget impact). Never mutates files automatically.
- A second scheduled GHA workflow (`docs-drift-review.yml`, weekly) that:
  1. Walks the per-repo code↔doc path map from Phase 0.
  2. For each mapping, compares the most recent commit timestamps on the code glob and the doc; flags pairs where code has moved but the doc has not within a configurable window.
  3. Optionally invokes the `radius-update-doc` skill on each flagged pair to draft a suggested patch.
  4. Files a single GitHub Issue per repo summarizing the drift list and linking to drafts. Never mutates files automatically.
- A short retention policy (4-week rolling window) and a logs storage convention (`.copilot-tracking/logs/` per repo, maintainer-only access).
- A `radius.skill-lifecycle-review.prompt.md` and a `radius.docs-drift-review.prompt.md` to run the same analyses manually.

**Dependencies**: Phases 3–6 live for at least four weeks so there are real signals to analyze.

**Verification**:

- Deterministic: workflows pass `actionlint`, redaction step blocks a seeded fake secret, retention enforced, recommendation issues contain every required section. The docs-drift workflow run against a seeded path map produces an issue listing the expected drift entries.
- Prompt: seed `.copilot-tracking/logs/` with three synthetic logs (a repeated failure, an unused skill, a roundabout but successful workflow). Manual run produces correct add/edit/remove recommendations.

---

## 4. Multi-repo rollout

Each phase runs in all five repos in parallel. The radius/ repo is the "reference implementation": land each phase in `radius/` first (within the same week), then copy the pattern to satellites. Satellite repos get a slimmer version of every phase because they have less surface area.

| Phase | radius/ | dashboard/ | docs/ | resource-types-contrib/ | bicep-types-aws/ |
|---|:-:|:-:|:-:|:-:|:-:|
| 0 — Meta-tooling foundation | ✅ (hosts templates and skills) | adopts templates | adopts templates | adopts templates | adopts templates |
| 1 — AGENTS.md + symlink | ✅ | ✅ | ✅ | ✅ | ✅ |
| 2 — Cloud setup | ✅ | ✅ | ✅ | ✅ | ✅ |
| 3 — Contributing docs | ✅ (audit + fill gaps) | ✅ (expand) | ✅ (create) | ✅ (audit + create) | ✅ (audit + expand) |
| 4 — Architecture docs | ✅ (most) | ✅ (plugins) | ⚪ (skip) | ✅ (resource model) | ✅ (pipeline) |
| 5 — Instructions | ✅ (audit + add) | ✅ (TS) | ✅ (Hugo) | ✅ (YAML, TF) | ⚪ (minimal) |
| 6 — Per-workflow conveniences | ✅ | ✅ (agent mode only) | ✅ (agent mode only) | ✅ | ⚪ (skip until needed) |
| 7 — Lifecycle agent | ✅ (hosts workflow) | reviewed | reviewed | reviewed | reviewed |

---

## 5. Naming conventions

Skills and agents use a `radius-` prefix; prompts use a `radius.` prefix (matching the repo's existing prompts). The shared `radius` namespace keeps chat completions grouped cleanly and avoids collisions with extensions.

| Artifact | Pattern | Example | Appears in chat as |
|---|---|---|---|
| Skill | `radius-<verb>-<noun>/SKILL.md` (add `<repo>` segment when ambiguous across repos) | `radius-build-cli/`, `radius-contrib-add-resource-type/` | listed in skill picker |
| Instruction | `<technology>.instructions.md` | `typespec.instructions.md` | auto-applied |
| Agent | `radius-<name>.agent.md` | `radius-resource-type-contributor.agent.md` | `@radius-resource-type-contributor` |
| Prompt | `radius.<action>.prompt.md` (add `<repo>` segment when ambiguous across repos) | `radius.create-pr.prompt.md`, `radius.contrib.add-recipe.prompt.md` | `/radius.create-pr`, `/radius.contrib.add-recipe` |
| Lifecycle workflow | `skill-lifecycle-review.yml` | `.github/workflows/skill-lifecycle-review.yml` | scheduled |

The `<repo>` segment is optional. Add it only when a skill or prompt is repo-specific and would otherwise collide with a similarly named asset in another repo (e.g., `radius-contrib-add-resource-type` lives in `resource-types-contrib/` and disambiguates from any future `radius-add-resource-type` work in `radius/`). Existing skills (`radius-build-cli`, `radius-build-images`, `radius-install-custom`, `architecture-documenter`, `contributing-docs-updater`) and prompts (`radius.create-pr`, `radius.code-review`) keep their current names; no rename migration is planned.

Repo short names (used when the `<repo>` segment is needed): `core` (radius/), `dash` (dashboard/), `contrib` (resource-types-contrib/), `docs` (docs/), `bicep-aws` (bicep-types-aws/).

---

## 6. CI gates (deterministic, run on every PR)

These checks live in `radius/` first and replicate to satellites:

- `AGENTS.md` exists at repo root.
- `.github/copilot-instructions.md` is a symlink to `AGENTS.md`.
- File-size budgets respected (`AGENTS.md` ≤ 1500 words, instructions ≤ 200 lines, skills ≤ 500 lines, prompts ≤ 300 lines).
- Every skill/prompt/agent file contains at least one link to a doc under `docs/` or `CONTRIBUTING.md`.
- `actionlint` over `.github/workflows/`.
- Markdown link check over `AGENTS.md`, `CONTRIBUTING.md`, `docs/contributing/`, `docs/architecture/`.
- `docs/architecture/README.md` lists every `*.md` sibling.
- `docs/contributing/README.md` capability index: every row has exactly one primary backing doc link and every linked path resolves. In `radius/` only, every leaf capability from [agent-ex-features.md](agent-ex-features.md) appears as a row; satellites list only the leaf capabilities the repo owns.

Docs drift is **not** a blocking CI gate. It is handled in two places: (1) Phase 0's code-review instructions, which advise reviewers (Copilot Code Review and any agent surface) to suggest doc updates when a PR touches mapped code paths, and (2) Phase 7's scheduled `docs-drift-review.yml` workflow, which files a weekly issue for any drift that slipped through review.

---

## 7. Authoring and extension guides

These docs are the home of all meta-tooling. They are created in **Phase 0** so that every later phase — including Phase 1's `AGENTS.md` — uses them rather than reinventing conventions. They are what makes the agent-ex system extensible by any contributor and what gives concrete shape to the "the system maintains itself" principle in [agent-ex-features.md](agent-ex-features.md).

- **`docs/contributing/contributing-agent-assets.md`** — the conventions doc:
  - The file-strategy rule from [Section 2](#2-file-strategy).
  - File-size budgets and the CI gates from [Section 6](#6-ci-gates-deterministic-run-on-every-pr).
  - Naming conventions from [Section 5](#5-naming-conventions).
  - Templates for `AGENTS.md`, instructions, skills, prompts, and agents.
- **`docs/contributing/authoring-contributing-docs.md`** — how to write a new contributing or architecture doc, the standard formats, and how to invoke the `radius-author-doc` skill or follow the same steps manually.
- **`docs/contributing/extending-agent-ex.md`** — the "add a new capability" walkthrough. Lists the decision tree (doc only? instruction? skill? prompt? custom agent?), the live files to update (the primary contributing doc, the capability index in `docs/contributing/README.md`, `AGENTS.md` if a new top-level link is needed, and any optional wrappers), and the validation steps. Read by both humans and the `radius-add-capability` agent mode. The planning docs `agent-ex-features.md` and `agent-ex-plan.md` are not in this list — they describe the original buildout and are not edited by ongoing capability work.
- **Repo-onboarding checklist** appended to `extending-agent-ex.md` — the minimum set of artifacts a new repo needs (an `AGENTS.md` from the template, `copilot-setup-steps.yml`, dev-container post-create script, `docs/contributing/README.md` with a docs index and a capability index for the capabilities the repo owns, and the docs-drift addition to `code-review.instructions.md`) to be in scope of the agent-ex system.

These docs together are the single source of truth for extending the system. They replace tribal knowledge and they let any contributor (human or AI) add a capability without reverse-engineering existing files.

---

## 8. Execution order

Each phase is implemented through Spec Kit. Before starting a phase, use `@radius-spec-kit-prompt-agent` to generate a `/speckit.specify` prompt scoped to that phase's deliverables.

1. Phase **0** (meta-tooling foundation) lands first in `radius/`. It is short — three conventions docs, two skills, one custom agent, two VS Code prompts, and the docs-drift addition to `code-review.instructions.md` — and the rest of the plan depends on it.
2. Phase **1** (AGENTS.md) starts in all five repos as soon as Phase 0 publishes the `AGENTS.md` template. Phase **2** (Cloud Agent bootstrap) is independent and can start at any time.
3. Phases **3, 4, 5** can run concurrently in any repo once that repo has completed Phase 0; `radius/` goes first, satellites follow.
4. Phase **6** starts in any repo once that repo has completed Phase 3 (skills, prompts, and custom agents need backing docs).
5. Phase **7** starts after Phases 3–6 have been live for ~4 weeks.

---
