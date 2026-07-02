# Contributing Bicep files

## Purpose

This is the primary doc for writing and modifying the `.bicep` files in Radius — the recipes and deployment templates that describe infrastructure, plus the Bicep test fixtures. It is reference material for anyone authoring Bicep. The detailed, rule-by-rule conventions live in the [Bicep instruction file](../../../../.github/instructions/bicep.instructions.md), which Copilot applies automatically to any `.bicep` file you edit; this doc gives the map of where the Bicep files live and how to keep them consistent.

> **Not the same as Bicep _types_.** Generating the Radius/AWS Bicep type extensions is a separate pipeline — see the [Bicep types migration guide](../../bicep-types-migration-guide.md). This doc is about authoring `.bicep` source (recipes, deployment templates, and test data).

## Where these files live

- Recipe and deployment-template `.bicep` files are spread across the tree, including `test/` fixtures and `pkg/**/testdata/`.
- Radius Bicep extensions are built from [`bicep-tools/`](../../../../bicep-tools/); the CLI downloads them via `rad bicep download`.

## Conventions

Follow the [Bicep instruction file](../../../../.github/instructions/bicep.instructions.md). Its emphasis for Radius:

- **Naming** — use the Radius resource naming and parameter conventions consistently.
- **Deployment patterns** — follow the established recipe/deployment-template structure so resources compose predictably.
- **Parameterize, don't hard-code** — expose environment- and application-specific values as parameters.

## Verification

- The file compiles with the Radius Bicep extension (`rad bicep download` first if you have not).
- Any test that consumes the fixture still passes (see [testing](../contributing-code-tests/README.md)).

## Related docs

- [Bicep types migration guide](../../bicep-types-migration-guide.md) — the separate type-generation pipeline.
- [Documentation index](../../README.md) — every contributing doc.
