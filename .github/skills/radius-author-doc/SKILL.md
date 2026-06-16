---
name: radius-author-doc
description: 'Create a NEW contributing doc (or expand a stub into the standard Purpose → Prerequisites → Steps → Verification → Troubleshooting format) for a contributor workflow that has none. Not for fixing an existing doc (use radius-update-doc) or for architecture docs (use radius-architecture-documenter).'
argument-hint: 'Topic and a starting code reference (file, package, command, or Make target)'
user-invocable: true
---

# Author a contributing doc

Draft a contributing doc grounded in real code, in the format the repository prescribes.

## Which doc skill?

| You want to…                                                         | Use                                                                              |
|----------------------------------------------------------------------|----------------------------------------------------------------------------------|
| **Create** a new contributing doc                                    | **this skill**                                                                   |
| **Fix** an existing doc that drifted from code                       | [radius-update-doc](../radius-update-doc/SKILL.md)                               |
| **Find** missing or stale docs, or assess a code change's doc impact | [radius-contributing-docs-updater](../radius-contributing-docs-updater/SKILL.md) |
| **Diagram** a subsystem / write an architecture doc                  | [radius-architecture-documenter](../radius-architecture-documenter/SKILL.md)     |

Backing doc: [authoring-contributing-docs.md](../../../docs/contributing/authoring-contributing-docs.md). This skill is a convenience wrapper — it adds no knowledge that is not already in that doc. The asset conventions, templates, and budgets it relies on live in [contributing-agent-assets.md](../../../docs/contributing/contributing-agent-assets.md).

## When to use

- A contributor workflow ("how do I do X?") has no doc, or its doc does not follow the standard format.
- You are invoked by [/radius.author-doc](../../prompts/radius.author-doc.prompt.md) or by the `radius-add-AI-capability` agent to write a capability's primary doc.

For an **architecture** doc ("how does a subsystem work?"), use the [radius-architecture-documenter](../radius-architecture-documenter/SKILL.md) skill instead — it owns `docs/architecture/`.

Do not use this skill to invent content. Every path, command, and flag must be verified against the repository.

## Inputs

- A **topic** (the contributor workflow to document).
- A **starting code reference** — a file, package, command, or Make target the doc will describe.

## Steps

1. **Confirm it's a contributing doc.** This skill authors **contributing docs** (`docs/contributing/`, `CONTRIBUTING.md`) — guides for *how to perform a task*. A capability's primary backing doc is always a contributing doc. For *how a subsystem works*, stop and use the [radius-architecture-documenter](../radius-architecture-documenter/SKILL.md) skill instead.
2. **Discover the current layout.** List `docs/contributing/` and read the nearest `README.md`/index before choosing a location. Place the doc in the narrowest section that fully covers the topic; only create a new page when none fits.
3. **Draft from the template.** Copy the contributing-doc template from [contributing-agent-assets.md](../../../docs/contributing/contributing-agent-assets.md#templates) and fill it in: **Purpose → Prerequisites → Steps → Verification → Troubleshooting**.
4. **Ground every reference in code.** Link to real files and symbols; verify each command and flag by reading the source or running it. Never invent a path. Links in `docs/contributing/` are relative to the file (repo-root files `../../`, architecture docs `../architecture/`, sibling contributing docs `./`).
5. **Update navigation.** Link the new page from the nearest index — a section `README.md` or [CONTRIBUTING.md](../../../CONTRIBUTING.md). When the doc backs a capability, follow [extending-agent-ex.md](../../../docs/contributing/extending-agent-ex.md) to add a capability-index row.
6. **Hand off for review.** A human reviews the draft before merge.

## Verification

- The doc uses all five contributing sections (Purpose, Prerequisites, Steps, Verification, Troubleshooting).
- Every command, path, flag, and link resolves to something real — no hallucinated paths.
- The doc is within review reach (one round of edits, not a rewrite).
- `cspell` passes:

  ```sh
  make spellcheck
  ```
