# Description

_Please explain the changes you've made._

## Type of change

<!--

Please select **one** of the following options that describes your change and delete the others. Clearly identifying the type of change you are making will help us review your PR faster, and is used in authoring release notes.

If you are making a bug fix or functionality change to Radius and do not have an associated issue link please create one now. 

-->

- This pull request fixes a bug in Radius and has an approved issue (issue link required).
- This pull request adds or changes features of Radius and has an approved issue (issue link required).
- This pull request is a minor refactor, code cleanup, test improvement, or other maintenance task and doesn't change the functionality of Radius (issue link optional).
- This pull request is a design document and only includes files in the eng/design-notes directory.

<!--

Please update the following to link the associated issue. This is required for some kinds of changes (see above).

-->

Fixes: #issue_number

## Contributor checklist
Please verify that the PR meets the following requirements, where applicable:

<!--
This checklist uses "TaskRadio" comments to make certain options mutually exclusive.
See: https://github.com/mheap/require-checklist-action?tab=readme-ov-file#radio-groups
For details on how this works and why it's required.
-->

- An overview of proposed schema changes is included in a linked GitHub issue.
    - [ ] Yes <!-- TaskRadio schema -->
    - [ ] Not applicable <!-- TaskRadio schema -->
- A design document PR is created in the eng/design-notes directory, if new APIs are being introduced.
    - [ ] Yes <!-- TaskRadio design-pr -->
    - [ ] Not applicable <!-- TaskRadio design-pr -->
- The design document has been reviewed and approved by Radius maintainers/approvers.
    - [ ] Yes <!-- TaskRadio design-review -->
    - [ ] Not applicable <!-- TaskRadio design-review -->
- A PR for [resource-types-contrib](https://github.com/radius-project/resource-types-contrib/) is created, if resource types or recipes are affected by the changes in this PR.
    - [ ] Yes <!-- TaskRadio recipes-pr -->
    - [ ] Not applicable <!-- TaskRadio recipes-pr -->
- A PR for [dashboard](https://github.com/radius-project/dashboard/) is created, if the Radius Dashboard is affected by the changes in this PR.
    - [ ] Yes <!-- TaskRadio recipes-pr -->
    - [ ] Not applicable <!-- TaskRadio recipes-pr -->
- A PR for the [documentation repository](https://github.com/radius-project/docs) is created, if the changes in this PR affect the documentation or any user facing updates are made.
    - [ ] Yes <!-- TaskRadio docs-pr -->
    - [ ] Not applicable <!-- TaskRadio docs-pr -->
