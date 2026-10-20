---
name: radius-update-doc
description: 'Patch an EXISTING contributor doc that has drifted from changed code — the smallest targeted edit, no rewrite. Use when a PR changes a command, flag, path, or workflow an existing doc describes, or when the code-review docs-impact step flags drift. Not for creating a new doc (use radius-author-doc) or finding which docs are missing/stale (use radius-contributing-docs-updater).'
argument-hint: 'A PR diff (or list of changed paths) and the affected doc path'
user-invocable: true
---

# Update a doc to match changed code

Propose a focused patch to an existing doc so its prose matches the current code. Do not rewrite the doc.

## Which doc skill?

| You want to…                                                         | Use                                                                              |
|----------------------------------------------------------------------|----------------------------------------------------------------------------------|
| **Create** a new contributing doc                                    | [radius-author-doc](../radius-author-doc/SKILL.md)                               |
| **Fix** an existing doc that drifted from code                       | **this skill**                                                                   |
| **Find** missing or stale docs, or assess a code change's doc impact | [radius-contributing-docs-updater](../radius-contributing-docs-updater/SKILL.md) |
| **Diagram** a subsystem / write an architecture doc                  | [radius-architecture-documenter](../radius-architecture-documenter/SKILL.md)     |

Backing docs: [authoring-contributing-docs.md](../../../docs/contributing/authoring-contributing-docs.md) (the format the patched doc must keep) and the [code ↔ doc path map](../../../docs/contributing/contributing-agent-assets.md#code--doc-path-map) (which maps changed code globs to the doc that documents them). This skill is a convenience wrapper and adds no knowledge beyond those docs.

## When to use

- A PR changes a command, flag, path, build step, or architecture that an existing doc describes.
- The [code-review docs-impact step](../../instructions/code-review.instructions.md) or the scheduled drift workflow flags a doc that lags its code.
- Invoked manually by a contributor with a diff and a stale doc.

Do not use this skill to author a brand-new doc — use [radius-author-doc](../radius-author-doc/SKILL.md) for that.

## Inputs

- A **PR diff** or the list of changed paths.
- The **affected doc** path (or let the path map identify it).

## Steps

1. **Read the diff.** Identify which commands, flags, file paths, Make targets, or workflows changed.
2. **Find the documented behavior.** Consult the [code ↔ doc path map](../../../docs/contributing/contributing-agent-assets.md#code--doc-path-map) to map the changed code glob to its backing doc. If the map has no row yet (it grows in Phase 3), search `docs/contributing/` and `docs/architecture/` for prose that references the changed command, flag, or path.
3. **Locate the stale section.** Pinpoint the specific lines whose prose no longer matches the code.
4. **Propose the smallest patch.** Edit only what drifted. Re-align the prose with the new code; keep the doc's prescribed format (contributing or architecture). Do not reflow unrelated paragraphs.
5. **Verify against the code.** Confirm every updated command, flag, and path resolves to the post-change source.

## Verification

- The edit is minimal and scoped to the drift — no rewrite, no unrelated reflow.
- The doc still follows its format (five contributing sections, or four architecture sections).
- Every updated path, command, and link resolves.
- `cspell` passes:

  ```sh
  make spellcheck
  ```
