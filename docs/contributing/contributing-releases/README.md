# How to create and publish a Radius release

## Prerequisites

- Determine the release version. This is in the form `<major>.<minor>.<patch>`.


## Creating an RC release

When starting the release process, we first kick it off by creating an RC release. This is a release candidate that we can test internally before releasing to the public which we can validate samples on.

If we find issues in validation, we can create additional RC releases until we feel confident in the release.

Follow the steps below to create an RC release.

```bash
# Create a directory
mkdir release
cd release

# Clone the repositories
git clone git@github.com:project-radius/radius.git
git clone git@github.com:project-radius/bicep.git
git clone git@github.com:project-radius/deployment-engine.git

# Run the release script with the desired version number
sh radius/.github/scripts/release.sh 0.21.0
```

> This will kick off some background tasks that may take around 30 minutes to complete. You can check the GitHub Actions tab for each of the repos to monitor the progress. Once the workflow runs are complete and successful, you can move on to release verification.


## Release verification

After creating a release (either an RC release or the final release), it's good to check that the release works in some small mainline scenarios and has the right versions for each container.

1. Download the desired verison of the rad CLI. For example, using version `v0.21`:

   ```sh
   Windows:
   $script=iwr -useb  https://get.radapp.dev/tools/rad/install.ps1; $block=[ScriptBlock]::Create($script); invoke-command -ScriptBlock $block -ArgumentList 0.21

   MacOS:
   curl -fsSL "https://get.radapp.dev/tools/rad/install.sh" | /bin/bash -s 0.21
   ```

1. Confirm that the version of `rad` aligns with what is expected by running:

   ```sh
   rad version
   RELEASE     VERSION      BICEP     COMMIT
   0.21.0      v0.21.0      0.7.14    4f8a3ef96ea537a2e9252e0c6a6bcc7a1f3ce782
   ```

1. Install Radius on a Kubernetes cluster by executing `rad install kubernetes` and verify that this command completes successfully:

   ```
   rad install kubernetes
   ```

1. Verify that each pod running in the radius-system namespace uses the right image and tag for each of the containers:

   ```
   kubectl describe pods -n radius-system -l control-plane=appcore-rp
   kubectl describe pods -n radius-system -l control-plane=de
   kubectl describe pods -n radius-system -l control-plane=ucp
   ```

   Checking the Containers section of each output to confirm the right image and tag are there. This would, for example, be radius.azurecr.io/appcore-rp:0.21 for the 0.21 release for the appcore-rp image. The following is an example where the rad version (highlighted in yellow) does not match with the tag label (highlighted in blue), and should be raised as an error.

   ![Example of version and tag mismatch](images/image-label.png)

1. Execute `rad deploy` to confirm a simple deployment works

   ```
   rad init
   rad deploy <simple bicep>
   ```

   Confirm the bicep file deploys successfully.


## Sample validation

> This step is manual, however it could be automated in the future.

Before a release can be finished, a set of tutorials and samples must be validated. Currently, the list is as follows:

* [Tutorial](https://edge.radapp.dev/getting-started/first-app/)
* [eShop](https://edge.docs.radapp.dev/getting-started/reference-apps/eshop/)

## Creating the final release

If sample validation passes, we can start the process of creating the final release.

1. Go through "Creating an RC release" above, substituting the final release version instead of the RC version.

   For example, if the RC version is `v0.21.0-rc1`, the final release version would be `v0.21.0`.
1. Purge the [CDN cache](https://ms.portal.azure.com/#@microsoft.onmicrosoft.com/resource/subscriptions/66d1209e-1382-45d3-99bb-650e6bf63fc0/resourcegroups/assets/providers/Microsoft.Cdn/profiles/Radius/endpoints/radius/overview)
1. Check the stable version marker

   The file https://get.radapp.dev/version/stable.txt should contain (in plain text) the channel you just created.
   
   You can find this file in the storage account under `version/stable.txt`.

1. Update the project-radius/docs repository

   Assuming that we are using v0.21.
   
   1. Create a new branch named `v0.21` from `edge`, substituting the new version number
   1. Within `docs/config.toml`:
      - Change `baseURL` to `https://radapp.dev/` instead of `https://edge.radapp.dev/`
      - Change `version` to `v0.21` instead of `edge`, substituting the new version number
      - Change `chart_version` (Helm chart) to `0.21.0`, substituting the new version number
   1. Within `docs/layouts/partials/hooks/body-end.html`:
      - Change `indexName` to `radapp-dev` instead of `radapp-dev-edge`
   1. In `docs/content/getting-started/install/_index.md` update the binary download links with the new version number
   1. Commit and push updates to be the new `v0.21` branch you created above.
   1. Update the [latest](https://github.com/project-radius/docs/settings/environments/750240441/edit) environment to allow the new version to be deployed, and not the old version. This requires Admin/PM action and is restricted to that set of people, so ping one of the PMs to edit this value.
   1. Verify https://radapp.dev now shows the new version.

1. Update the project-radius/samples repository to point to latest release

   ```
   git checkout edge
   git pull origin edge
   git checkout -b v0.21
   git pull origin v0.21
   git push origin v0.21
   ```


## How releases work

Each release belongs to a *channel* named like `<major>.<minor>`. Releases will only interact with assets from their channel. For example, the `0.1` `rad` CLI will:

- Download `rad-bicep` from the `0.1` channel
- Create an environment using the `0.11` version of the RP and environment setup script

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
1. Update the file radius/.github/workflows/validate-bicep.yaml to use the release version (eg. v0.21) instead of edge for validating the biceps in the docs and samples repositories. Also modify the version from `env.REL_CHANNEL` to <major>.<minor> (eg. 0.21) for downloading the `rad-bicep-corerp`.
1. Push the commit to the remote and create a pull request targeting the release branch.
   ```bash
   git push origin <USERNAME>/<BRANCHNAME>
   ```
1. After pull request is approved, merge into the release branch and tag!
   ```bash
   # replace v0.21.X with the version we want to patch (if we release v0.21.1 already, we would then release v0.21.2, etc.)
   git tag v0.21.1 
   git push --tags
   ```
