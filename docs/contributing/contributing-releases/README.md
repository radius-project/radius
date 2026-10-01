# Create and monitor a Radius release

## Purpose

Release maintainers approve an immutable release plan, monitor its publication, and resume failed stages without changing the approved version or source. The same procedure covers RC, final, and patch releases across `radius-project/resource-types-contrib`, `radius-project/radius`, `radius-project/docs`, `radius-project/samples`, and `azure-octo/deployment-engine`. Automation creates Radius and sibling tags, stages artifacts, verifies installation, publishes the release, and coordinates docs and samples.

## Prerequisites

- Choose `rc`, `final`, or `patch` and its `X.Y` channel. Prepare Release computes the version; new RCs use `-rc.N`, starting at `-rc.1`, and the historical `-rcN` form is never mixed into the same version, because SemVer orders every dotted candidate before every legacy one and `rad upgrade` would treat that move as a downgrade. A final uses the last validated RC's product code. `versions.yaml` keeps the previous stable release supported until the final release moves it to `deprecated`. Label selected fixes `backport release/X.Y` and **rebase-merge** their generated backports before preparation.
- Have maintainer access to the organization repositories, including `resource-types-contrib`, and permission to approve the `release` environment. Generated release PRs require `Validate release plan`; release branches require `Validate release branch commits` and up-to-date branches. Ordinary `main` PRs use squash merge; backports use rebase merge.
- Release the required resource type namespaces and merge their generated Radius update before preparation, as described in [Release resource type namespaces](#release-resource-type-namespaces).
- Configure the release App identities, including `RADIUS_RELEASE_BOT` Actions write, Contents read, Pull requests read, and Metadata read on `docs` and `samples`. Give the Radius repository Actions package-write access to the GHCR `dashboard` and `deployment-engine` packages. Helm publication waits up to ten minutes for a channel image whose publisher is still building the planned source before treating the mismatch as a conflict; an existing full-version tag is never awaited. Extend the publisher App (`RADIUS_PUBLISHER_BOT`) installation on `azure-octo` to `deployment-engine` with Contents read; Prepare Release and the controller verify the signed Deployment Engine tag with that token because the repository is private. Coordination records same-repository deployment receipts with `GITHUB_TOKEN` and `deployments: write` in the `release-coordination` environment, which must stay free of protection rules.
- Require the dashboard publisher to stamp `org.opencontainers.image.revision` with the exact built commit. Its `Build Image` step (`yarn run build-image`) and its Dockerfile set no OCI labels today, so a companion publisher change is still a rollout prerequisite, and the release manifest has required the label since it was introduced, not only since the chart cutover. Missing provenance, conflicting digests, or incomplete platforms must not be waived.
- Create the matching **GPG-signed Deployment Engine tag** in `azure-octo/deployment-engine` before preparation. Prepare Release reports the exact required tag if it is missing; the controller also blocks before mutation. In a clean Deployment Engine clone, verify the intended source and run `git tag -s <version> -m "release tag <version>"`, then `git push origin <version>`. This remains manual until [deployment-engine#456](https://github.com/azure-octo/deployment-engine/issues/456) is resolved. Never manually tag the Radius repositories.
- Use the Teams release thread for approvals and exceptional recovery. Set the optional `RELEASE_TEAMS_WEBHOOK` secret to an HTTPS Adaptive Card endpoint for automated updates. GitHub summaries remain authoritative when notifications are unavailable.

## Steps

### Release resource type namespaces

Complete this step before starting Radius release preparation so the approved product commit includes the intended resource types.

1. In `radius-project/resource-types-contrib`, run the [Release Namespace](https://github.com/radius-project/resource-types-contrib/actions/workflows/release-namespace.yaml) workflow from `main` for each namespace consumed by Radius under `resourceTypes` in [`deploy/manifest/defaults.yaml`](../../../deploy/manifest/defaults.yaml). Select the namespace and the appropriate semantic version `bump` (`patch`, `minor`, or `major`). Leave `prerelease_label` empty and `dry_run` and `force` disabled. Namespace versions are independent of the Radius version; even for a Radius RC, publish stable namespace releases because prereleases do not notify Radius.
2. Wait for each workflow to succeed. An unchanged namespace is skipped and does not need a new release. For each published release, confirm that [Notify Radius](https://github.com/radius-project/resource-types-contrib/actions/workflows/notify-radius.yaml) succeeds and triggers Radius's [Update Resource Types](https://github.com/radius-project/radius/actions/workflows/update-resource-types.yaml) workflow.
3. Review the generated `chore(resource-types-contrib): updates` PR from `bot/update-resource-types` in `radius-project/radius`. Successive notifications refresh the same open PR. Confirm that `deploy/manifest/defaults.yaml` pins the intended stable namespace releases and that the copied manifests and generated artifacts are included. Wait for required checks and approval, then merge the PR into `main` before preparation. If all namespaces are unchanged and Radius already has the intended pins, no new PR is required.
4. Record the namespace releases and merged update PR in the Teams release thread. A first RC inherits this update from `main`. For a subsequent RC or a patch with resource type changes, label the update `backport release/X.Y` and rebase-merge its generated backport into `release/X.Y` before preparing the release.

Stop if the update is missing or incomplete. Publishing namespace releases alone is not enough: their resource-type update must be merged into the code that Radius will release.

### Approve the plan

1. Run [Prepare Release](https://github.com/radius-project/radius/actions/workflows/prepare-release.yaml) from `main` with `release-type`, `channel`, and optional comma-separated `backport-pr-numbers`.
2. Review the generated draft PR's version, product commit, frozen sibling commits, selected backports, expected outputs, and schema-v2 plan under `.github/release-plans/`. Curate only Highlights and Upgrading in RC or final notes; patch notes are generated completely. Retain the warning that chart-default images are pinned to the full version: pod restarts no longer pick up later patches, and explicit image overrides remain respected.
3. If the base advances, rerun Prepare Release with the same inputs. A frozen sibling head that advances does not invalidate the plan; rerun only when the later sibling change belongs in the release. It regenerates the plan and generated prose while preserving the allowed curated sections. If preparation reports a pending backport, merge it and rerun; do not omit the fix to bypass validation.
4. Mark the release PR ready, obtain review, and **squash-merge** it to `main`. For the first RC, the controller creates `release/X.Y`. For an existing channel, review and **rebase-merge** the generated metadata backport to `release/X.Y`; the initial controller run waits without mutation until that merge.

### Monitor and approve publication

1. Follow the [release controller](https://github.com/radius-project/radius/actions/workflows/release-controller.yaml) summary. Confirm its version and metadata-bearing source commit. It verifies the signed Deployment Engine tag and locked image, reconciles the frozen sibling commits, then creates the Radius tag last.
2. Follow the linked [release build](https://github.com/radius-project/radius/actions/workflows/build-release.yaml). GoReleaser stages core artifacts; the retained Bicep container publisher, Helm, and paired Bicep extension lock complete before verification. `testrp` and `magpiego` are test-workflow outputs, not release assets or channel aliases.
3. Review `release-manifest.json` and the staged-installation result. **Finals and patches** wait for approval in the `release` environment; approve only the verified version/source pair. **RCs** publish automatically after the same mandatory gate. Outputs are rechecked after approval before alias promotion or publication.
4. Follow the exact downstream run URLs. For RCs, review and merge docs and samples upmerge PRs, then resume the release. Automation binds the pull request each upmerge run opened and requires it to be merged before running sample tests; both repositories squash-merge, so the source commits never become reachable from `edge`, and a run with nothing to merge is complete. All three successful receipts are required for final preparation and publication. Receipts exist only for RCs published with this coordination in place; if the last validated RC predates it, cut another RC before preparing the final. Finals publish docs and samples and run sample tests; patches run sample tests without recutting their release branches.

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

Resume revalidates the committed plan and source, accepts matching tags and locks, reuses active correlated runs, and retries failed jobs in the exact tag-build run. Later branch commits are allowed only while the approved source remains reachable. Conflicting tags, plans, branches, or digests stop recovery. Approval rejection does not bypass verification or environment protection.

Use [Approve Release](https://github.com/radius-project/radius/actions/workflows/approve-release.yaml) with the same approved version and source only when the automatic merge event did not start reconciliation. Both gateways dispatch the default-branch controller; neither accepts a replacement plan or exposes App credentials to branch-selected code.

## Verification

- The summary identifies the approved version/source, every mandatory stage, elapsed duration, expected and observed outputs, downstream run URLs, and the exact resume command. Keep the summary and release-thread links as the release record.
- The manifest binds the approved plan to binaries, checksums, Go/linker metadata, SBOMs, image digests and platforms, full-version chart references, and downstream outputs. The isolated kind installation uses the staged CLI and chart with digest overrides. Its same-version upgrade disables only the version-transition check; it is not evidence of a cross-version upgrade.
- Each CLI asset has an SPDX 2.x JSON SBOM, such as `rad_linux_amd64.sbom.json`. Its binary retains its own `.sha256` sidecar. Production image SBOMs are per-platform BuildKit attestations, inspected by immutable digest:

  ```bash
  docker buildx imagetools inspect \
    "ghcr.io/radius-project/ucpd@sha256:<digest>" \
    --format '{{ json (index .SBOM "linux/amd64").SPDX }}'
  ```

- The GitHub Release is public only after verification succeeds. RCs advance no aliases. Finals and patches promote eligible `X.Y` and `latest` aliases from locked digests without rebuilding; an older release never replaces a newer alias. Main publishes only `edge` from snapshots, together with the moving `edge-<os>-<arch>` tags of the platform manifests the `edge` index is assembled from, and is outside the release transaction.
- Complete a real release cycle with this runbook before enabling the final migration in production. Local fixtures and draft PRs do not satisfy that rollout gate.

## Troubleshooting

### Break-glass recovery

Stop and record the failed stage, version, source, plan, manifest, and exact run URLs in the release thread. Fix the underlying automation or prerequisite through review, then use the same approved recovery gateway. There is no manual preparation fallback. Never move an approved tag, replace a locked artifact, edit an approved plan, or disable the manifest, installation, RC-coordination, or approval gates. Published content is corrected with a new version, not an overwrite.

### Lost dispatch response

Downstream tasks have GitHub deployment receipts in the `release-coordination` environment, one per version and task (`release-coordination:<version>:<task>`), bound to the Radius source SHA. If a dispatch response was lost before its returned run ID was persisted, automation stops instead of dispatching twice. Find the accepted run, verify its workflow, branch, inputs, and source, and add an `in_progress` status with its exact `log_url` to the reported receipt before resuming. If no run was accepted, reconcile the receipt explicitly before retrying. Never guess a run from a time window. For an upmerge, the receipt's environment URL is the pull request the run opened; automation binds it once, from the single upmerge pull request into `edge` created while the run's pull-request step ran, then checks only that pull request's merge state. If it cannot bind exactly one pull request, add an `in_progress` status with the run's `log_url` and the pull request as `environment_url` to the receipt before resuming.

### Backport conflicts or missing prerequisites

Follow the source PR's generated conflict handoff, resolve it on the pinned release-branch base, and rebase-merge the reviewed backport. For a missing signed Deployment Engine tag, provenance label, package permission, or successful previous-RC receipt, complete that prerequisite and resume without changing the release identity. A notification failure alone does not affect publication safety.
