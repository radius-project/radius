# Contributing to application model changes

The Radius application model and API are defined via a OpenAPI specification. Instead of manually defining each OpenAPI spec, [CADL](https://microsoft.github.io/typespec/) is used to generate the OpenAPI JSON files.
## Step 1: Update CADL and generate Bicep types and API client

In order to update or create a new application model specification follow these steps:
1. Create or update the applicable CADL files within the `cadl` directory in the root of the radius repo
2. Run the following command to generate the OpenAPI spec with the newly added changes:
    ```bash  
    cadl compile .
    ```
3. Generate the client code by running autorest. For example, to generate the LinkRP resources run:
    ```bash 
    autorest pkg/linkrp/api/README.md --tag=link-2022-03-15-privatepreview
    ```
4. Add any necessary changes to the Radius resource provider to support the newly added types
5. Add any necessary tests, as needed
6. Open a pull request in the radius repo

## Step 2: Update docs and samples

Visit the [docs](https://github.com/project-radius/docs/) and [samples](https://github.com/project-radius/samples/) repository and open PRs with the changes to the resource(s).

## Step 3: Merge pull requests in order

1. **Bicep Repository**: Creating a pull request in radius that contains application model changes triggers an automated pull request in bicep repo with the bicep type changes. You need to merge this PR to see the application model changes in bicep.
   - This may cause "Validate Bicep" failures on the the Radius PR pipeline runs as validate bicep tasks runs validation of Bicep files from radius, docs, samples repos. So, make sure to have the PRs from radius, docs and samples repositories in the merge ready state before you merge the Bicep PR.
2. **Docs Repository**: Merge the PR from docs repo with updated Bicep files changes.
3. **Samples Repository**: Merging the PR in samples repo may not be straightforward, as we currently have a cyclic dependency between samples and radius repositories (_i.e "Test Quickstarts" task in samples pipeline run would fail as it runs on the main branch of radius which doesn't have the latest changes as radius PR is blocked on the samples PR for bicep files update._) You need to force merge the samples PR.
4. **Radius Repository**: After the PRs from the bicep, docs and samples repositories are merged, re-run the checks to make sure there are no failures to merge the Radius PR.