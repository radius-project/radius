# How to create and publish a Radius release

## Pre-requisites

- Determine the release version number. This is in the form `<major>.<minor>.<patch>`

### Creating an RC release

When starting the release process, we first kick it off by creating an RC release. This is a release candidate that we can test internally before releasing to the public which we can validate samples on.

If we find issues in validation, we can create additional RC releases until we feel confident in the release.

Follow the steps below to create an RC release.

1. Clone the [project-radius/radius](https://github.com/project-radius/radius) repo locally.
1. Create a new branch from `main`.
1. In your local branch, update the `versions.yaml` file in the project-radius/radius repo
The `versions.yaml` file is a declarative version tracking file that the Radius community maintains. Update it to reflect the new release candidate version that we would like to release. ([Example](https://github.com/project-radius/radius/pull/6077/files))
1. Push these changes to a remote branch and create a pull request against `main`.
1. After maintainer approval, merge the pull request to `main`.
1. Verify that [GitHub actions triggers a build](https://github.com/project-radius/radius/actions), and that the build completes. This will build and push Radius assets.
1. In the project-radius/radius repo, run the [Release verification](https://github.com/project-radius/samples/actions/workflows/release-verification.yaml) workflow.

### Test tutorials and samples

In the project-radius/samples repo, run the [Test Quickstarts](https://github.com/project-radius/samples/actions/workflows/test.yaml) workflow. 

> For now, this is a manual task. Soon, this workflow will be triggered automatically.

> There is a possiblity that the workflow run failed from flaky tests. Try re-running, and if the failure is persistent, then there should be further investigation.

If this workflow run fails, or if we encounter an issue with an RC release, please refer to "Patching" below.

### Creating the final release

If sample validation passes, we can start the process of creating the final release.

1. Go through steps 1-4 of "Creating an RC release" above, substituting the final release version instead of the RC version. For example, if the RC version number is `0.1.0-rc1`, the final release version would be `0.1.0`.

1. After creating the pull request, there should be an automatically-generated release notes comment. Create a new release note document in the [release-notes](../../release-notes/) directory. Follow the directory's README.md for instructions on how to create a new release note document. Include this file in the release version pull request. [Example](https://github.com/project-radius/radius/pull/6092/files)

1. Cherry-pick the release notes into the release branch (see the "Patching" section for details). This will ensure that the release notes are included in the release branch. [Example](https://github.com/radius-project/radius/pull/6114/files)

1. Purge the [CDN cache](https://ms.portal.azure.com/#@microsoft.onmicrosoft.com/resource/subscriptions/66d1209e-1382-45d3-99bb-650e6bf63fc0/resourcegroups/assets/providers/Microsoft.Cdn/profiles/Radius/endpoints/radius/overview).
    ```bash
    $ az login
    $ az cdn endpoint purge --subscription 66d1209e-1382-45d3-99bb-650e6bf63fc0 --resource-group assets --name radius --profile-name Radius --content-paths "/*"
    ```
  
   For now, this is a manual task. Soon, this will be automated.

1. In the project-radius/docs repository, run the [Release docs](https://github.com/project-radius/docs/actions/workflows/release.yaml) workflow.

1. In the project-radius/docs repository, run the [Release samples](https://github.com/project-radius/samples/actions/workflows/release.yaml) workflow.

1. In the project-radius/radius repo, run the [Release verification](https://github.com/project-radius/samples/actions/workflows/release-verification.yaml) workflow.

## How releases work

Each release belongs to a *channel* named like `<major>.<minor>`. Releases will only interact with assets from their channel. For example, the `0.1` `rad` CLI will:

- Download `rad-bicep` from the `0.1` channel
- Create an environment using the `0.1` version of the RP and environment setup script

> ⚠️ Compatibility ⚠️ <br>
At this time we do not guarantee compatibility across releases or provide a migration path. For example, the behavior of a `0.1` `rad` CLI talking to a `0.2` control plane is unspecifed. We expect the project to change too frequently to provide compatibility guarantees at this time.

Conceptually we scope channels to a major+minor pair because this allows us to freely patch assets as needed without needing to change the intermediate pieces. For example pushing a `v0.1.1` tag will update the assets in the `v0.1` channel. This works as long as it is a *true* patch release and maintains compatibility.

## Patching

Let's say we have a bug in a release which needs to be patched for an already created release.

1. Make sure the commit that we want to add to a patch is merged and validate in `main` first if it affects `main`.

1. Create a new branch based off the release branch we want to patch. Ex:
   ```bash
   git checkout release/0.<VERSION>
   git checkout -b <USERNAME>/<BRANCHNAME>
   ```

1. Cherry-pick the commit that is on `main` onto the branch. PLEASE USE `-x` HERE TO ENSURE VERSION HISTORY IS PRESERVED.
   ```bash
   git cherry-pick -x <COMMIT HASH>
   ```

1. If breaking changes have been made to our Bicep fork: 
   
   Update the file radius/.github/workflows/validate-bicep.yaml to use the release version (eg. v0.21) instead of edge for validating the biceps in the docs and samples repositories. Also modify the version from `env.REL_CHANNEL` to `<major>.<minor>` (eg. 0.21) for downloading the `rad-bicep-corerp`.

1. Push the commit to the remote and create a pull request targeting the release branch.
   ```bash
   git push origin <USERNAME>/<BRANCHNAME>
   ```

1. After pull request is approved, merge into the release branch and tag!
   ```bash
   # replace v0.21.X with the version we want to patch (if we release 0.21.1 already, we would then release 0.21.2, etc.)
   git tag v0.21.X
   git push --tags
   ```

## Cadence

We follow a monthly release cadence. Any contributions that have been merged through the pull-request process will be present in the next scheduled release.
