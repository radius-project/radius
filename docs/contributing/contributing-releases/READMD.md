# How to create and publish a Project Radius release

> 🚧🚧🚧 Under Construction 🚧🚧🚧
>
> This guide refers to resources and processes that can only be accessed by the Radius team. This will be updated as we migrate to public resources like GitHub releases.

Our release process for Project Radius is based on git tags. Pushing a new tag with the format: `v.<major>.<minor>.<patch>` will trigger a release build.

## Pre-requisites

- Find the storage account on Azure under 'Radius Dev' subscription. It is called `radiuspublic`
- Determine the release version. This is in the form `v.<major>.<minor>.<patch>`
- Determine the release channel This is in the form `<major>.<minor>`


### Creating an RC release

When starting the release process, we first kick it off by creating an RC release. This is a release candidate that we can test internally before releasing to the public which we can validate samples on.

If we find issues in validation, we can create additional RC releases until we feel confident in the release.

Follow the steps below to create an RC release.

1. In the Bicep fork on the `bicep-extensibility`

   ```bash
   git checkout bicep-extensibility
   git pull origin bicep-extensibility
   git checkout -b release/0.17 # ensure branch is created
   git pull origin release/0.17 # ensure branch is up to date
   git tag v0.17.0-rc1 # Update to the next RC version if doing a new release
   git push origin --tags # push the tag
   git push origin release/0.17 # push the branch up to origin
   ```
   
   Side note, in the bicep-extensibility branch, we have seen the build fail to trigger after pushing the tag once, but works after recreation. To delete and recreate:
   
   ```
   git tag delete v0.17.0-rc1
   git push origin --tags
   git tag v0.17.0-rc1 # Update to the next RC version if doing a new release
   git push origin --tags
   ```

   Verify that GitHub actions triggers a build in response to the tag, and that the build completes.

   Next, check the timestamps in the `tools` container of the storage account. There should be new builds of `rad-bicep` and the VS Code extension that correspond to the channel. Look at the paths `tools/bicep/<channel>/<architecture>/` and `tools/vscode/<channel>`. These should reflect the new build.

   ```bash
   az storage blob directory list -c tools -d bicep-extensibility --account-name radiuspublic --output table
   az storage blob directory list -c tools -d vscode-extensibility --account-name radiuspublic --output table
   ```

2. In the project-radius/deployment-engine repo:

   Create a new branch from main based off the release version called `release/0.<VERSION>`. For example, `release/0.12`. This branch will be used for patching/servicing.
   
   ```bash
   git checkout main
   git pull origin main
   git checkout -b release/0.17
   git pull origin release/0.17
   git tag v0.17.0-rc1
   git push origin --tags
   git push origin release/0.17
   ```

   Verify that GitHub actions triggers a build in response to the tag, and that the build completes. This will push the Deployment Engine container to our container registry.

3. In the project-radius/radius repo:

   Create a new branch from main based off the release version called `release/0.<VERSION>`. For example, `release/0.12`. This branch will be used for patching/servicing.
   
   ```bash
   git checkout main
   git pull origin main
   git checkout -b release/0.17
   git pull origin release/0.17
   git tag v0.17.0-rc1
   git push --tags
   git push origin release/0.17
   ```

   Verify that GitHub actions triggers a build in response to the tag, and that the build completes. This will push the AppCore RP and UCP containers to our container registry.


### Test tutorials and samples

> This step is manual, however it could be automated in the future.

Before a release can be finished, all [tutorials](https://edge.radapp.dev/user-guides/tutorials/) and [samples](https://edge.radapp.dev/user-guides/samples/) must be tested and validated. This is done by running through each tutorial and sample, step by step, confirming each step works as expected on a local environment. We strive to validate multiple OSs and Kubernetes cluster types. 

1. Install the latest release candidate of the CLI
For MacOS
```
curl -fsSL "https://radiuspublic.blob.core.windows.net/tools/rad/install.sh" | /bin/bash -s 0.17-rc1
```

For Windows
```
$script=iwr -useb  https://radiuspublic.blob.core.windows.net/tools/rad/install.ps1; $block=[ScriptBlock]::Create($script); invoke-command -ScriptBlock $block -ArgumentList 0.17-rc1
```

Because we have not forked for samples and docs yet, please use the `edge` channel for validation. Specifically using `edge.radapp.dev` for the docs and following along.

1. Run through each tutorial, step by step, confirming each step works as expected
1. Run through each quickstart, step by step, confirming each step works as expected
1. Run through each reference app, step by step, confirming each step works as expected

These include eshop, container-apps, dapr quickstart, etc.

Different environments/OSs to test on:
- Unix/MacOS
- Windows

Different cluster types to test on:
- AKS
- KinD
- k3d (codespace environment gives this for free)


*If we encounter an issue with an RC release, please refer to "Patching" below.*


### Creating the final release

If sample validation passes, we can start the process of creating the final release.

1. Go through steps 1-3 of "Creating an RC release" above, substituting the final release version instead of the RC version.

   For example, if the RC version is `v0.17.0-rc1`, the final release version would be `v0.17.0`.
1. Purge the [CDN cache](https://ms.portal.azure.com/#@microsoft.onmicrosoft.com/resource/subscriptions/66d1209e-1382-45d3-99bb-650e6bf63fc0/resourcegroups/assets/providers/Microsoft.Cdn/profiles/Radius/endpoints/radius/overview)
1. Check the stable version marker

   The file https://get.radapp.dev/version/stable.txt should contain (in plain text) the channel you just created.
   
   You can find this file in the storage account under `version/stable.txt`.

1. Update the project-radius/docs repository

   Assuming that we are using v0.16.
   
   1. Create a new branch named `v0.16` from `edge`, substituting the new version number
   1. Within `docs/config.toml`:
      - Change `baseURL` to `https://radapp.dev/` instead of `https://edge.radapp.dev/`
      - Change `version` to `v0.16` instead of `edge`, substituting the new version number
      - Change `chart_version` (Helm chart) to `0.16.0`, substituting the new version number
   1. Within `docs/layouts/partials/hooks/body-end.html`:
      - Change `indexName` to `radapp-dev` instead of `radapp-dev-edge`
   1. In `docs/content/getting-started/_index.md` update the binary download links with the new version number
   1. Commit and push updates to be the new `v0.16` branch you created above.
   1. Update the [latest](https://github.com/project-radius/docs/settings/environments/750240441/edit) environment to allow the new version to be deployed, and not the old version. This requires Admin/PM action and is restricted to that set of people, so ping one of the PMs to edit this value.
   1. Verify https://radapp.dev now shows the new version.

1. Update the project-radius/samples repository to point to latest release

   ```
   git checkout edge
   git pull origin edge
   git checkout -b v0.16
   git pull origin v0.16
   git push origin v0.16
   ```

### Post release sanity check

After creating a release, it's good to sanity check that the release works in some small mainline scenarios and has the right versions for each container.

1. Download the released version rad CLI. You can download the binary here: https://radapp.dev/getting-started/ if you just created a release. If you are doing a point release (ex 0.12), you can use the following URL format:


   ```sh
   Windows:
   $script=iwr -useb  https://get.radapp.dev/tools/rad/install.ps1; $block=[ScriptBlock]::Create($script); invoke-command -ScriptBlock $block -ArgumentList 0.12

   MacOS:
   curl -fsSL "https://get.radapp.dev/tools/rad/install.sh" | /bin/bash -s 0.12

   Direct binary downloads
   https://get.radapp.dev/tools/rad/<version>/<macos-x64 or windows-x64 or linux-x64>/rad
   ```

   Note: if you download the direct binary, execute `rad bicep download` to also download the corresponding bicep compiler binary. The scripts above will download the bicep compiler by default.

2. Confirm that the version of `rad` aligns with what is expected by running:

   ```sh
   rad version
   RELEASE     VERSION      BICEP     COMMIT
   0.12.0-rc3  v0.12.0-rc3  0.7.14    4f8a3ef96ea537a2e9252e0c6a6bcc7a1f3ce782
   ```

3. Install radius on a kubernetes cluster by executing `rad install kubernetes`

   ```
   rad install kubernetes
   ```

   Verify this command completes successfully 

4. Verify that each pod running in the radius-system namespace uses the right image and tag for each of the containers.

   ```
   kubectl describe pods -n radius-system -l control-plane=appcore-rp
   kubectl describe pods -n radius-system -l control-plane=de
   kubectl describe pods -n radius-system -l control-plane=ucp
   ```

   Checking the Containers section of each output to confirm the right image and tag are there. This would, for example, be radius.azurecr.io/appcore-rp:0.12 for the 0.12 release for the appcore-rp image.

5. Execute `rad deploy` to confirm a simple deployment works

   ```
   rad init
   rad deploy <simple bicep>
   ```

   Confirm the bicep file deploys successfully.


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
3. Cherry-pick the commit that is on `main` onto the branch. PLEASE USE `-x` HERE TO ENSURE VERSION HISTORY IS PRESERVED.
   ```bash
   git cherry-pick -x <COMMIT HASH>
   ```
5. Update the file radius/.github/workflows/validate-bicep.yaml to use the release version (eg. v0.17) instead of edge for validating the biceps in the docs and samples repositories. Also modify the version from `env.REL_CHANNEL` to <major>.<minor> (eg. 0.17) for downloading the `rad-bicep-corerp`.
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

