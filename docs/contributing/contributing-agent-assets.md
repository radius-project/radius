<!-- markdownlint-disable MD060 -->
# Contributing agent assets

## Purpose

This document is the conventions reference for every **agent asset** in the Radius repository — the files that orient and guide AI agents (GitHub Copilot in VS Code, Copilot CLI, Copilot Cloud Agent, and Claude Code) and the humans who work alongside them. It defines the file-strategy rule that governs what goes where, the file-size budgets, the deterministic CI gates, the naming conventions, and a copy-paste template for each asset type.

Use this document when you add or change any of these files:

- `AGENTS.md` (repo-root orientation file)
- `.github/instructions/*.instructions.md` (path-scoped coding conventions)
- `.github/skills/*/SKILL.md` (multi-step workflow wrappers)
- `.github/prompts/*.prompt.md` (VS Code slash-command shortcuts)
- `.github/agents/*.agent.md` (custom agents)

This document encodes [Sections 2, 5, and 6 of the Agent Ex plan](../../specs/002-agent-ex/agent-ex-plan.md). For *how to write* a contributing or architecture doc, see [authoring-contributing-docs.md](./authoring-contributing-docs.md). For the end-to-end *"add a capability"* walkthrough, see [extending-agent-ex.md](./extending-agent-ex.md).

## The file-strategy rule

One rule governs every agent asset:

> **Capability lives in docs. Tool-specific UX is just a wrapper. Wrappers link to docs.**

And its corollary:

> **Every leaf capability is backed by one primary contributor doc** under [CONTRIBUTING.md](../../CONTRIBUTING.md) or `docs/contributing/`. A skill, prompt, or custom agent is optional; the primary doc is not.

Concretely:

- Anything an agent must *know* to do its job belongs in a doc — [CONTRIBUTING.md](../../CONTRIBUTING.md), `docs/contributing/`, or [docs/architecture/](../architecture/).
- Skills, prompts, and custom agents are **conveniences** over that knowledge. They must not be the only place a capability lives.
- Every skill, prompt, and custom agent **must link to a backing doc** that contains the same steps in prose, so any tool (or human) can follow the workflow without the wrapper.

This rule keeps a single, durable knowledge base that humans read directly and every supported agent can also read, and it prevents capabilities from being trapped behind one tool.

## Where each asset lives

| Asset | Location | Read by | Holds |
|---|---|---|---|
| Orientation | `AGENTS.md` (repo root) | every agent; Copilot family via symlink | ≤ 2-page map to everything else |
| Copilot entry point | `.github/copilot-instructions.md` → `AGENTS.md` | Copilot family (incl. GitHub.com) | symlink to `AGENTS.md` |
| How-to knowledge | [CONTRIBUTING.md](../../CONTRIBUTING.md), `docs/contributing/` | everyone | setup, build, test, debug, schema, releases |
| Architecture | [docs/architecture/](../architecture/) | everyone | code-grounded subsystem explanations |
| Coding conventions | `.github/instructions/*.instructions.md` | Copilot surfaces; Claude via `.claude/rules/` | path-scoped rules linters can't enforce |
| Workflow wrappers | `.github/skills/*/SKILL.md` | Copilot surfaces; Claude via `.claude/skills/` | multi-step Radius-specific workflows |
| Slash commands | `.github/prompts/*.prompt.md` | VS Code only | shortcuts to a backing doc |
| Custom agents | `.github/agents/*.agent.md` | Copilot surfaces; Claude via `.claude/agents/` | scoped agent personas |

See the [tool landscape table in the plan](../../specs/002-agent-ex/agent-ex-plan.md#1-tool-landscape) for the full file-to-tool matrix.

## File-size budgets

Always-on context costs tokens before a task starts, so each asset has a budget. Keep knowledge that an agent reads on demand in a doc instead of in always-on context.

| Asset | Budget |
|---|---|
| `AGENTS.md` | ≤ 1500 words (≈ 2 pages) |
| `*.instructions.md` | ≤ 200 lines |
| `SKILL.md` | ≤ 500 lines |
| `*.prompt.md` | ≤ 300 lines |
| `*.agent.md` | ≤ 200 lines |

Contributing and architecture docs have no fixed line budget — they are read on demand — but should stay focused on a single workflow or subsystem.

## Naming conventions

Skills and agents use a `radius-` prefix; prompts use a `radius.` prefix (matching the repository's existing prompts). The shared `radius` namespace keeps chat completions grouped and avoids collisions with extensions.

| Asset | Pattern | Example | Appears in chat as |
|---|---|---|---|
| Skill | `radius-<verb>-<noun>/SKILL.md` | `radius-build-cli/` | listed in skill picker |
| Instruction | `<technology>.instructions.md` | `golang.instructions.md` | auto-applied |
| Agent | `radius-<name>.agent.md` | `radius-resource-type-contributor.agent.md` | `@radius-resource-type-contributor` |
| Prompt | `radius.<action>.prompt.md` | `radius.create-pr.prompt.md` | `/radius.create-pr` |

**The `<repo>` segment is optional.** Add it only when a skill or prompt is repo-specific and would otherwise collide with a similarly named asset in another repo (for example `radius-contrib-add-resource-type` in `resource-types-contrib/`). Repo short names: `core` (radius/), `dash` (dashboard/), `contrib` (resource-types-contrib/), `docs` (docs/), `bicep-aws` (bicep-types-aws/).

Existing assets keep their current names; no rename migration is planned. The skills currently in the repo are `architecture-documenter`, `contributing-docs-updater`, `radius-build-cli`, `radius-build-images`, `radius-code-review`, and `radius-install-custom`; the existing custom agent is `issue-investigator`. New skills and agents follow the `radius-` prefix going forward.

## CI gates

These deterministic checks run on every PR. Some are enforced today; the remainder are planned as the Agent Ex system lands (see [plan Section 6](../../specs/002-agent-ex/agent-ex-plan.md#6-ci-gates-deterministic-run-on-every-pr)). Author assets so they would pass all of them:

- `AGENTS.md` exists at the repo root.
- `.github/copilot-instructions.md` is a symlink to `AGENTS.md`.
- File-size budgets respected (see table above).
- Every skill, prompt, and agent file contains at least one link to a doc under `docs/` or [CONTRIBUTING.md](../../CONTRIBUTING.md).
- `actionlint` passes over `.github/workflows/`.
- Markdown link check passes over `AGENTS.md`, [CONTRIBUTING.md](../../CONTRIBUTING.md), `docs/contributing/`, and [docs/architecture/](../architecture/).
- [docs/architecture/README.md](../architecture/README.md) lists every `*.md` sibling.
- `docs/contributing/README.md` capability index: every row links to exactly one primary backing doc and every linked path resolves (the index itself is added in Phase 3 of the plan).
- Spellcheck (`cspell`) passes (see [.github/configs/.cspell.yml](../../.github/configs/.cspell.yml)).

Docs drift is **not** a blocking gate. It is handled as advisory guidance by the docs-drift code-review instructions and by a scheduled weekly drift workflow.

## Templates

Each template below is a starting point. Copy it, fill in the placeholders, and trim to the file-size budget. Every YAML frontmatter block is valid YAML; every embedded shell block is safe Bash.

### `AGENTS.md` template

```markdown
# <Repo name>

<One sentence: what this repo is and what it ships.>

## Tech stack and layout

<One paragraph on the stack. One paragraph on the top-level directory layout.>

## Build and test

See [CONTRIBUTING.md](./CONTRIBUTING.md).

## How the system works

See [docs/architecture/README.md](./docs/architecture/README.md).

## Conventions

Path-scoped coding conventions live in [.github/instructions/](./.github/instructions/).

## Agent conveniences

- Skills: [.github/skills/](./.github/skills/)
- Custom agents: [.github/agents/](./.github/agents/)
- Prompts (VS Code): [.github/prompts/](./.github/prompts/)

## How to contribute

See [CONTRIBUTING.md](./CONTRIBUTING.md) and
[docs/contributing/extending-agent-ex.md](./docs/contributing/extending-agent-ex.md).
```

### Instruction template

```markdown
---
applyTo: "<glob>,<glob>"
description: <One line describing when these rules apply.>
---

# <Technology> instructions

<Only project-specific rules that a linter cannot enforce. Keep to ~200 lines.>

- <Rule.>
- <Rule.>
```

### Skill template

```markdown
---
name: radius-<verb>-<noun>
description: '<What it does and when to use it. One or two sentences.>'
argument-hint: '<Optional inputs, or leave blank for default behavior>'
---

# <Skill title>

<One sentence describing the workflow.>

Backing doc: [<doc title>](../../docs/contributing/<path>.md)

## Steps

1. <Step.>
2. <Step.>

## Verification

<How to confirm success.>
```

### Prompt template

```markdown
---
agent: agent
name: radius.<action>
description: <One line describing the shortcut.>
tools:
  - edit
  - search
  - runCommands
---

# <Prompt title>

Backing doc: [<doc title>](../../docs/contributing/<path>.md)

<Instructions the prompt runs. Must be reproducible by any agent reading the
backing doc.>
```

### Custom agent template

```markdown
---
name: radius-<name>
description: <One line describing the agent persona and its scope.>
tools: ["read", "search", "edit", "web", "shell"]
---

# <Agent title>

Backing doc: [<doc title>](../../docs/contributing/<path>.md)

<The agent's role, scope, and step-by-step behavior.>
```

## Code ↔ doc path map

The docs-drift code-review instructions and the scheduled drift workflow consult a per-repo map from a code glob to the contributor doc that documents its behavior. The map lands **empty** here and grows as Phase 3 fills out the contributing docs.

| Code glob | Backing doc |
|---|---|
| _(none yet — populated in Phase 3)_ | |

## Verification

After adding or changing an agent asset, confirm:

- The asset is under its file-size budget.
- A skill, prompt, or agent links to a backing doc under `docs/` or [CONTRIBUTING.md](../../CONTRIBUTING.md).
- The name matches the [naming conventions](#naming-conventions).
- Every linked path resolves (run a link check, or verify paths manually until the CI gate is live).
- `cspell` passes:

  ```sh
  make spellcheck
  ```

## Troubleshooting

- **Spellcheck fails on a technical term.** Add the term to [.cspellignore](../../.cspellignore) (one word per line) rather than rewording.
- **A link check flags a path.** Links in `docs/contributing/` are relative to the file. From this directory, repo-root files are `../../`, the architecture docs are `../architecture/`, and sibling contributing docs are `./`.
- **Unsure which asset type to create.** Use the decision tree in [extending-agent-ex.md](./extending-agent-ex.md).
