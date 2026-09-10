# Review note: PR 2 - GoReleaser configuration and snapshot validation

- **Pull request**: [#12736](https://github.com/radius-project/radius/pull/12736)
- **Plan phase**: [PR 2](../2026-03-goreleaser-release-lifecycle-implementation-plan.md#pr-2-goreleaseryaml-with-check-and-snapshot-ci)
- **Stack index**: [README](./README.md)

## Verdict

The configuration meets the PR 2 exit criteria and the design constraints checked against the GoReleaser source and documentation (v2.17.1, plus the v2.18 release notes for the bump in change 7): six builds with the same linker flags, `CGO_ENABLED=0`, and `GOARM=7` as the Make build (the v0.60.0 baseline shows no `-trimpath` or `-gcflags`, and GoReleaser adds neither by default), raw `rad_<os>_<arch>` assets for the seven CLI targets, split SHA-256 sidecars, `dockers_v2` images that declare `linux/arm/v7` explicitly, draft-release settings with `use_existing_draft` and `replace_existing_artifacts`, and the explicit retry budget from the design. The five `Dockerfile.goreleaser` files keep the base image, packages, user, working directory, exposed port, and entrypoint of their production counterparts, and the `ucpd` image receives the same `self-hosted` manifests the Make build copies. The pinned installer checksums match the upstream `checksums.txt` for v2.17.1. The snapshot job on the pull request produced 29 binaries, 7 sidecars, and 15 per-platform images in 22 minutes.

The changes below make the verifier actionable and testable, guard the duplicated Dockerfiles from the layer that introduces them, move the pin to the current GoReleaser release, and trim the transitional workflow. No configuration change was needed.

## Changes made in this review

### 1. `verify-goreleaser-snapshot.sh` checks the images GoReleaser built

- **What changed**: the verifier only inspected the `dockers_v2` configuration; it now also reads the `Docker Image` entries of `artifacts.json` and requires, for every production image, exactly the platforms in `targets.json` and a name under the configured repository (`GORELEASER_IMAGE_REGISTRY`, defaulting to `ghcr.io/radius-project`). A snapshot built with `--skip=docker` or `--skip=publish` has no images, so `make goreleaser-snapshot` passes `--skip-images` in that case and the verifier says so instead of failing.
- **Why**: the plan asks the snapshot job to diff snapshot output, not only configuration, against the parity manifest for image definitions. The configuration check cannot see a build that quietly produced fewer platforms, and GoReleaser's snapshot mode writes one image per platform, so the platforms are visible in the metadata.
- **Value**: a missing platform or a wrong repository fails the pull request instead of the first tagged release.
- **Impact**: none on the existing pull request run, which already builds all fifteen images. Local runs that skip Docker keep working through the Make switch.

### 2. Verifier failures show the expected and actual values

- **What changed**: `assert_json_equal` prints both JSON documents before failing.
- **Why**: "CLI asset names do not match the parity contract" gives a reviewer nothing to act on; the two lists do.
- **Value**: a failing snapshot job is diagnosable from the log.
- **Impact**: output only.

### 3. Verifier runs without GNU `sha256sum` and accepts absolute artifact paths

- **What changed**: the hard requirement on `sha256sum` became the same `sha256sum`, `shasum`, `openssl` fallback the parity collector uses, and artifact paths that are already absolute are used as-is instead of being prefixed with the repository root. The configuration path can be overridden with `GORELEASER_CONFIG_FILE`, matching the switch the later layers add.
- **Why**: the installer supports macOS, so `make goreleaser-snapshot GORELEASER_ARGS=--skip=docker` is the documented local loop there, and it failed at the verification step. Absolute paths are what a test fixture in a temporary directory produces.
- **Value**: the local loop works on every platform the installer supports, and the verifier can be tested without writing into the repository tree.
- **Impact**: none for CI, where GoReleaser writes paths relative to the repository root and `sha256sum` exists.

### 4. Deterministic tests for the verifier

- **What changed**: `verify-goreleaser-snapshot_test.sh` builds the metadata GoReleaser writes for a snapshot (a binary and a bare-hash sidecar per CLI asset, one loaded image per production platform), runs the verifier against the committed `.goreleaser.yaml` and `targets.json`, and then checks that a missing asset, a wrong sidecar, an extra build target, a missing image platform, a snapshot without images, two configuration drifts, and three Dockerfile parity breaks (a changed user, a changed base image, a missing file, each in a staged copy of the repository) each fail. `make test` runs it through `test-verify-goreleaser-snapshot`, next to the parity collector test.
- **Why**: PR 1 set the standard of a fixture test per release script. The verifier is the gate that decides whether the GoReleaser output still matches the release contract, and it had no test, so a regression in its `jq` or `yq` logic would only surface as a green job that checks nothing.
- **Value**: the verifier's negative paths are exercised on every `make test`, and the committed configuration is checked against the contract without GoReleaser or Docker.
- **Impact**: about six seconds added to `make test`; `jq` and `yq` are already required by the parity collector test.

### 5. Snapshot workflow skips docs-only pull requests and pins the current actions

- **What changed**: `goreleaser-snapshot.yaml` gained the `changes` job that `build.yaml` already uses, so the snapshot job is skipped when only documentation-class files changed; `push`, `merge_group`, and manual runs are never filtered. The QEMU and Buildx actions are pinned to the v4.3.0 releases that `main` uses.
- **Why**: the snapshot run takes about 22 minutes, and this workflow runs on every pull request until PR 18 folds it into the split build workflows. The action pins were one Dependabot bump behind `main`, which would have produced a bump pull request immediately after merge.
- **Value**: fewer runner minutes on documentation pull requests, and no post-merge dependency churn.
- **Impact**: none on the checks a code change receives.

### 6. Dockerfile parity guard moved down from PR 13

- **What changed**: the verifier now runs the static parity check that PR 13 introduces, copied verbatim (`normalize_dockerfile` and `verify_dockerfile_parity`): every production image's `Dockerfile.goreleaser` must carry the same `FROM`, `RUN`, `ENV`, `USER`, `WORKDIR`, `EXPOSE`, `ENTRYPOINT`, and `CMD` directives as its `Dockerfile`, compared on meaning rather than formatting. The test covers a changed user, a changed base image, and a missing file.
- **Why**: this pull request introduces the duplicate Dockerfiles, so the guard belongs here rather than eleven layers up. Folding each pair into one Dockerfile was evaluated and rejected for this layer: the `Dockerfile` copies keep producing the production images through the Make path until PR 13 cuts releases over, and changing the Make build context or those Dockerfiles here would make PR 2 non-inert, contrary to its rollback story. PR 18 rewrites the Make image targets anyway, so one Dockerfile per image is decided there.
- **Value**: drift between the pairs fails the snapshot job from the first merge, and the rebase of PR 13 carries the identical functions, so nothing is duplicated in the end state.
- **Impact**: none on the current pairs, which match.

### 7. GoReleaser pinned to v2.18.1

- **What changed**: `build/tools.yaml` pins v2.18.1 with the four upstream checksums, and `build/tools.generated.mk` was regenerated with the tool updater.
- **Why**: v2.18.1 is the current release, and the v2.18 notes change nothing the configuration relies on: `retry` durations are typed as strings in the schema, which the configuration already uses, plus `dockers_v2` annotation-scope and Python wheel fixes.
- **Value**: the stack lands on the current release instead of opening with a pending tool-update pull request.
- **Impact**: none on the configuration; `goreleaser check` passes with v2.18.1 without warnings.

### 8. Pull request description

- **What changed**: the body names v2.18.1, lists `make test-verify-goreleaser-snapshot` under "How to test", describes what the verifier now covers, and the file change summary includes the verifier test, `build/test.mk`, the workflow gate, and this note.
- **Why**: the description is what reviewers read first, so it should match the branch.
- **Value**: accuracy only.
- **Impact**: description only; no code change.

## Findings left as-is

- **Image SBOM attestations off**: `sbom: false` preserves parity with the current images, which carry no attestations; PR 14 turns them on together with the CLI SBOMs.

## Verification

- `bash .github/scripts/verify-goreleaser-snapshot_test.sh` passes in about six seconds (positive case, `--skip-images`, missing asset, wrong sidecar, extra target, missing platform, missing images, two configuration drifts, three Dockerfile parity breaks).
- `go run ./cmd/tool-updater generate-make` regenerated the include from the manifest and `go test ./internal/tooling/...` passes; the repository installer downloads and verifies v2.18.1 against the pinned checksum, and `goreleaser check` validates the configuration with v2.18.1 without warnings.
- The build-matrix and built-image checks pass against the `artifacts.json` uploaded by the pull request's own snapshot run (seven `rad` targets, fifteen per-platform images), and the same metadata fails the repository check when `GORELEASER_IMAGE_REGISTRY` names another registry.
- `shfmt -i 4 -ci -d` reports no differences; ShellCheck passes for both scripts; Prettier passes for the workflow; `make -n goreleaser-snapshot` composes `--skip-images` only when `GORELEASER_ARGS` skips Docker or publishing.
