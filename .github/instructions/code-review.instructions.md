---
applyTo: '**/*'
---

## Code Review Guidelines

This is the rubric for what a good Radius code review contains. It applies to any review Copilot performs. For the end-to-end procedure of reviewing a specific pull request — fetching the PR, syncing to the latest head, resolving line numbers, and staging comments — use the [`radius-code-review`](../skills/radius-code-review/SKILL.md) skill, which references this rubric.

When reviewing code in a specific language or technology, also apply the matching `.github/instructions/*.instructions.md` file (Go, Shell, Make, Docker, GitHub Workflows, Bicep, Markdown).

### Principles

- Be concise and direct — short imperative statements, not paragraphs.
- Give actionable feedback; avoid vague directives like "be more accurate."
- Be specific: exact file paths, line numbers, and clear issue descriptions.
- Prefer specific over general: "Use `const` instead of `let` on line 42" beats "improve variable declarations."
- Provide rationale — explain why a change is needed, not just what to change.
- Show a corrected example when it clarifies a non-trivial issue.
- Avoid generic praise; focus only on changes that need to be made.

### What to evaluate

- **Correctness**: bugs, unhandled edge cases, race conditions, and regressions.
- **Security**: input validation, authentication and authorization, injection sinks, unsafe deserialization, secret or credential handling, and supply-chain risk from new dependencies.
- **Idiomatic usage**: does the code follow language best practices?
- **Readability and simplicity**: is the code clear, maintainable, and free of unnecessary complexity?
- **Performance**: are there avoidable performance problems?
- **Consistency**: does the change align with existing codebase patterns?

### Reviewing tests

- Use parallel execution where possible.
- Consolidate copy/paste tests into parameterized cases.
- Check for clear names, adequate assertions, and proper setup/teardown and test doubles.
- Confirm coverage of edge cases and error conditions.

### PR title

Ensure the title clearly and accurately describes the change. If it is vague, overly broad, or misleading, suggest a better one.

### Documentation impact

Assess whether the change requires updates to contributor documentation in `docs/contributing/` or `docs/architecture/`. Common triggers:

- New or changed CLI commands/flags
- Build system changes (Makefile targets, scripts)
- New packages, directories, or commands
- Test framework or workflow changes
- Configuration or prerequisite changes
- Architecture changes

Use the [`radius-contributing-docs-updater`](../skills/radius-contributing-docs-updater/SKILL.md) skill for the assessment. To make the suggestion concrete, consult the [code ↔ doc path map](../../docs/contributing/contributing-agent-assets.md#code--doc-path-map): when the PR touches a mapped code glob, name the specific backing doc that likely needs updating. The map grows as the contributing docs are filled out, so also search `docs/contributing/` and `docs/architecture/` for prose that references any changed command, flag, or path. If doc updates are needed, name the docs and the required change, and suggest the [`radius-update-doc`](../skills/radius-update-doc/SKILL.md) skill to draft the patch. This assessment is **advisory, not a blocking gate**.

### Output format

Structure review comments as:

```text
path/to/file.ext
    Line X: Specific issue description and requested change.
```

Include an overall assessment summarizing the key findings and recommendations.

### Handling limitations

- If a file is too large to analyze completely, focus on the most critical changes and note the limitation.
- If a file cannot be accessed, note that in the review.
