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

A design note exists to get decisions approved. Write it so a reviewer can find the decisions, the trade-offs, and the data flow on the first page, then read deeper only where they need to.

### Structure

The [design template](../../../../eng/design-notes/templates/design-template.md) is ordered from "what we decide" to "how we build it":

| Tier         | Sections                                                                                                                                                                         | Reviewer action                              |
|--------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|----------------------------------------------|
| 1. Decide    | Review guide, Decisions for review, Overview, Terms, Objectives, User Experience                                                                                                 | Read and approve                             |
| 2. Support   | Design (diagrams, data flow, decision details, API, CLI, implementation, errors), Test plan, Security, Compatibility, Monitoring, Development plan, Open Questions, Alternatives | Skim; sign off on Security and Compatibility |
| 3. Reference | Appendix                                                                                                                                                                         | Read when needed                             |

Keep every template heading. If a section does not apply, write one sentence saying why instead of deleting it or padding it.

### Decisions for review

The **Decisions for review** table near the top is the review meeting agenda. Each row is a choice a reviewer could reasonably disagree with, such as a new API shape, a storage choice, a new dependency, a security boundary, or a breaking change. Each row links to a **Decision details** subsection with the context, the options and their advantages and disadvantages, the proposed option, and how we'll know it's right.

In the review meeting, the author walks through the table row by row. The meeting ends when every row is Accepted, Rejected, or Deferred. The author then updates the Status column and records the outcomes in **Design Review Notes**.

### Writing guidelines

- Lead each section with its conclusion; put detail after.
- Be complete. Concise means fewer words, not fewer facts: move detail down a tier instead of deleting it.
- Use short sentences, bullets, and tables. Link to code instead of describing it.
- Include one architecture diagram and one sequence diagram, in Mermaid, for the main path.
- Move code blocks longer than about 20 lines, full schemas, per-file change lists, and exhaustive tables to the Appendix.
- State each fact once and link to it. Summarize the feature spec instead of repeating it.
- Fill in **Authoring notes**: say which sections were AI-drafted and what you verified by hand.

### Using an agent

The [radius-author-design-note](../../../../.github/skills/radius-author-design-note/SKILL.md) skill drafts or revises a design note following these steps. The author owns the result and reviews it before sharing.

## Spec Kit for Spec-Driven Development

The Radius team optionally uses [Spec Kit](https://github.com/github/spec-kit) to help manage and organize specifications. Spec Kit is an open-source toolkit from GitHub that enables structured, AI-assisted specification development.

For full details on what Spec Kit is and how to use it, see the [Spec Kit documentation](https://github.com/github/spec-kit).

## Multi-Repo Workspace Setup

Spec Kit is designed to work within a single repository. Since Radius is a multi-repo project where specifications are kept in the design-notes repository while implementation code lives across multiple repositories, we use a VS Code workspace configuration to bridge this gap.

### Setting Up the Workspace

1. Clone the design-notes repository alongside your other Radius repositories:

   ```bash
   git clone https://github.com/radius-project/design-notes.git
   ```

2. Open the VS Code workspace file located at `design-notes/design-notes.code-workspace`

3. This workspace includes the following repositories:
   - design-notes
   - radius
   - dashboard
   - docs
   - resource-types-contrib

### Workflows

The multi-repo workspace enables two key workflows:

- **Authoring specifications**: Open the workspace to author specifications in context with the rest of the Radius repositories, providing visibility into related code and documentation across the project.

- **Implementing specifications**: When spec authors are ready to implement code, they can run Spec Kit's implementation prompts from the VS Code workspace. This gives them the entire context of both specifications and code together, making it easier to translate designs into working implementations.

## Related Resources

- [design-notes Repository](https://github.com/radius-project/design-notes)
- [Spec Kit Documentation](https://github.com/github/spec-kit)
- [Design Note Template](../../../../eng/design-notes/templates/design-template.md)
- [Feature Spec Template](../../../../eng/design-notes/templates/feature-spec-template.md)
