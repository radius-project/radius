---
name: radius-add-ai-capability
description: Walks a contributor through adding a new AI capability to the Agent Ex system (such as a custom agent, prompt, skill, or instruction) — choosing the right asset type, authoring or extending the primary contributing doc, scaffolding any optional wrappers, and updating the live files. Scoped to documentation and agent-asset files only.
---

# Add an AI capability

You guide a contributor through adding a new AI capability to the Radius Agent Ex system, following the repository's own walkthrough.

Backing doc: [extending-agent-ex.md](../../docs/contributing/extending-agent-ex.md). This is the source of truth — follow it exactly; do not invent steps. Supporting references: the conventions, budgets, and templates in [contributing-agent-assets.md](../../docs/contributing/contributing-agent-assets.md) and the doc formats in [authoring-contributing-docs.md](../../docs/contributing/authoring-contributing-docs.md).

## Scope

- You edit only documentation and agent-asset files: `CONTRIBUTING.md`, `docs/contributing/`, `docs/architecture/`, `.github/instructions/`, `.github/skills/`, `.github/prompts/`, `.github/agents/`, and `AGENTS.md`.
- You **never** edit the planning docs [agent-ex-features.md](../../specs/002-agent-ex/agent-ex-features.md) or [agent-ex-plan.md](../../specs/002-agent-ex/agent-ex-plan.md). They record the original buildout; ongoing capability work updates the live docs only.
- You do not touch product or feature source code.

## Workflow

1. **Confirm the AI capability is real and recurring** — not a one-off task. If it is a one-off, say so and stop.
2. **Decide where the capability lives** using the decision tree in [extending-agent-ex.md](../../docs/contributing/extending-agent-ex.md#1-decide-where-the-capability-lives):
   - Every capability gets a **primary contributing doc** (mandatory).
   - Add an **instruction** only for a coding convention a linter cannot enforce.
   - Add a **skill**, **prompt**, or **custom agent** only when it passes the **two-of-four rule** (project-specific, multi-step/non-obvious, frequently repeated, error-prone without guidance). Fewer than two → the doc alone is enough.
3. **Author or extend the primary doc** in the correct format (Purpose → Prerequisites → Steps → Verification → Troubleshooting for contributing docs). Use the [radius-author-doc](../skills/radius-author-doc/SKILL.md) skill to draft it. Ground every reference in real code.
4. **Scaffold any optional wrappers** by copying the matching template from [contributing-agent-assets.md](../../docs/contributing/contributing-agent-assets.md#templates). Every wrapper must link to its backing doc, carry no information absent from that doc, respect the [file-size budgets](../../docs/contributing/contributing-agent-assets.md#file-size-budgets), and match the [naming conventions](../../docs/contributing/contributing-agent-assets.md#naming-conventions).
5. **Update the live files** listed in [extending-agent-ex.md](../../docs/contributing/extending-agent-ex.md#4-update-the-live-files): the primary doc (always), the capability index in [docs/contributing/README.md](../../docs/contributing/README.md) once it exists (Phase 3), `AGENTS.md` only when a new top-level link is needed, and any wrappers you scaffolded.
6. **Validate** against the [Verification](../../docs/contributing/extending-agent-ex.md#verification) section: one primary doc, every wrapper links to it and stays within budget, names match conventions, links resolve, `cspell` passes (`make spellcheck`), and no planning docs were edited.

## Output

Produce a coherent set of edits that pass the [CI gates](../../docs/contributing/contributing-agent-assets.md#ci-gates) without manual cleanup, and summarize which files you created or changed and why.
