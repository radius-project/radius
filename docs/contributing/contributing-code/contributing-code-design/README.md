# Working with Design Documents and Specifications

This document explains how to work with design documents and specifications for Radius development.

## Overview

Radius uses a design-driven development approach where major features and changes are documented in design notes before implementation begins. This ensures clarity, transparency, and community consensus on significant changes.

Minor changes such as documentation updates or small bug fixes may be reviewed and implemented directly via a GitHub issue. For larger changes, such as new feature design, a design note pull-request and review is required.

## The design-notes Repository

The [radius-project/design-notes](https://github.com/radius-project/design-notes) repository is the central location for:

- Design proposals and architectural decisions
- Feature specifications
- Enhancement proposals

See the [design-notes README](https://github.com/radius-project/design-notes#readme) for the full review process and instructions for creating design documents.

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
- [Design Note Template](https://github.com/radius-project/design-notes/blob/main/template/YYYY-MM-design-template.md)
- [Feature Spec Template](https://github.com/radius-project/design-notes/blob/main/template/YYYY-MM-feature-spec-template.md)
