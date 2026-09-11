# Review note: PR 16 - Publication gate, verification, and coordination

- **Pull request**: [#12948](https://github.com/radius-project/radius/pull/12948)
- **Plan phase**: [PR 16](../2026-03-goreleaser-release-lifecycle-implementation-plan.md#pr-16-publication-gate-verification-and-notifications)
- **Stack index**: [README](./README.md)

## Verdict

The layer completes the design's verify and finalize stages the way the design describes them. One verification job observes the staged draft with the parity collector, compares every output with the committed plan, the immutable digest locks, and the Deployment Engine lock, installs the staged `rad` binary and the downloaded chart on an isolated kind cluster with digest-pinned images, and retains the manifest as evidence. Finals and patches then wait on the `release` environment, the finalizer observes every output again and refuses to publish on any drift, and the approved manifest is attached to the release before an alias moves. RCs pass through the same gate without an approval wait, which is the decided policy, and the reconciliation-time approval from PR 15 is gone. Docs and samples coordination uses the run identifier that GitHub's REST API version `2026-03-10` returns from workflow dispatch, checked against the live API, and records it on deployment receipts; the controller finds the exact tag-build run and reruns its failed jobs. Stage summaries and the optional Teams card share the manifest, and a notification failure cannot change the gate. All suites pass.

Three parts of the layer do not match the systems it talks to, and each would have stopped the first release: the upmerge completion check, the visibility of the draft release to the verification job, and the permissions of the coordination token. They are fixed below, together with one state-store improvement and a few small items. Cross-layer finding 2 receives its final evaluation here.

## Changes made in this review

### 1. An upmerge is complete when its pull request has merged

- **What changed**: after a successful upmerge run, coordination reads the run's jobs. When the `Create pull request` step ran, it binds the single upmerge pull request into `edge` that was created while that step ran and stores the pull request as the receipt's environment URL. From then on only that pull request's merge state decides: open keeps the receipt in progress with a "merge, then resume" recovery, closed without merging fails it, and merged completes it. A run whose step was skipped had nothing to merge and completes without a pull request. If the binding is not unique, coordination stops, and the runbook tells the operator how to bind the pull request on the receipt by hand; that binding is honored on resume. Three tests cover the open, merged, skipped, ambiguous, and closed cases.
- **Why**: the layer completed an upmerge when the source branch head was reachable from `edge`. Both `docs` and `samples` allow only squash merges: their repository settings reject merge commits and rebase merges, and the last merged docs upmerge has a single parent. The merged upmerge therefore never contains the source commits, and the check could never pass. Every RC would have stayed at "merge the upmerge PR, then resume" forever, and no final could have been prepared.
- **Value**: the final-release gate that the design specifies closes, with the same "bind once, then resume the exact thing" discipline the layer applies to runs.
- **Impact**: the binding reads the pull-request step's start and end times from the jobs API and accepts exactly one candidate in that window; ambiguity fails closed rather than guessing. An exact binding without a window needs the docs and samples upmerge workflows to write their run URL into the pull request body; that one-line change downstream is worth making later, after which the binding only has to prefer the marker. Receipts exist only for RCs published with this layer, so the runbook now says to cut another RC when the last validated RC predates it.

### 2. The verification job can see the draft release

- **What changed**: `verify-release` runs with `contents: write`, and the cutover contract test asserts that value with the reason.
- **Why**: GitHub lists a draft release only for identities with push access, and `contents: read` is not push access, so the parity collector would have reported the staged draft as missing on every run. The job's steps cannot publish anything: publication and alias promotion stay in the finalizer behind the gate, and the job runs only the repository's code at the tagged commit.
- **Value**: the gate can observe the draft it verifies.
- **Impact**: the job holds the same repository permission as the finalizer, which already inspects the same draft. The summary jobs keep `contents: read`, so their "GitHub Release" line now says "not published" instead of "absent" while a draft they cannot see exists.

### 3. The coordination token can read what it verifies

- **What changed**: the coordination token requests `contents: read` and `pull_requests: read` next to `actions: write`; the runbook, the plan's token table, and the design say so.
- **Why**: an installation token holds only the permissions it requests. Verifying that a docs or samples release created the channel branch reads a git ref, and verifying an upmerge reads pull requests; with Actions write alone those reads are outside the token's grant.
- **Value**: the destination checks the layer performs after each run can run.
- **Impact**: none on the installation, which already grants Contents and Pull requests write to those repositories' own workflows; the token asks for less than the installation allows.

### 4. Receipts share one environment

- **What changed**: every coordination receipt is a deployment in the `release-coordination` environment with the task `release-coordination:<version>:<task>`, bound to the source SHA as before. Conflict detection is unchanged: a second receipt for the same version and task, or one at another source, still stops the run.
- **Why**: a deployment created through the API creates its environment. Naming the environment after the version and task would have added three environments per RC, three per final, and one per patch to the repository settings, none of them removed by anything, while the task filter offers the same conflict checks.
- **Value**: coordination state stays in one environment whose deployments are listed per version and task.
- **Impact**: the runbook asks that `release-coordination` stay free of protection rules, because a branch policy on it would reject deployments to a commit.

### 5. Small items

- The verification job's Buildx pin matches the rest of the workflow (v4.3.0).
- The composite action is formatted with the repository Prettier configuration.
- The SBOM contract test counts with `awk` instead of `grep | wc -l`, which ShellCheck flags.
- `upmerges` joins the dictionary; the runbook and the design use it.

## Findings left as-is

- **Cross-layer finding 2** (state stores): `release-manifest.json` consolidates the verification evidence into one immutable asset that binds expected and observed outputs to the plan and source. The eleven per-stage assets remain the staging idempotency store because they exist before a manifest can, and every staging rerun needs them. The `automation/release-state-<version>` branch remains the Deployment Engine lock's system of record because every verification, including re-verification after publication, reads the lock from it; pruning it would remove evidence that resume paths need. The finding is closed with this evaluation.
- **Finding the tag build**: after creating the Radius tag the controller waits 90 seconds for the tag-build run to appear and fails its last job when it does not, although the tag exists and the build proceeds; a rerun of the controller reconciles. This is noisy rather than wrong, and the runbook explains resume.
- **Asset downloads**: verification and the post-approval recheck each download all CLI assets and SBOMs from the draft, so the finalizer downloads them a second time. The cost is a few minutes of runner time per release; it buys the guarantee that the recheck observes the real draft.
- **Shell style**: the layer's new shell files follow the `.editorconfig` shfmt profile, while `release-cutover_test.sh` matches neither profile exactly; nothing in CI checks either, as noted at PR 15.

## Verification

- Node: coordination (11, three added), manifest (4), status (2), publication (10), dispatch and publication resume (5), assets (15), resolve (6), and lock (5) suites pass.
- Shell: cutover (9, including the updated gate contract), controller (40), SBOM contract, resume (5), controller plan (8), preparation (17), plan validation (13), parity, and staged installation (5) suites pass; ShellCheck is clean for the changed scripts.
- Prettier with the repository configuration passes for the workflows, the composite action, and the Node scripts; actionlint reports only the `queue` key it does not know; markdownlint and cspell pass for the runbook, the plan, the design, and these notes.
- GitHub facts were checked against the live API and documentation: the `2026-03-10` API version is accepted, the workflow dispatch endpoint documents a `workflow_run_id` response, `docs` and `samples` allow only squash merges and the latest merged docs upmerge has one parent, and the releases API documents that only push access lists drafts.
