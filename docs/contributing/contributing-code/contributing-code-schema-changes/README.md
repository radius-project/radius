# Contributing schema changes

This page will explain the process to make a change to Radius' REST API (eg: adding a new property, or adding a new resource type).The Radius Application model and API are defined via a OpenAPI specification. Instead of manually defining each OpenAPI spec, [TypeSpec](https://microsoft.github.io/typespec/) is used to generate the OpenAPI JSON files. You should read and follow these steps to make REST API changes.

## Step 1: Update TypeSpec and generate Bicep types and API client

In order to update or create a new schema follow these steps:

1. Create or update the applicable TypeSpec files (named after resource type) within the `typespec` directory in the root of the Radius repo.
1. Run `tsp format --check "**/*.tsp"` in the typespec folder to make sure that the format of added or updated files are good.
1. You can run `tsp format **/*.tsp` to apply the formatting of TypeSpec compiler.
1. Run `make generate` to generate the OpenAPI spec and API clients:

    ```bash
    make generate
    ```

    This will generate the OpenAPI spec and API client for all namespaces and run mockgen to generate mocks.
    <details>
    <summary>Alternately, if you would like to manually generate the OpenAPI spec and API client, follow these steps:</summary>

    1. Run the following command to generate the OpenAPI spec with the newly added changes

        ```bash
        npx tsp compile .
        ```

    1. Generate the client code by running autorest. For example, to generate the `Applications.Core` resources run:

        ```bash
        autorest pkg/corerp/api/README.md --tag=link-2023-10-01-preview
        ```

        The autotrest configuration file (_i.e README.md_) is generally found in `pkg/<NAMESPACE>/api/` directory and has details on which tag to use.
    </details>
1. Add any necessary changes to the Radius resource provider to support the newly added types.
1. Add any necessary tests, as needed.
1. Open a pull request in the Radius repo.

## Step 2: Update docs and samples

Visit the [docs](https://github.com/radius-project/docs/) and [samples](https://github.com/radius-project/samples/) repository and open PRs with the changes to the resource(s). Some checks will fail until you begin merging PRs below.

## Step 3: Merge pull requests in order

⚠️ Make sure you have PRs open and ready to merge within the radius, docs, and samples repositories. Do not proceed until all the PRs are ready and approved.

1. **Samples Repository**: Merging the PR in samples repo may not be straightforward, as we currently have a cyclic dependency between samples and radius repositories (_i.e "Test Quickstarts" task in samples pipeline run would fail as it runs on the main branch of Radius which doesn't have the latest changes as Radius PR is blocked on the samples PR for bicep files update._) You need to have a repo admin force merge the samples PR.
2. **Radius Repository**: After the PR from the samples repositories are merged, re-run the checks to make sure there are no failures to merge the Radius PR.
3. **Docs Repository**: Rerun any failed checks and merge the PR from docs repo with updated Bicep files changes.

# Testing schema changes locally

If you would like to test that your schema changes are compilable in a Bicep template, you can do so by publishing them to a file system using the [Bicep CLI](https://learn.microsoft.com/en-us/azure/azure-resource-manager/bicep/).

## Step 1: Download the Bicep CLI

1. Follow the steps in the Bicep [documentation](https://learn.microsoft.com/en-us/azure/azure-resource-manager/bicep/install) to download Bicep.

Note: Alternatively, if you already have the Radius CLI installed, you can choose to use the Bicep binary that is installed as part of Radius. The Bicep binary gets downloaded to `./.rad/bin/rad-bicep`. You can use this file path instead.

## Step 2: Create a file directory  

1. Create a file directory in your location of choice. Keep the directory path handy for the next steps.

## Step 3: Upload the new schema types to the file directory

1. Run `make generate` to generate the OpenAPI spec and API clients:

    ```bash
    make generate
    ```

1. `cd` into the `bicep-types-radius/generated` folder
1. Run `bicep publish-provider <file> --target <ref>` to upload the schema changes to your file system. The file uploaded will be the `index.json` file as it contains all references to the types schema. The `<file-name>` can be named as desired, but we recommend using an archive (i.e. `.zip`, `.tgz`, etc). This will make it easier to view the files that get uploaded if needed.

    ```bash
    bicep publish-extension index.json --target <directory-path>/<file-name>
    ```

## Step 4: Update the `bicepconfig.json` to use your newly published types

1. Update the `bicepconfig.json` file in the root folder to reference your new published types.

    ```json
    {
        "experimentalFeaturesEnabled": {
            "extensibility": true
        },
        "extensions": {
            "radius": "<file-path>",
            "aws": "br:biceptypes.azurecr.io/aws:latest"
        }
    }
    ```

1. Once Bicep restores the new extensions, you should be able to use the new schema changes in your Bicep templates.

Note: You can also choose to publish the types to an OCI registry. The `--target` field will be your OCI registry endpoint when running the `bicep publish-extension` command. Make sure to update the `radius` extension field with your OCI registry endpoint in the `bicepconfig.json`.
