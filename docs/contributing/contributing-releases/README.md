# Create and monitor a Radius release

## Purpose

Release maintainers approve an immutable release plan, monitor its publication, and resume failed stages without changing the approved version or source. The same procedure covers RC, final, and patch releases across `radius-project/resource-types-contrib`, `radius-project/radius`, `radius-project/docs`, `radius-project/samples`, and `azure-octo/deployment-engine`. Automation creates Radius and sibling tags, stages artifacts, verifies installation, publishes the release, and coordinates docs and samples.

## Prerequisites

- Choose `rc`, `final`, or `patch` and its `X.Y` channel. Prepare Release computes the version; new RCs use `-rc.N`, starting at `-rc.1`, and the historical `-rcN` form is never mixed into the same version, because SemVer orders every dotted candidate before every legacy one and `rad upgrade` would treat that move as a downgrade. A final uses the last validated RC's product code. `versions.yaml` keeps the previous stable release supported until the final release moves it to `deprecated`. Label selected fixes `backport release/X.Y` and **rebase-merge** their generated backports before preparation.
- Every version listed under `supported` in `versions.yaml` must have a Radius Git tag before preparing another release. Complete any pending final or patch metadata backport first. Open release plans enforce the same prerequisite.
- Have maintainer access to the organization repositories, including `resource-types-contrib`, and permission to approve the `release` environment. In the repository rulesets, require `action-semantic-pull-request` and `Validate release plan` on `main`, and `Validate release branch commits` with up-to-date branches on `release/*`, in addition to the existing required checks. The title policy uses the `action-semantic-pull-request` commit status, not the green `Validate pull request title` job, because a `[WIP]` title leaves the status pending. Merging the workflows does not configure these rulesets. Ordinary `main` PRs use squash merge; backports use rebase merge. Create branches directly in the organization repositories, not personal forks.
- For the `release/*` status-check rule, enable **Do not require status checks on creation** (`do_not_enforce_on_create`). The controller creates a new channel from an approved, checked `main` commit that has not yet been the subject of a release-branch PR. Requiring the release-branch-only check on that initial creation would block the first RC. Existing release branches still require their checks and an up-to-date base.
- Protect the `release` environment with required reviewers. The release controller refuses to start unless it requires reviewers and pauses there before any mutation; finals and patches wait there again after verification. Enable **Prevent self-review** if the person who merges a release PR must not also approve it, and restrict deployments to `main`, where the controller runs, and to `v*` release tags. Recommended: add tag rulesets for `v*` in `radius`, `recipes`, `dashboard`, and `bicep-types-aws`, and a branch ruleset for `automation/release-state-*` in `radius`, that block updates and deletion by anyone except the release App. The controller never moves a tag and rejects a Deployment Engine lock that the release App did not write, but only rulesets stop someone else from moving a tag or deleting the lock.
- Release from a branch on the current release pipeline. A tag runs the workflows in the tagged commit, not those on `main`. Prepare Release and the controller require draft-only GoReleaser publication and a `build-release.yaml` that runs on `v*` tags with the complete staging, verification, approval, finalization, and coordination job graph. The check validates the dependency graph, final/RC approval conditions, mandatory verification and recheck steps, and their ordering before alias promotion and publication. It also rejects another workflow with a tag filter, such as the legacy `build.yaml`. A branch cut earlier, such as `release/0.61`, must first receive the complete current release pipeline, including the scripts and Make targets it calls, through a reviewed pull request to `release/<major>.<minor>`. A passing source check does not replace the live release-cycle verification below.
- Release the required resource type namespaces and recipe packs and merge their generated Radius update before preparation, as described in [Release resource type namespaces and recipe packs](#release-resource-type-namespaces-and-recipe-packs).
- Configure the release App identities, including `RADIUS_RELEASE_BOT` Actions write, Contents read, Pull requests read, and Metadata read on `docs` and `samples`. Give the Radius repository Actions package-write access to the GHCR `dashboard` and `deployment-engine` packages for gated channel promotion. Helm publication waits up to ten minutes for a missing full-version image whose publisher is still building the planned source; an existing full-version tag with conflicting provenance fails immediately. Extend the publisher App (`RADIUS_PUBLISHER_BOT`) installation on `azure-octo` to `deployment-engine` with Contents read and to `radius-publisher` with Actions read and Contents write for correlated staging and promotion dispatches. Prepare Release and the controller verify the signed Deployment Engine tag with that token because the repository is private. Coordination records same-repository deployment receipts with `GITHUB_TOKEN` and `deployments: write` in the `release-coordination` environment, which must stay free of protection rules.
- Require the dashboard publisher to stamp `org.opencontainers.image.revision` with the exact built commit. Verify the actual image build reached by `yarn run build-image`, not just the workflow's source checkout. The publisher change must be included in the dashboard commit frozen by preparation, including on an existing `release/X.Y` branch. Both external-image pinning and the release manifest require the label. Missing provenance, conflicting digests, or incomplete platforms must not be waived.
- Deploy the coordinated publisher changes before using this Radius pipeline. `radius-publisher` must support separate Deployment Engine source/destination tags, immutable source-annotated Radius and AWS Bicep types, and `promote-bicep-types.yml`. Dashboard release builds must stage full-version tags without updating channels. The Deployment Engine sender must use its full-version `ghcr-tag` for both mirroring and functional tests while retaining the ACR `container-tag` as `source_tag`. Merge and deploy `radius-publisher` first, then this Radius change and the dashboard change in either order, and the Deployment Engine sender last; each step sends inputs that only the previous one accepts. The current Deployment Engine sender also rejects dotted RC tags such as `v0.62.0-rc.1`. Create no Radius or Deployment Engine release tag until all four changes are deployed, and do not mix old channel-writing publishers with the new finalizer.
- Create the matching **GPG-signed Deployment Engine tag** in `azure-octo/deployment-engine` before preparation. Prepare Release reports the exact required tag if it is missing; the controller also blocks before mutation. In a clean Deployment Engine clone, verify the intended source and run `git tag -s <version> -m "release tag <version>"`, then `git push origin <version>`. This remains manual until [deployment-engine#456](https://github.com/azure-octo/deployment-engine/issues/456) is resolved. Never manually tag the Radius repositories.
- Use the Teams release thread for approvals and exceptional recovery. Set the optional `RELEASE_TEAMS_WEBHOOK` secret to an HTTPS Adaptive Card endpoint for automated updates. GitHub summaries remain authoritative when notifications are unavailable.

### Confirm readiness before the first production cycle

Check the live repository settings and publisher contracts, not just the merged workflow files:

1. Confirm the required reviewers, branch/tag deployment restrictions, required check contexts, App installation permissions, and GHCR package access listed above. The `release-coordination` environment must remain unprotected; it stores receipts rather than publication approvals.
2. Confirm the publisher workflows accept `release_identifier` and expose the exact run names `deployment-engine / <identifier>`, `bicep-types / <identifier>`, `bicep-types-aws / <identifier>`, and `bicep-types.promote / <identifier>`. Confirm the frozen dashboard source produces its required revision label. Successful edge builds do not exercise signed release tags, stable alias promotion, or the release approval gates.
3. Record an approved RC-to-final cycle on the complete pipeline, including the staged installation, all three RC coordination receipts, and a controlled failure/resume exercise. Verify that resuming preserves locked digests and that a digest mismatch blocks publication. Run any destructive failure injection only against disposable test artifacts, never existing releases. Separately verify an upgrade from the previous supported release; the automatic installation gate only exercises a same-version Helm upgrade.

Do not declare production readiness from a green snapshot or an earlier pipeline's tag build. Keep the exact workflow run URLs and verification artifacts with the release record.

## Steps

### Release resource type namespaces and recipe packs

Complete this step before starting Radius release preparation so the approved product commit includes the intended resource types.

1. In `radius-project/resource-types-contrib`, run the [Release Namespace](https://github.com/radius-project/resource-types-contrib/actions/workflows/release-namespace.yaml) workflow from `main` for each namespace consumed by Radius under `resourceTypes` in [`deploy/manifest/defaults.yaml`](../../../deploy/manifest/defaults.yaml). Select the namespace and the appropriate semantic version `bump` (`patch`, `minor`, or `major`). Leave `prerelease_label` empty and `dry_run` and `force` disabled. Namespace versions are independent of the Radius version; even for a Radius RC, publish stable namespace releases because prereleases do not notify Radius.
2. In the same repository, run [Release Recipe Pack](https://github.com/radius-project/resource-types-contrib/actions/workflows/release-recipe-pack.yaml) from `main` for each pack under `recipePacks` in [`deploy/manifest/defaults.yaml`](../../../deploy/manifest/defaults.yaml). Select the pack and appropriate `bump`; leave `prerelease_label` empty and `dry_run` and `force` disabled. Pack versions are independent of Radius. Unchanged packs are skipped, and a pack release fails if a referenced recipe image cannot be pulled. Release the `kubernetes` pack whenever its recipes change: `rad` creates the default recipe pack from the pinned release, so an unreleased change does not reach Radius.
3. Wait for each namespace and recipe-pack workflow to succeed. Unchanged namespaces and packs do not need new releases. For each published release, confirm that [Notify Radius](https://github.com/radius-project/resource-types-contrib/actions/workflows/notify-radius.yaml) succeeds and triggers Radius's [Update Resource Types](https://github.com/radius-project/radius/actions/workflows/update-resource-types.yaml) workflow.
4. Review the generated `chore(resource-types-contrib): updates` PR from `bot/update-resource-types` in `radius-project/radius`. Successive notifications refresh the same open PR. Confirm that `deploy/manifest/defaults.yaml` pins the intended stable namespace and recipe-pack releases and that the copied manifests and generated artifacts are included. If the PR changes the `kubernetes` recipe-pack pin, it should also update the compiled pack (`deploy/manifest/recipe-packs/kubernetes/default.json`) unless the new release left the pack's Bicep unchanged. Wait for required checks and approval, then merge the PR into `main` before preparation. If all namespaces and packs are unchanged and Radius already has the intended pins, no new PR is required.
5. Record the namespace and recipe-pack releases and merged update PR in the Teams release thread. A first RC inherits this update from `main`. For a subsequent RC or a patch with resource-type or recipe-pack changes, label the update `backport release/X.Y` and rebase-merge its generated backport into `release/X.Y` before preparing the release.

Stop if the update is missing or incomplete. Publishing namespaces and recipe packs alone is not enough: their generated update must be merged into the code that Radius will release.

### Approve the plan

1. Run [Prepare Release](https://github.com/radius-project/radius/actions/workflows/prepare-release.yaml) from `main` with `release-type`, `channel`, and optional comma-separated `backport-pr-numbers`.
2. Review the generated draft PR's version, product commit, frozen sibling commits, selected backports, expected outputs, and schema-v2 plan under `.github/release-plans/`. Curate only Highlights and Upgrading in RC or final notes; patch notes are generated completely. Retain the warning that chart-default images are pinned to the full version: pod restarts no longer pick up later patches, and explicit image overrides remain respected.
3. If the base advances, rerun Prepare Release with the same inputs. A frozen sibling head that advances does not invalidate the plan; rerun only when the later sibling change belongs in the release. It regenerates the plan and generated prose while preserving the allowed curated sections. If preparation reports a pending backport, merge it and rerun; do not omit the fix to bypass validation.
4. Mark the release PR ready, obtain review, and **squash-merge** it to `main`. For the first RC, the controller creates `release/X.Y`. For an existing channel, review and **rebase-merge** the generated metadata backport to `release/X.Y`; the initial controller run waits without mutation until that merge.

### Monitor and approve publication

Both automatic `pull_request_target` events and manual gateway dispatches execute the default-branch controller, including backport merges into older release branches. Resolver and controller jobs pin tooling to the executing workflow commit. Deployment Engine verification uses the approved plan's `expectedOutputs`, passed between jobs with the resolved plan artifact, rather than the controller checkout's current platform defaults. Publication jobs use the immutable tagged commit, including across an approval wait.

1. Follow the [release controller](https://github.com/radius-project/radius/actions/workflows/release-controller.yaml) summary. It verifies the signed Deployment Engine tag, then pauses on its **Approve release reconciliation** job in the `release` environment. Review the version, metadata-bearing source commit, Deployment Engine commit, and frozen sibling commits it shows, then approve. The Radius controller makes no changes before this approval; external repositories may already have staged immutable artifacts from their own tag builds. The controller then publishes or adopts and locks the full-version Deployment Engine image, reconciles the frozen sibling commits, and creates the Radius tag last.
2. Follow the linked [release build](https://github.com/radius-project/radius/actions/workflows/build-release.yaml). GoReleaser stages core artifacts; the retained Bicep image, Helm chart, and both Bicep type publishers complete before verification. Radius explicitly dispatches and awaits AWS types from the plan's frozen AWS source commit. Bicep type artifacts use full-version tags and carry source/revision/version annotations; existing matching versions are reusable, not overwritten. The release Bicep image also references these full-version extensions, so staged installation does not depend on channel aliases that have not been promoted yet. Edge builds retain their `latest` extension references. `testrp` and `magpiego` are test-workflow outputs, not release assets or channel aliases.
3. Review `release-manifest.json` and the staged-installation result. **Finals and patches** wait for a second approval, **Approve final or patch publication**, in the `release` environment; approve only the verified version/source pair. **RCs** publish without this second approval after the same mandatory gate. Outputs are rechecked after approval before alias promotion or publication.
4. Follow the exact downstream run URLs. For RCs, review and merge docs and samples upmerge PRs, then resume the release. Automation binds the pull request each upmerge run opened and requires it to be merged before running sample tests; both repositories squash-merge, so the source commits never become reachable from `edge`, and a run with nothing to merge is complete. All three successful receipts are required for final preparation and publication; the release build rechecks them during verification and again after publication approval, before any alias moves. Receipts exist only for RCs published with this coordination in place; if the last validated RC predates it, cut another RC before preparing the final. Finals publish docs and samples and run sample tests; patches run sample tests without recutting their release branches.

For a final release, run [Publish Bicep Recipes](https://github.com/radius-project/resource-types-contrib/actions/workflows/publish-bicep-recipes.yaml) from `main` in `radius-project/resource-types-contrib` with `release_version` set to the version without `v`, such as `0.56.0`. This remains a separate maintainer action: the Radius controller does not dispatch it. Confirm the selected recipe source is intended for the release, wait for publication to succeed, and record its run URL. This publishes release-version recipe-image tags and moves the `latest` tags referenced by recipe packs.

### Bicep extension release contract

Newly prepared plans select `expectedOutputs.bicepExtensionsContract: ghcr-v1` and the canonical AWS/Radius targets in `.github/release-parity/targets.json`. Prepare Release copies this complete contract into the approved schema-v2 plan; never edit an approved plan to opt in. The Radius integration is **merge-blocked until the AWS companion and operational gates below are verified**: the proposed AWS workflow has not been audited or delivered by this Radius change. Plans without the selector retain their legacy ACR channel/RC contract. Unknown selectors and GHCR targets without a selector fail closed.

New-format releases require one `bicep-extension-lock.json` release asset, with the following shape (example values, not publication evidence):

```json
{
  "schemaVersion": 1,
  "version": "v0.62.0-rc.1",
  "releaseSourceCommit": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
  "artifacts": [
    {
      "name": "aws-bicep-types",
      "source": {
        "repository": "radius-project/bicep-types-aws",
        "commit": "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
      },
      "version": "0.62.0-rc.1",
      "reference": "ghcr.io/radius-project/bicep-types-aws:0.62.0-rc.1",
      "digest": "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
      "generation": {
        "workflow": ".github/workflows/publish-release-bicep.yaml",
        "runId": 123,
        "runAttempt": 1,
        "artifactId": 456,
        "artifactDigest": "sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
      }
    },
    {
      "name": "radius-bicep-types",
      "source": {
        "repository": "radius-project/radius",
        "commit": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
      },
      "version": "0.62.0-rc.1",
      "reference": "ghcr.io/radius-project/bicep-types-radius:0.62.0-rc.1",
      "digest": "sha256:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee",
      "generation": {
        "workflow": ".github/workflows/build-release.yaml",
        "runId": 789,
        "runAttempt": 1,
        "artifactId": 987,
        "artifactDigest": "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
      }
    }
  ]
}
```

The artifact array has exactly AWS then Radius, at the same full stable or dotted RC version. Radius binds to the controller-resolved metadata-bearing release SHA, not the product parent or current `main`; AWS binds to the plan's one frozen `bicep-types-aws` sibling SHA. Each manifest must be a non-executable Bicep provider artifact with the provider config and single type layer, `bicep.serialization.format: v1`, and `org.opencontainers.image.source`, `.revision`, and `.version` annotations matching its entry. The generation attempt identifies the original generation, not a later upload retry.

The trusted default-branch controller authenticates AWS evidence through GitHub's API before creating the Radius tag: fixed workflow, exact source/tag, successful publishing attempt, original successful generation attempt, artifact ownership, upload timestamps, and content digests. The Radius producer authenticates its own captured snapshot. Capture generated inputs and outputs once, including AWS schemas. The offline contract validator checks shape and source/digest binding; a self-reported run ID is not authentication. The tag build reads the controller's same-repository evidence, validates both live full-version outputs, and uploads the immutable pair before verification.

`plannedBicepExtensions(plan, sourceSha)` returns the exact expected identities; `validateBicepExtensionLock(plan, sourceSha, lock)` validates the pair; `verifyBicepExtensionOutputs(plan, sourceSha, lock, observed)` also checks the collected OCI manifests. The collector's optional `--plan-file <approved-plan.json>` is required for GHCR targets. It downloads the lock with existing release assets, fetches manifests by their observed digest, checks the raw bytes against that digest, and stores the lock in `downstream.bicepExtensionLock`. `verify-release-manifest.mjs` includes that evidence in installation approval and post-approval rechecking. Existing release-asset upload/download helpers enforce immutability and reject missing lock evidence, even when a caller requests optional download.

The serialized finalizer promotes eligible stable GHCR `X.Y` and `latest` aliases and mirrors the locked bytes to ACR channel tags, after the existing approval and output recheck. RCs mirror full RC tags after verification and never advance stable aliases. ACR development `latest` and GHCR `edge` remain main-owned. Newer partially finalized aliases also prevent an older release from moving that alias group backward. Writes are not transactional: inspect `bicep-publication-<run-id>-<attempt>` for verified, pending, or superseded destinations, then retry against the same lock. No rollback of successfully copied manifests is implied.

#### Radius producer and recovery

[`__publish-release-bicep.yaml`](../../../.github/workflows/__publish-release-bicep.yaml) is the restricted reusable full-version producer called at the existing release Bicep staging point for `ghcr-v1` plans. Only `build-release.yaml` running on an approved full-version Radius tag may call it. It reads the committed schema-v2 GHCR plan, resolves the merged generated release PR, and reruns the existing controller's source/plan validation. A release source need not be current `main`; neither protected-tag requirements nor the main/edge eligibility policy change.

The read-only capture job generates once and retains a native Bicep OCI snapshot for 90 days. A fresh publishing job authenticates the run, original successful capture attempt, artifact ownership, upload timestamp, and downloaded content digest before using ORAS to copy data. It receives only the repository's package-write token: no App key, Azure secret, or OIDC permission is added, and no downloaded scripts or binaries run under publishing credentials. The producer writes only `ghcr.io/radius-project/bicep-types-radius:<full-version>`, such as `0.62.0` or `0.62.0-rc.1`. Its per-tag concurrency group is `radius-bicep-types-release-refs/tags/<version-with-v>`; any future full-version writer must share that serialization. Matching existing content is verified and reused; conflicting content fails without overwrite. Authorization and transport errors do not count as absent tags.

The reusable output `record` is a JSON string containing **one Radius entry** in the R1 artifact shape, with `generation.workflow: .github/workflows/build-release.yaml`. The `manifest-digest` output provides its digest separately. A successful attempt also retains `radius-bicep-extension.json` and `receipt.json` in `radius-bicep-extension-<run-id>-<run-attempt>`. An interrupted upload retains a partial receipt; a completed upload whose package is private retains an uploaded receipt but returns no success record. The existing package bootstrap remains upload first, then administrator makes the package Public and retries with the original evidence. Public visibility is checked through GitHub; anonymous restore remains a separate activation rehearsal gate.

Same-run retries reuse `bicep-types-radius-<run-id>.tar` and preserve its original generation attempt. Prefer the existing Resume Release procedure. If a separate tag build is needed before the paired lock exists, supply **both** `generation-run-id` and `generation-run-attempt` to `build-release.yaml`, identifying the original successful capture, not the latest publishing attempt. These are authenticated against the same approved tag/source. A prior tag-build run without these inputs or an existing full-version tag without its snapshot blocks regeneration. Missing, expired, corrupt, unsuccessful, or mismatched capture evidence stops recovery; artifact expiry before durable evidence was accepted requires maintainer resolution through the release process, not rebuilding the same version. Once the paired lock exists, reconciliation reuses it without generating again.

#### Required AWS companion protocol

The following is the **new proposed A1 protocol**, not a description of an existing verified AWS implementation. Audit the AWS repository and implement the companion before merging this Radius activation. If names collide, revise both sides before merge; there is no runtime alternate-workflow input or registry fallback.

| Surface                        | Required value                                                                                                                                 |
|--------------------------------|------------------------------------------------------------------------------------------------------------------------------------------------|
| Source                         | The approved plan's frozen `radius-project/bicep-types-aws` SHA and controller-created release tag                                             |
| Workflow                       | `.github/workflows/publish-release-bicep.yaml`, on tag push or same-tag manual recovery                                                        |
| Successful publishing artifact | `aws-bicep-extension-<publishing-run-id>-<publishing-attempt>`, containing only `aws-bicep-extension.json`, one AWS record in the schema above |
| Original generation job        | `Capture versioned AWS Bicep types`                                                                                                            |
| Original raw snapshot          | `bicep-types-aws-<generation-run-id>.tar`, retaining its Actions ID, digest, and original successful attempt; maximum 64 MiB in this contract  |
| Retry identity                 | Publishing run/attempt may differ; original generation workflow/run/attempt/artifact ID/digest must not change                                 |

The controller uses an existing release-App token limited to AWS Actions/Contents/Metadata **read** for discovery and native artifact downloads. Its App installation must explicitly include AWS and grant Actions read; this permission is a setup gate, not assumed current configuration. The App key remains in trusted default-branch controller code. A narrow installation token does not narrow the authority of the key itself.

Authenticated AWS evidence is stored once in `.github/release-state/bicep-extensions.json` on `automation/bicep-release-state-<version-without-v>`, using an explicitly scoped Radius Contents-write token. This separate ref has one commit, parented by the exact Radius release SHA, changing only that evidence file. The wrapper contains `schemaVersion: 1`, the `v`-prefixed `version`, `releaseSourceCommit`, the single AWS `artifact`, and separate `publishing` workflow/run/attempt/artifact ID/artifact digest. Identical retries reuse it; conflicting contents or history fail. The Deployment Engine state ref and its one-commit invariant are unchanged. Tag jobs read this evidence with same-repository `GITHUB_TOKEN`; they receive no new cross-repository App credential. Legacy plans do not collect AWS Actions evidence or create this ref.

#### Ordered merge and writer handoff

1. Land the upstream release foundation through its owning maintainers, then synchronize the parent-owned Radius stack: writer restrictions, cloud credential isolation, main publishing, R1 contract, and R2 full-version producer. These preparatory layers do not establish working AWS release integration.
2. Audit and implement A1 in the frozen AWS source, including source-owned GHCR writes, captured schemas, this exact evidence contract, and original-snapshot recovery. If feasible, stage GHCR full versions alongside legacy ACR publication while only the old writer owns compatibility aliases. Determine the exact old-dispatch removal; if it requires a separate AWS or private-publisher companion, prepare that authorized change. Do not assume a private workflow toggle exists.
3. Verify package creation with each source repository's token. A first push may create a private package; an administrator then makes it Public and retries with retained evidence. Demonstrate anonymous restore. Grant the Radius repository explicit Actions write access to the AWS package, auditing all candidate jobs exposed to that repository-scoped grant. Neither AWS artifact read access nor `packages: write` alone supplies this cross-package grant; do not substitute a broad PAT.
4. Configure and verify AWS App read permissions and the tagged finalizer's ACR authorization using the existing `BICEPTYPES_CLIENT_ID`, `BICEPTYPES_TENANT_ID`, and `BICEPTYPES_SUBSCRIPTION_ID` secrets at a scope the job can read. `finalize-release` is a **direct job**, not a reusable callee: inspect its actual OIDC claims and audience. The repository's customized subject includes `repository_owner_id`, `repository_id`, and `context`; this tag-ref context differs from the main publisher's environment context. Do not invent a `job_workflow_ref` claim, change the global subject template, broaden trust to candidate/Test identities, or change legacy bindings, shared approval environments, or tag policy.
5. Pause new release preparation, approval, and tag creation through the existing maintainer process before incompatible cross-repository changes. Finish all in-flight unpublished legacy releases while their required writers still operate; a failed/stopped run is not a drained release. Postpone cutover if a legacy plan still needs publication.
6. During that maintenance pause, drain the old Radius/AWS Bicep-only writers, apply the audited companion removals and this Radius cutover in the agreed order, and confirm sole ownership of every compatibility alias. A1 and Radius cannot merge atomically. Preserve unrelated external publishers and ACR resources. Resume new release creation only when the frozen sources contain the required implementation.
7. Rehearse RC and stable releases with the actual companion and credentials: source-bound evidence, immutable pair, public restore, post-approval substitution rejection, alias ordering, ACR digest parity, partial failure, and resume. Local mocks do not satisfy this gate. Change generated/checked-in consumers only in the following consumer layer once every referenced public artifact exists.

Published historical releases retain read-only ACR reconciliation. Do not promise automatic recovery of an unpublished legacy plan after its required writer has been disabled: finish it before cutover or coordinate an explicit maintenance restoration/handoff with no concurrent alias owners. Never rewrite its approved plan to the new contract. See the [migration design](../../../eng/design-notes/tools/2026-09-bicep-extension-ghcr-migration.md#parallel-execution-and-queueing).

### Resume

Run [Resume Release](https://github.com/radius-project/radius/actions/workflows/resume-release.yaml) from `main` with the failed summary's `version` (including `v`, for example `v0.61.0-rc.1`) and `source-commit`. The source is the release PR squash commit for a first RC, or the generated metadata backport merge commit for an existing channel. Do not substitute the current branch tip.

> **Unknown publisher outcome**: If a dispatch response is lost or returns a server error, the monitor continues looking for its correlated run until its existing timeout and does not send another dispatch in that invocation. Only explicitly rejected rate-limited dispatches are retried automatically. Before resuming an unknown outcome, inspect the publisher runs for the reported `<version>-<source-commit>` identifier and the destination image; wait for an accepted run instead of starting another. A missing run-list result alone does not prove rejection. This also applies after canceling or interrupting the controller. The current protocol does not provide exactly-once publication across separate invocations; that requires publisher-side idempotency.

Resume revalidates the committed plan and source, verifies Deployment Engine against the original output contract, accepts matching tags and locks, reuses active correlated runs, and retries failed jobs in the exact tag-build run. Updated controller tooling does not change the approved platform requirements on resume. The Deployment Engine tag must still point at the commit shown in the same run's validation summary. A full-version Deployment Engine image from another source is a conflict even before its digest is locked; registry errors fail instead of dispatching again. Every resume rejects divergence from the Deployment Engine digest lock on `automation/release-state-<version>` or a lock commit that the release App did not write. Later branch commits are allowed only while the approved source remains reachable. Conflicting tags, plans, branches, or digests stop recovery. Approval rejection does not bypass verification or environment protection. Runs for the same version and source commit queue without canceling the active run, with a maximum of 100 pending attempts.

Use [Approve Release](https://github.com/radius-project/radius/actions/workflows/approve-release.yaml) with the same approved version and source only when the automatic merge event did not start reconciliation. Both gateways dispatch the default-branch controller, and neither accepts a replacement plan. Neither gateway holds release credentials, so running one from another branch cannot change the controller code that uses them. This protects the controller code path, not the credentials: the release and publisher App keys are repository secrets, so any workflow a writer pushes can still read them.

## Verification

- The summary identifies the approved version/source, every mandatory stage, elapsed duration, expected and observed outputs, downstream run URLs, and the exact resume command. Keep the summary and release-thread links as the release record.
- The manifest binds the approved plan to binaries, checksums, Go/linker metadata, SBOMs, image digests and platforms, full-version chart references, and downstream outputs. The isolated kind installation uses the staged CLI and chart with digest overrides. Its same-version upgrade disables only the version-transition check; it is not evidence of a cross-version upgrade.
- Each CLI asset has an SPDX 2.x JSON SBOM, such as `rad_linux_amd64.sbom.json`. Its binary retains its own `.sha256` sidecar. Production image SBOMs are per-platform BuildKit attestations, inspected by immutable digest:

  ```bash
  docker buildx imagetools inspect \
    "ghcr.io/radius-project/ucpd@sha256:<digest>" \
    --format '{{ json (index .SBOM "linux/amd64").SPDX }}'
  ```

- The GitHub Release is public only after verification succeeds. For Radius-built images and CLI OCI artifacts, RCs advance no aliases; finals and patches promote eligible `X.Y` and `latest` aliases from locked digests without rebuilding, and an older release never replaces a newer published release's aliases. Main publishes only `edge` from snapshots, together with the moving `edge-<os>-<arch>` tags of the platform manifests the `edge` index is assembled from, and is outside the release transaction.
- Dashboard and Deployment Engine `X.Y` aliases are promoted from their verified source-bound locks in the same finalizer as Radius images. ORAS preserves the dashboard's single-manifest digest. Radius and AWS Bicep `X.Y` aliases are promoted by correlated publisher runs using the exact verified full-version digests before the GitHub Release becomes public. RCs move none of these aliases. External `latest` tags retain their independently owned edge behavior.
- A new stable channel needs no legacy bootstrap. An existing Bicep channel without comparable source/version annotations is rejected rather than blindly overwritten. Migrating such a channel is a separately reviewed compatibility prerequisite against known publication history; do not edit old immutable artifacts or blindly annotate an alias to bypass the check. Limit registry writes to the serialized publishers and finalizer: OCI registries do not provide a portable atomic create-only tag operation.
- Complete the [readiness checks](#confirm-readiness-before-the-first-production-cycle) with this runbook before production use. Local fixtures and draft PRs do not satisfy that rollout gate.

## Troubleshooting

### Break-glass recovery

Stop and record the failed stage, version, source, plan, manifest, and exact run URLs in the release thread. Fix the underlying automation or prerequisite through review, then use the same approved recovery gateway. There is no manual preparation fallback. Never move an approved tag, replace a locked artifact, edit an approved plan, or disable the manifest, installation, RC-coordination, or approval gates. Published content is corrected with a new version, not an overwrite.

Controller-only fixes on `main` are picked up by a new Resume Release invocation. Fixes to `build-release.yaml`, its called workflows, or its release scripts are different: the tag build still executes the original tagged code. Merge and backport those fixes before preparing a new release version; rerunning the old tag does not adopt them.

### Resuming a staged draft

Prefer Resume Release, which retries failed jobs in the exact existing build without repeating successful jobs. The tag preflight has the push access GitHub requires to observe drafts. Every staging attempt also reloads the optional image and CLI locks after ensuring the source-bound draft exists, even when a failed-job retry retains the original preflight's `absent` result. Matching locks are reused; a conflicting lock stops the run. Never delete a lock to make a rebuild pass.

### Lost dispatch response

Downstream tasks have GitHub deployment receipts in the `release-coordination` environment, one per version and task (`release-coordination:<version>:<task>`), bound to the Radius source SHA. If a dispatch response was lost before its returned run ID was persisted, automation stops instead of dispatching twice. Find the accepted run, verify its workflow, branch, inputs, and source, and add an `in_progress` status with its exact `log_url` to the reported receipt before resuming. If no run was accepted, reconcile the receipt explicitly before retrying. Never guess a run from a time window. For an upmerge, the receipt's environment URL is the pull request the run opened; automation binds it once, from the single upmerge pull request into `edge` created while the run's pull-request step ran, then checks only that pull request's merge state. If it cannot bind exactly one pull request, add an `in_progress` status with the run's `log_url` and the pull request as `environment_url` to the receipt before resuming.

### Backport conflicts or missing prerequisites

Selected fixes require either a merged backport or their exact source commit already present in the release branch's ancestry. A source included when that branch was created needs no backport PR. An empty cherry-pick without ancestry proof stops for reconciliation; do not treat it as a completed backport.

Follow the source PR's generated conflict handoff, resolve it on the pinned release-branch base, and rebase-merge the reviewed backport. For a missing signed Deployment Engine tag, provenance label, package permission, or successful previous-RC receipt, complete that prerequisite and resume without changing the release identity. A notification failure alone does not affect publication safety.
