# Review note: PR 9 - Idempotent tag and branch reconciliation

- **Pull request**: [#12770](https://github.com/radius-project/radius/pull/12770)
- **Plan phase**: [PR 9](../2026-03-goreleaser-release-lifecycle-implementation-plan.md#pr-9-idempotent-tag-and-branch-reconciliation)
- **Stack index**: [README](./README.md)

## Verdict

The layer implements the design's idempotency contract for branches and tags faithfully, and replaces a script that did `git push --tags` from a fresh checkout with one that reasons about remote state. Every rule is there: an absent branch is created at the planned commit with a create-only lease, an existing branch must contain the planned commit, an absent tag is pushed by name with the same lease, a tag at the planned commit is success and anywhere else is a conflict that is never moved, and every destination is re-read from the remote after mutation, with query failures propagated instead of read as "missing". Annotated tags are dereferenced, concurrent identical creation is accepted and concurrent divergence rejected. Version selection now asks all four release repositories, so a run that tagged `radius` and died before `recipes` resumes instead of skipping, and the cherry-pick gate on `main` pushes is preserved as a small tested script. The 20-minute job timeout finally sits above the Deployment Engine monitor budget it encloses. The 27 real-git scenarios cover the contract and the races; shfmt, ShellCheck, actionlint, and Prettier are clean.

## Changes made in this review

### 1. An accurate message when a branch push fails outright

- **What changed**: when the create-only push of the release branch fails and the remote still has no such branch, the script now says the push itself failed and points at the git error above, instead of claiming that the planned commit is not reachable from a branch called `<missing>`.
- **Why**: that path is the authentication or transport failure, which is the one a release engineer needs to recognize quickly; the reachability wording described a different, non-existent situation.
- **Value**: the first line of the failure names the actual problem.
- **Impact**: message only; the 18 reconciliation scenarios still pass.

## Findings left as-is

- **Sibling repositories without a planned commit**: `recipes`, `dashboard`, and `bicep-types-aws` are reconciled at their checked-out `main` head when neither a tag nor a release branch exists yet, which is the previous behavior. Freezing those commits earlier is cross-layer finding 3 and is evaluated at PR 15, where the release plan is introduced.
- **`release-should-skip.sh` query errors**: an `ls-remote` failure stops the script through `set -e` rather than through an explicit message. PR 15 deletes this script when the controller takes over the trigger, so it is not worth hardening here.

## Verification

- `release-create-tag-and-branch_test.sh` (18 scenarios) and `release-get-version_test.sh` (9 scenarios) pass; shfmt and ShellCheck are clean for the five scripts; actionlint and Prettier pass for `release.yaml`.
