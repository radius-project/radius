# Writing Guidelines for Contributor Documentation

Standards for creating and updating docs in `docs/contributing/`.

## Content Types

Every contributor doc should be one of these two types. Decide the type before writing and do not mix types on a single page.

| Type | Purpose | Reader's need | Examples |
| ------ | --------- | --------------- | ---------- |
| **Guide** | Walk the reader through a task or workflow | "Help me do something" | First commit walkthrough, build locally, run tests, debug the control plane, submit a PR |
| **Reference** | Provide facts, context, and explanations for lookup | "Help me understand something" | Code organization, config options, Make targets, architecture decisions, design rationale |

### Writing each type

- **Guides** have a clear goal, concrete steps, and a visible result. State prerequisites upfront or link to them. Use numbered steps for the main workflow. A guide can range from a short recipe ("run these three commands") to a full walkthrough ("set up your environment and make your first contribution") — scope it to a single task or scenario.
- **Reference** pages describe what exists and why. Use tables, structured lists, and diagrams to make content scannable. Include background reasoning ("why is it designed this way") alongside the facts rather than splitting it into a separate page. Architecture docs in `docs/architecture/` are reference pages.

If a page starts drifting between types (e.g., explaining architecture in the middle of build steps), split it and cross-link.

## Structure

- **One topic per document**: Each doc should cover a single, well-scoped topic.
- **README.md as entry point**: Each directory should have a `README.md` that introduces the topic and links to sub-pages if any.
- **Logical ordering**: For multi-step guides, use numbered prefixes in directory names (e.g., `00-prerequisites/`, `01-setup/`).

## Formatting

- Use ATX-style headers (`#`, `##`, `###`). Do not skip levels.
- Use fenced code blocks with language identifiers for all code and commands:

  ````text
  ```bash
  make build
  ```
  ````

- Use relative links for internal cross-references. Prefer linking to directories (which resolve to `README.md`) over explicit `README.md` links.
- Use tables for structured comparisons or reference data.
- Use numbered lists for sequential steps, bullet lists for unordered items.

## Tone and Style

- **Direct and concise**: Use imperative mood for instructions ("Run the command", not "You should run the command").
- **Assume minimal context**: A new contributor may be reading this doc first. State prerequisites explicitly or link to the prerequisites doc.
- **Show, don't tell**: Prefer command examples and code snippets over abstract descriptions.
- **Avoid jargon**: Define project-specific terms on first use or link to a glossary.

## Commands and Paths

- Always verify commands work by checking the codebase (Makefile targets, scripts, configs).
- Use the repository root as the working directory unless stated otherwise.
- Show expected output when it helps the reader confirm success.
- Use `$` prefix for shell commands to distinguish them from output:

  ```bash
  $ make build
  Building...
  ```

## Cross-References

When creating or updating a doc, always check whether these files need corresponding updates:

| File | When to Update |
| ------ | ---------------- |
| `CONTRIBUTING.md` | New top-level topic added |
| Index or table-of-contents pages | New contribution pathway or section added |
| Parent `README.md` | New sub-page added to a section |

## Content Accuracy

- Every command in a doc must correspond to a real Makefile target, script, or executable in the repo.
- File paths must be valid relative to the repository root.
- Configuration examples must match the actual config file format and location.
- Version numbers and tool requirements must reflect current state.
