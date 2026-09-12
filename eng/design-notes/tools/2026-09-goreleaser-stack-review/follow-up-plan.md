# Follow-up plan from the findings left as-is

This plan ranks the findings that the eighteen review notes left as-is and keeps only the ones worth an engineering change. Each item names the value, the files, the verification, and the effort. The last section lists what was skipped and why. Nothing here is required for the stack to work; the dashboard provenance label recorded as cross-layer finding 11 remains the one external prerequisite for rollout.

The first hardening bundle is implemented on `dp/release-snapshot-hardening`: item 3, item 6, and the CLI snapshot-isolation portion of item 5. The image scanner pin in item 5 remains deferred until after the first RC-plus-final cycle.

## Ranked items

### 1. Unify the Dockerfile pairs (cross-layer finding 7, PR 18 note)

- **Why**: five production images keep two Dockerfiles each, one for the Make path and one for GoReleaser, held equal by a parity guard. One file per image removes the duplication, the guard, and its test, and makes the developer build use the same definition as the release.
- **What to change**:
  - `deploy/images/{ucpd,applications-rp,dynamic-rp,controller,pre-upgrade}`: delete `Dockerfile`, rename `Dockerfile.goreleaser` to `Dockerfile`; update the five `dockerfile:` paths in `.goreleaser.yaml`.
  - `build/docker.mk`, the Go branch of `generateDockerTargets`: stage a GoReleaser-shaped context under `$(OUT_DIR)/context/<name>` (the binary at `linux/amd64/<name>`, and `deploy/manifest/built-in-providers/self-hosted/` at its repository path), then build that context with `--platform linux/amd64 --build-arg TARGETPLATFORM=linux/amd64`. Drop `copy-manifests` when nothing else uses it.
  - `test/testrp/Dockerfile` and `test/magpiego/Dockerfile`: switch to `ARG TARGETPLATFORM` and `COPY ${TARGETPLATFORM}/<name> /<name>` so every Go image shares one layout.
  - `.github/scripts/verify-goreleaser-snapshot.sh`: delete `verify_dockerfile_parity` and its cases in `verify-goreleaser-snapshot_test.sh`.
- **Verification**: `make -n docker-build-ucpd` for the staged commands; `make docker-build` and `make goreleaser-check goreleaser-snapshot` on a machine with Docker; the snapshot verifier suite; one noncloud functional test run in CI, which builds the test images through the same target.
- **Effort and risk**: about an hour plus a CI run; low risk once `TARGETPLATFORM` is passed explicitly, because the classic builder does not set it.

### 2. Decide how patch releases obtain their Deployment Engine tag (PR 12 note)

- **Why**: every release type now requires the signed tag `v<version>` in `azure-octo/deployment-engine`, and that repository has no tag for the last Radius patch. Each Radius patch therefore costs a maintainer a GPG-signed tag and a Deployment Engine image rebuild even when that component did not change. The chart pin from PR 17 already produces `deployment-engine:<patch>` by retagging the channel digest, so the rebuild adds nothing.
- **Recommendation**: keep the rule for RCs and finals, and let a patch plan reuse the channel's newest signed Deployment Engine tag that is not newer than the patch version when no exact tag exists. Record the choice in the plan so the manifest stays exact about what the release ships.
- **What to change**:
  - `prepare-release.sh`: resolve `deploymentEngine.signedTag` (the exact tag when present, otherwise the newest `v<channel>.N` tag not above the version) and write it into the plan; `validate-release-plan.sh` regenerates and compares it like every other field.
  - `verify-deployment-engine-tag.sh`, `validate-release-controller-plan.sh`, and the controller's `validate` and `publish-deployment-engine` jobs: read the tag from the plan instead of deriving it from the version; `reconcile-release-controller-lock.mjs` already stores `signedTag`.
  - `verify-release-manifest.mjs` and its test: compare the lock's `signedTag` with the plan field rather than with the version; the helm pin step in `__build-helm-chart.yaml` checks the same field.
  - Runbook and plan document: a patch that must ship a Deployment Engine change tags that repository first, and Prepare Release then selects the exact tag.
- **Verification**: preparation (17), plan validation (13), controller plan (8), controller contract (40), manifest (5), and helm-pin fixtures, plus one patch dry run through Prepare Release.
- **Effort and risk**: about a day; medium, because it touches the release identity chain. Needs a maintainer decision first.

### 3. Widen the tag-build discovery window in the controller (PR 16 note)

- **Why**: after creating the Radius tag, `resumeReleasePublication` waits 90 seconds for the build run to appear and fails the controller's last job otherwise, although the tag exists and the build starts. Under load, GitHub can take longer to schedule a `push` run, so a successful release would end with a red controller run and an "incomplete" Teams card.
- **What to change**: in `dispatch-release-controller.mjs`, raise the deadline to about five minutes with ten-second polls, raise the `create-radius-tag` job timeout in `__release-controller.yaml` to match, and make the timeout message say what to do: the tag exists, so start the build with `gh workflow run build-release.yaml --ref v<version>`, which the resume path already recognizes as a valid run event. Adjust the timing in `dispatch-release-controller_test.mjs`.
- **Verification**: dispatch suite (5) and controller contract (40).
- **Effort and risk**: an hour; low.

### 4. Make the release manifest self-contained and prune state branches (PR 13, 15, and 16 notes)

- **Why**: the Deployment Engine lock lives only on `automation/release-state-<version>`, one branch per release that nothing deletes, and every verification after publication reads it from there. Embedding the lock in `release-manifest.json` makes the published evidence complete and lets the branches be pruned.
- **What to change**:
  - Phase A: `verify-release-manifest.mjs` adds a `deploymentEngine` section holding the lock; `recheckPublication` compares it; `verify-release-publication.sh` reads the lock from the published manifest asset when the release is already published, and from the branch only while staging.
  - Phase B: a scheduled workflow deletes `automation/release-state-*` branches whose release is published with a manifest asset that carries the lock, keeping the newest few channels; one runbook sentence on where the lock lives after pruning.
  - Phase C, optional and larger: move the eleven per-stage lock assets off the public release page. The natural store is the same state branch, written with the contents API without a `sha`, which creates a file only when it does not exist and gives the create-if-absent semantics the locks need. This touches `release-assets.mjs`, every lock reader, the parity allowlist, and their tests; do it only after a full release cycle has run on the current design.
- **Verification**: manifest (5), publication (10), installation (6), parity, and cutover suites; a resumed published release reading the lock from the manifest.
- **Effort and risk**: half a day each for phases A and B, low risk; two to three days for phase C, medium risk.

### 5. Pin and isolate SBOM generation (PR 14 note)

- **Why**: every pull-request snapshot queries the Go module proxy through `--enrich golang` for seven binaries, so an outage there fails unrelated pull requests, and the image SBOMs come from whatever scanner the runner's BuildKit bundles rather than from the pinned Syft.
- **What to change**:
  - Route the CLI SBOM command through a small wrapper in `build/scripts` that adds `--enrich golang` only when `GORELEASER_SNAPSHOT` is not set, so releases keep the license data and pull requests stay offline.
  - Pin the image scanner: set `sbom: false` on the five `dockers_v2` entries and pass `--attest=type=sbom,generator=<pinned scanner image by digest>` through their `flags`, tracked with the other pinned tools; update the SBOM contract test, which asserts `sbom: true` today.
- **Verification**: a snapshot run with the proxy blocked; the SBOM contract suite; an RC draft showing attestations from the pinned generator.
- **Effort and risk**: two to three hours; low.

### 6. Stop re-uploading the snapshot images in pull-request builds (PR 18 note)

- **Why**: the pull-request image job downloads the snapshot images the snapshot job already retained for a day and uploads them again, plus Bicep, as `container-images-<version>`; nothing consumes the artifact, and the upload costs about a gigabyte and several minutes per pull request.
- **What to change**: in `__build-images.yaml`, upload only the Bicep image tar as `bicep-image-<version>`, and point people to `core-snapshot-images-<sha>` for the core images; update the sentence in the build guide that lists the artifacts.
- **Verification**: actionlint and the cutover contract test.
- **Effort and risk**: half an hour; none.

## Recorded outside those sections, also worth doing

- **Dashboard provenance label** (cross-layer finding 11): the companion change in `radius-project/dashboard` that adds `org.opencontainers.image.revision` to the image build must land before PR 16 is enabled; every release stops at the manifest gate until then.
- **Exact upmerge binding** (PR 16 note, change 1): a one-line change in the docs and samples upmerge workflows that writes the run URL into the pull request body lets `coordinate-release.mjs` bind the pull request by marker instead of by the run step's time window; the binding code then prefers the marker and keeps the window as the fallback.

## Suggested order

1. Items 3, 6, and the first half of item 5 together, as one small hardening change on top of the stack: half a day, no design decisions.
2. Item 2 as soon as maintainers decide the patch policy, before the first patch release under the new lifecycle.
3. Item 1 by whoever next has Docker at hand.
4. Item 4, phases A and B, and the scanner pin from item 5 after the first RC-plus-final cycle has run; phase C only if the release page clutter is judged worth the change.
5. The two cross-repository items whenever the docs, samples, and dashboard repositories can take them; the dashboard label first.

## Skipped as nitpicks or deliberate choices

- Shell style profiles, pre-existing shfmt and Prettier drift in untouched files, and actionlint's unknown `queue` key: nothing in CI enforces them, and each layer left neighboring lines alone on purpose.
- RC release notes rendered from the channel boundary: a documented behavior change that gives the fuller view for validating a candidate.
- The second asset download in the post-approval recheck: the price of observing the real draft, by design.
- Retry by dispatch instead of the rerun API, full-history run discovery, the `Capture release metadata` step, and the unit-test Node version: design-consistent or harmless.
- The one-time window before the first `edge` tags exist, and the alias index digest not matching `latest` byte for byte: nothing depends on either.
- Items that later layers removed (step-level `only_changed` conditions, the shadow comparison, `release-should-skip.sh`, the transitional double approval), the retained Python files, the pre-existing Zizmor findings, and the CLI matrix, which stays by maintainer decision.
