# Description

This PR fixes several issues with GitHub Actions workflows that were causing failures on fork PRs and dependabot PRs.

## Issues Fixed

### 1. `/ok-to-test` command not working for fork PRs
When triggered via `issue_comment`, the workflow was using `.headRepository.owner.login` to get the repository owner, but `gh pr view --json` provides this in a separate field called `headRepositoryOwner`. This caused the repository to be extracted as `/radius` instead of `owner/radius`, failing the checkout step.

**Fix:** Updated the jq extraction to use `.headRepositoryOwner.login` for the owner and `.headRepository.name` for the repo name.

### 2. `privateKey option is required` error on fork PRs
The `create-github-app-token` action was failing on fork PRs because secrets aren't available in that context.

**Fix:** Added `if: github.repository == 'radius-project/radius'` conditionals to skip these steps when running on forks.

### 3. Fork PRs now require `/ok-to-test` command for cloud tests
Previously, the `tests` job would run on any `pull_request` event (except dependabot). This caused failures on fork PRs because secrets aren't available.

**Fix:** Added a check that `pull_request` events only run cloud tests when the PR head is from the same repository (not a fork). Fork PRs must use the `/ok-to-test` command from an OWNER or MEMBER to trigger cloud tests.

### 4. `report-test-results` job failing on fork/dependabot PRs
The `report-test-results` job has `if: always()` so it runs even when other jobs are skipped, but it requires the GitHub App token which isn't available on forks.

**Fix:** Changed the job condition to `if: always() && github.repository == 'radius-project/radius'` to skip the entire job on forks.

## Behavior Change

**Fork PRs:** Cloud functional tests (corerp-cloud, ucp-cloud) will no longer automatically run on fork PRs. A maintainer must comment `/ok-to-test` on the PR to trigger the cloud tests. This is a security best practice since cloud tests require access to secrets.

## Files Changed
- `.github/workflows/functional-test-cloud.yaml` - All three fixes above
- `.github/workflows/triage-bot.yaml` - Added repository check to token step
- `.github/workflows/long-running-azure.yaml` - Added repository check to token step

## Type of change

- This pull request is a minor refactor, code cleanup, test improvement, or other maintenance task and doesn't change the functionality of Radius (issue link optional).

## Contributor checklist
Please verify that the PR meets the following requirements, where applicable:

- An overview of proposed schema changes is included in a linked GitHub issue.
    - [ ] Yes <!-- TaskRadio schema -->
    - [x] Not applicable <!-- TaskRadio schema -->
- A design document PR is created in the [design-notes repository](https://github.com/radius-project/design-notes/), if new APIs are being introduced.
    - [ ] Yes <!-- TaskRadio design-pr -->
    - [x] Not applicable <!-- TaskRadio design-pr -->
- The design document has been reviewed and approved by Radius maintainers/approvers.
    - [ ] Yes <!-- TaskRadio design-review -->
    - [x] Not applicable <!-- TaskRadio design-review -->
- A PR for the [samples repository](https://github.com/radius-project/samples) is created, if existing samples are affected by the changes in this PR.
    - [ ] Yes <!-- TaskRadio samples-pr -->
    - [x] Not applicable <!-- TaskRadio samples-pr -->
- A PR for the [documentation repository](https://github.com/radius-project/docs) is created, if the changes in this PR affect the documentation or any user facing updates are made.
    - [ ] Yes <!-- TaskRadio docs-pr -->
    - [x] Not applicable <!-- TaskRadio docs-pr -->
- A PR for the [recipes repository](https://github.com/radius-project/recipes) is created, if existing recipes are affected by the changes in this PR.
    - [ ] Yes <!-- TaskRadio recipes-pr -->
    - [x] Not applicable <!-- TaskRadio recipes-pr -->
