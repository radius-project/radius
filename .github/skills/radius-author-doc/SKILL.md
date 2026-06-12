---
name: radius-author-doc
description: 'Draft a new contributing or architecture doc from a topic and a starting code reference, using the repository standard format. Use when a contributor workflow or subsystem has no doc yet, or an existing doc must be expanded into the standard format.'
argument-hint: 'Topic and a starting code reference (file, package, command, or Make target)'
user-invocable: true
---

# Author a contributing or architecture doc

Draft a documentation page grounded in real code, in the format the repository prescribes for its type.

Backing doc: [authoring-contributing-docs.md](../../../docs/contributing/authoring-contributing-docs.md). This skill is a convenience wrapper — it adds no knowledge that is not already in that doc. The asset conventions, templates, and budgets it relies on live in [contributing-agent-assets.md](../../../docs/contributing/contributing-agent-assets.md).

## When to use

- A contributor workflow ("how do I do X?") has no doc, or its doc does not follow the standard format.
- A subsystem ("how does X work?") needs a code-grounded architecture doc.
- You are invoked by [/radius.author-doc](../../prompts/radius.author-doc.prompt.md) or by the `radius-add-AI-capability` agent to write a capability's primary doc.

Do not use this skill to invent content. Every path, command, and flag must be verified against the repository.

## Inputs

- A **topic** (the workflow or subsystem to document).
- A **starting code reference** — a file, package, command, or Make target the doc will describe.

## Steps

1. **Pick the format.** Documenting *how to perform a task* → a **contributing doc** (`docs/contributing/`, `CONTRIBUTING.md`). Explaining *how a subsystem works* → an **architecture doc** ([docs/architecture/](../../../docs/architecture/)). A capability's primary backing doc is always a contributing doc.
2. **Discover the current layout.** List `docs/contributing/` (or `docs/architecture/`) and read the nearest `README.md`/index before choosing a location. Place the doc in the narrowest section that fully covers the topic; only create a new page when none fits.
3. **Draft from the template.** Copy the matching template from [contributing-agent-assets.md](../../../docs/contributing/contributing-agent-assets.md#templates) and fill it in:
   - Contributing doc — **Purpose → Prerequisites → Steps → Verification → Troubleshooting**.
   - Architecture doc — **Entry points → Key packages → Flow → Change-safety**, with one Mermaid diagram.
4. **Ground every reference in code.** Link to real files and symbols; verify each command and flag by reading the source or running it. Never invent a path. Links in `docs/contributing/` are relative to the file (repo-root files `../../`, architecture docs `../architecture/`, sibling contributing docs `./`).
5. **Update navigation.** Link the new page from the nearest index — a section `README.md`, [CONTRIBUTING.md](../../../CONTRIBUTING.md), or [docs/architecture/README.md](../../../docs/architecture/README.md). When the doc backs a capability, follow [extending-agent-ex.md](../../../docs/contributing/extending-agent-ex.md) to add a capability-index row.
6. **Hand off for review.** A human reviews the draft before merge.

## Verification

- The doc uses the correct section set for its type (all five contributing sections, or all four architecture sections).
- Every command, path, flag, and link resolves to something real — no hallucinated paths.
- The doc is within review reach (one round of edits, not a rewrite).
- `cspell` passes:

  ```sh
  make spellcheck
  ```
