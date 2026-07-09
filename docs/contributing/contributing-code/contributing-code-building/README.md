# Building the code

## Purpose

This is the authoritative guide for building Radius from source. It covers building the binaries (including the `rad` CLI), building the container images for the control-plane services, and regenerating checked-in generated code. Radius uses a [GNU Make](https://www.gnu.org/software/make/) `Makefile` (split into includes under [build/](../../../../build/)) to automate these tasks. If you are making your first contribution, the [first-commit walkthrough](../contributing-code-first-commit/first-commit-02-building/index.md) links here for the canonical steps.

## Prerequisites

- The repository cloned locally. See [Creating your own fork](../contributing-code-forks/index.md).
- The tools listed in the [prerequisites guide](../contributing-code-prerequisites/README.md) — at minimum Go (the version pinned in [go.mod](../../../../go.mod)) and GNU Make.
- For building container images: a working Docker daemon and a registry you can push to.
- For `make generate`: the extra code-generation tools listed under [Install code-generation tools](../contributing-code-prerequisites/README.md#install-code-generation-tools).

Run `make` (or `make help`) with no arguments at any time to print every target and its description.

## Steps

### Build the repository

Build all packages and binaries with:

```sh
make build
```

This runs `build-packages`, `build-binaries`, and `build-bicep`. The first run may take a few minutes because it downloads and builds dependencies; later builds reuse cached output. Binaries are written to `./dist/<GOOS>_<GOARCH>/release/`.

To build a single binary instead of everything — useful when iterating on the CLI — use its `build-<name>` target. For example, to build only the `rad` CLI:

```sh
make build-rad
```

To build with debug symbols (`-gcflags "all=-N -l"`), set `DEBUG=1`:

```sh
DEBUG=1 make build-rad
```

### Build, test, lint, and check formatting

This combined command builds the code, runs unit tests, runs the Go linters, and checks JSON/TS/JS/MJS formatting. Run it to verify your local changes before opening a pull request:

```sh
make build test lint format-check
```

If `format-check` reports issues, or if you added or changed any `.ts`, `.js`, `.mjs`, or `.json` files, reformat them with:

```sh
make format-write
```

If you changed any shell scripts, lint them with ShellCheck — install the pinned version once with `make install-shellcheck`, then run `make lint-shell`. See the [shell scripts and Makefiles guide](../contributing-code-shell-and-make/README.md#linting-shell-scripts-with-shellcheck) for details.

See the [tests guide](../contributing-code-tests/) for the full test matrix and the [writing code guide](../contributing-code-writing/) for linting details.

### Build the container images

Build the control-plane service images with `make docker-build`, and push them with `make docker-push`. By default the registry is your OS username and the tag is `latest`; override them with environment variables:

- `DOCKER_REGISTRY` — destination registry.
- `DOCKER_TAG_VERSION` — image tag.

These commands assume you are already logged in to the target registry (`docker login`, `az acr login`, etc.). For example, to build and push to a specific registry:

```sh
DOCKER_REGISTRY=ghcr.io/my-registry make docker-build docker-push
```

If you work with Radius frequently, set `DOCKER_REGISTRY` in your shell profile. The [radius-build-images](../../../../.github/skills/radius-build-images/SKILL.md) skill wraps this workflow, including single-image and multi-architecture builds.

### Generate code

When you change API schemas or Go APIs that have mocks, regenerate the checked-in generated code as part of your commit. Radius **checks in** generated code so that not every contributor has to install the generators. The PR process validates that the generated files are up to date.

After installing the [code-generation prerequisites](../contributing-code-prerequisites/README.md#install-code-generation-tools), run:

```sh
make generate
```

This runs several generators in sequence and may take a few minutes. **Commit** the resulting changes alongside your code change. For details on the TypeSpec → Swagger → Go pipeline, see the [schema changes guide](../contributing-code-schema-changes/README.md).

## Verification

- `make build` completes without errors and produces binaries under `./dist/<GOOS>_<GOARCH>/release/` (for example `rad`, `applications-rp`, `ucpd`, `dynamic-rp`, `controller`).
- `make build test lint format-check` passes end to end.
- After `make docker-build`, the images appear in `docker images`.
- After `make generate`, `git status` shows only the generated changes you expect, and no generated files remain stale.

## Troubleshooting

- **A `make` command fails on a missing dependency.** Review the [prerequisites guide](../contributing-code-prerequisites/README.md) and install the missing tool.
- **Docker push fails with an authentication error.** Confirm you are logged in to the registry named in `DOCKER_REGISTRY`.
- **You need to report a build problem.** Dump every Makefile variable with `make dump` (the output is large, so redirect it to a file) and include it in your report.
- **Still stuck.** Ask in the [Radius Discord forum](https://discordapp.com/channels/1113519723347456110/1115302284356767814), or [open an issue](https://github.com/radius-project/radius/issues/new/choose) so we can improve the tooling and these instructions.
