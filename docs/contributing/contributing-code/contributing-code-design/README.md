# Working with Design Documents and Specifications

This document explains how to work with design documents and specifications for Radius development.

## Overview

Radius uses a design-driven development approach where major features and changes are documented in design notes before implementation begins. This ensures clarity, transparency, and community consensus on significant changes.

Minor changes such as documentation updates or small bug fixes may be reviewed and implemented directly via a GitHub issue. For larger changes, such as new feature design, a design note pull-request and review is required.

## Where design notes live

Design notes live in this repository under [eng/design-notes/](../../../../eng/design-notes/README.md), grouped by topic area. They include:

- Design proposals and architectural decisions
- Feature specifications
- Enhancement proposals

Older notes are in the [radius-project/design-notes](https://github.com/radius-project/design-notes) repository. See the [migration plan](../../../../eng/design-notes/migration-plan.md) for what was moved.

Templates are in [eng/design-notes/templates/](../../../../eng/design-notes/templates/):

- [design-template.md](../../../../eng/design-notes/templates/design-template.md) for technical designs
- [feature-spec-template.md](../../../../eng/design-notes/templates/feature-spec-template.md) for feature specifications
- [threat-model-template.md](../../../../eng/design-notes/templates/threat-model-template.md) for threat models

## Writing a design note

A design note exists to get decisions approved. Write it so a reviewer sees the decisions on the first page, then follows each one to its trade-offs and data flow.

### Structure

The [design template](../../../../eng/design-notes/templates/design-template.md) is ordered from "what we decide" to "how we build it":

| Tier         | Sections                                                                                                                                                                         | Reviewer action                                                              |
|--------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|------------------------------------------------------------------------------|
| 1. Agenda    | Review guide, Decisions for review, Overview, Terms, Objectives, User Experience                                                                                                 | Read fully; use as the meeting agenda                                        |
| 2. Support   | Design (diagrams, data flow, decision details, API, CLI, implementation, errors), Test plan, Security, Compatibility, Monitoring, Development plan, Open Questions, Alternatives | Read Decision details; sign off on Security and Compatibility; skim the rest |
| 3. Reference | Appendix                                                                                                                                                                         | Read when needed                                                             |

Keep every template heading. If a section does not apply, write one sentence saying why instead of deleting it or padding it.

### Decisions for review

The **Decisions for review** table near the top is the review meeting agenda. Each row is a choice a reviewer could reasonably disagree with, such as a new API shape, a storage choice, a new dependency, a security boundary, or a breaking change. Each row links to a **Decision details** subsection with the context, the options and their advantages and disadvantages, the proposed option, and how we'll know it's right.

In the review meeting, the author walks through the table row by row. The meeting ends when every row is Accepted, Rejected, or Deferred. The author then updates the Status column and records the outcomes in **Design Review Notes**.

### Steps

1. **Read before writing.** Read the feature spec, the issue, and the code paths the design touches. Read related notes under `eng/design-notes/` and `docs/architecture/`. Record what you verified in **Authoring notes**.
2. **List the decisions first.** Before any prose, fill in the **Decisions for review** table. A decision is a choice a reviewer could reasonably disagree with. Aim for 2–6. Do not list settled facts as decisions.
3. **Copy the template.** Create `eng/design-notes/<area>/YYYY-MM-<short-name>.md` from the [design template](../../../../eng/design-notes/templates/design-template.md). Keep every heading.
4. **Write Tier 1** (Review guide through User Experience). A reviewer who reads only this part should understand the problem, the proposed direction, and which decisions they are being asked to make.
5. **Draw the flow.** Add one architecture diagram and one sequence diagram for the main path. See [Diagrams](#diagrams).
6. **Write Decision details.** One subsection per decision ID: context, options table with advantages and disadvantages, proposed option, and how we'll know it's right.
7. **Write the remaining Tier 2 sections**, from contracts (API, CLI) to implementation details, error handling, tests, security, compatibility, monitoring, and development plan. In Implementation Details, give each component a one-line summary, then name the packages and types and cover data model, state transitions, concurrency, retries, configuration, and limits where they apply.
8. **Move low-level detail to the Appendix.** Code blocks longer than about 20 lines, full schemas, per-file change lists, scripts, and exhaustive tables go in the Appendix, linked from the section that needs them.
9. **Tighten without losing facts.** Apply the [writing guidelines](#writing-guidelines). Cut filler and repetition. Move a too-detailed fact down a tier instead of deleting it. When revising a note, check that every fact in the original still appears somewhere.
10. **Self-review** against the [verification checklist](#verification-checklist). The human author owns the note and reviews any AI draft before sharing it.

### Writing guidelines

- **Lead with the point.** The first sentence of each section states the conclusion; detail follows.
- **Layered, not lossy.** Each tier summarizes the one below it. A reader can stop at any tier and have an accurate picture; an implementer who reads to the end has every detail. Concise means fewer words, not fewer facts.
- **Short.** Sentences under about 25 words, paragraphs of three sentences or fewer. Prefer bullets and tables.
- **Concrete.** Name the package, type, field, flag, or API path. Link to code instead of describing it.
- **No filler.** No restating headings, no "This section describes…", no marketing words (seamless, robust, powerful, leverage), no summaries of what was just said.
- **One home per fact.** State each fact once and link to it. Summarize the feature spec instead of repeating it.
- **Every heading stays.** If a section does not apply, write one sentence saying why (for example, "No CLI changes."). Never pad a section to look complete.
- **Mark uncertainty.** Unverified claims go in **Authoring notes** or **Open Questions**, not stated as fact.
- **No invented paths.** Every path, symbol, command, and flag must exist, or be clearly marked as new in this design.
- **Disclose AI help.** Fill in **Authoring notes**: say which sections were AI-drafted and what you verified by hand.
- Do not hard-wrap prose (see [markdown.instructions.md](../../../../.github/instructions/markdown.instructions.md)).

### Diagrams

- Use Mermaid so diagrams render on GitHub and diff as text.
- Architecture: a `flowchart`, with new components visually distinct (for example, a `classDef` for new nodes).
- Data flow: a `sequenceDiagram` for the main path. Add a second only if the failure or async path differs materially.
- State: a `stateDiagram-v2` when the design adds a lifecycle or status field.
- Keep each diagram under about 12 nodes. Split rather than crowd.

### Verification checklist

- [ ] All template headings are present; sections that do not apply say why in one sentence.
- [ ] Every row in **Decisions for review** has an ID, options, recommendation, risk, and status, and links to a matching **Decision details** subsection.
- [ ] Tier 1 states the problem, the proposed direction, and the decisions being asked for, without requiring the rest of the document.
- [ ] There is at least one architecture diagram and one sequence diagram.
- [ ] Security and Compatibility are filled in, even if only to state there is no change.
- [ ] **Authoring notes** say what was AI-drafted and what was verified by hand.
- [ ] Every path, symbol, and link resolves, or is marked as new.
- [ ] No code block longer than about 20 lines outside the Appendix.
- [ ] Implementation Details names the packages and types per component, so another developer could implement and estimate the work.
- [ ] No fact from the source material (spec, issue, or prior draft) was dropped; detail cut from upper tiers appears in Tier 2 or the Appendix.
- [ ] Markdown lint and `make spellcheck` pass.

### Using an agent

The [radius-author-design-note](../../../../.github/skills/radius-author-design-note/SKILL.md) skill drafts or revises a design note by following these steps. The author owns the result and reviews it before sharing.

## Spec Kit in this repository

Spec Kit runs from this repository. Its configuration, scripts, and templates are in [.specify/](../../../../.specify/), and specifications are created under [specs/](../../../../specs/). Run Spec Kit commands from the repository root.

To give an agent context from other Radius repositories (for example `dashboard`, `docs`, or `resource-types-contrib`), clone them next to this repository and add them to a VS Code multi-root workspace.

## Related Resources

- [eng/design-notes/](../../../../eng/design-notes/README.md)
- [Legacy design-notes repository](https://github.com/radius-project/design-notes) (older notes only)
- [Spec Kit Documentation](https://github.com/github/spec-kit)
- [Design Note Template](../../../../eng/design-notes/templates/design-template.md)
- [Feature Spec Template](../../../../eng/design-notes/templates/feature-spec-template.md)
