<!-- markdownlint-disable MD024 -->

# How to create and publish a Radius release

## Purpose

This document is the maintainers' reference for cutting and publishing a Radius release across the `radius-project/radius`, `radius-project/docs`, `radius-project/samples`, and `azure-octo/deployment-engine` repositories. It covers release candidates, final releases, and patch releases, and the branching, tagging, and validation steps each requires. It is intended for project maintainers with release responsibility, not day-to-day contributors.

## Prerequisites

Before starting a release, ensure you have:

- **Release type and channel**: Choose `rc`, `final`, or `patch` and the target `<major>.<minor>` channel. Prepare Release computes and validates the version.
- **Repository access**: Write access to `radius-project/radius`, `radius-project/docs`, `radius-project/samples`, and `azure-octo/deployment-engine`.
- **GPG signing configured**: The `azure-octo` org requires [verified tags](https://docs.github.com/en/authentication/managing-commit-signature-verification/displaying-verification-statuses-for-all-of-your-commits). [Set up GPG signing locally](https://docs.github.com/en/authentication/managing-commit-signature-verification/generating-a-new-gpg-key) before starting.
- **Local clone of `radius-project/radius`**: Clone directly from the organization repo, not a personal fork. CI workflows require access to organization secrets that are not available in forks.

  ```bash
  git clone git@github.com:radius-project/radius.git
  ```

- **Required release checks configured**: The `Validate release plan` check is required for generated release pull requests to `main`. The `release/*` ruleset requires `Validate release branch commits` with **Require branches to be up to date before merging** enabled; this makes the backport's recorded base SHA fail closed if the release branch advances. Backport pull requests use rebase merge; ordinary `main` pull requests continue to use squash merge.

> **Important**: For the entire release process, create branches directly in repositories under the `radius-project` organization. Do not use personal forks.

## Terminology

| Term                | Description                                                                                                   | Example                        |
|---------------------|---------------------------------------------------------------------------------------------------------------|--------------------------------|
| **RC release**      | A release candidate for internal validation before public release. Create additional RCs if validation fails. | `v0.56.0-rc.1`, `v0.56.0-rc.2` |
| **Final release**   | A public release, built from the last validated RC.                                                           | `v0.56.0`                      |
| **Patch release**   | A bug-fix release for an existing final release.                                                              | `v0.56.1`                      |
| **Release channel** | A `<major>.<minor>` pair that groups all releases for a version.                                              | `0.56`                         |
| **Release branch**  | A branch in the format `release/<channel>` that holds release code.                                           | `release/0.56`                 |

New release candidates use the dotted SemVer form `-rc.N`, starting at `-rc.1`. Historical `-rcN` releases remain valid inputs for verification and upgrade tooling, but do not use that form for new tags.

## How releases work

### Release channels

Each release belongs to a channel named `<major>.<minor>`. The `rad` CLI and control plane for a given channel only interact with assets from that channel. Patch releases within a channel (e.g., `v0.56.1`) maintain backward compatibility with the original release.

> **Compatibility**: Cross-channel compatibility is not guaranteed. For example, the behavior of a `0.55` `rad` CLI talking to a `0.56` control plane is unspecified.

### Cadence

Radius follows a monthly release cadence. All contributions merged to `main` through the pull-request process are included in the next scheduled release.

### Release automation

Seven GitHub Actions workflows drive release preparation, validation, reconciliation, and publication. **No one manually creates tags in `radius-project` repos.** The release controller creates tags for repositories in the `radius-project` organization. (The [Deployment Engine repo](https://github.com/azure-octo/deployment-engine) in the `azure-octo` organization still requires manual tagging — see the release steps below.)

1. **[Prepare Release](https://github.com/radius-project/radius/actions/workflows/prepare-release.yaml)** (`prepare-release.yaml`): Manually dispatched with a release type and channel. It applies the fixed version policy, validates selected backports, freezes the exact sibling-repository commits, renders `CHANGELOG.md` and the release notes with git-cliff, updates `versions.yaml`, commits the schema-v2 plan under `.github/release-plans/`, and opens a signed draft pull request against `main`.
2. **[Release plan](https://github.com/radius-project/radius/actions/workflows/release-plan.yaml)** (`release-plan.yaml`): Validates generated release pull requests and merge-queue groups from trusted base-branch code. It compares the PR-body plan, committed plan, trusted regeneration, changed files, backports, and live sibling branch heads. Any drift requires rerunning Prepare Release.
3. **[Release backport](https://github.com/radius-project/radius/actions/workflows/release-backport.yaml)** (`release-backport.yaml`): Runs when a merged `main` pull request has a `backport release/<channel>` label. It cherry-picks the squash commit with `-x` and opens a pull request against the release branch. A conflict produces a draft pull request with an exact resolution handoff instead of committing conflict markers.
4. **[Release controller](https://github.com/radius-project/radius/actions/workflows/release-controller.yaml)** (`release-controller.yaml`): Triggered when a generated release pull request is squash-merged to `main`, when its generated metadata backport is rebase-merged to `release/<channel>`, or by an approved default-branch dispatch from Approve Release or Resume Release. This workflow:
   - Resolves exactly one merged generated release pull request and binds its approved plan to the metadata-bearing source commit
   - Reads the immutable committed plan, validates it against the metadata-bearing commit and `versions.yaml`, rejects a conflicting version/source pair, and runs preflight checks for every destination before mutation
   - Verifies the signed Deployment Engine tag, waits for release-environment approval, publishes the correct RC or stable image tag, and locks the verified GHCR digest for the controller run
   - Reconciles branches and tags in `recipes`, `dashboard`, and `bicep-types-aws` at the exact commits frozen in the plan
   - Creates the Radius release branch when needed and pushes the Radius tag last with the release App, which triggers the release build
   - Treats matching completed state as success and rejects an existing tag at any other commit

   For an existing channel, the controller run started by the `main` release pull request records that it is waiting and performs no mutation. The generated metadata backport removes the branch's legacy `release.yaml` before its merge push can trigger workflows. Merging that backport starts the controller again with the release-branch commit and continues reconciliation.
5. **[Approve Release](https://github.com/radius-project/radius/actions/workflows/approve-release.yaml)** (`approve-release.yaml`): A secretless manual gateway for an explicit approved start when the automatic merge event was unavailable. It accepts the approved version and exact source commit and dispatches the default-branch controller.
6. **[Resume Release](https://github.com/radius-project/radius/actions/workflows/resume-release.yaml)** (`resume-release.yaml`): The corresponding secretless recovery gateway after a failed or interrupted controller run. It dispatches the default-branch controller, which revalidates the approved plan, skips matching completed work, and resumes the first incomplete stage. Runs for the same version and source commit queue without canceling one another.
7. **[Release build](https://github.com/radius-project/radius/actions/workflows/build-release.yaml)** (`build-release.yaml`): Triggered by the App-created Radius `v*` tag. This workflow:
   - Runs GoReleaser once to build the CLI assets and checksums, publish immutable full-version production images, and stage a draft GitHub Release from the prepared file in `docs/release-notes/`
   - Builds the Bicep, `testrp`, and `magpiego` images under immutable full-version tags using their existing paths until their consumers move in the final migration phase
   - Publishes immutable full-version CLI OCI artifacts and records image and CLI digests for finalization
   - Publishes the Helm chart and dispatches Bicep types publishing while the GitHub Release remains a draft
   - Verifies the expected image platform sets and promotes the channel and `latest` image and CLI OCI aliases from the recorded immutable digests for final and patch releases; RCs advance no mutable aliases
   - Publishes the draft GitHub Release only after GoReleaser, Helm, Bicep types, and alias promotion succeed

   The release carries internal JSON lock assets for the core, retained images, and complete image set. A rerun verifies these locks and skips the immutable work they already cover; a full-version tag that no longer matches its lock stops the release instead of being rebuilt or moved. Tags pushed by an interrupted attempt are not yet locked, so a rerun re-stages them rather than stranding the release. Main-branch builds publish Radius images and CLI OCI artifacts only as `edge`. The `latest` alias always points to the most recent stable release after this cutover.

   #### Release SBOMs

   Each raw `rad` binary has an SPDX 2.x JSON SBOM beside it on the GitHub Release, named by adding `.sbom.json` to the binary asset name. For example, `rad_linux_amd64.sbom.json` describes `rad_linux_amd64`. The release workflow generates these documents with the pinned Syft version and verifies their structure before publication. SBOM assets deliberately carry no `.sha256` sidecar, so the split checksum contract still covers exactly the `rad` binaries.

   Production image SBOMs are SPDX JSON predicates in per-platform BuildKit attestations attached to the immutable full-version OCI image index. BuildKit generates them with its own bundled scanner during the image build, so they are independent of the pinned Syft used for the CLI assets. They are not duplicate GitHub Release assets. Inspect one by immutable digest with Docker Buildx:

   ```bash
   docker buildx imagetools inspect \
      "ghcr.io/radius-project/ucpd@sha256:<digest>" \
      --format '{{ json (index .SBOM "linux/amd64").SPDX }}'
   ```

   The release workflow requires a valid attestation for every published production-image platform before it locks the image digest or publishes the GitHub Release.

The automated flow after dispatching Prepare Release:

```text
Prepare Release opens a draft release PR against main
   → maintainer curates Highlights and Upgrading, then merges the PR
      → first RC: approve the release environment
         → release-controller publishes prerequisites and creates the Radius tag last
      → existing channel: release-backport opens a PR against release/X.Y
         → rebase-merge the backport PR
            → approve the release environment
               → release-controller publishes prerequisites and creates the Radius tag last
               → build-release.yaml stages and verifies immutable artifacts
                  → stable aliases are promoted and GitHub Release is published
```

#### When does tag creation happen?

| Scenario          | Trigger                                          | What happens                                                                       |
|-------------------|--------------------------------------------------|------------------------------------------------------------------------------------|
| **First RC**      | Generated release PR merged to `main`            | The controller creates `release/X.Y` and pushes the RC tag after all prerequisites |
| **Subsequent RC** | Generated release backport merged to `release/*` | The controller validates the backport commit and pushes the new RC tag last        |
| **Final release** | Generated release backport merged to `release/*` | The controller validates the backport commit and pushes the final tag last         |
| **Patch release** | Generated release backport merged to `release/*` | The controller validates the backport commit and pushes the patch tag last         |

### Preparing release changes

Run the [Prepare Release](https://github.com/radius-project/radius/actions/workflows/prepare-release.yaml) workflow from `main` with these inputs:

| Input                 | Value                                                           |
|-----------------------|-----------------------------------------------------------------|
| `release-type`        | `rc`, `final`, or `patch`                                       |
| `channel`             | The target `X.Y` channel, such as `0.61`                        |
| `backport-pr-numbers` | Optional comma-separated merged `main` pull requests to include |

The workflow computes the version from the repository state; release engineers do not type or edit it. The generated draft pull request contains the `versions.yaml` and `CHANGELOG.md` updates, generated release notes, and an immutable schema-v2 plan under `.github/release-plans/` with the approved product commit, frozen sibling commits, expected outputs, included backports, and the rule for resolving the later metadata-bearing release commit. The same plan appears in the pull request body for review. Review it, curate only Highlights and Upgrading in the generated release notes, then mark the pull request ready and squash-merge it to `main`.

If `main` advances while the generated release pull request is open, rerun Prepare Release with the same inputs. The workflow regenerates the plan and generated sections from the new base while preserving the existing Highlights and Upgrading text. Patch notes contain no curated sections and are regenerated completely.

If an explicit pull request still needs a backport, the workflow adds the channel label and fails with the pending pull request number. Merge the generated backport pull request, then rerun Prepare Release. It never silently excludes a selected backport.

### Backporting changes to a release branch

After a pull request is squash-merged to `main`, add the `backport release/<channel>` label to include it in a subsequent RC or patch. The release-backport workflow opens one pull request at a time from `automation/backport-<source-pr>-to-<channel>` to `release/<channel>` and records the source pull request and squash commit in its body. When that pull request merges, the release-branch push selects the next pending labeled change. This serial ordering keeps every backport pinned to the current release-branch base. Release preparation stops until every selected backport is merged.

If the cherry-pick conflicts, the workflow opens a draft pull request containing a conflict-handoff file and posts the exact recovery commands on the source pull request. Follow those commands, force-push the resolved branch with `--force-with-lease`, delete the handoff commit by resetting to the release branch as instructed, and mark the pull request ready.

Always **rebase-merge** backport pull requests. Rebase merging is enabled for this repository so the original Conventional Commit title remains on the release branch; the `release-branch-commits` check validates every commit message. Do not squash-merge a backport pull request.

The generated release pull request receives the same backport label automatically whenever the release branch already exists. A first RC needs no backport because the release controller creates the new release branch from the merged `main` commit.

### Resuming a release

If the release controller fails after validating the plan, run [Resume Release](https://github.com/radius-project/radius/actions/workflows/resume-release.yaml) with the values reported in the failed run summary:

| Input           | Value                                                                                                                                          |
|-----------------|------------------------------------------------------------------------------------------------------------------------------------------------|
| `version`       | The approved version including the `v` prefix, such as `v0.61.0-rc.1`                                                                          |
| `source-commit` | For a first RC, the generated release PR squash commit; for an existing channel, the merged generated release-backport commit on `release/X.Y` |

Resume Release resolves the merged release PR again, reads its committed plan, checks that the plan and `versions.yaml` select the same version and source, runs preflight checks for every frozen sibling commit, verifies the signed Deployment Engine tag and published image, and reconciles each destination. The first verified Deployment Engine digest is stored on `automation/release-state-<version>` and every resume rejects divergence from that lock. An existing matching tag or active correlated publisher run is accepted; a tag at another commit stops before later mutation. Never change the version or source commit to work around a conflict.

Use [Approve Release](https://github.com/radius-project/radius/actions/workflows/approve-release.yaml) for an explicit approved start when the automatic merge event was unavailable. It runs the same default-branch validation and reconciliation path as Resume Release without exposing release App credentials to branch-selectable workflow code.

## Creating an RC release

When starting the release process, first create an RC release. If validation fails, create additional RCs (incrementing the RC number) until validation passes.

### Step 1: Start a Teams release thread

Before performing any release actions, start and join a meeting in the team's Microsoft Teams channel dedicated to releases. Title the thread with the target final release version for the entire release cycle (for example, use "Release v0.56.0", not "Release v0.56.0-rc.1").

Turn on transcription for the meeting. Recording is not necessary. Verbally announce each step as you perform it, and post updates in the thread with the same information. This creates a detailed timeline of the release process that can be reviewed later for improvements and serves as a record of the release.

Use this same thread throughout the entire release lifecycle, including all RCs and the final release:

- **Log every action** as you perform it, including which step you are on, what commands you ran, and the result (success or failure).
- **Log any issues** encountered during the release, including error messages, failed workflows, and the resolution.
- **Announce completion** of the release in the thread once all steps are finished and validation passes.

This detailed release log helps the team improve future releases by reviewing the overall timeline, identifying inefficiencies, errors, or bottlenecks, and preserving institutional knowledge about the release process.

### Step 2: Tag the Deployment Engine

Run the following in a local clone of the [Deployment Engine repo](https://github.com/azure-octo/deployment-engine), replacing `vX.Y.Z-rc.N` with the RC version (e.g., `v0.56.0-rc.1`):

```bash
git checkout main
git pull origin main
git tag -s vX.Y.Z-rc.N -m "release tag vX.Y.Z-rc.N"
git push origin vX.Y.Z-rc.N
```

> **Note**: This manual tagging step is a temporary workaround. Ideally the [Deployment Engine Release Workflow](https://github.com/azure-octo/deployment-engine/actions/workflows/release.yaml) would handle this, but GPG signing is not yet configured there. See [azure-octo/deployment-engine#456](https://github.com/azure-octo/deployment-engine/issues/456).

### Step 3: Prepare the RC

Run the [Prepare Release](https://github.com/radius-project/radius/actions/workflows/prepare-release.yaml) workflow from `main` with `release-type` set to `rc` and `channel` set to the target `X.Y` channel. For a subsequent RC, first label and merge every fix that must be included as described in [Backporting changes to a release branch](#backporting-changes-to-a-release-branch).

The workflow computes `rc.1` for a new channel or increments the highest existing RC number. It opens a signed draft release pull request containing `versions.yaml`, `CHANGELOG.md`, generated release notes, and the committed schema-v2 release plan.

### Step 4: Review and merge the release pull request

Verify the generated version, product commit, included backports, and expected outputs in the release plan. Curate Highlights and Upgrading in the generated release notes. Mark the pull request ready, obtain approval, and squash-merge it to `main`.

### Step 5: Verify the automated release

After merging, the [release controller](https://github.com/radius-project/radius/actions/workflows/release-controller.yaml) automatically resolves and validates the approved release plan. When its **Approve release reconciliation** job reaches the `release` environment, approve it before expecting any publisher dispatch, sibling tag, or Radius tag.

- **First RC**: The controller publishes Deployment Engine, reconciles sibling repositories, creates the `release/X.Y` branch from the approved `main` commit, and pushes the `vX.Y.Z-rc.N` Radius tag last. The App-created tag triggers the [release build](https://github.com/radius-project/radius/actions/workflows/build-release.yaml) workflow. Verify the release using the checklist below.
- **Subsequent RCs**: The controller records that the approved plan is waiting for its generated metadata backport and performs no mutation. Merge that backport in [Step 6](#step-6-merge-the-generated-release-backport-subsequent-rcs-only), which starts the controller with the exact release-branch commit, then return here to verify.

Monitor and verify:

1. Approve the controller's **Approve release reconciliation** job in the `release` environment after reviewing the version and source commit shown in the workflow summary.
2. The [release controller](https://github.com/radius-project/radius/actions/workflows/release-controller.yaml) completes successfully. For the first RC, confirm it created the `release/X.Y` [branch](https://github.com/radius-project/radius/branches) and the `vX.Y.Z-rc.N` [tag](https://github.com/radius-project/radius/tags) after Deployment Engine and all sibling repositories succeeded.
3. The [release build](https://github.com/radius-project/radius/actions/workflows/build-release.yaml) workflow (triggered by the tag push) completes successfully. This workflow also dispatches Bicep types publishing automatically.
4. An RC release marked as pre-release appears on [GitHub Releases](https://github.com/radius-project/radius/releases).

### Step 6: Merge the generated release backport (subsequent RCs only)

> **Skip this step for the first RC.** The release branch was just created from `main` and already contains all changes.

The release pull request is automatically labeled for the channel. After it merges to `main`, the release-backport workflow opens a pull request against `release/X.Y`. Verify that it contains the release metadata commit and that the `release-branch-commits` check passes, then **rebase-merge** it. The merge event starts the release controller with that exact commit; after its prerequisites succeed, it creates the Radius tag last. Return to [Step 5](#step-5-verify-the-automated-release) to verify the release completed successfully.

### Step 7: Run validation workflows

1. In `radius-project/radius`, run the [Release verification](https://github.com/radius-project/radius/actions/workflows/release-verification.yaml) workflow from the `release/X.Y` branch. Enter the RC version number without the `v` prefix as the version (e.g., `0.56.0-rc.1`).

2. In `radius-project/docs`, run the [Upmerge docs to edge](https://github.com/radius-project/docs/actions/workflows/upmerge.yaml) workflow from the **previous** release branch (e.g., run from `v0.55` when releasing `v0.56`).

   > This generates a PR. Get approval and merge it before proceeding. The PR excludes branch-specific files (`docs/config.toml` and `docs/layouts/partials/hooks/body-end.html`).

3. In `radius-project/samples`, run the [Upmerge samples to edge](https://github.com/radius-project/samples/actions/workflows/upmerge.yaml) workflow from the **previous** release branch.

   > This generates a PR. Get approval and merge it before proceeding. The PR excludes `bicepconfig.json`.

4. In `radius-project/samples`, run the [Test Samples](https://github.com/radius-project/samples/actions/workflows/test.yaml) workflow from the `edge` branch. Enter the RC version number without the `v` prefix as the version (e.g., `0.56.0-rc.1`).

   > Run this only after the upmerge PR has been merged to `edge`. If tests fail, check logs and existing issues in the samples repo. Flaky tests may pass on re-run. If failures persist, file an issue and raise it with maintainers.

### Step 8: Assess results

If all validation workflows pass, proceed to [creating the final release](#creating-the-final-release).

If validation fails, fix the issues on `main`, then create a new RC (increment the RC number, e.g., `rc.2`, `rc.3`) by repeating the steps above.

## Creating the final release

The final release is built from the **last validated RC** on the release branch. The generated release backport changes only the version, changelog, release notes, and immutable release plan, so the final release contains exactly the same product code as the validated RC.

### Step 1: Update the Teams release thread

Post an update in the Teams release thread (started during the [RC release](#step-1-start-a-teams-release-thread)) indicating that the final release process is beginning. Continue logging every action, result, and issue in this thread throughout the final release steps.

### Step 2: Tag the Deployment Engine

Run the following in a local clone of the [Deployment Engine repo](https://github.com/azure-octo/deployment-engine), replacing `vX.Y.Z` with the final version (e.g., `v0.56.0`):

```bash
git checkout main
git pull origin main
git tag -s vX.Y.Z -m "release tag vX.Y.Z"
git push origin vX.Y.Z
```

> **Note**: Same temporary workaround as for [RC releases](#step-2-tag-the-deployment-engine). See [azure-octo/deployment-engine#456](https://github.com/azure-octo/deployment-engine/issues/456).

### Step 3: Prepare the final release

Run the [Prepare Release](https://github.com/radius-project/radius/actions/workflows/prepare-release.yaml) workflow from `main` with `release-type` set to `final` and the validated RC's `X.Y` channel. The workflow removes the RC suffix according to policy, renders the canonical changelog section and contributor list, and opens a signed draft release pull request.

### Step 4: Review and merge the release pull request

Verify that the plan references the last validated RC and contains no unmerged backports. Curate Highlights and Upgrading, mark the pull request ready, obtain approval, and squash-merge it to `main`.

### Step 5: Merge the generated release backport

After the release pull request merges, the release-backport workflow opens a pull request against `release/X.Y`. Verify that it changes only release metadata and notes, then **rebase-merge** it. Do not squash-merge it.

### Step 6: Verify the automated release

After the generated release backport is merged to `release/X.Y`, the [release controller](https://github.com/radius-project/radius/actions/workflows/release-controller.yaml) binds the approved plan to that exact commit, publishes and verifies prerequisites, and creates the `vX.Y.Z` Radius tag last. The App-created tag triggers the [release build](https://github.com/radius-project/radius/actions/workflows/build-release.yaml) workflow. No manual tag creation is needed.

Monitor and verify:

1. Approve the controller's **Approve release reconciliation** job in the `release` environment after reviewing the version and source commit shown in the workflow summary.
2. The [release controller](https://github.com/radius-project/radius/actions/workflows/release-controller.yaml) completes successfully and creates the `vX.Y.Z` [tag](https://github.com/radius-project/radius/tags) last.
3. The [release build](https://github.com/radius-project/radius/actions/workflows/build-release.yaml) workflow (triggered by the tag push) completes successfully. Its summary must show successful GoReleaser, Helm, Bicep types, and finalization jobs.
4. A final release (not pre-release) appears on [GitHub Releases](https://github.com/radius-project/radius/releases), and the full-version, `X.Y`, and `latest` production image and CLI OCI tags resolve to the same recorded digests.

### Step 7: Publish docs and samples

1. In `radius-project/docs`, run the [Release docs](https://github.com/radius-project/docs/actions/workflows/release.yaml) workflow from the `edge` branch. Enter the version number without the `v` prefix (e.g., `0.56.0`).

2. In `radius-project/samples`, run the [Release samples](https://github.com/radius-project/samples/actions/workflows/release.yaml) workflow from the `edge` branch. Enter the version number without the `v` prefix (e.g., `0.56.0`).

### Step 8: Run validation workflows

1. In `radius-project/radius`, run the [Release verification](https://github.com/radius-project/radius/actions/workflows/release-verification.yaml) workflow from the `release/X.Y` branch. Enter the final version number without the `v` prefix as the version (e.g., `0.56.0`).

2. In `radius-project/samples`, run the [Test Samples](https://github.com/radius-project/samples/actions/workflows/test.yaml) workflow from the `edge` branch. Enter the final version number without the `v` prefix as the version (e.g., `0.56.0`).

   > If tests fail, check logs and existing issues in the samples repo. Flaky tests may pass on re-run. If failures persist, file an issue and raise it with maintainers.

If all workflows pass, the release is complete. Post a final update in the Teams release thread announcing the successful release and summarizing the timeline.

## Patching

Use this process to fix a bug in an already-released version.

Before preparing a patch, create and push the signed [Deployment Engine](https://github.com/azure-octo/deployment-engine) tag with the patch version, following the same process as the [final release](#step-2-tag-the-deployment-engine-1). This tag is required even when Deployment Engine source did not change because it identifies the downstream image used by the Radius release. Prepare Release verifies the signature before changing labels or files.

### Step 1: Start a Teams release thread

Start a new thread in the team's Microsoft Teams release channel titled with the patch version (e.g., "Patch Release v0.56.1"). As with RC and final releases, log every action, result, and issue in this thread throughout the patch release process.

### Step 2: Merge the fix to main

Open a PR with the bug fix targeting `main`. After approval, squash-merge it, add the `backport release/X.Y` label, and rebase-merge the generated backport pull request before preparing the patch.

### Step 3: Prepare the patch release

Run the [Prepare Release](https://github.com/radius-project/radius/actions/workflows/prepare-release.yaml) workflow from `main` with `release-type` set to `patch` and the target `X.Y` channel. The workflow verifies that every selected fix has a merged backport, increments the channel's patch version, generates patch notes, and opens a signed draft release pull request.

### Step 4: Review and merge the generated pull requests

Review the version, included fixes, changelog, and expected outputs. Mark the release pull request ready and squash-merge it to `main`. Then verify the generated release backport and **rebase-merge** it to `release/X.Y`.

### Step 5: Verify the automated release

After the generated release backport is merged to `release/X.Y`, the [release controller](https://github.com/radius-project/radius/actions/workflows/release-controller.yaml) binds the approved plan to that exact commit, publishes and verifies prerequisites, and creates the `vX.Y.Z` Radius tag last. The App-created tag triggers the [release build](https://github.com/radius-project/radius/actions/workflows/build-release.yaml) workflow. No manual tag creation is needed.

Monitor and verify:

1. Approve the controller's **Approve release reconciliation** job in the `release` environment after reviewing the version and source commit shown in the workflow summary.
2. The [release controller](https://github.com/radius-project/radius/actions/workflows/release-controller.yaml) completes successfully and creates the `vX.Y.Z` [tag](https://github.com/radius-project/radius/tags) last.
3. The [release build](https://github.com/radius-project/radius/actions/workflows/build-release.yaml) workflow (triggered by the tag push) completes successfully. Its summary must show successful GoReleaser, Helm, Bicep types, and finalization jobs.
4. A patch release appears on [GitHub Releases](https://github.com/radius-project/radius/releases), and the full-version, `X.Y`, and `latest` production image and CLI OCI tags resolve to the same recorded digests.

### Step 6: Run validation workflows

1. In `radius-project/radius`, run the [Release verification](https://github.com/radius-project/radius/actions/workflows/release-verification.yaml) workflow from the `release/X.Y` branch. Enter the patch version number without the `v` prefix as the version (e.g., `0.56.1`).

2. In `radius-project/samples`, run the [Test Samples](https://github.com/radius-project/samples/actions/workflows/test.yaml) workflow from the `edge` branch. Enter the patch version number without the `v` prefix as the version (e.g., `0.56.1`).

   > If tests fail, check logs and existing issues in the samples repo. Flaky tests may pass on re-run. If failures persist, file an issue and raise it with maintainers.

If all workflows pass, the patch release is complete. Post a final update in the Teams release thread announcing the successful patch and summarizing the timeline.

## Break-glass manual preparation

> **Temporary fallback:** Use this path only when Prepare Release or release-backport automation is unavailable. It remains until the final migration sweep. Record why break-glass was required in the Teams release thread and open an issue for the automation failure.

1. Choose the release type and channel strictly from the [version policy](#terminology). Install the release tools with `make install-yq install-jq install-git-cliff`.
2. For an existing channel, backport and rebase-merge every required product change to `release/X.Y` before generating the plan. The release branch tip at preparation time becomes the immutable product commit; do not advance it afterward.
3. From a clean branch named `automation/prepare-release-<version-without-v>` at `main`, collect the approved inputs and run the same trusted preparation scripts used by the workflow:

    ```bash
    bash .github/scripts/collect-release-backports.sh \
       --repository radius-project/radius --channel X.Y \
       --output /tmp/release-backports.json
    bash .github/scripts/capture-release-sibling-commits.sh \
       --channel X.Y --output /tmp/release-siblings.json
    bash .github/scripts/prepare-release.sh \
       --release-type <rc|final|patch> --channel X.Y \
       --backports-file /tmp/release-backports.json \
       --sibling-repositories-file /tmp/release-siblings.json \
       --output-dir /tmp/release-preparation
    ```

4. Commit only `versions.yaml`, `CHANGELOG.md`, the generated `docs/release-notes/vX.Y.Z[-rc.N].md`, and `.github/release-plans/vX.Y.Z[-rc.N].yaml`. Open a pull request against `main` using `/tmp/release-preparation/pr-title.txt` as its title and `/tmp/release-preparation/release-pr-body.md` as its body. The normal Release plan check must pass; record its pull request number, then squash-merge it.
5. For a first RC, stop here; the release controller creates the release branch from the approved `main` commit. For an existing channel, use the tested backport helper to pin the exact product commit, generate the required PR-body markers, preserve the cherry-pick trailer, and remove a branch-local legacy release workflow:

   ```bash
    version=vX.Y.Z-rc.N # Or vX.Y.Z for a final or patch release
    channel=X.Y
    release_pr=<RELEASE_PR_NUMBER>
    release_commit=<RELEASE_PREPARATION_COMMIT>
    product_commit="$(yq -r '.source.productCommit' \
       ".github/release-plans/${version}.yaml")"
    output=/tmp/release-backport

    git fetch origin "${release_commit}" \
       "refs/heads/release/${channel}:refs/remotes/origin/release/${channel}"
    rm -rf "${output}"
    bash .github/scripts/create-release-backport.sh \
       --source-pr "${release_pr}" --source-commit "${release_commit}" \
       --source-title "chore(release): prepare ${version}" \
       --source-url \
          "https://github.com/radius-project/radius/pull/${release_pr}" \
       --channel "${channel}" --expected-base "${product_commit}" \
       --output-dir "${output}"

    test "$(cat "${output}/status.txt")" = success || {
       cat "${output}/conflict-handoff.md"
       exit 1
    }
    branch="$(cat "${output}/branch.txt")"
    git switch -c "${branch}"
    git commit --file "${output}/commit-message.txt" \
       --author "$(cat "${output}/author.txt")"
    git push origin "${branch}"
    gh pr create --base "release/${channel}" --head "${branch}" \
       --title "$(cat "${output}/title.txt")" \
       --body-file "${output}/pull-request-body.md"
   ```

6. Verify that the backport pull request's parent is the plan's frozen product commit and that it changes only release metadata plus the optional legacy-workflow deletion, then **rebase-merge** it. The helper-generated branch name and body markers let the controller recognize the merge automatically. Never move or replace an existing release tag.

## After every release

### Review and improve this document

Review this document while the release experience is fresh. Identify any stale instructions, missing details, unclear language, unnecessary complexity, or troubleshooting guidance that would improve the next release. Submit a pull request against `main` with the updates.
