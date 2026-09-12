# Review note: PR 15 - Release controller and trigger swap

- **Pull request**: [#12872](https://github.com/radius-project/radius/pull/12872)
- **Plan phase**: [PR 15](../2026-03-goreleaser-release-lifecycle-implementation-plan.md#pr-15-release-controller-and-trigger-swap)
- **Stack index**: [README](./README.md)

## Verdict

The layer is the design's release transaction, built with the discipline the design asks for. One reusable controller resolves exactly one merged generated release pull request, reads the plan committed under `.github/release-plans/`, binds it to the metadata-bearing commit, preflights every destination with the reconciliation script in check-only mode, verifies the signed Deployment Engine tag, waits for approval, publishes and verifies the Deployment Engine image and locks its digest, reconciles the sibling repositories at the commits frozen in the plan, and creates the Radius tag last with the release App so the tag starts the release build. Concurrency is keyed by version and source commit with a queue that never cancels an attempt, and every job checks out the executing workflow's commit, so an approval wait cannot change the code between stages. Approve Release and Resume Release are secretless gateways that dispatch the default-branch controller. The legacy `versions.yaml` trigger is deleted outright, and the generated metadata backport removes each release branch's copy before it can fire. Plan validation regenerates the plan from trusted code on pull requests and in the merge group, the controller rejects a conflicting version or source pair, and the failure-injection suite resumes after each stage without a duplicate dispatch or a moved tag. The image verifier's label expectations hold against the image published today: the `0.60` Deployment Engine image carries `org.opencontainers.image.ref` and `org.opencontainers.image.revision`. All suites pass, and ShellCheck, Prettier, markdownlint, and cspell are clean.

Two cross-layer findings land here and are fixed.

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

## Findings left as-is

- **Deployment Engine lock on a state branch** (cross-layer finding 2): the lock is written before any release exists, as a signed App commit on `automation/release-state-<version>`, and every resume verifies it. That is a sound durable store; its cost is one branch per release that nothing deletes. Evaluated at PR 16 together with the release manifest: the branch can be pruned once the release is published and the manifest carries the digest.
- **Approval before mutation**: the `approve` job gates every release, including candidates, on the `release` environment before the Deployment Engine publish. PR 16 moves the approval to publication and removes this job, so the double gate is transitional.
- **Which code runs**: a backport merge on a release branch runs that branch's copy of the controller, while the gateways dispatch the default branch; a channel created before this layer has no controller copy and starts through Approve Release, which the runbook explains.
- **Unlocked image inspection is allowed to fail**: the step continues on error so an invalid unlocked image is republished rather than trusted; a transient registry failure after five attempts takes the same path, which the publisher tolerates.
- **Shell style**: the layer converts most release scripts to the `shfmt -i 4 -ci` spacing, which is the style the older scripts use; nothing in CI checks either.

## Verification

- Shell: controller contract (39), resume failure injection (5), controller plan (8), sibling capture (3), Deployment Engine image (4) and tag (7), preparation (17), plan validation (13 with the two replacement tests), merge group (4), backport creation (4) and collection (3), reconciliation (19), version selection (8), cutover (8), and SBOM suites pass; ShellCheck is clean for the twenty-one new or changed scripts.
- Node: dispatch (3), lock (5), resolve (6), and monitor (22) suites pass.
- actionlint reports only the `queue` key it does not know yet; Prettier with the repository configuration passes for the workflows and the Node scripts; markdownlint and cspell pass for the runbook, the plan, and these notes.
- The Deployment Engine image labels were read from the published `0.60` image with oras. Fetching one branch from each live sibling repository into a scratch repository took five seconds in total and 16 MB of objects, well inside the five-minute validation jobs.
