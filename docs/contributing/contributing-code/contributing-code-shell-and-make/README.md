# Contributing shell scripts and Makefiles

## Purpose

This is the primary doc for writing and modifying the build automation in Radius — the Bash scripts that support the build and the GNU Make targets that drive it. It is reference material for anyone editing a `.sh` script, the `Makefile`, or a `build/*.mk` include. The detailed conventions live in two instruction files — [shell](../../../../.github/instructions/shell.instructions.md) and [Make](../../../../.github/instructions/make.instructions.md) — which Copilot applies automatically to matching files; this doc gives the map of where the scripts and Make includes live.

## Where these files live

- **Make**: the root `Makefile` includes topic-scoped files under [`build/`](../../../../build/) (`build.mk`, `test.mk`, `docker.mk`, `generate.mk`, and others). Run `make help` to list every target.
- **Shell**: helper scripts live under `build/`, `.github/`, and `deploy/`; many are invoked by Make targets or CI workflows.

## Conventions

Follow the [shell instruction file](../../../../.github/instructions/shell.instructions.md) and the [Make instruction file](../../../../.github/instructions/make.instructions.md). Their emphasis for Radius:

- **Safe Bash** — start scripts with `set -euo pipefail`, quote expansions, and validate arguments.
- **Well-structured Make** — declare `.PHONY` targets, keep recipes small, and add a `##`-style help comment so `make help` stays complete.
- **One home per target** — add a new target to the `build/*.mk` include that owns its topic rather than the root `Makefile`.

## Verification

- `make help` lists your new/changed target with its description.
- The target runs to completion, and any script passes a quick `shellcheck` and `bash -n` syntax check.

## Related docs

- [Building the repo](../contributing-code-building/README.md) — the primary `make` targets contributors run.
- [Documentation index](../../README.md) — every contributing doc.
