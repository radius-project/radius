---
name: radius-author-design-note
description: 'Draft or revise a Radius design note (technical design doc) under eng/design-notes/ using the design template, written so a human reviewer can quickly find the decisions, trade-offs, and data flow. Use when asked to write, draft, restructure, shorten, or make reviewable a design note or design doc. Not for feature specs, contributing docs (use radius-author-doc), or architecture docs (use radius-architecture-documenter).'
argument-hint: 'Feature or problem, plus a feature spec, issue, or starting code reference'
user-invocable: true
---

# Author a design note

Write a design note that a reviewer can approve from its first page. Build it from decisions down to implementation detail. Be concise.

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

## Steps

1. **Read before writing.** Read the feature spec, issue, and the code paths the design touches. Read related notes under `eng/design-notes/` and `docs/architecture/`. Record what you verified; it goes in **Authoring notes**.
2. **List the decisions first.** Before any prose, write the "Decisions for review" table. A decision is a choice a reviewer could reasonably disagree with: a new API shape, a storage choice, a dependency, a security boundary, a breaking change. Aim for 2–6. Do not list settled facts as decisions.
3. **Copy the template.** Create `eng/design-notes/<area>/YYYY-MM-<short-name>.md` from the template. Keep every heading.
4. **Write Tier 1** (Review guide through User Experience). It must stand alone: a reviewer who reads only this part can approve or reject the design.
5. **Draw the flow.** Add one Mermaid architecture diagram and one sequence diagram for the main path. Prefer a diagram plus a short numbered list over paragraphs.
6. **Write Decision details.** One subsection per decision ID: context, options table (advantages and disadvantages), proposed option, and how we'll know it's right.
7. **Write the remaining Tier 2 sections**, going from contracts (API, CLI) to implementation details, error handling, tests, security, compatibility, monitoring, and development plan.
8. **Move bulk to the Appendix.** Code blocks longer than about 20 lines, full schemas, scripts, and exhaustive tables go in the Appendix, with a link from the section that needs them.
9. **Prune.** Apply the writing rules below. Cut every sentence that does not help a reviewer decide or an implementer build.
10. **Self-review** against the Verification checklist, then hand off. A human author owns the note and reviews the draft before it is shared.

## Writing rules

- **Lead with the point.** First sentence of each section states the conclusion; detail follows.
- **Short.** Sentences under about 25 words. Paragraphs of three sentences or fewer. Prefer bullets and tables.
- **Concrete.** Name the package, type, field, flag, or API path. Link to code instead of describing it.
- **No filler.** No restating headings, no "This section describes…", no marketing words (seamless, robust, powerful, leverage), no summaries of what was just said.
- **One home per fact.** Say it once and link to it. Do not repeat the feature spec; summarize and link.
- **Every heading stays.** If a section does not apply, write one sentence saying why (for example, "No CLI changes."). Never pad a section to look complete.
- **Mark uncertainty.** Unverified claims go in Authoring notes or Open Questions, not stated as fact.
- **No invented paths.** Every path, symbol, command, and flag must exist, or be clearly marked as new in this design.
- **Do not hard-wrap prose** (see [markdown.instructions.md](../../instructions/markdown.instructions.md)).

## Diagrams

- Use Mermaid so diagrams render on GitHub and diff as text.
- Architecture: `flowchart`, with new components visually distinct (for example, a `classDef` for new nodes).
- Data flow: `sequenceDiagram` for the main path; add a second only if the failure or async path differs materially.
- State: `stateDiagram-v2` when the design adds a lifecycle or status field.
- Keep each diagram under about 12 nodes. Split rather than crowd.

## Verification

- [ ] All template headings are present; non-applicable sections say why in one sentence.
- [ ] The Decisions table has an ID, options, recommendation, risk, and status for every row, and each row links to a matching Decision details subsection.
- [ ] Overview, Decisions, Goals, and User Experience are readable without the rest of the document.
- [ ] There is at least one architecture diagram and one sequence diagram.
- [ ] Security and Compatibility are filled in, even if only to state there is no change.
- [ ] Authoring notes say what was AI-drafted and what was verified by hand.
- [ ] Every path, symbol, and link resolves, or is marked as new.
- [ ] No code block over about 20 lines outside the Appendix.
- [ ] Markdown lint passes (see the [radius-markdown-lint](../radius-markdown-lint/SKILL.md) skill) and `make spellcheck` passes.
