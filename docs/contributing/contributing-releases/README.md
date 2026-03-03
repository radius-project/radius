# How to create and publish a Radius release

## Prerequisites

- Determine the release version number. This is in the form `<major>.<minor>.<patch>`

## Terminology

- **RC Release**: A release candidate that we can test internally before releasing to the public which we can run validation on. If we find issues in validation, we can create additional RC releases until we feel confident in the release. Example: `v0.21.0-rc1` or `v0.21.0-rc2`
- **Final Release**: A release that is ready to be published to the public. Example: `v0.21.0`
- **Patch Release**: A release that contains bug fixes and patches for an already-created release. Example: `v0.21.1`
- **Release Branch**: A branch in the `radius-project/radius` repo that contains the release version. Example: `release/0.21`

## How releases work

Each release belongs to a _channel_ named `<major>.<minor>`. Releases will only interact with assets from their channel. For example, the `0.1` `rad` CLI will:

- Create an environment using the `0.1` version of the RP and environment setup script

> ⚠️ Compatibility ⚠️
> At this time we do not guarantee compatibility across releases or provide a migration path. For example, the behavior of a `0.1` `rad` CLI talking to a `0.2` control plane is unspecifed. We expect the project to change too frequently to provide compatibility guarantees at this time.

Conceptually we scope channels to a major+minor pair because this allows us to freely patch assets as needed without needing to change the intermediate pieces. For example pushing a `v0.1.1` tag will update the assets in the `v0.1` channel. This works as long as it is a _true_ patch release and maintains compatibility.

## Cadence

We follow a monthly release cadence. Any contributions that have been merged through the pull-request process will be present in the next scheduled release.

## Release Process

For the entire release process, directly clone repositories under the radius-project organization and create branches off of them. Do not create branches in your personal forks when creating pull requests. This is due to an error accessing the GitHub token when forks are used.

### Cherry-pick workflow (applies to RC, final, and patch releases)

All three release flows (RC, final, patch) follow the same pattern: changes are **first merged to `main`**, then **cherry-picked onto the release branch** (`release/x.y`). The release branch is what actually gets tagged and built.

| Release type | What gets cherry-picked to the release branch                                                                      |
|--------------|--------------------------------------------------------------------------------------------------------------------|
| **RC**       | `versions.yaml` update + optionally any additional commits (bug fixes, late features) that must be part of this RC |
| **Final**    | A single commit containing the version bump (`v0.X.0-rcN` → `v0.X.0`) and release notes                            |
| **Patch**    | Bug-fix commit(s) + `versions.yaml` update                                                                         |

> Always use `git cherry-pick -x` to preserve version history.

### Creating an RC release

When starting the release process, we first kick it off by creating an RC release. If we find issues in validation, we can create additional RC releases until we feel confident in the release.

> **Key concept:** The RC release is built from the **release branch** (`release/x.y`), not directly from `main`. Changes are first merged to `main` and then cherry-picked to the release branch. This applies to the `versions.yaml` update as well as any optional commits (bug fixes, late features) that must be included in the RC.

Follow the steps below to create an RC release.

1. Run the following commands in a local clone of the [Deployment Engine Repo](https://github.com/azure-octo/deployment-engine), replacing `vX.Y.Z-rc1` with the rc release version:

```bash
git checkout main
git pull origin main
git tag vX.Y.Z-rc1
git push origin vX.Y.Z-rc1
```

> Note: `azure-octo` org requires the "verified" tag on git tags ([read more](https://docs.github.com/en/authentication/managing-commit-signature-verification/displaying-verification-statuses-for-all-of-your-commits)), so you will have to [set up GPG signing locally](https://docs.github.com/en/authentication/managing-commit-signature-verification/generating-a-new-gpg-key).
>
> Note: This is a temporary workaround. We should ideally run the [Deployment Engine Release Workflow](https://github.com/azure-octo/deployment-engine/actions/workflows/release.yaml) workflow, but the GPG signing is not set up. Issue ref: <https://github.com/azure-octo/deployment-engine/issues/456>

1. Clone the [radius-project/radius](https://github.com/radius-project/radius) repo locally, or use your existing local copy.

   ```bash
   git clone git@github.com:radius-project/radius.git
   ```

1. Create a new branch from `main`.

   ```bash
   git checkout main
   git checkout -b <USERNAME>/release-<MAJOR>.<MINOR>.0-rc<N>
   ```

1. In your local branch, update the `versions.yaml` file to add the new release candidate as a supported version that we would like to release. The `versions.yaml` file is a declarative version tracking file that the Radius community maintains ([Example](https://github.com/radius-project/radius/pull/6077/files)).

   Example:

   ```yaml
   supported:
   - channel: '0.41'
      version: 'v0.41.0-rc1'
   - channel: '0.40'
      version: 'v0.40.0'
   deprecated:
   - channel: '0.39'
      version: 'v0.39.0'
   ```

1. Push these changes to a remote branch and create a pull request against `main`.

   ```bash
   git push origin <USERNAME>/<BRANCHNAME>
   ```

1. After maintainer approval, merge the pull request to `main`.

1. Cherry-pick the `versions.yaml` commit - and optionally any additional commits that must be included in this RC - into the release branch. Create a new branch from the release branch (format: `release/x.y`) and open a PR targeting the release branch.

   ```bash
   git checkout release/0.<MINOR>
   git pull origin release/0.<MINOR>
   git checkout -b <USERNAME>/cherry-pick-rc<N>-to-release-branch
   ```

   Cherry-pick the `versions.yaml` change (required):

   ```bash
   git cherry-pick -x <VERSIONS_YAML_COMMIT_HASH>
   ```

   **(Optional)** If specific bug fixes or other commits from `main` need to be included in this RC, cherry-pick them as well:

   ```bash
   git cherry-pick -x <OPTIONAL_COMMIT_HASH_1>
   git cherry-pick -x <OPTIONAL_COMMIT_HASH_2>
   ```

   > PLEASE USE `-x` HERE TO ENSURE VERSION HISTORY IS PRESERVED.
   >
   > You can get commit hashes by running `git log --oneline main`.

   Push and create a PR targeting the release branch:

   ```bash
   git push origin <USERNAME>/cherry-pick-rc<N>-to-release-branch
   ```

1. After maintainer approval, merge the cherry-pick PR into the release branch.

1. You may need to wait around ~20 minutes for the release assets to be built and published.

1. There should be a GitHub workflow run in progress [here](https://github.com/radius-project/radius/actions/workflows/build.yaml) that was triggered by the `vX.Y.Z-rc<N>` tag. Monitor this workflow to ensure that it completes successfully. If it does, then the release candidate has been created.

1. In the `bicep-types-aws` repository, there should be a run of the `Update extensibility provider types` job that was triggered by the `vX.Y.Z-rc<N>` tag that needs to be approved and started. Approve the run of this job and monitor it to ensure that it completes successfully.

1. Verify that an RC release was created on GitHub Releases for the current version ([Example](https://github.com/radius-project/radius/releases)).

1. In the `radius-project/radius` repo, run the [Release verification](https://github.com/radius-project/radius/actions/workflows/release-verification.yaml) workflow. Run the workflow from the release branch (format: `release/x.y`) and use the Radius RC release version number being released.

1. In the `radius-project/docs` repo, run the [Upmerge docs to edge](https://github.com/radius-project/docs/actions/workflows/upmerge.yaml) workflow. Run the workflow from the current branch (e.g. if you are working on release `v0.35`, then you'd run this workflow from the `v0.34` branch).

   > This workflow will generate a PR which you will need to get approval and merge before proceeding. The PR will not include changes to `docs/config.toml` and `docs/layouts/partials/hooks/body-end.html`, because those files are specific to the branch.

1. In the `radius-project/samples` repo, run the [Upmerge samples to edge](https://github.com/radius-project/samples/actions/workflows/upmerge.yaml) workflow. Run the workflow from the current branch (e.g. if you are working on release `v0.35`, then you'd run this workflow from the `v0.34` branch).

   > This workflow will generate a PR which you will need to get approval and merge before proceeding. The PR will not include changes to `bicepconfig.json` because that file is specific to the branch.

1. In the `radius-project/samples` repo, run the [Test Samples](https://github.com/radius-project/samples/actions/workflows/test.yaml) workflow. Run the workflow from the `edge` branch and using the Radius RC release version number being released.

   > The `Test Samples` workflow should only be run once the upmerge PR has been merged to `edge`.
   > If this workflow run fails, then there should be further investigation. Try checking the logs to see what failed and why, and checking if there is already an issue open for this failure in the samples repo. Sometimes, the workflow run will fail because of flaky tests. Try re-running, and if the failure is persistent, then file an issue in the samples repo and raise it with the maintainers.

1. If these workflows pass, then the release candidate has been successfully created and validated. We can now proceed to creating the final release. If the workflows fail, then we need to fix the issues and create a new RC release (increment the RC number, e.g. `rc2`, `rc3`, etc.) by repeating the steps above.

### Creating the final release

Once an RC release has been created and validated, we can proceed to creating the final release.

> **Key principle:** The final release is built directly from the **last validated RC** on the release branch. Since the RC has already been tested and accepted, the only change needed on the release branch is a single cherry-pick that bumps the version in `versions.yaml` from the RC version (e.g. `v0.41.0-rc3`) to the final version (e.g. `v0.41.0`) and adds the release notes. This minimizes risk by ensuring the final release contains exactly the same code as the validated RC, plus only the version bump.

Follow the steps below to create a final release.

1. Run the following commands in a local clone of the [Deployment Engine Repo](https://github.com/azure-octo/deployment-engine), replacing `vX.Y.Z` with the release version:

```bash
git checkout main
git pull origin main
git tag vX.Y.Z
git push origin vX.Y.Z
```

> Note: `azure-octo` org requires the "verified" tag on git tags ([read more](https://docs.github.com/en/authentication/managing-commit-signature-verification/displaying-verification-statuses-for-all-of-your-commits)), so you will have to [set up GPG signing locally](https://docs.github.com/en/authentication/managing-commit-signature-verification/generating-a-new-gpg-key).
>
> Note: This is a temporary workaround. We should ideally run the [Deployment Engine Release Workflow](https://github.com/azure-octo/deployment-engine/actions/workflows/release.yaml) workflow, but the GPG signing is not set up. Issue ref: <https://github.com/azure-octo/deployment-engine/issues/456>

1. Move to your local copy of the `radius-project/radius` repo.

1. Create a new branch from `main`.

1. In your local branch, update the `versions.yaml` file to reflect the final release version (changing from the RC version to the final version, e.g. `v0.41.0-rc3` → `v0.41.0`) ([Example](https://github.com/radius-project/radius/pull/6992/files#diff-1c4cd801df522f4a92edbfb0fea95364ed074a391ea47c284ddc078f512f7b6a)).

1. In this same branch, create a new release note document in the [release-notes](../../release-notes/) directory. There will be an automatically-generated release notes comment on the PR - use it as a starting point. Follow the directory's `README.md` for instructions on how to create a new release note document ([Example](https://github.com/radius-project/radius/pull/6092/files)).

1. Commit both the `versions.yaml` change and the release notes in a **single commit**. This is important because only one cherry-pick will be applied to the release branch.

1. Push these changes to a remote branch and create a pull request against `main`.

1. After maintainer approval, merge the pull request to `main`.

1. Cherry-pick **only this single commit** (containing the version bump and release notes) onto the release branch. Since the release branch already contains the validated RC code, this is the only change needed. Do **not** cherry-pick any other commits - the goal is for the final release to be identical to the last accepted RC except for the version number and release notes.

   ```bash
   git checkout release/0.<MINOR>
   git pull origin release/0.<MINOR>
   git checkout -b <USERNAME>/final-release-<MAJOR>.<MINOR>.0
   git cherry-pick -x <COMMIT_HASH>
   ```

   > PLEASE USE `-x` HERE TO ENSURE VERSION HISTORY IS PRESERVED.
   >
   > You can get the commit hash by running `git log --oneline main`.

   Push and create a PR targeting the release branch ([Example](https://github.com/radius-project/radius/pull/6114/files)):

   ```bash
   git push origin <USERNAME>/final-release-<MAJOR>.<MINOR>.0
   ```

1. After maintainer approval, merge the pull request into the release branch.

1. There should be a GitHub workflow run in progress [here](https://github.com/radius-project/radius/actions/workflows/build.yaml) that was triggered by the `vX.Y.Z` tag. Monitor this workflow to ensure that it completes successfully.

1. In the `bicep-types-aws` repository, there should be a run of the `Update extensibility provider types` job that was triggered by the `vX.Y.Z` tag that needs to be approved and started. Approve the run of this job and monitor it to ensure that it completes successfully.

1. You may need to wait around ~20 minutes for the release assets to be published.

1. Verify that a release was created on GitHub Releases for the current version ([Example](https://github.com/radius-project/radius/releases)).

1. In the project-radius/docs repository, run the [Release docs](https://github.com/radius-project/docs/actions/workflows/release.yaml) workflow. Use the workflow from the edge branch and add the Radius version number (X.Y.Z) that is being released.

1. In the project-radius/samples repository, run the [Release samples](https://github.com/radius-project/samples/actions/workflows/release.yaml) workflow. Use the workflow from the edge branch and add the Radius version number (X.Y.Z) that is being released.

1. In the `radius-project/radius` repo, run the [Release verification](https://github.com/radius-project/radius/actions/workflows/release-verification.yaml) workflow. Run the workflow from the release branch (format: `release/x.y`) and use the Radius release version number being released.

1. In the `radius-project/samples` repo, run the [Test Samples](https://github.com/radius-project/samples/actions/workflows/test.yaml) workflow. Run the workflow from the `edge` branch and using the Radius release version number being released.

   > If this workflow run fails, then there should be further investigation. Try checking the logs to see what failed and why, and checking if there is already an issue open for this failure in the samples repo. Sometimes, the workflow run will fail because of flaky tests. Try re-running, and if the failure is persistent, then file an issue in the samples repo and raise it with the maintainers.

1. If these workflows pass, then the release has been successfully created and validated. If the workflows fail, then we need to fix the issues and create a new release.

## Patching

Let's say we have a bug in a release that needs to be patched for an already-created release.

1. Merge the bug fix into the `main` branch of the repo that needs to be fixed.

1. Once these changes are merged into `main`, create a new branch from `main` in the repo that needs to be patched. Update `versions.yaml` to reflect the new patch version that we would like to release.

1. Push these changes to a remote branch and create a pull request against `main`.

1. After maintainer approval, merge the pull request to `main`.

1. Create a new branch from the release branch we want to patch. The release branch should already exist in the repo. Release branches are in the format `release/x.y`.

   ```bash
   git checkout release/0.<VERSION>
   git checkout -b <USERNAME>/<BRANCHNAME>
   ```

1. Cherry-pick the bug fix as well as the `versions.yaml` changes from the previous steps in this PR. This will ensure that the version changes are included in the release branch. You can get the commit hash by running `git log --oneline` in the main branch. PLEASE USE `-x` HERE TO ENSURE VERSION HISTORY IS PRESERVED.

   ```bash
   git cherry-pick -x <BUGFIX_COMMIT HASH>
   git cherry-pick -x <VERSIONFILE_COMMIT HASH>
   ```

1. Create a PR to merge into the release branch in the repo that needs to be patched.

1. After maintainer approval, merge the pull request into the release branch.

1. There should be a GitHub workflow run in progress [here](https://github.com/radius-project/radius/actions/workflows/build.yaml) that was triggered by the `vX.Y.Z` tag. Monitor this workflow to ensure that it completes successfully.

1. You may need to wait around ~20 minutes for the release assets to be published.

1. Verify that a release was created on GitHub Releases for the current version ([Example](https://github.com/radius-project/radius/releases)).

1. In the `radius-project/radius` repo, run the [Release verification](https://github.com/radius-project/radius/actions/workflows/release-verification.yaml) workflow. Run the workflow from the release branch (format: `release/x.y`) and use the Radius release version number being released.

1. In the `radius-project/samples` repo, run the [Test Samples](https://github.com/radius-project/samples/actions/workflows/test.yaml) workflow. Run the workflow from the `edge` branch and using the Radius release version number being released.

   > If this workflow run fails, then there should be further investigation. Try checking the logs to see what failed and why, and checking if there is already an issue open for this failure in the samples repo. Sometimes, the workflow run will fail because of flaky tests. Try re-running, and if the failure is persistent, then file an issue in the samples repo and raise it with the maintainers.

1. If these workflows pass, then the release has been successfully created and validated. If the workflows fail, then we need to fix the issues and create a new release.
