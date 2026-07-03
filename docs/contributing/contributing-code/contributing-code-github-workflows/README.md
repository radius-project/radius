# Contributing to GitHub Actions workflows

## Purpose

This is the primary doc for adding or changing the CI/CD workflows that build, test, and release Radius. It is reference material for anyone editing the automation under `.github/workflows/`. The detailed, rule-by-rule conventions live in the [GitHub Workflows instruction file](../../../../.github/instructions/github-workflows.instructions.md), which Copilot applies automatically to any `.github/workflows/*.yml`/`*.yaml` file you edit; this doc gives the map of where the workflows live and how to change them safely.

## Where these files live

- `.github/workflows/` — the workflow definitions that run on every push, pull request, and release.
- Reusable/shared workflows are referenced from the org-level [`radius-project/.github`](https://github.com/radius-project/.github) repository (for example the spellcheck and linter workflows), so a fix there can affect every repo.

## Conventions

Follow the [GitHub Workflows instruction file](../../../../.github/instructions/github-workflows.instructions.md). Its emphasis for Radius:

- **Fork-testability** — a workflow must be runnable from a fork without access to repository secrets; gate secret-dependent steps rather than assuming they exist.
- **Least privilege** — set explicit `permissions:` blocks; default to read-only and grant write only where needed.
- **Pin and cache** — pin action versions and cache dependencies to keep runs fast and reproducible.

## Verification

- The workflow you changed runs green on your pull request (open it as a draft first if you want to iterate).
- The [github-workflows.instructions.md](../../../../.github/instructions/github-workflows.instructions.md) checklist is satisfied — especially the fork-testability and `permissions:` items.

## Related docs

- [Building the repo](../contributing-code-building/README.md) — the `make` targets that CI invokes.
- [Testing](../contributing-code-tests/README.md) — the test tiers the workflows run.
- [Documentation index](../../README.md) — every contributing doc.
