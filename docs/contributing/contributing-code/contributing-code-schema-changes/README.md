# Contributing schema changes

This page will explain the process to make a change to Radius' REST API (eg: adding a new property, or adding a new resource type).The Radius application model and API are defined via a OpenAPI specification. Instead of manually defining each OpenAPI spec, [CADL](https://microsoft.github.io/typespec/) is used to generate the OpenAPI JSON files. You should read and follow these steps to make REST API changes.

## Step 1: Update CADL and generate Bicep types and API client

In order to update or create a new schema follow these steps:
1. Create or update the applicable CADL files (named after resource type) within the `cadl` directory in the root of the radius repo
2. Run the following command to generate the OpenAPI spec with the newly added changes:
    ```bash  
    npx cadl compile .
    ```
3. Generate the client code by running autorest. For example, to generate the LinkRP resources run:
    ```bash 
    autorest pkg/linkrp/api/README.md --tag=link-2022-03-15-privatepreview
    ```    
    autotrest cnfiguration file i.e README.md is generally found in pkg/\<namespace\>/api/ directory, and it file has the details on the what tag-link to be used.
4. You can alternatively run the below command instead of steps 2 and 3 to generate OpenAPI spec and the API client.
    ```bash  
    make generate
    ```
    which is used to generate OpenAPi Specs and API client for all namespace and runs mockgen to generate mocks.
5. Add any necessary changes to the Radius resource provider to support the newly added types
6. Add any necessary tests, as needed
7. Open a pull request in the radius repo

Creating a pull request in the radius repo that contains application model changes triggers an automated pull request in bicep repo with the bicep type changes. You will merge this in step 3.

## Step 2: Update docs and samples

Visit the [docs](https://github.com/project-radius/docs/) and [samples](https://github.com/project-radius/samples/) repository and open PRs with the changes to the resource(s). Some checks will fail until you begin merging PRs below.

## Step 3: Merge pull requests in order
⚠️ Make sure you have PRs open and ready to merge within the radius, bicep, docs, and samples repositories. Do not proceed until all the PRs are ready and approved.
1. **Bicep Repository**: Begin by merging the bicep repo PR. This will update the Bicep types which will allow the other PRs to properly build and be merged.
2. **Docs Repository**: Rerun any failed checks and merge the PR from docs repo with updated Bicep files changes.
3. **Samples Repository**: Merging the PR in samples repo may not be straightforward, as we currently have a cyclic dependency between samples and radius repositories (_i.e "Test Quickstarts" task in samples pipeline run would fail as it runs on the main branch of radius which doesn't have the latest changes as radius PR is blocked on the samples PR for bicep files update._) You need to have a repo admin force merge the samples PR.
4. **Radius Repository**: After the PRs from the bicep, docs and samples repositories are merged, re-run the checks to make sure there are no failures to merge the Radius PR.