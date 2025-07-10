# Description

This PR adds support for pre-mounting Terraform binaries from a container image to improve performance and reduce internet dependencies when executing Terraform recipes. Previously, Radius downloaded Terraform binaries at runtime for each recipe execution. This feature allows users to specify a container image containing Terraform binaries that will be copied to the Radius pods during initialization, eliminating the need for runtime downloads.

## Key Changes

- Added `--terraform-container` flag to `rad install kubernetes` command
- Extended Helm chart with `global.terraform` configuration section
- Modified applications-rp and dynamic-rp deployments to support init containers for Terraform binary mounting
- Updated Terraform install logic to check for pre-mounted binaries before downloading
- Added comprehensive documentation and unit tests

## Type of change

- This pull request adds or changes features of Radius and has an approved issue (issue link required).

Fixes: #9957

## Contributor checklist
Please verify that the PR meets the following requirements, where applicable:

- An overview of proposed schema changes is included in a linked GitHub issue.
    - [ ] Not applicable <!-- TaskRadio schema -->
- A design document PR is created in the [design-notes repository](https://github.com/radius-project/design-notes/), if new APIs are being introduced.
    - [ ] Not applicable <!-- TaskRadio design-pr -->
- The design document has been reviewed and approved by Radius maintainers/approvers.
    - [ ] Not applicable <!-- TaskRadio design-review -->
- A PR for the [samples repository](https://github.com/radius-project/samples) is created, if existing samples are affected by the changes in this PR.
    - [ ] Not applicable <!-- TaskRadio samples-pr -->
- A PR for the [documentation repository](https://github.com/radius-project/docs) is created, if the changes in this PR affect the documentation or any user facing updates are made.
    - [ ] Yes <!-- TaskRadio docs-pr -->
- A PR for the [recipes repository](https://github.com/radius-project/recipes) is created, if existing recipes are affected by the changes in this PR.
    - [ ] Not applicable <!-- TaskRadio recipes-pr -->
