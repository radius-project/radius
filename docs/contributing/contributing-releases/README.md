# How to create and publish a Radius release

## Pre-requisites

- Determine the release version number. This is in the form `<major>.<minor>.<patch>`

### Creating an RC release

When starting the release process, we first kick it off by creating an RC release. This is a release candidate that we can test internally before releasing to the public which we can validate samples on.

If we find issues in validation, we can create additional RC releases until we feel confident in the release.

Follow the steps below to create an RC release.

1. Clone the [radius-project/radius](https://github.com/radius-project/radius) repo locally.

1. Create a new branch from `main`.

1. In your local branch, update the `versions.yaml` file to to reflect the new release candidate version that we would like to release. The `versions.yaml` file is a declarative version tracking file that the Radius community maintains. ([Example](https://github.com/radius-project/radius/pull/6077/files))

1. Push these changes to a remote branch and create a pull request against `main`.

1. After maintainer approval, merge the pull request to `main`.

1. There should now be a GitHub workflow run in progress [here](https://github.com/radius-project/radius/actions/workflows/release.yaml). Monitor this workflow to ensure that it completes successfully. If it does, then the release candidate has been created and successfully validated.

### Creating the final release

1. Clone the [radius-project/bicep](https://github.com/radius-project/bicep) repo locally.

1. Create a new branch from `bicep-extensibility`.

1. In your local branch, update the `version.json` file.
Update it to reflect the new release version that we would like to release. ([Example](https://github.com/radius-project/bicep/pull/703/files))

1. Push these changes to a remote branch and create a pull request against `bicep-extensibility`.

1. After maintainer approval, merge the pull request to `bicep-extensiblity`.

1. Create a new branch from the release branch. The release branch has format `release/x.y`. For example, if the release version is 0.1.0, the release branch would be `release/0.1`.

1. Create a PR to merge into the release branch in the bicep repo. Cherry-pick the `version.json` changes from the previous steps in this PR. This will ensure that the version changes are included in the release branch. [Example](https://github.com/radius-project/bicep/pull/704/files)

   ```bash
   git cherry-pick -x <COMMIT HASH>
   ```

1. After maintainer approval, merge the pull request to the release branch.

1. Move to your local copy of the `radius-project/radius` repo.

1. Create a new branch from `main`.

1. In your local branch, update the `versions.yaml` file to to reflect the new release version that we would like to release. ([Example](https://github.com/radius-project/radius/pull/6992/files#diff-1c4cd801df522f4a92edbfb0fea95364ed074a391ea47c284ddc078f512f7b6a))

1. Push these changes to a remote branch and create a pull request against `main`.

1. In this PR, create a new release note document in the [release-notes](../../release-notes/) directory using the automatically generated release notes comment. Follow the directory's README.md for instructions on how to create a new release note document. Include this file in the release version pull request. [Example](https://github.com/radius-project/radius/pull/6092/files)

1. After maintainer approval, merge the pull request to `main`.

1. Create a PR to merge into the release branch (format: release/x.y) in the radius repo. Cherry-pick the `versions.yaml` changes and the release notes from the previous steps in this PR. This will ensure that the version changes and release notes are included in the release branch. [Example](https://github.com/radius-project/radius/pull/6114/files)

   ```bash
   git cherry-pick -x <COMMIT HASH>
   ```

1. After maintainer approval, merge the pull request to `main`.

1. There should now be a GitHub workflow run in progress [here](https://github.com/radius-project/radius/actions/workflows/release.yaml). Monitor this workflow to ensure that it completes successfully. If it does, then the release candidate has been created and successfully validated.

1. Download the Radius Bicep .vsix file from here: https://github.com/radius-project/bicep/releases. Scroll down to the most recent release and download the .vsix file.

1. Upload the Radius Bicep .vsix to the [VS marketplace](https://marketplace.visualstudio.com/manage). You may need access permissions, if so, ask a maintainer. Click on the ... for Radius Bicep, then Update, then upload the .vsix file. The site will verify it then the version number should be updated to the right one.

## How releases work

Each release belongs to a *channel* named `<major>.<minor>`. Releases will only interact with assets from their channel. For example, the `0.1` `rad` CLI will:

- Download `rad-bicep` from the `0.1` channel
- Create an environment using the `0.1` version of the RP and environment setup script

> ⚠️ Compatibility ⚠️
At this time we do not guarantee compatibility across releases or provide a migration path. For example, the behavior of a `0.1` `rad` CLI talking to a `0.2` control plane is unspecifed. We expect the project to change too frequently to provide compatibility guarantees at this time.

Conceptually we scope channels to a major+minor pair because this allows us to freely patch assets as needed without needing to change the intermediate pieces. For example pushing a `v0.1.1` tag will update the assets in the `v0.1` channel. This works as long as it is a *true* patch release and maintains compatibility.

## Patching

Let's say we have a bug in a release that needs to be patched for an already-created release.

1. In the `radius-project/bicep` repo, in the release branch, change the `version.json` version to the new release number. Create a pull request and merge this change.

1. Go through steps 1-4 of "Creating an RC release" above on the `main` branch, substituting the patch release version instead of the final release version. For example, if the final release version number is 0.1.0, the patch release version would be 0.1.1.

1. After creating the pull request, there should be an automatically generated release notes comment. Create a new release note document in the [release-notes](../../release-notes/) directory. Follow the directory's README.md for instructions on how to create a new patch release note document. Include this file in the release version pull request. [Example](https://github.com/radius-project/radius/pull/6092/files)

1. Now we can start patching the release branch. Make sure the commit that we want to add to a patch is merged and validated in `main` first if it affects `main`.

1. Create a new branch based off of the release branch we want to patch. Ex:

   ```bash
   git checkout release/0.<VERSION>
   git checkout -b <USERNAME>/<BRANCHNAME>
   ```

1. Cherry-pick the commit that is on `main` onto the branch. PLEASE USE `-x` HERE TO ENSURE VERSION HISTORY IS PRESERVED.

   ```bash
   git cherry-pick -x <COMMIT HASH>
   ```

1. If breaking changes have been made to our Bicep fork:

   Update the file radius/.github/workflows/validate-bicep.yaml to use the release version (eg. v0.21) instead of edge for validating the biceps in the docs and samples repositories. Also, modify the version from `env.REL_CHANNEL` to `<major>.<minor>` (eg. `0.21`) for downloading the `rad-bicep-corerp`.

1. Cherry-pick the `version.yaml` changes and release notes onto the branch from the PR opened against the main. This will ensure that the release notes are included in the release branch. [Example](https://github.com/radius-project/radius/pull/6114/files). The release branch should now contain all needed patch changes, an updated release version, and patch release notes.

1. Push the commits to the remote and create a pull request targeting the release branch.

   ```bash
   git push origin <USERNAME>/<BRANCHNAME>
   ```

1. Merge the release branch PR into the release branch (this is the branch with the patch changes, updated patch version, and release notes). Then, merge the PR created against `main` into the main branch (this will only contain the updated patch version and the release notes). The release branch changes must be merged before the PR is merged into the main since the workflow in the main branch builds the release based on the head of the release branch. If the changes are not merged first to the release branch and then to the main branch, the patch release will not contain the necessary code fixes.

1. Verify that a patch release was created on Github Releases for the current patch version. [Example](https://github.com/radius-project/radius/releases)

1. Rerun steps 8-9 described [here](#creating-the-final-release) to upload updated rad-vscode-bicep.vsix file

## Cadence

We follow a monthly release cadence. Any contributions that have been merged through the pull-request process will be present in the next scheduled release.
