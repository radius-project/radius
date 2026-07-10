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

## Linting shell scripts with ShellCheck

Every tracked `.sh` script is validated with [ShellCheck](https://github.com/koalaman/shellcheck) as part of the pull request checks, so run it locally before you push. Install the pinned version (into a user-owned bin directory — no `sudo`) and lint every tracked script with:

```sh
make install-shellcheck
make lint-shell
```

`make lint-shell` applies the shared configuration in [`.shellcheckrc`](../../../../.github/linters/.shellcheckrc) and skips the third-party Spec Kit tooling under `.specify/`. The version and per-platform checksums that `make install-shellcheck` pins live in [`build/tools.mk`](../../../../build/tools.mk) (`SHELLCHECK_VERSION`).

The easy path is the [dev container](../../../../.devcontainer/), which installs the ShellCheck CLI on the `PATH` and the ShellCheck VS Code extension for you, so you can run `make lint-shell` (and see findings inline as you edit) without installing anything.

Resolve every finding rather than leaving it. When a warning is a genuine false positive, suppress it narrowly with a `# shellcheck disable=SCxxxx` directive immediately above the affected command and add a short comment explaining why.

## Verification

- `make help` lists your new/changed target with its description.
- The target runs to completion, and every script passes `make lint-shell` (ShellCheck) and a quick `bash -n` syntax check.

## Related docs

- [Building the repo](../contributing-code-building/README.md) — the primary `make` targets contributors run.
- [Documentation index](../../README.md) — every contributing doc.
