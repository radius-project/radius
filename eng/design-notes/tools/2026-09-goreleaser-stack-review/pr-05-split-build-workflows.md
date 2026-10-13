# Review note: PR 5 - Split the build workflow by trigger

- **Pull request**: [#12749](https://github.com/radius-project/radius/pull/12749)
- **Plan phase**: [PR 5](../2026-03-goreleaser-release-lifecycle-implementation-plan.md#pr-5-split-buildyaml-by-trigger)
- **Stack index**: [README](./README.md)

## Verdict

The split is faithful to the plan's "identical behavior" requirement on everything the pull request could exercise. Compared line by line with `build.yaml` on `main`: the trigger union is preserved across the three entry points, the concurrency groups reduce to the same expression for each event, every job keeps its condition, `needs`, permissions, timeout, environment, and check name, and the five moved jobs keep their steps and step conditions. The `Build Check` required status keeps its name and its skip-tolerant logic in all three workflows, and the inline summary became a tested script. actionlint and Prettier pass for the eight files, and the summary script's six tests pass.

One thing the mechanical split lost is invisible on a pull request: the reusable workflows only receive the secrets their callers pass, and the Bicep types job reads two. That is change 1.

## Changes made in this review

### 1. The Bicep types callers inherit secrets

- **What changed**: the `build-and-push-bicep-types` jobs in `build-main.yaml` and `build-release.yaml` pass `secrets: inherit` to `__build-bicep-types.yaml`.
- **Why**: a called workflow sees only the secrets its caller passes, and the old monolithic job read `RADIUS_PUBLISHER_BOT_CLIENT_ID` and `RADIUS_PUBLISHER_BOT_PRIVATE_KEY` directly. After the split the "Get App Token" step would have run with empty credentials on the first push to `main` and on the first tag. Passing the two secrets by name would only work if they are repository secrets; the old job read them under the `publish-bicep` environment, which only the called job carries, and GitHub documents that environment secrets cannot be passed from a caller. `inherit` covers both cases: the inherited set includes organization, repository, and environment secrets, and the called job resolves them from its own environment.
- **Value**: the main-branch and tag builds keep publishing Bicep types after the split instead of failing at a step no pull request run reaches.
- **Impact**: none on pull request validation; the other reusable workflows use only `github.token` and stay without secrets.

## Findings left as-is

- **Step-level `only_changed` conditions in `__build-cli.yaml`**: every step already sits behind a job-level condition in each caller, so the input and the `Skip` step never take effect. The dead code predates this layer (the same gate existed in `build.yaml`) and PR 18 removes it with the matrix.
- **Image job on release-branch pushes and merge groups**: no step condition matches those events, so the job only collects metrics. This is the old behavior carried over; PR 18 reshapes the image path.
- **Cancel-in-progress on the release workflow**: the design asks release concurrency to never cancel a tagged release. The split keeps the old setting on purpose; the release controller in PR 13 and PR 15 replaces this workflow's tag path.

## Verification

- `actionlint` and Prettier pass for the eight workflow files after the change; `build-summary_test.sh` passes; shfmt and ShellCheck are clean for the summary script and its test.
- The failing checks visible on the pull request right after the stack push were runs cancelled by the newer push, not failures.
