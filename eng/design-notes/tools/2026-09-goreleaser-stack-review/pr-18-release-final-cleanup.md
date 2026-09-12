# Review note: PR 18 - Final sweep, last consumers, and hardening

- **Pull request**: [#12956](https://github.com/radius-project/radius/pull/12956)
- **Plan phase**: [PR 18](../2026-03-goreleaser-release-lifecycle-implementation-plan.md#pr-18-final-sweep-last-consumers-and-hardening)
- **Stack index**: [README](./README.md)

## Verdict

The layer finishes the migration as the plan describes it. One reusable snapshot job builds the GoReleaser snapshot once per pull request, merge-queue entry, or push to `main`, and exports the CLI binaries and the native per-platform images as artifacts. The CLI and image workflows become read-only consumers of that snapshot; only two main-only jobs write to the registry, each guarded by a check that the run's commit is still the head of `main`, and main builds queue instead of cancelling one another. Edge images are assembled from the snapshot's per-platform images with Buildx from resolved digests, so nothing is compiled twice. The Python and shell version parsers are replaced by one function in the release-version helper, with a table test over tags, RCs, pull requests, `main`, and merge-queue refs. Test images leave the release outputs, keep their historical parity baselines, and receive attempt-scoped tags in the functional workflows, with the ACI fixture taking its image as a parameter. The obsolete workflows, scripts, and multi-architecture Make generators are deleted, and the runbook is reduced to approval, monitoring, resume, and fail-closed recovery. All twelve Node suites, all twenty-seven shell suites, the chart tests, and the documentation linters pass at the top of the stack.

Two pending cross-layer items are evaluated here: the seven-runner CLI matrices stay by maintainer preference, and the Dockerfile pairs stay with a recipe. One publication detail is changed: edge publication no longer leaves a set of snapshot-version tags in the production packages on every merge.

## Changes made in this review

### 1. Edge publication leaves no snapshot-version tags behind

- **What changed**: `goreleaser-snapshot-artifacts.sh push-edge` tags each per-platform snapshot image with a moving `edge-<os>-<arch>` tag before pushing it, resolves that tag's digest, and assembles the `edge` index from the digests. The test asserts the moving tags and rejects a snapshot-version push; the runbook and the build guide describe the tags.
- **Why**: the layer pushed the per-platform images under their snapshot names, such as `ucpd:0.0.0-snapshot-<sha>-linux-amd64`, to make them addressable for `imagetools create`. GHCR never garbage-collects tags, so every merge to `main` would have added fifteen tags to the five production packages, visible to everyone browsing them, and no API removes a tag without deleting the manifest that the `edge` index needs. Moving tags carry the same manifests under a constant name set.
- **Value**: the production packages keep `edge`, the release versions, and the channels as their tag set, plus three stable per-platform aliases.
- **Impact**: consumers may pull `edge-linux-arm64` directly; older platform manifests become untagged versions, as the previous multi-architecture push left them.

### 2. Small items

- The snapshot script rejects a tag other than `edge` with a message instead of exiting silently, and both new shell files end with a newline.

## Findings left as-is

- **Cross-layer finding 6** (seven-runner CLI matrix): kept. The platform list is one YAML anchor shared by the export and edge-publication jobs, so a new platform or a new publishing style is one list edit, which the maintainer values over runner count. The cost is fourteen runner starts per build, each downloading the whole snapshot to move one file; a single-runner variant with hard-coded upload steps was prepared during the review and withdrawn on that preference.
- **Cross-layer finding 7** (Dockerfile pairs): the pairs stay. The recipe for unifying them is to stage a GoReleaser-shaped context in `docker-build-<name>` (the binary under `<platform>/` and the built-in provider manifests under their repository path), build it from `Dockerfile.goreleaser` renamed to `Dockerfile` with `TARGETPLATFORM` passed as a build argument, convert the two test-image Dockerfiles to the same layout, and delete the parity guard in `verify-goreleaser-snapshot.sh`. It is about an hour for someone who can run `make docker-build` and the noncloud functional tests; this review environment has no Docker, so it is not attempted here.
- **Pull-request image artifact**: the pull-request image job re-uploads the snapshot images it downloaded, plus the Bicep image, as `container-images-<version>`, while the snapshot job already retains the same images for a day. Nothing in the repository consumes the artifact; it exists for people, as before this layer. Dropping it would save about a gigabyte of artifact traffic per pull request.
- **`only_changed`**: every caller workflow still consumes it; only the CLI workflow's input was dead, and the layer removed it.
- **Python**: `.python-version` and the SemVer validator stay because the Copilot setup workflow and the test-result transformer still use Python; no release workflow does.
- **Zizmor**: the four pre-existing high-severity findings in the cloud test workflow's trigger handling predate the stack.
- **Shell style**: the two new scripts follow the older shfmt profile; nothing in CI checks either.

## Verification

- Node: all twelve suites pass (coordination 11, dispatch 5, draft 4, monitor 22, publication 10, lock 5, assets 15, status 2, resolve 6, backports 5, commit titles 13, manifest 5).
- Shell: all twenty-seven suites pass, including the snapshot artifact suite with the moving-tag assertions, cutover (11), controller (40), OCI artifacts (17), and installation (6); ShellCheck is clean for the changed scripts.
- Chart: helm unittest passes (136). actionlint and Prettier pass for the changed workflows; markdownlint and cspell pass for the runbook, the build guide, the skills, the plan, the design, and these notes.
- Facts checked outside the repository: GHCR retains tags until their package version is deleted, and deleting a version removes the manifest, so the only tag-free option is to never create the tag.
