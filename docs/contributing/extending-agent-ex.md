<!-- markdownlint-disable MD060 -->
# Extending Agent Ex

## Purpose

This document is the walkthrough for **adding a new capability** to the Agent Ex system and for **onboarding a new repository** into its scope. It tells you which asset type to create (doc only, instruction, skill, prompt, or custom agent), which live files to update, and how to validate the result.

Use it when a new contributor workflow emerges — a new tool in an existing workflow, a brand-new top-level capability, a new agent surface, or a new repo joining the initiative.

The planning docs [agent-ex-features.md](../../specs/002-agent-ex/agent-ex-features.md) and [agent-ex-plan.md](../../specs/002-agent-ex/agent-ex-plan.md) describe the original buildout. They are **not** edited by ongoing capability work — do not add your capability to them.

For conventions, budgets, and templates, see [contributing-agent-assets.md](./contributing-agent-assets.md). For how to write the backing doc itself, see [authoring-contributing-docs.md](./authoring-contributing-docs.md).

## Prerequisites

- The capability is real and recurring — not a one-off task.
- You know the workflow well enough to write or extend its primary contributing doc.
- You have read the [file-strategy rule](./contributing-agent-assets.md#the-file-strategy-rule): capability lives in docs; wrappers link back to docs.

## Steps

### 1. Decide where the capability lives

Every capability **must** have a primary contributing doc. On top of that doc, decide which optional wrappers (if any) to add. Work down this decision tree:

1. **Always: a primary contributing doc.** Author a new doc, or extend an existing one, under [CONTRIBUTING.md](../../CONTRIBUTING.md) or `docs/contributing/`. This is mandatory. Stop here if the workflow is fully covered by prose.
2. **Add a path-scoped instruction** when the capability is a *coding convention* tied to a file type that a linter cannot enforce (`.github/instructions/<technology>.instructions.md`).
3. **Add a skill** when the workflow is multi-step and benefits from a repeatable, agent-invocable wrapper (`.github/skills/radius-<verb>-<noun>/SKILL.md`).
4. **Add a prompt** when VS Code users want a slash-command shortcut to that workflow (`.github/prompts/radius.<action>.prompt.md`). VS Code only.
5. **Add a custom agent** when the capability needs a scoped persona with its own tool set (`.github/agents/radius-<name>.agent.md`).

**The two-of-four rule** (from [Design Principle 6 in agent-ex-features.md](../../specs/002-agent-ex/agent-ex-features.md#design-principles)): only add a skill, prompt, or custom agent if it satisfies **at least two** of:

- project-specific,
- multi-step / non-obvious,
- frequently repeated,
- error-prone without guidance.

Standard workflows (`go test`, `npm install`) belong in docs, not wrappers.

### 2. Author or extend the primary contributing doc

Follow [authoring-contributing-docs.md](./authoring-contributing-docs.md): contributing docs use **Purpose → Prerequisites → Steps → Verification → Troubleshooting**; architecture docs use **Entry points → Key packages → Flow → Change-safety**. Ground every reference in real code.

### 3. Scaffold any optional wrappers

Copy the matching template from [contributing-agent-assets.md](./contributing-agent-assets.md#templates) for each wrapper you decided to add. Every wrapper **must link to its backing doc** and contain no information that isn't also in that doc. Respect the [file-size budgets](./contributing-agent-assets.md#file-size-budgets) and [naming conventions](./contributing-agent-assets.md#naming-conventions).

### 4. Update the live files

After authoring, update exactly these live files (and no planning docs):

| File | When to update |
|---|---|
| The primary contributing doc | Always — this is step 2. |
| The capability index in [docs/contributing/README.md](./README.md) | Once the capability index exists (Phase 3), add or update the row mapping the capability to its primary doc. |
| `AGENTS.md` (repo root) | Only when the capability needs a new top-level link. |
| Optional wrappers | Whichever you scaffolded in step 3. |

Do **not** edit [agent-ex-features.md](../../specs/002-agent-ex/agent-ex-features.md) or [agent-ex-plan.md](../../specs/002-agent-ex/agent-ex-plan.md).

### 5. Validate

Run the checks in the [Verification](#verification) section below.

## Onboarding a new repository

When a new repo joins the Agent Ex scope, give it the minimum set of artifacts so it is in scope of the system. Use this checklist:

- [ ] `AGENTS.md` at the repo root, created from the [AGENTS.md template](./contributing-agent-assets.md#agentsmd-template).
- [ ] `.github/copilot-instructions.md` as an OS symlink to `AGENTS.md`.
- [ ] `copilot-setup-steps.yml` for Cloud Agent bootstrap (mirrors the dev-container post-create script).
- [ ] A dev-container post-create script (for example [.devcontainer/post-create.sh](../../.devcontainer/post-create.sh)) that is idempotent and safe to run on a CI runner.
- [ ] `docs/contributing/README.md` with a docs index and a capability index for the capabilities the repo owns.
- [ ] The docs-drift assessment step in [.github/instructions/code-review.instructions.md](../../.github/instructions/code-review.instructions.md).

Satellite repos adopt a slimmer version of each artifact because they have less surface area than `radius/`.

## Verification

The change is complete when:

- The capability has exactly one primary backing contributing doc.
- Every wrapper links to that doc and stays within its file-size budget.
- Wrapper and doc names match the [naming conventions](./contributing-agent-assets.md#naming-conventions).
- Once the capability index exists (Phase 3), a row for the capability exists in the capability index in [docs/contributing/README.md](./README.md).
- The change passes the [CI gates](./contributing-agent-assets.md#ci-gates) without manual cleanup — notably the link check and `cspell` (`make spellcheck`).
- No edits were made to the planning docs.

**Behavioral check**: an agent given only this document can scaffold a correctly-named, correctly-placed new skill (with a backing-doc link) and can execute the onboarding checklist for a new repo, with no extra guidance.

## Troubleshooting

- **Not sure whether to add a wrapper.** Apply the two-of-four rule. If it satisfies fewer than two, the doc alone is enough.
- **The capability touches several file types.** It may be several capabilities. Give each its own primary doc and index row.
- **A wrapper would duplicate doc content.** That's expected to be avoided — move the knowledge into the doc and have the wrapper link to it.
- **Tempted to update the planning docs.** Don't. They record the original buildout; ongoing work updates the live docs and the capability index only.
