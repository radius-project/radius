# Review note: PR 17 - Helm chart immutable image tags

- **Pull request**: [#12953](https://github.com/radius-project/radius/pull/12953)
- **Plan phase**: [PR 17](../2026-03-goreleaser-release-lifecycle-implementation-plan.md#pr-17-helm-chart-immutable-image-tags)
- **Stack index**: [README](./README.md)

## Verdict

The layer does what the plan asks with little code. The chart helper returns the chart's app version unchanged, so final and patch charts pin every component image to the release version while RC and edge charts keep their behavior. The external Deployment Engine and dashboard images receive full-version tags before the chart is packaged, by retagging their verified digests with ORAS rather than rebuilding anything or moving a channel alias: the Deployment Engine tag is checked against the controller's lock, the dashboard tag against the plan's frozen source commit, and both against the expected platform set. The verification gate from PR 16 expects full-version references everywhere and rejects a stable chart that renders a channel tag. The release-note templates carry the patch-pickup notice, and the Helm client test drives a real Helm install and upgrade with the actual chart templates to show that defaults advance to the new version, explicit overrides survive, and a cleared override adopts the defaults while other stored values remain. Chart unit tests, the Go Helm client tests, and every release suite pass. The rollout prerequisites the pull request lists are real: the repository needs Actions write on the two GHCR packages, and the dashboard image carries no provenance label today.

Two things needed work: the pin step raced the dashboard publisher, and the dashboard-label prerequisite already gates the previous layer.

## Changes made in this review

### 1. The external-image pin waits for a publisher that is still running

- **What changed**: `pin-image` awaits the channel reference for a bounded time, ten minutes by default polled every thirty seconds, when the reference is absent or still serves another source, and reports the mismatch only when the wait runs out. An existing full-version tag is never awaited, because a mismatch there is a conflict. The helm job's timeout allows for the wait. A test covers a publisher that finishes during the wait and the immediate rejection of a differing immutable tag.
- **Why**: the controller creates the dashboard's sibling tag and the Radius tag within seconds of each other, and the dashboard's build workflow then publishes the channel image from its tag. In the last patch release the dashboard build took four minutes and the Radius image build twelve, so the pin would normally find the image ready, but dashboard builds have taken eleven minutes, and the pin failed closed the moment the channel image still carried the previous source. Resume Release would have recovered it, but as a routine failure rather than an exceptional one.
- **Value**: chart publication no longer depends on which of two parallel builds finishes first.
- **Impact**: a genuine source conflict on the channel image is reported after the wait instead of at once; the message is unchanged, and the conflict on an existing full-version tag is still immediate.

### 2. The dashboard provenance prerequisite is recorded where it first applies

- **What changed**: cross-layer finding 11 in the index, a sentence in the runbook and the plan that PR 16's manifest already requires the label, a rollout gate on PR 16's description, and the exact companion change named in the runbook: the dashboard's `Build Image` step and Dockerfile set no OCI labels, so the image build must add `org.opencontainers.image.revision` from the built commit.
- **Why**: PR 16's manifest compares that label on every dashboard platform with the plan's dashboard commit. The published `0.59`, `0.60`, and `latest` dashboard images carry no OCI labels at all, checked with oras, so the gate fails for every release until `radius-project/dashboard` labels its image. PR 17 stated the prerequisite only for the chart cutover.
- **Value**: the rollout order is visible from the stack itself: the dashboard change must land before PR 16 is enabled, not before PR 17.
- **Impact**: none on code; a companion change in the dashboard repository remains required.

### 3. Small items

- The helm workflow's Buildx pin matches the other workflows (v4.3.0).
- The gate contract test's new block is indented like the rest of its function.
- The runbook says the release-note notice is permanent. The templates and the preparation tests already treat it that way, while the runbook said to retain it only for the first release that ships the policy.

## Findings left as-is

- **Package access**: retagging with `GITHUB_TOKEN` requires the `dashboard` and `deployment-engine` packages to grant the repository write access; the runbook and the rollout gates say so, and no App scope is widened.
- **Single-manifest dashboard image**: the published dashboard image is a single `linux/amd64` manifest rather than an index; the parity targets expect exactly that, and the pin and the manifest handle both forms.
- **RC charts**: the publishers produce the RC tags themselves (`0.61.0-rc.1` for both images), so the pin verifies them without retagging.
- **Shell style**: `release-cutover_test.sh` matches neither shfmt profile, as before this layer.

## Verification

- Chart: helm unittest passes (136 tests, including the new final and patch pinning cases); `go test ./pkg/cli/helm` passes with the three real Helm upgrade cases.
- Shell: OCI artifacts (17, one added), cutover (9), parity, installation (6), preparation (17), and SBOM suites pass; ShellCheck is clean for the changed scripts.
- Node: manifest suite (5) passes. actionlint and Prettier pass for the helm workflow and the changed Node and chart test files; markdownlint and cspell pass for the chart README, the release-note templates, the runbook, the plan, and these notes.
- Facts checked outside the repository: the dashboard image labels and manifest form with oras; the `Build Image` step of the dashboard's `build.yaml`; build timings of the `v0.60.2` dashboard and Radius runs; the plan's `linux/amd64` platform expectation for the dashboard.
