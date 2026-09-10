# Review note: PR 1 - Parity manifest tooling

- **Pull request**: [#12735](https://github.com/radius-project/radius/pull/12735)
- **Plan phase**: [PR 1](../2026-03-goreleaser-release-lifecycle-implementation-plan.md#pr-1-parity-manifest-tooling)
- **Stack index**: [README](./README.md)

## Verdict

The collector, the checked-in baselines, and the manual workflow meet the PR 1 exit criteria: the manifest captures raw CLI assets and sidecars, Go and linker metadata, production and test image platforms, tags, digests, and labels, Helm metadata, release classification and note source, and downstream outputs. The changes below are hardening and cleanup, not behavior fixes.

## Changes made in this review

### 1. Host-aware runtime check in `release-parity-manifest.sh`

- **What changed**: the runtime metadata check executed `rad_linux_amd64` unconditionally. It now selects the asset that matches the host (`uname -s` / `uname -m`, the same mapping the `build/scripts/install-*.sh` installers use) and accepts `RELEASE_PARITY_RUNTIME_ASSET` as an explicit override. The test pins the override to `rad_linux_amd64` so it stays deterministic on any host.
- **Why**: the script already went out of its way to run without the GNU tools, which only matters on macOS, but the runtime check made every macOS run fail after downloading roughly half a gigabyte of assets.
- **Value**: `make release-parity-manifest` works on the machines the script was written to support, and the later staged-verification reuse (PR 16) inherits the same behavior without a special case.
- **Impact**: none for CI, which runs on `linux/amd64` and resolves to the same asset as before. The manifest schema and contents are unchanged.

### 2. Hash each checksum sidecar once and drop the `sha256_text_file` alias

- **What changed**: `collect_cli_assets` hashed every `.sha256` sidecar twice; it now computes the digest once and reuses it. `sha256_text_file` was a one-line alias of `sha256_file` and is removed.
- **Why**: dead indirection and repeated work with no difference in output.
- **Value**: less code to read for the same manifest.
- **Impact**: none on output; the baselines still validate byte for byte.

### 3. Remove two unused setup steps from `release-parity-manifest.yaml`

- **What changed**: the `Setup Python` and `Setup Docker Buildx` steps are gone.
- **Why**: nothing in the workflow runs Python (the Make install targets are shell only), and `docker buildx imagetools inspect` reads registries through the default builder that ships on `ubuntu-24.04` runners, so no builder instance has to be created.
- **Value**: two fewer pinned third-party actions to keep updated and a slightly shorter run.
- **Impact**: none on the produced manifest.

### 4. Reformat `release-parity-manifest_test.sh` to the collector's style

- **What changed**: the test mixed two- and four-space indentation. It is now formatted with `shfmt -i 4 -ci`, the profile the collector already conforms to. The fake `docker`, `helm`, and `oras` tools and the two multi-line `jq` programs were reindented by hand to the same four spaces because shfmt does not reach into heredocs and quoted strings. The JSON and YAML fixtures keep two-space indentation, which is the convention for those formats in this repository.
- **Why**: the repository's shell instructions ask for four-space indentation, and a test that sits next to its subject should read the same way. The `.editorconfig` shfmt profile (`-bn -sr -kp`) was not applied: no script in the repository conforms to it, nothing enforces it, and `-kp` breaks the alignment of continuation lines. Matching the sibling script and the newest scripts in the tree is the useful notion of consistency here.
- **Value**: one style to read across both files, and no formatting noise from shfmt-aware editors on later edits.
- **Impact**: whitespace only; the test passes unchanged. Later layers that append to this file may conflict on rebase in the reindented regions; those conflicts are resolved as part of the stack rebase.

### 5. Pull request description

- **What changed**: the body said "PR 1 of 3 in the GoReleaser migration stack". It now points at GitHub stack #12738 and the implementation plan, mentions the host-aware runtime check and its override, and lists these review notes in the file change summary.
- **Why**: the stack has 18 pull requests; a stale count misleads reviewers about the scope they are looking at.
- **Value**: reviewers reach the plan and the stack view from the first pull request.
- **Impact**: description only; no code change.

## Findings left as-is

- **`copilot-setup-steps` failure on this branch**: the failed step is `Install PostgreSQL client`, an apt mirror error unrelated to the change; the same workflow passes on `main`.

## Verification

- `bash .github/scripts/release-parity-manifest_test.sh` passes (determinism, contract, generated-RC-notes detection, invalid checksum, missing platform, missing downstream artifact, and both committed baselines).
- `shfmt -i 4 -ci -d` reports no differences for either script; ShellCheck passes for both scripts; Prettier passes for the workflow.
