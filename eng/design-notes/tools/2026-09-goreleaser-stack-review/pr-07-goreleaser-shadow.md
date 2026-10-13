# Review note: PR 7 - Shadow GoReleaser release on tags

- **Pull request**: [#12760](https://github.com/radius-project/radius/pull/12760)
- **Plan phase**: [PR 7](../2026-03-goreleaser-release-lifecycle-implementation-plan.md#pr-7-shadow-goreleaser-release-on-tags)
- **Stack index**: [README](./README.md)

## Verdict

The layer does what the plan asks and keeps production authority intact. The shadow job runs the canonical configuration for the real tag with the image registry redirected to `ghcr.io/radius-project/dev` and the GitHub Release disabled through templated defaults in `.goreleaser.yaml`, uploads the CLI binaries, sidecars, and metadata as workflow artifacts, and the three advisory jobs sit outside the build gate and the summary. The comparison is thorough: same-run CLI binaries byte for byte with their embedded build metadata and runtime version output, the production images locked by digest right after publication, per-platform runtime configuration and auxiliary manifests, and the Radius-owned payload of every image extracted with `docker export` and hashed file by file, with the base-image layers deliberately left to the static Dockerfile check because the two builds resolve them independently. The one known difference, the bare-hash sidecar content, is recorded in the report rather than silenced. The nine shadow tests and the digest-capture test pass, the payload tool builds and behaves on a synthetic archive, `goreleaser check` passes with the shadow environment, and shfmt and ShellCheck are clean.

Two things needed attention: a parity failure was hard to see and hard to explain, and one line of the workflow did not pass the Prettier check the lint workflow runs.

## Changes made in this review

### 1. A parity mismatch now shows what differed

- **What changed**: `assert_json_equal` in `verify-goreleaser-shadow.sh` prints the expected and actual JSON before failing, and the embedded build metadata is compared before the binary digests so that a byte difference comes with the metadata diff that explains it, or with an explicit statement that the metadata is identical.
- **Why**: the job exists to surface unexplained differences during an RC-plus-final cycle. "embedded build metadata for rad_linux_amd64 do not match" without the two documents costs a release cycle to diagnose.
- **Value**: a failing parity job is diagnosable from its log.
- **Impact**: output only; the test suite's expected messages are unchanged and all nine tests pass.

### 2. Advisory failures are reported in the run summary

- **What changed**: each of the three advisory jobs ends with an `if: failure()` step that writes what failed to the job summary, and the shadow job's message states that the production release is unaffected.
- **Why**: `continue-on-error` keeps the run green and reports the job as successful to `needs`, which the author already worked around with outputs for job chaining. For humans the same masking applied: the existing summary step only rendered when the report existed, which is only on success.
- **Value**: the plan's "fails loudly" requirement is met where release engineers look, without touching the production gate.
- **Impact**: none on success; three extra steps that only run on failure.

### 3. Action pins and formatting

- **What changed**: QEMU and Buildx are pinned to the v4.3.0 releases `main` uses, matching the lower layers, and the `needs` list of the parity job is wrapped the way Prettier requires.
- **Why**: the pins trailed `main` by one Dependabot bump, and the parity job's one-line `needs` list exceeds the configured print width, which fails the Prettier check the lint workflow runs.
- **Value**: no post-merge dependency churn; the lint check passes.
- **Impact**: none functionally.

## Findings left as-is

- **Baseline runtime contract on the production image**: `verify_image_baseline_contract` compares the current production image against the newest final-release baseline (v0.60.0). An intended runtime change between releases fails this advisory job even though GoReleaser parity holds, until the baseline is regenerated with the parity collector. Acceptable for an advisory job whose purpose is to make every difference explicit.
- **Transitional scope**: PR 13 removes the shadow comparison, its test, and the payload tool once the tag path cuts over, so nothing here is worth deeper refactoring.

## Verification

- `verify-goreleaser-shadow_test.sh` (9 tests) and `capture-release-image-digests_test.sh` pass; shfmt and ShellCheck are clean for the four scripts.
- `go vet` and `go build` pass for the payload manifest tool, and it reports the expected entries and binary hash for a synthetic exported image.
- actionlint and Prettier pass for `build-release.yaml` after the change; `goreleaser check` passes with the shadow registry and the release switch off.
