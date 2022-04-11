# How to create and publish a Project Radius release

Our release process for Project Radius is based on git tags. Pushing a new tag with the format: `v.<major>.<minor>.<patch>` will trigger a release build.

## Pre-requisites

- Find the storage account on Azure under 'Radius Dev' subscription. It is called `radiuspublic`
- Determine the release version. This is in the form `v.<major>.<minor>.<patch>`
- Determine the release channel This is in the form `<major>.<minor>`

### Test tutorials and samples

> This step is manual, however it could be automated in the future.

Before a release can be started, all [tutorials](https://edge.radapp.dev/user-guides/tutorials/) and [samples](https://edge.radapp.dev/user-guides/samples/) must be validated across local (Windows and macOS), Kubernetes, and Azure.

1. Install the latest edge rad CLI release
1. Run through each tutorial, step by step, confirming each step works as expected on a local environment (make sure to be on the edge site)
1. Run through each sample, step by step, confirming each step works as expected on a local environment (make sure to be on the edge site)
1. Repeat on Windows local, Kubernetes, and Azure

Do not start the release until the following scenarios are validated:

| OS | WebApp Tutorial | Dapr Tutorial | eShop Sample | Container Apps Sample |
|:--:|:---------------:|:-------------:|:------------:|:---------------------:|
| macOS local   | ✅ | ✅ | ✅ | ✅ |
| Windows local | ✅ | - | - | ✅ |
| Kubernetes    | ✅ | ✅ | ✅ | ✅ |
| Azure         | ✅ | ✅ | ✅ | ✅ |

## Performing a release

1. In the Bicep fork: `radius-compiler` branch at the time of writing

   ```bash
   # replace v0.1.0 with the release version
   git tag v0.1.0
   git push --tags
   ```

   Verify that GitHub actions triggers a build in response to the tag, and that the build completes.

   Next, check the timestamps in the `tools` container of the storage account. There should be new builds of `rad-bicep` and the VS Code extension that correspond to the channel. Look at the paths `tools/bicep/<channel>/<architecture>/` and `tools/vscode/<channel>`. These should reflect the new build.

   ```bash
   az storage blob directory list -c tools -d bicep --account-name radiuspublic --output table
   az storage blob directory list -c tools -d vscode --account-name radiuspublic --output table
   ```

2. In the azure/radius repo:

   Create a new branch from main based off the release version called `release/0.<VERSION>`. For example, `release/0.10`.


   ```bash
   # replace v0.1.0 with the release version
   git tag v0.1.0
   git push --tags
   ```

   Verify that GitHub actions triggers a build in response to the tag, and that the build completes.

   Next, check the timestamps in the `environment` container of the storage account. There should be new copies of our environment setup assets that correspond to the channel.  Look at the path `environment/<channel>/`. These should reflect the new build.

   ```bash
   az storage blob directory list -c environment -d <channel> --account-name radiuspublic --output table
   ```
3. Updating Helm chart

   Helm charts upload is automatic after v0.10.0. If the tools command mentioned in step 1 & 2 return the current version then skip the manual steps below:

   ```bash
   cd deploy/Chart
   # Replace version: 0.X.0 with this release version in Chart.yaml
   # Replace tag: 0.X with this release version in values.yaml
   helm package .
   az acr helm push -n radius radius-0.9.0.tgz --force
   # To verify upload worked
   az acr helm list -n radius
   ```

4. Check the stable version marker

   If this is a patch release - you can stop here, you are done.
   
   If this is a new minor release - check the stable version marker.
   
   The file https://radiuspublic.blob.core.windows.net/version/stable.txt should contain (in plain text) the channel you just created.
   
   You can find this file in the storage account under `version/stable.txt`.

5. Update the docs
   
   1. Within GitHub delete the branch for the prior Radius release, *e.g. `release/0.1`*.
   1. Create a new branch named `release/X.Y` from `main`, using the release version number.
   1. Update the branch information for the docs. Example: https://github.com/project-radius/radius/commit/f4b81b8881d590fbf077280db6a05482ed44188b
   1. Within `docs/config.toml` update the `baseURL` parameter  to be `https://radapp.dev/` instead of `https://edge.radapp.dev/`.
   1. Within `.github/workflows/website.yml` update the branch to be the new `release\X.Y` branch you created above.
   1. Within `.github/workflows/website.yml` update `${{ secrets.EDGE_DOCS_SITE_PUBLISHPROFILE }}` to `${{ secrets.DOCS_SITE_PUBLISHPROFILE }}` and "edge-radius" to "radius".
   1. In `docs/content/getting-started/install-cli.md` update the binary download links with the new version number, and delete commands for unstable/version commands, including all sub-headers.
   1. Commit and push updates to be the new `release\vX.Y` branch you created above.
   1. Verify https://radapp.dev now shows the new version.

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
2. Create a new branch based off the release branch we want to patch. Ex:
   ```bash
   git checkout release/0.<VERSION>
   git checkout -b <USERNAME>/<BRANCHNAME>
   ```
3. Cherry-pick the commit that is on `main` onto the branch.
   ```bash
   git cherry-pick <COMMIT HASH>
   ```
4. Push the commit to the remote and create a pull request targeting the release branch.
   ```bash
   git push origin <USERNAME>/<BRANCHNAME>
   ```
5. After pull request is approved, merge into the release branch and tag!
   ```bash
   # replace v0.10.X with the version we want to patch (if we release 0.10.1 already, we would then release 0.10.2, etc.)
   git tag v0.10.1 
   git push --tags
   ``` 

