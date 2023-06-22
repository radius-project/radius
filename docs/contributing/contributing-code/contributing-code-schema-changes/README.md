# Contributing to application model changes
The Radius application model and API are defined via a OpenAPI specification. Instead of manually defining each OpenAPI spec, [CADL](https://microsoft.github.io/typespec/) is used to generate the OpenAPI JSON files.
## Adding changes to CADL and generate bicep types and API client.

1) To update or create a new application model specification in the radius/swagger directory, you need to add/update the corresponding resource CADL file in radius/cadl.


2) Run the below command to generate the openapi spec with the newly added changes:
    ```bash  
    cadl compile .
    ```
3) And generate the client code by running the autorest command:<br>
E.g: For linkrp resources
    ```bash 
    autorest pkg/linkrp/api/README.md --tag=link-2022-03-15-privatepreview
    ```
4) Add necessary changes to radius and create a PR.

## Updating the docs and samples repositories

Update relevant bicep files if any, in the [docs](https://github.com/project-radius/samples/) and [samples](https://github.com/project-radius/docs/) repository.

## Merge pull requests in order

1) **Bicep Repository**: Creating a pull request in radius that contains application model changes triggers an automated pull request in bicep repo with the bicep type changes. You need to merge this PR to see the application model changes in bicep.<br>
This may causes "Validate Bicep" failures on the the Radius PR pipeline runs as validate bicep tasks runs validation of bicep files from radius, docs, samples repos.
2) **Docs Repository**: Merge the PR from docs repo with updated bicep files changes.
3) **Samples Repository**: Merging the PR in samples repo may not be straight forward. Because currently we have a cyclic dependency between samples and radius repositories i.e "Test Quickstarts" task in samples pipeline run would fail as it runs on the main branch of radius which doesn't have the latest changes as radius PR is blocked on the samples PR for bicep files update. So, you need to force merge the samples PR.
4) **Radius Repository**: After PR from the bicep, docs and samples repositories are merged, re-run the checks to make sure there are no failures to merge the radius PR.