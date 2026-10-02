# Review note: PR 3 - Advisory Conventional Commit title check

- **Pull request**: [#12737](https://github.com/radius-project/radius/pull/12737)
- **Plan phase**: [PR 3](../2026-03-goreleaser-release-lifecycle-implementation-plan.md#pr-3-conventional-commit-title-check-advisory)
- **Stack index**: [README](./README.md)

## Verdict

The layer meets the PR 3 exit criteria with one small workflow change. The workflow uses `pull_request_target` without checking out or executing pull request code, so the elevated token never meets untrusted input and the check can comment on fork pull requests; the job holds only `pull-requests: write`; both actions are pinned by commit to their current releases (`amannn/action-semantic-pull-request` v6.1.1 and `marocchino/sticky-pull-request-comment` v3.0.5, SHAs verified against the tags); the advisory mode is a single `continue-on-error` line with a comment explaining that removing it makes the check required, which is exactly what PR 8 does. The eleven allowed types match the git-cliff parsers PR 4 introduces one for one, and the contributing guide's type table maps to the same changelog groups. The repository squash-merge settings are `PR_TITLE` and `PR_BODY`, so the title really becomes the commit subject.

## Changes made in this review

### 1. Pull request description

- **What changed**: "PR 3 of 3" became "PR 3 in GitHub stack #12738".
- **Why**: the count was written before the stack grew to 18 pull requests.
- **Value**: accuracy only.
- **Impact**: description only; no code change.

### 2. A validation that never ran no longer clears the comment or reports green

- **What changed**: the comment steps branch on the validation step's `outcome` as well as on `error_message`. An invalid title (`outcome == 'failure'` with a message) posts the fix-it comment, a valid title (`outcome == 'success'`) clears it, and a failure without a message, which the action produces only when it stopped before validating the title (an API error, for example), leaves any earlier comment in place and fails the job with an annotation pointing at the step log.
- **Why**: the original conditions looked only at `error_message`, and the action sets that output from its title-validation path alone (verified in the bundled `dist/index.js` at the pinned commit). Any other failure looked like a valid title: the earlier warning was deleted and the run stayed green under `continue-on-error`, so a broken check was invisible.
- **Value**: the advisory promise still holds for titles, while an infrastructure failure shows as a red run instead of silently hiding feedback.
- **Impact**: none on valid or invalid titles; PR 8 replaces the comment steps when the check becomes required.

## Findings left as-is

- **Rebase merges stay enabled at the repository level**: a rebase merge would land commit subjects that bypass the title convention. On `main` this cannot happen, because every merge goes through the ruleset's merge queue and the queue's merge method is `SQUASH`; release branches are merged by the release automation. Nothing to change in this layer.

## Verification

- `actionlint` and Prettier pass for the workflow; the verifier and parity tests from the lower layers pass on this branch after the rebase.
- The pinned action's bundle contains a single `setOutput('error_message', ...)`, reached only from the title-validation error path; API and context errors go straight to `setFailed` without setting it.
- The pinned commits resolve to the `v6.1.1` and `v3.0.5` tags upstream, and both are the latest releases as of 2026-09-10.
