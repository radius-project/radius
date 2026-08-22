<!-- markdownlint-disable MD024 -->

# How to create and publish a Radius release

## Purpose

This document is the maintainers' reference for cutting and publishing a Radius release across the `radius-project/radius`, `radius-project/docs`, `radius-project/samples`, and `azure-octo/deployment-engine` repositories. It covers release candidates, final releases, and patch releases, and the branching, tagging, and validation steps each requires. It is intended for project maintainers with release responsibility, not day-to-day contributors.

## Prerequisites

Before starting a release, ensure you have:

- **Release version number**: Determine the version in the form `<major>.<minor>.<patch>` (e.g., `0.56.0`).
- **Repository access**: Write access to `radius-project/radius`, `radius-project/docs`, `radius-project/samples`, and `azure-octo/deployment-engine`.
- **GPG signing configured**: The `azure-octo` org requires [verified tags](https://docs.github.com/en/authentication/managing-commit-signature-verification/displaying-verification-statuses-for-all-of-your-commits). [Set up GPG signing locally](https://docs.github.com/en/authentication/managing-commit-signature-verification/generating-a-new-gpg-key) before starting.
- **Local clone of `radius-project/radius`**: Clone directly from the organization repo, not a personal fork. CI workflows require access to organization secrets that are not available in forks.

  ```bash
  git clone git@github.com:radius-project/radius.git
  ```

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

Two GitHub Actions workflows drive the release process. **No one manually creates tags in `radius-project` repos.** Tags for repos in the `radius-project` organization are created automatically by the `release.yaml` workflow. (The [Deployment Engine repo](https://github.com/azure-octo/deployment-engine) in the `azure-octo` organization still requires manual tagging — see the release steps below.)

1. **[Release Radius](https://github.com/radius-project/radius/actions/workflows/release.yaml)** (`release.yaml`): Triggered whenever `versions.yaml` is pushed to `main` or a `release/*` branch. This workflow:
   - Scans `versions.yaml` in `.supported[]` order and selects the first `.version` whose tag is missing from any of `radius`, `recipes`, `dashboard`, or `bicep-types-aws`
   - **Automatically creates and pushes the version tag** (e.g., `v0.56.0-rc.1`) for `radius`, `recipes`, `dashboard`, and `bicep-types-aws`
   - Creates the release branch (`release/<channel>`) if it does not already exist
   - Dispatches Deployment Engine image publishing to GHCR
   - Reconciles matching existing branches and tags as successful state, rejects conflicting tag targets, and resumes any repositories left incomplete by a failed run
   - Waits on a `main` push only when the release branch exists but does not yet contain the triggering commit (this prevents duplicate work before the `versions.yaml` change is cherry-picked)

   > **Note**: The workflow always checks out and reads `versions.yaml` from `main`, even when triggered by a push to a `release/*` branch. This means the version must be merged into `main` before the cherry-pick to the release branch triggers tag creation.
   >
   > **Important**:
   >
   > - Add the new release version at the top of the `supported` list in `versions.yaml`.
   > - Change only one version per PR.
   > - If more than one incomplete version is present in `supported`, `release.yaml` fails rather than guessing which one to release.
2. **[Release build](https://github.com/radius-project/radius/actions/workflows/build-release.yaml)** (`build-release.yaml`): Triggered by `v*` tag pushes (created by `release.yaml` above). This workflow:
   - Builds CLI binaries and container images
   - Dispatches Bicep types publishing
   - Creates the GitHub Release (auto-generated notes for RCs, or from `docs/release-notes/` for final and patch releases)

   During the GoReleaser migration, this workflow also runs advisory shadow jobs. They publish full-version production-image candidates only under `ghcr.io/radius-project/dev`, upload candidate CLI binaries and checksums as workflow artifacts, and compare them with the production outputs from the same tag. The parity report is attached as `goreleaser-shadow-parity-<commit>`. A shadow failure does not block or alter the current production release; inspect and resolve unexplained differences before the GoReleaser cutover.

The automated flow after merging a `versions.yaml` change:

```text
Merge versions.yaml change (to main or release/* branch)
  → release.yaml detects new version in versions.yaml
    → release.yaml creates git tag + release branch (if needed)
      → tag push triggers build-release.yaml
        → build-release.yaml publishes artifacts + creates GitHub Release
```

#### When does tag creation happen?

| Scenario          | Trigger                                      | What happens                                                                |
|-------------------|----------------------------------------------|-----------------------------------------------------------------------------|
| **First RC**      | `versions.yaml` merged to `main`             | `release.yaml` creates the release branch from `main` and pushes the RC tag |
| **Subsequent RC** | `versions.yaml` cherry-picked to `release/*` | `release.yaml` runs on the release branch and pushes the new RC tag         |
| **Final release** | Version bump cherry-picked to `release/*`    | `release.yaml` runs on the release branch and pushes the final tag          |
| **Patch release** | `versions.yaml` cherry-picked to `release/*` | `release.yaml` runs on the release branch and pushes the patch tag          |

### Cherry-pick workflow

All release types follow the same pattern: changes merge to `main` first, then cherry-pick to the release branch (`release/<channel>`). The release branch is what gets tagged and built.

| Release type      | What to cherry-pick to the release branch                         |
|-------------------|-------------------------------------------------------------------|
| **First RC**      | Nothing — the release branch is created automatically from `main` |
| **Subsequent RC** | `versions.yaml` update + any additional bug fixes                 |
| **Final release** | A single commit with the version bump and release notes           |
| **Patch release** | Bug-fix commits + `versions.yaml` update + patch release notes    |

> Always use `git cherry-pick -x` to preserve traceability.
>
> **Key concept:** The RC release is built from the **release branch** (`release/x.y`), not directly from `main`. After the initial RC is created, the release branch is used for subsequent RCs and for the final release. Changes for RC-2 and all subsequent RCs are first merged to `main` and then cherry-picked to the release branch. This applies to the `versions.yaml` update as well as any optional commits (bug fixes, late features) that must be included in the RC.

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
git tag vX.Y.Z-rc.N
git push origin vX.Y.Z-rc.N
```

> **Note**: This manual tagging step is a temporary workaround. Ideally the [Deployment Engine Release Workflow](https://github.com/azure-octo/deployment-engine/actions/workflows/release.yaml) would handle this, but GPG signing is not yet configured there. See [azure-octo/deployment-engine#456](https://github.com/azure-octo/deployment-engine/issues/456).

### Step 3: Update versions.yaml

Create a branch from `main` in the `radius-project/radius` repo:

```bash
git checkout main
git pull origin main
git checkout -b <USERNAME>/release-X.Y.0-rc.N
```

Edit `versions.yaml` to add the new RC as a supported version. Move the oldest supported version to the `deprecated` list if needed ([example PR](https://github.com/radius-project/radius/pull/6077/files)).

```yaml
supported:
  - channel: '0.56'
    version: 'v0.56.0-rc.1'
  - channel: '0.55'
    version: 'v0.55.0'
deprecated:
  - channel: '0.54'
    version: 'v0.54.0'
```

### Step 4: Merge to main

Push the branch and create a PR against `main`:

```bash
git push origin <USERNAME>/release-X.Y.0-rc.N
```

After approval, merge the PR to `main`.

### Step 5: Verify the automated release

After merging, the [Release Radius](https://github.com/radius-project/radius/actions/workflows/release.yaml) workflow automatically runs because `versions.yaml` changed on `main`.

- **First RC**: The workflow creates the `release/X.Y` branch from `main` and pushes the `vX.Y.Z-rc.N` tag. The tag push then triggers the [release build](https://github.com/radius-project/radius/actions/workflows/build-release.yaml) workflow. No manual tag creation is needed. Verify the release using the checklist below.
- **Subsequent RCs**: The workflow detects that the release branch already exists and **skips tag creation**. This is expected — the tag will be created when the cherry-pick lands on the release branch in [Step 6](#step-6-cherry-pick-additional-changes-subsequent-rcs-only). Skip ahead to Step 6 for now and return to verify after completing it.

Monitor and verify:

1. The [Release Radius](https://github.com/radius-project/radius/actions/workflows/release.yaml) workflow completes successfully. For the first RC, confirm it created the `release/X.Y` [branch](https://github.com/radius-project/radius/branches) and the `vX.Y.Z-rc.N` [tag](https://github.com/radius-project/radius/tags).
2. The [release build](https://github.com/radius-project/radius/actions/workflows/build-release.yaml) workflow (triggered by the tag push) completes successfully. This workflow also dispatches Bicep types publishing automatically.
3. An RC release marked as pre-release appears on [GitHub Releases](https://github.com/radius-project/radius/releases).

### Step 6: Cherry-pick additional changes (subsequent RCs only)

> **Skip this step for the first RC.** The release branch was just created from `main` and already contains all changes.

For subsequent RCs (`rc.2`, `rc.3`, etc.), cherry-pick the `versions.yaml` update and any bug fixes onto the release branch:

```bash
git checkout release/X.Y
git pull origin release/X.Y
git checkout -b <USERNAME>/cherry-pick-rc.N-to-release-branch
git cherry-pick -x <VERSIONS_YAML_COMMIT_HASH>
git cherry-pick -x <OPTIONAL_FIX_COMMIT_HASH>
```

> Use `git log --oneline main` to find commit hashes.

Push and create a PR targeting the release branch:

```bash
git push origin <USERNAME>/cherry-pick-rc.N-to-release-branch
```

After approval, merge the PR. This triggers the release automation on the release branch, creating the new RC tag. Return to [Step 5](#step-5-verify-the-automated-release) to verify the release completed successfully.

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

The final release is built from the **last validated RC** on the release branch. The only change needed is a single cherry-pick that bumps the version and adds release notes. This ensures the final release contains exactly the same code as the validated RC.

### Step 1: Update the Teams release thread

Post an update in the Teams release thread (started during the [RC release](#step-1-start-a-teams-release-thread)) indicating that the final release process is beginning. Continue logging every action, result, and issue in this thread throughout the final release steps.

### Step 2: Tag the Deployment Engine

Run the following in a local clone of the [Deployment Engine repo](https://github.com/azure-octo/deployment-engine), replacing `vX.Y.Z` with the final version (e.g., `v0.56.0`):

```bash
git checkout main
git pull origin main
git tag vX.Y.Z
git push origin vX.Y.Z
```

> **Note**: Same temporary workaround as for [RC releases](#step-2-tag-the-deployment-engine). See [azure-octo/deployment-engine#456](https://github.com/azure-octo/deployment-engine/issues/456).

### Step 3: Update versions.yaml and create release notes

Create a branch from `main`:

```bash
git checkout main
git pull origin main
git checkout -b <USERNAME>/final-release-X.Y.0
```

1. **Update `versions.yaml`**: Change the RC version to the final version ([example PR](https://github.com/radius-project/radius/pull/6992/files#diff-1c4cd801df522f4a92edbfb0fea95364ed074a391ea47c284ddc078f512f7b6a)).

   ```yaml
   supported:
     - channel: '0.56'
       version: 'v0.56.0'   # was v0.56.0-rc.1
   ```

2. **Create a draft release notes file**: Add `docs/release-notes/vX.Y.Z.md` using the [release notes template](../../release-notes/template.md). See the [release notes README](../../release-notes/README.md) for instructions ([example PR](https://github.com/radius-project/radius/pull/6092/files)).

3. **Push and create a PR** against `main`. The PR will receive an auto-generated release notes comment — use it to fill in the changelog and contributor list in your release notes file. Push an update with the completed release notes.

> The PR will be squash-merged into a single commit on `main`, which is the commit you will cherry-pick to the release branch.

### Step 4: Merge to main

After approval, squash-merge the PR.

### Step 5: Cherry-pick to the release branch

Cherry-pick the squash-merged commit (version bump + release notes) onto the release branch.

```bash
git checkout release/X.Y
git pull origin release/X.Y
git checkout -b <USERNAME>/final-release-X.Y.0-cherry-pick
git cherry-pick -x <COMMIT_HASH>
```

> Use `git log --oneline main` to find the commit hash.

Push and create a PR targeting the release branch ([example PR](https://github.com/radius-project/radius/pull/6114/files)):

```bash
git push origin <USERNAME>/final-release-X.Y.0-cherry-pick
```

After approval, merge the PR.

### Step 6: Verify the automated release

After the cherry-pick PR is merged to the `release/X.Y` branch, the [Release Radius](https://github.com/radius-project/radius/actions/workflows/release.yaml) workflow automatically runs because `versions.yaml` changed on a `release/*` branch. It reads the final version from `versions.yaml`, creates and pushes the `vX.Y.Z` tag, and the tag push triggers the [release build](https://github.com/radius-project/radius/actions/workflows/build-release.yaml) workflow. No manual tag creation is needed.

Monitor and verify:

1. The [Release Radius](https://github.com/radius-project/radius/actions/workflows/release.yaml) workflow completes successfully and creates the `vX.Y.Z` [tag](https://github.com/radius-project/radius/tags).
2. The [release build](https://github.com/radius-project/radius/actions/workflows/build-release.yaml) workflow (triggered by the tag push) completes successfully. Allow up to ~20 minutes for release assets to be published.
3. A final release (not pre-release) appears on [GitHub Releases](https://github.com/radius-project/radius/releases).

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

> **Note**: If the patch includes a fix to the [Deployment Engine](https://github.com/azure-octo/deployment-engine), you must also tag the Deployment Engine with the patch version (e.g., `vX.Y.Z`) before proceeding, following the same process as in the [RC](#step-2-tag-the-deployment-engine) and [Final release](#step-2-tag-the-deployment-engine-1) sections.

### Step 1: Start a Teams release thread

Start a new thread in the team's Microsoft Teams release channel titled with the patch version (e.g., "Patch Release v0.56.1"). As with RC and final releases, log every action, result, and issue in this thread throughout the patch release process.

### Step 2: Merge the fix to main

Open a PR with the bug fix targeting `main`. After approval, merge it.

### Step 3: Update versions.yaml and create patch release notes

Create a branch from `main`:

```bash
git checkout main
git pull origin main
git checkout -b <USERNAME>/patch-X.Y.Z
```

1. Update `versions.yaml` to reflect the new patch version (e.g., `v0.56.1`).
2. Create patch release notes at `docs/release-notes/vX.Y.Z.md` using the [patch release notes template](../../release-notes/template_patch.md).

Push and create a PR against `main`:

```bash
git push origin <USERNAME>/patch-X.Y.Z
```

After maintainer approval, merge the PR.

### Step 4: Cherry-pick to the release branch

Cherry-pick the bug fix, the `versions.yaml` update, and the patch release notes onto the release branch:

```bash
git checkout release/X.Y
git pull origin release/X.Y
git checkout -b <USERNAME>/patch-X.Y.Z-cherry-pick
git cherry-pick -x <BUGFIX_COMMIT_HASH>
git cherry-pick -x <VERSIONS_AND_RELNOTES_COMMIT_HASH>
```

> Use `git log --oneline main` to find commit hashes.

Push and create a PR targeting the release branch:

```bash
git push origin <USERNAME>/patch-X.Y.Z-cherry-pick
```

After approval, merge the PR.

### Step 5: Verify the automated release

After the cherry-pick PR is merged to the `release/X.Y` branch, the [Release Radius](https://github.com/radius-project/radius/actions/workflows/release.yaml) workflow automatically runs because `versions.yaml` changed on a `release/*` branch. It reads the patch version from `versions.yaml`, creates and pushes the `vX.Y.Z` tag, and the tag push triggers the [release build](https://github.com/radius-project/radius/actions/workflows/build-release.yaml) workflow. No manual tag creation is needed.

Monitor and verify:

1. The [Release Radius](https://github.com/radius-project/radius/actions/workflows/release.yaml) workflow completes successfully and creates the `vX.Y.Z` [tag](https://github.com/radius-project/radius/tags).
2. The [release build](https://github.com/radius-project/radius/actions/workflows/build-release.yaml) workflow (triggered by the tag push) completes successfully. Allow up to ~20 minutes for release assets to be published.
3. A patch release appears on [GitHub Releases](https://github.com/radius-project/radius/releases).

### Step 6: Run validation workflows

1. In `radius-project/radius`, run the [Release verification](https://github.com/radius-project/radius/actions/workflows/release-verification.yaml) workflow from the `release/X.Y` branch. Enter the patch version number without the `v` prefix as the version (e.g., `0.56.1`).

2. In `radius-project/samples`, run the [Test Samples](https://github.com/radius-project/samples/actions/workflows/test.yaml) workflow from the `edge` branch. Enter the patch version number without the `v` prefix as the version (e.g., `0.56.1`).

   > If tests fail, check logs and existing issues in the samples repo. Flaky tests may pass on re-run. If failures persist, file an issue and raise it with maintainers.

If all workflows pass, the patch release is complete. Post a final update in the Teams release thread announcing the successful patch and summarizing the timeline.

## After every release

### Review and improve this document

Review this document while the release experience is fresh. Identify any stale instructions, missing details, unclear language, unnecessary complexity, or troubleshooting guidance that would improve the next release. Submit a pull request against `main` with the updates.
