---
name: radius-markdown-lint
description: 'Lint and format Markdown files using markdownlint-cli2 and markdown-table-formatter. Use when checking or fixing Markdown formatting, table alignment, or markdownlint rule violations after editing .md files.'
argument-hint: 'Optional: the path(s) or glob of Markdown files to lint — or leave blank to lint files you changed'
user-invocable: true
---

# Lint Markdown

Check and fix Markdown formatting using the repository's two Markdown tools, driven by `pnpm`.

## Overview

Markdown quality is enforced by two tools, both installed as dev dependencies in the root `package.json`:

- **`markdownlint-cli2`** — applies the markdownlint rules in [`.github/linters/.markdownlint-cli2.yaml`](../../linters/.markdownlint-cli2.yaml), which extends [`.github/linters/.markdownlint.yml`](../../linters/.markdownlint.yml). It honors the `ignores`/`gitignore` settings in that config (so `node_modules`, `.git`, etc. are skipped automatically).
- **`markdown-table-formatter`** — normalizes Markdown table alignment and padding. It has no config file and only accepts CLI flags (`--check`, `--columnpadding`, `--verbose`). It does **not** read `.gitignore`, so scope it to specific files or directories rather than the whole tree.

## Prerequisites

1. **Node.js + pnpm**: pnpm is pinned via the `packageManager` field in `package.json`. If pnpm is missing, run `corepack enable pnpm`.
2. **Install dependencies**: run `pnpm install --frozen-lockfile` from the repository root so the tool binaries are available.

## Procedure

### Step 1: Determine the scope

Prefer linting only the files you changed instead of the entire repository.

- For changed files: `git diff --name-only --diff-filter=d HEAD '*.md'`
- For a directory: use a glob such as `"docs/**/*.md"`.
- Avoid passing `"./**/*.md"` to `markdown-table-formatter` — it will traverse `node_modules` because it does not respect `.gitignore`.

### Step 2: Check (read-only)

Run both tools in check mode. Replace `<glob-or-paths>` with the scope from Step 1.

```bash
# Check table formatting (exits non-zero if any table needs reformatting)
pnpm exec markdown-table-formatter "<glob-or-paths>" --check

# Check markdownlint rules
pnpm exec markdownlint-cli2 "<glob-or-paths>" --config "./.github/linters/.markdownlint-cli2.yaml"
```

### Step 3: Fix

Apply automatic fixes, then re-run the checks in Step 2 to confirm a clean result.

```bash
# Reformat tables in place
pnpm exec markdown-table-formatter "<glob-or-paths>"

# Auto-fix markdownlint violations where possible
pnpm exec markdownlint-cli2 "<glob-or-paths>" --config "./.github/linters/.markdownlint-cli2.yaml" --fix
```

### Step 4: Resolve remaining issues

Not every markdownlint rule is auto-fixable. For violations that remain after Step 3, edit the Markdown manually following the rule messages. Do not hard-wrap prose to satisfy line length — the MD013 line-length rule is intentionally disabled (see the Markdown authoring guidelines in `.github/instructions/markdown.instructions.md`).

### Step 5: Report result

Summarize what was checked, what was fixed automatically, and any violations that require manual attention.

## Quick Reference

| Goal                        | Command                                                                                           |
|-----------------------------|---------------------------------------------------------------------------------------------------|
| Check table formatting      | `pnpm exec markdown-table-formatter "<glob>" --check`                                             |
| Fix table formatting        | `pnpm exec markdown-table-formatter "<glob>"`                                                     |
| Check markdownlint rules    | `pnpm exec markdownlint-cli2 "<glob>" --config "./.github/linters/.markdownlint-cli2.yaml"`       |
| Fix markdownlint rules      | `pnpm exec markdownlint-cli2 "<glob>" --config "./.github/linters/.markdownlint-cli2.yaml" --fix` |
| List changed Markdown files | `git diff --name-only --diff-filter=d HEAD '*.md'`                                                |
