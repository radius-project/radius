# Create and monitor a Radius release

## Purpose

Release maintainers approve an immutable release plan, monitor its publication, and resume failed stages without changing the approved version or source. The same procedure covers RC, final, and patch releases. Automation creates Radius and sibling tags, stages artifacts, verifies installation, publishes the release, and coordinates docs and samples.

## Prerequisites

- Choose `rc`, `final`, or `patch` and its `X.Y` channel. Prepare Release computes the version; new RCs use `-rc.N`, starting at `-rc.1`, and the historical `-rcN` form is never mixed into the same version, because SemVer orders every dotted candidate before every legacy one and `rad upgrade` would treat that move as a downgrade. A final uses the last validated RC's product code. `versions.yaml` keeps the previous stable release supported until the final release moves it to `deprecated`. Label selected fixes `backport release/X.Y` and **rebase-merge** their generated backports before preparation.
- Have maintainer access to the organization repositories and permission to approve the `release` environment. Generated release PRs require `Validate release plan`; release branches require `Validate release branch commits` and up-to-date branches. Ordinary `main` PRs use squash merge; backports use rebase merge.
- Configure the release App identities, including `RADIUS_RELEASE_BOT` Actions write, Contents read, Pull requests read, and Metadata read on `docs` and `samples`. Give the Radius repository Actions package-write access to the GHCR `dashboard` and `deployment-engine` packages. Helm publication waits up to ten minutes for a channel image whose publisher is still building the planned source before treating the mismatch as a conflict; an existing full-version tag is never awaited. Extend the publisher App (`RADIUS_PUBLISHER_BOT`) installation on `azure-octo` to `deployment-engine` with Contents read; Prepare Release and the controller verify the signed Deployment Engine tag with that token because the repository is private. Coordination records same-repository deployment receipts with `GITHUB_TOKEN` and `deployments: write` in the `release-coordination` environment, which must stay free of protection rules.
- Require the dashboard publisher to stamp `org.opencontainers.image.revision` with the exact built commit. Its `Build Image` step (`yarn run build-image`) and its Dockerfile set no OCI labels today, so a companion publisher change is still a rollout prerequisite, and the release manifest has required the label since it was introduced, not only since the chart cutover. Missing provenance, conflicting digests, or incomplete platforms must not be waived.
- Create the matching **GPG-signed Deployment Engine tag** in `azure-octo/deployment-engine` before preparation. Prepare Release reports the exact required tag if it is missing; the controller also blocks before mutation. In a clean Deployment Engine clone, verify the intended source and run `git tag -s <version> -m "release tag <version>"`, then `git push origin <version>`. This remains manual until [deployment-engine#456](https://github.com/azure-octo/deployment-engine/issues/456) is resolved. Never manually tag the Radius repositories.
- Use the Teams release thread for approvals and exceptional recovery. Set the optional `RELEASE_TEAMS_WEBHOOK` secret to an HTTPS Adaptive Card endpoint for automated updates. GitHub summaries remain authoritative when notifications are unavailable.

## Steps

### Approve the plan

1. Run [Prepare Release](https://github.com/radius-project/radius/actions/workflows/prepare-release.yaml) from `main` with `release-type`, `channel`, and optional comma-separated `backport-pr-numbers`.
2. Review the generated draft PR's version, product commit, frozen sibling commits, selected backports, expected outputs, and schema-v2 plan under `.github/release-plans/`. Curate only Highlights and Upgrading in RC or final notes; patch notes are generated completely. Retain the warning that chart-default images are pinned to the full version: pod restarts no longer pick up later patches, and explicit image overrides remain respected.
3. If the base advances, rerun Prepare Release with the same inputs. A frozen sibling head that advances does not invalidate the plan; rerun only when the later sibling change belongs in the release. It regenerates the plan and generated prose while preserving the allowed curated sections. If preparation reports a pending backport, merge it and rerun; do not omit the fix to bypass validation.
4. Mark the release PR ready, obtain review, and **squash-merge** it to `main`. For the first RC, the controller creates `release/X.Y`. For an existing channel, review and **rebase-merge** the generated metadata backport to `release/X.Y`; the initial controller run waits without mutation until that merge.

### Monitor and approve publication

1. Follow the [release controller](https://github.com/radius-project/radius/actions/workflows/release-controller.yaml) summary. Confirm its version and metadata-bearing source commit. It verifies the signed Deployment Engine tag and locked image, reconciles the frozen sibling commits, then creates the Radius tag last.
2. Follow the linked [release build](https://github.com/radius-project/radius/actions/workflows/build-release.yaml). GoReleaser stages core artifacts; the retained Bicep publisher, Helm, and Bicep types complete before verification. `testrp` and `magpiego` are test-workflow outputs, not release assets or channel aliases.
3. Review `release-manifest.json` and the staged-installation result. **Finals and patches** wait for approval in the `release` environment; approve only the verified version/source pair. **RCs** publish automatically after the same mandatory gate. Outputs are rechecked after approval before alias promotion or publication.
4. Follow the exact downstream run URLs. For RCs, review and merge docs and samples upmerge PRs, then resume the release. Automation binds the pull request each upmerge run opened and requires it to be merged before running sample tests; both repositories squash-merge, so the source commits never become reachable from `edge`, and a run with nothing to merge is complete. All three successful receipts are required for final preparation and publication. Receipts exist only for RCs published with this coordination in place; if the last validated RC predates it, cut another RC before preparing the final. Finals publish docs and samples and run sample tests; patches run sample tests without recutting their release branches.

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
