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

## Managing external CLI pins

The canonical versions, release sources, platform assets, and SHA-256 checksums for downloaded CLI tools live in [`build/tools.yaml`](../../../../build/tools.yaml). Run `make update-tools` to check every source and refresh available versions and checksums. The command regenerates the committed [`build/tools.generated.mk`](../../../../build/tools.generated.mk) include that supplies metadata to the existing `make install-<tool>` targets. The updater is built into the ignored `bin/` directory before it runs, which avoids Windows security software blocking Go's temporary `go run` executable. A newer release is adopted only once it is at least `cooldownDays` old, reducing the risk of pinning a release that is withdrawn shortly after publication. See the [tool updater reference](../../../../internal/tooling/README.md) for the manifest schema and supported values.

Bicep remains on a manually tested pin (`update: false`) because upgrades must preserve ACR authentication, local registry trust, and extension compatibility. Its `versionFiles` entry synchronizes the version in `pkg/cli/bicep/tools/download_tools.go`, keeping `rad bicep download`, `make install-bicep`, and Bicep container staging on the same release. Local registries require [explicit hostname trust](../contributing-code-tests/running-functional-tests.md#local-bicep-registry-trust); do not disable registry or certificate validation to test an upgrade.

Tool installers use [`build/scripts/retry-curl.sh`](../../../../build/scripts/retry-curl.sh) for temporary download failures. The helper uses increasing delays with jitter, up to 10 retries within a 300-second budget per request. Each attempt has a 15-second connection timeout and a total timeout of at most 60 seconds, reduced to the remaining budget. HTTP 408, 429, 500, 502, 503, and 504, connection failures, incomplete transfers, and HTTP/2 stream resets can be retried. A valid `Retry-After` header from the final response sets the minimum wait, whether it contains seconds or an HTTP date. If that wait would use all of the remaining budget, the request fails without another attempt. Permanent errors, such as HTTP 403 or 404 and invalid certificates, fail at once. Installers verify published or pinned checksums before they replace a binary. Run `make test-retry-curl test-install-bicep` to check the shared policy and Bicep installation without network access.

## Updating resource types and recipe packs

[`deploy/manifest/defaults.yaml`](../../../../deploy/manifest/defaults.yaml) pins the resource-types-contrib commits that Radius ships. `make update-resource-types`, `make update-recipe-packs`, and `make update-resource-types-and-recipe-packs` move the pins; `make sync-resource-types` regenerates the vendored files from the current pins without changing them.

These targets compile the pinned Kubernetes recipe pack (`recipe-packs/kubernetes/default.bicep`) into [`deploy/manifest/recipe-packs/kubernetes/default.json`](../../../../deploy/manifest/recipe-packs/kubernetes/default.json), which `rad` embeds to create the default recipe pack. Compiling requires the Bicep CLI version pinned in `build/tools.yaml`:

```bash
make install-bicep                  # installs into $HOME/.local/bin by default
export PATH="$HOME/.local/bin:$PATH" # or set BICEP=/path/to/bicep
make sync-resource-types
```

The script fails if `bicep` is missing or is a different version. Commit the regenerated `default.json` together with the `defaults.yaml` change; CI fails if the committed file does not match a fresh compile of the pinned pack.

## Linting shell scripts with ShellCheck

Every tracked `.sh` script is validated with [ShellCheck](https://github.com/koalaman/shellcheck) as part of the pull request checks, so run it locally before you push. Install the pinned version (into a user-owned bin directory — no `sudo`) and lint every tracked script with:

```sh
make install-shellcheck
make lint-shell
```

`make lint-shell` applies the shared configuration in [`.shellcheckrc`](../../../../.github/linters/.shellcheckrc) and skips the third-party Spec Kit tooling under `.specify/`. The version and per-platform checksums that `make install-shellcheck` pins live in [`build/tools.yaml`](../../../../build/tools.yaml) (`SHELLCHECK_VERSION`).

The easy path is the [dev container](../../../../.devcontainer/), which installs the ShellCheck CLI on the `PATH` and the ShellCheck VS Code extension for you, so you can run `make lint-shell` (and see findings inline as you edit) without installing anything.

Resolve every finding rather than leaving it. When a warning is a genuine false positive, suppress it narrowly with a `# shellcheck disable=SCxxxx` directive immediately above the affected command and add a short comment explaining why.

## Verification

- `make help` lists your new/changed target with its description.
- The target runs to completion, and every script passes `make lint-shell` (ShellCheck) and a quick `bash -n` syntax check.

## Related docs

- [Building the repo](../contributing-code-building/README.md) — the primary `make` targets contributors run.
- [Documentation index](../../README.md) — every contributing doc.
