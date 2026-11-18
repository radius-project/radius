---
name: radius-author-design-note
description: 'Draft or revise a Radius design note (technical design doc) under eng/design-notes/ using the design template, written so a human reviewer can quickly find the decisions, trade-offs, and data flow. Use when asked to write, draft, restructure, shorten, or make reviewable a design note or design doc. Not for feature specs, contributing docs (use radius-author-doc), or architecture docs (use radius-architecture-documenter).'
argument-hint: 'Feature or problem, plus a feature spec, issue, or starting code reference'
user-invocable: true
---

# Author a design note

Write a design note that a reviewer can approve from its first page. Build it from decisions down to implementation detail. Be complete in content and concise in wording: each tier adds detail, and nothing is lost.

Backing doc: [Working with design documents](../../../docs/contributing/contributing-code/contributing-code-design/README.md#writing-a-design-note). Template: [design-template.md](../../../eng/design-notes/templates/design-template.md).

## Which doc skill?

| You want to…                                      | Use                                                                          |
|---------------------------------------------------|------------------------------------------------------------------------------|
| **Propose** a technical design for review         | **this skill**                                                               |
| **Explain** how an existing subsystem works       | [radius-architecture-documenter](../radius-architecture-documenter/SKILL.md) |
| **Create** a contributor how-to doc               | [radius-author-doc](../radius-author-doc/SKILL.md)                           |
| **Fix** a contributing doc that drifted from code | [radius-update-doc](../radius-update-doc/SKILL.md)                           |

## Inputs

- The problem or feature, and the feature spec or issue if one exists.
- Starting code references: packages, types, commands, or API paths the design touches.
- For a revision: the existing design note.

If the problem or the intended scope is unclear, ask the user before drafting.

## Workflow

The backing doc is the source of truth. Follow it in order:

1. [Structure](../../../docs/contributing/contributing-code/contributing-code-design/README.md#structure): the three tiers, and keep every template heading.
2. [Steps](../../../docs/contributing/contributing-code/contributing-code-design/README.md#writing-steps): read first, list decisions before prose, then write from Tier 1 down to the Appendix.
3. [Writing guidelines](../../../docs/contributing/contributing-code/contributing-code-design/README.md#writing-guidelines): layered, not lossy; concise wording, complete facts.
4. [Diagrams](../../../docs/contributing/contributing-code/contributing-code-design/README.md#diagrams): Mermaid architecture and sequence diagrams.
5. [Verification checklist](../../../docs/contributing/contributing-code/contributing-code-design/README.md#verification-checklist): self-review before handing the draft to the human author.

## Agent-specific notes

- Do not mark the note Approved, fill in a decision's **Outcome**, or fill in **Design Review Notes**; only the human author does that after review.
- Report which sections you drafted and which claims you could not verify, so the author can complete **Authoring notes**.
- When revising an existing note, report any fact you moved to the Appendix so the author can confirm nothing was lost.
