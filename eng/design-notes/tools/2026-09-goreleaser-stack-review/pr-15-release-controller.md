# Review note: PR 15 - Release controller and trigger swap

- **Pull request**: [#12872](https://github.com/radius-project/radius/pull/12872)
- **Plan phase**: [PR 15](../2026-03-goreleaser-release-lifecycle-implementation-plan.md#pr-15-release-controller-and-trigger-swap)
- **Stack index**: [README](./README.md)

## Verdict

The layer implements the design's release reconciliation stages. One reusable controller resolves exactly one merged generated release pull request, reads the plan committed under `.github/release-plans/`, binds it to the metadata-bearing commit, preflights every destination with the reconciliation script in check-only mode, verifies the signed Deployment Engine tag, waits for approval, publishes and verifies the Deployment Engine image and locks its digest, reconciles the sibling repositories at the commits frozen in the plan, and creates the Radius tag last with the release App so the tag starts the release build. Concurrency is keyed by version and source commit with up to 100 pending attempts and no cancellation of the active run. Every job checks out the executing workflow's commit, so an approval wait cannot change the code between stages; image verification also retains the plan's original output contract across resume invocations. Approve Release and Resume Release are secretless gateways that dispatch the default-branch controller. The legacy `versions.yaml` trigger is deleted outright, and the generated metadata backport removes each release branch's copy before it can fire. Plan validation regenerates the plan from trusted code on pull requests and in the merge group, and the controller rejects a conflicting version or source pair. Stage-boundary failure injection exercises idempotent tag reconciliation; separate monitor tests cover delayed visibility after an accepted dispatch. These checks do not prove exactly-once publication across interrupted invocations or replace the live App-tag-trigger and release-cycle soak gates.

The original cross-layer findings and follow-up correctness fixes are recorded below.

## Changes made in this review

### 1. Both signed-tag verifications can read the Deployment Engine repository

- **What changed**: the `validate` and `publish-deployment-engine` jobs mint a publisher App token for `azure-octo/deployment-engine` with Contents read and verify the signed tag with it instead of `GITHUB_TOKEN`. The plan's PR 15 text names the exception to the "validation runs on `GITHUB_TOKEN`" rule.
- **Why**: the repository is private and belongs to another organization, so `GITHUB_TOKEN` receives a 404 for every tag, which the script until PR 12 reported as a missing tag. With the PR 12 script the controller would instead stop at validation with "the token cannot read azure-octo/deployment-engine" on every run. Either way no release could pass the first job (cross-layer finding 10).
- **Value**: the controller's hard block on the signed tag works as the design intends.
- **Impact**: the publisher App installation on `azure-octo` must include `deployment-engine`, which PR 12 already requires and the runbook lists.

### 2. Frozen sibling commits are authoritative

- **What changed**: plan validation no longer requires the plan's sibling commits to equal the live branch heads. It checks that each frozen commit is still reachable from the branch it was captured on, by fetching that branch into a scratch repository, and regenerates the plan from the plan's own sibling entries. Two tests replace the drift test: a sibling `main` that advances after planning passes, and a planned commit that no branch contains fails. The runbook says when a rerun of Prepare Release is needed. This closes cross-layer finding 3.
- **Why**: the design records the required repositories in the release plan precisely so that a later run does not recompute them from mutable branch heads. The equality check did the opposite: any unrelated merge to `recipes`, `dashboard`, or `bicep-types-aws` while the release pull request was open failed the pull request check and the merge group, and forced a regenerated plan that then raced the next sibling merge. The reachability check keeps the guarantee that matters, which is that the controller can create the sibling release branch and tag at the frozen commit.
- **Value**: a release pull request stays mergeable through ordinary sibling activity, and the reviewed plan is what gets released.
- **Impact**: a sibling change merged after preparation is included only by rerunning Prepare Release, which is now the documented way to say it belongs in the release. The validator fetches one branch from each sibling repository per run; measured cost is recorded below.

### 3. Existing-channel validation and frozen output verification

- **What changed**: the controller passes the channel directly to `jq` rather than setting it only for the upstream `yq` process. The resolved plan artifact now carries its `expectedOutputs` as `release-targets.json`; both image-verification jobs download that exact artifact by ID and pass it to the verifier. Missing or invalid Deployment Engine platform contracts fail explicitly.
- **Why**: valid sibling refs such as `release/0.61` were rejected when `jq` could not see `CHANNEL`. Separately, using the controller checkout's `targets.json` changed the meaning of an approved plan when a resume ran newer tooling.
- **Coverage**: real-Git fixtures cover subsequent RCs, finals, patches, wrong-channel rejection, and output-contract extraction. Image-verifier fixtures change controller defaults between attempts while preserving the approved targets and locked digest.

### 4. Ambiguous dispatches fail closed

- **What changed**: after a server error or lost dispatch response, the monitor waits for the correlated run until the existing deadline without issuing another dispatch. Explicit rate-limit rejections retain bounded retries. An unknown outcome is propagated to the controller summary with inspection instructions instead of an immediate resume command.
- **Why**: a dispatch can be accepted before its run becomes visible. Retrying after one empty lookup duplicated accepted work. The stage-resume fixture's local completion marker did not exercise that boundary.
- **Coverage**: the actual monitor module is exercised with delayed visibility, lost responses, network timeouts, failed discovery, and deadline expiry. Full protection across interrupted invocations still needs publisher-side idempotency.

### 5. Release lookup is scoped to the known branch

The resolver filters closed pull requests by the exact `owner:automation/prepare-release-<version>` head instead of enumerating every closed `main` pull request. Repository identity, merged state, branch matching, and ambiguity checks remain in place.

## Remaining boundaries and follow-ups

- **Deployment Engine lock on a state branch** (cross-layer finding 2): the lock is written before any release exists, as a signed App commit on `automation/release-state-<version>`, and every resume verifies it. That is a sound durable store; its cost is one branch per release that nothing deletes. Evaluated at PR 16 together with the release manifest: the branch can be pruned once the release is published and the manifest carries the digest.
- **Approval before mutation**: the `approve` job gates every release, including candidates, on the `release` environment before the Deployment Engine publish. PR 16 moves the approval to publication and removes this job, so the double gate is transitional.
- **Which code runs**: since [December 8, 2025](https://github.blog/changelog/2025-11-07-actions-pull_request_target-and-environment-branch-protections-changes/), `pull_request_target` uses the default branch's workflow and execution reference, including backport merges into older release branches. Those branches need no local controller copy. Their legacy push-triggered `release.yaml` still needs removal.
- **Native publisher run IDs**: a coordinated publisher change can replace new-run discovery with `workflow_dispatch` and its [returned run ID](https://github.blog/changelog/2026-02-19-workflow-dispatch-api-now-returns-run-ids/). It requires a matching publisher trigger and App `actions: write`; it does not remove the need for idempotency when responses are lost.
- **Image provenance**: revision and ref labels are consistency checks against the signed tag, not authenticated build provenance. Digest-bound build attestations require a coordinated publisher change before the controller can enforce them.
- **Unlocked image inspection is allowed to fail**: the step continues on error so an invalid unlocked image is republished rather than trusted; a transient registry failure after five attempts takes the same path, which the publisher tolerates.
- **Shell style**: the layer converts most release scripts to the `shfmt -i 4 -ci` spacing, which is the style the older scripts use; nothing in CI checks either.

## Verification

- Run `make test-release-controller test-monitor-remote-workflow` for the controller, source binding, output contract, resolver, lock, and dispatch regressions. These fixtures do not dispatch a live release.
- Older actionlint versions reject `queue`; [GitHub supports `queue: max`](https://github.blog/changelog/2026-05-07-github-actions-concurrency-groups-now-allow-larger-queues/) with cancellation disabled. Other workflow validation errors must still be fixed.
- The live App-created tag triggering `build-release.yaml` and the required release-cycle soak remain rollout gates; local fixtures cannot establish them.
