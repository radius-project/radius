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
- Fill out the template. Do not omit sections.

### 4. Open the pull request

- Give the pull request a Conventional Commit title, for example `docs: add <topic> design note`. See [Use a Conventional Commit pull request title](../../contributing-pull-requests/README.md#3-use-a-conventional-commit-pull-request-title).
- Open the pull request **before** the design review meeting where you want to discuss it. Major designs are reviewed in the design review meeting. Smaller designs may be reviewed asynchronously on the pull request.

### 5. Respond to the review

Reviewers record major questions and feedback as comments on the pull request, so the discussion is visible to people who were not in the meeting. The design may be approved as-is or returned for iteration.

If the design is approved:

- Incorporate any outstanding feedback.
- Reply to and resolve every comment, explaining how the feedback was resolved.
- One or more approvers approve the pull request, and it is merged.

If the design is not approved:

- Leave a comment on the pull request that summarizes the major feedback and the next steps. As the author, you own documenting the next steps so the review can pick up where it left off.
- Iterate on the proposal and prepare for the next review.

### 6. Implement the design

Development work on the design begins only after the design is approved. If something comes up during implementation that needs team review, open a pull request with proposed updates to the design note. It goes through the same review process.

## Verification

- The design note is in the repository where the implementation lives. For this repository, it is under a topic directory in `eng/design-notes/`.
- The file name follows `YYYY-MM-description.md`.
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
