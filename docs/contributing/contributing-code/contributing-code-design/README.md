# Working with Design Documents and Specifications

## Purpose

This document explains how to propose a design for a larger Radius change: where the design note goes, which template to start from, and how it is reviewed. Radius uses a design-driven development approach where major features and changes are documented in design notes before implementation begins. This ensures clarity, transparency, and community consensus on significant changes.

Minor changes such as documentation updates or small bug fixes may be reviewed and implemented directly via a GitHub issue. For larger changes, such as new feature design, a design note pull request and review is required.

## Prerequisites

- A GitHub issue that describes the problem or feature. Design notes describe **why** and **how** to make a change once there is agreement that it is appropriate for the project. Use an issue, not a design note, for a feature request.
- A clone of the repository where the implementation will live.

## Steps

### 1. Choose the repository

Open the design note pull request in the repository where the implementation lives, so the design is reviewed and kept next to the code that implements it. For changes to this repository, design notes go in [`eng/design-notes/`](../../../../eng/design-notes/README.md).

The [radius-project/design-notes](https://github.com/radius-project/design-notes) repository is archived. Do not open new pull requests there. Its documents for this repository were migrated to `eng/design-notes/`; see the [migration plan](../../../../eng/design-notes/migration-plan.md).

### 2. Start from a template

Copy the template that fits from [`eng/design-notes/templates/`](../../../../eng/design-notes/templates/):

| Template                                                                                    | Use it for                                                                                   |
|---------------------------------------------------------------------------------------------|----------------------------------------------------------------------------------------------|
| [feature-spec-template.md](../../../../eng/design-notes/templates/feature-spec-template.md) | The user scenarios and goals for a new feature. A feature spec should precede a design note. |
| [design-template.md](../../../../eng/design-notes/templates/design-template.md)             | The technical design: how the change will be built.                                          |
| [threat-model-template.md](../../../../eng/design-notes/templates/threat-model-template.md) | A threat model for a system, feature, or component.                                          |

### 3. Name and place the document

- Name the document with the year and month followed by a short description: `YYYY-MM-description.md`, for example `2024-06-private-bicep-registries.md`.
- Place it in the topic directory that fits the area. The [design notes README](../../../../eng/design-notes/README.md#directory-structure) lists the topic directories.
- Put images and other supporting assets in a directory with the same name as the document, without `.md`.
- Fill out the template following [Writing a design note](#writing-a-design-note). Keep every `##` and `###` heading; if a section does not apply, say why in one sentence.

### 4. Open the pull request

- Give the pull request a Conventional Commit title, for example `docs: add <topic> design note`. See [Use a Conventional Commit pull request title](../../contributing-pull-requests/README.md#3-use-a-conventional-commit-pull-request-title).
- Open the pull request **before** the design review meeting where you want to discuss it. In the meeting, walk the [Decisions for review](#decisions-for-review) table row by row. Major designs are reviewed in the design review meeting. Smaller designs may be reviewed asynchronously on the pull request.

### 5. Respond to the review

Reviewers record major questions and feedback as comments on the pull request, so the discussion is visible to people who were not in the meeting. The design may be approved as-is or returned for iteration.

If the design is approved:

- Incorporate any outstanding feedback, and fill in the Status column and **Outcome** line for every decision.
- Reply to and resolve every comment, explaining how the feedback was resolved.
- One or more approvers approve the pull request, and it is merged.

If the design is not approved:

- Leave a comment on the pull request that summarizes the major feedback and the next steps. As the author, you own documenting the next steps so the review can pick up where it left off.
- Iterate on the proposal and prepare for the next review.

### 6. Implement the design

Development work on the design begins only after the design is approved. If something comes up during implementation that needs team review, open a pull request with proposed updates to the design note. It goes through the same review process.

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

The **Decisions for review** table near the top is the review meeting agenda. Each row is a choice a reviewer could reasonably disagree with, such as a new API shape, a storage choice, a new dependency, a security boundary, or a breaking change. Use short option names, not letters, so the table reads on its own. Each row links to a **Decision details** subsection with the context, the options and their advantages and disadvantages, the proposed option, its consequences, how we'll know it's right, and the outcome.

In the review meeting, the author walks through the table row by row. The meeting ends when every row is Accepted, Rejected, or Deferred. The author then updates the Status column **and** the **Outcome** line of each decision (the chosen option, the reason, the date, and who decided), and summarizes the meeting in **Design Review Notes**.

### Design notes as the decision record

Merged design notes are the Radius record of architecture decisions. Each **Decision details** subsection follows the [MADR](https://adr.github.io/madr/) model: context, options with pros and cons, decision, consequences, and confirmation ("How we'll know it's right").

- Decision IDs such as `D1` are local to a note. Refer to a decision from elsewhere as `<note path>#d1-<title>`.
- To replace an earlier decision, write a new design note that links to it. In the old note, set that decision's Status to **Superseded** and its Outcome to `Superseded by <link>`. If the whole note is replaced, set the note's header Status to `Superseded by <link>`.
- Do not rewrite an accepted decision in place. Record the change in a new note so the history stays readable.

### Writing steps

1. **Read before writing.** Read the feature spec, the issue, and the code paths the design touches. Read related notes under `eng/design-notes/` and `docs/architecture/`. Record what you verified in **Authoring notes**.
2. **List the decisions first.** Before any prose, fill in the **Decisions for review** table. A decision is a choice a reviewer could reasonably disagree with. Aim for 2–6. Do not list settled facts as decisions.
3. **Copy the template.** Create `eng/design-notes/<area>/YYYY-MM-<short-name>.md` from the [design template](../../../../eng/design-notes/templates/design-template.md), as in [Name and place the document](#3-name-and-place-the-document). Keep every `##` and `###` heading.
4. **Write Tier 1** (Review guide through User Experience). A reviewer who reads only this part should understand the problem, the proposed direction, and which decisions they are being asked to make.
5. **Draw the flow.** Add one architecture diagram and one sequence diagram for the main path. See [Diagrams](#diagrams).
6. **Write Decision details.** One subsection per decision ID: context, options table with advantages and disadvantages, proposed option, consequences, and how we'll know it's right. Leave **Outcome** for after review.
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

- [ ] Every `##` and `###` template heading is present; sections that do not apply say why in one sentence. `####` component and user story headings are added or removed as needed.
- [ ] Every row in **Decisions for review** has an ID, options, recommendation, risk, and status, and links to a matching **Decision details** subsection.
- [ ] Tier 1 states the problem, the proposed direction, and the decisions being asked for, without requiring the rest of the document.
- [ ] There is at least one architecture diagram and one sequence diagram.
- [ ] Security and Compatibility are filled in, even if only to state there is no change.
- [ ] **Authoring notes** say what was AI-drafted and what was verified by hand.
- [ ] Every path, symbol, and link resolves, or is marked as new.
- [ ] No code block longer than about 20 lines outside the Appendix.
- [ ] Implementation Details names the packages and types per component, so another developer could implement and estimate the work.
- [ ] No fact from the source material (spec, issue, or prior draft) was dropped; detail cut from upper tiers appears in Tier 2 or the Appendix.
- [ ] Markdown lint and `make spellcheck` pass (see [Verification](#verification)).

### Using an agent

The [radius-author-design-note](../../../../.github/skills/radius-author-design-note/SKILL.md) skill drafts or revises a design note by following these steps. The author owns the result and reviews it before sharing.

## Verification

- The design note is in the repository where the implementation lives. For this repository, it is under a topic directory in `eng/design-notes/`.
- The file name follows `YYYY-MM-description.md`.
- The note passes the [verification checklist](#verification-checklist) for writing.
- Spellcheck and Markdown lint pass on the changed files:

  ```bash
  make spellcheck
  pnpm exec markdownlint-cli2 "<path-to-design-note>" --config "./.github/linters/.markdownlint-cli2.yaml"
  ```

- The design pull request is approved and merged before implementation starts.

## Troubleshooting

- **Spellcheck flags a correctly spelled term.** Add the term to [`.cspellignore`](../../../../.cspellignore) at the repository root.
- **No topic directory fits.** Create a new topic directory under `eng/design-notes/` and add it to the table in the [design notes README](../../../../eng/design-notes/README.md#directory-structure).
- **The implementation spans more than one repository.** Ask the maintainers on the issue which repository should hold the design note before you start writing it.
