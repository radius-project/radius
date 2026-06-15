---
description: 'Conventions for authoring Markdown in this repository'
applyTo: '**/*.md'
---

# Markdown Guidelines

Formatting rules for Markdown are enforced by tooling. This file covers only the conventions the linters cannot check and points to the source of truth for everything else.

## Source of truth

Do not restate or duplicate lint rules here — change the configuration instead, otherwise this file will drift from what is actually enforced.

- **Rules:** `markdownlint-cli2`, configured by [`.github/linters/.markdownlint-cli2.yaml`](../linters/.markdownlint-cli2.yaml), which extends [`.github/linters/.markdownlint.yml`](../linters/.markdownlint.yml).
- **Tables:** formatted by `markdown-table-formatter`.

Both tools are installed as dev dependencies in the root `package.json`. See the `markdown-lint` skill for how to run them.

## Conventions not enforced by tooling

- **Do not hard-wrap prose.** Write each paragraph as a single long line. The line-length rule (MD013) is intentionally disabled; manual line breaks create noisy diffs and break reflowing in editors. This applies to all prose — paragraphs, list item text, blockquote content, and image/link alt text. Code blocks and tables are excluded.

## Before committing

Run the Markdown linters (see the `markdown-lint` skill) on the files you changed and fix any reported issues.
