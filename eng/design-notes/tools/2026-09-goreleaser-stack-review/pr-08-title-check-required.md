# Review note: PR 8 - Required Conventional Commit title check

- **Pull request**: [#12763](https://github.com/radius-project/radius/pull/12763)
- **Plan phase**: [PR 8](../2026-03-goreleaser-release-lifecycle-implementation-plan.md#pr-8-conventional-commit-title-check-becomes-required)
- **Stack index**: [README](./README.md)

## Verdict

The layer turns the advisory check into a blocking one the right way. The required signal is the `action-semantic-pull-request` commit status rather than the job, which is what makes `[WIP]` work: the action leaves that status pending for a work-in-progress title, the job stays green, and the pull request stays unmergeable without being reported as broken. The action itself leaves an invalid title pending too, so the explicit failure status is necessary, and the merge-group job reports the same context as success on the queue commit so queued entries never stall on a check that cannot run there. Dropping the explicit type list means the action's Conventional Commit defaults apply, the contributing guide and `cliff.toml` follow (`style` excluded, `deps` gone from the table), and the plan text is corrected to match. The maintainer follow-up to add the context to the `main` ruleset only after the workflow reaches `main` is the correct sequencing; adding it earlier would deadlock every stacked pull request below.

## Changes made in this review

### 1. `deps` parser removed from `cliff.toml`

- **What changed**: the `^deps` commit parser is gone, and the changelog configuration test replaces its `deps:` fixture commit with a `style:` commit that must be excluded.
- **Why**: this layer removes `deps` from the accepted title types, so no squash-merged subject can start with it any more; dependency updates keep arriving as `chore(deps)` and `ci(deps)`, which the first parser groups. Cross-layer finding 5 in the index.
- **Value**: the parser list matches the policy exactly, and the test proves the new `style` exclusion.
- **Impact**: none on rendered output; the real unreleased range renders the same entries.

### 2. Least privilege and an honest failure status

- **What changed**: the validation job holds `pull-requests: read` instead of `write`; the action only reads the pull request and posts commit statuses, which is what `statuses: write` covers. The failure status distinguishes an invalid title from a validation that never ran, and links to the workflow run.
- **Why**: the `write` scope had no consumer, and its comment attributed it to `wip`, which needs `statuses: write` instead. The failure step fired on any failure of the action, so an API error would have been reported to contributors as "PR title validation failed"; and a failed status without a link sends them searching for the run the contributing guide tells them to read.
- **Value**: the token carries only what the job uses, and a red status says whether the title is wrong or the check is broken, one click from the log.
- **Impact**: none on valid titles or the `[WIP]` path.

## Findings left as-is

- **Two jobs share the display name** "Validate pull request title": only one runs per event, so the check list shows one entry either way.
- **Required context**: the ruleset change is a maintainer action after merge, as the pull request body says; the current required checks on `main` are Functional Test Run, Lint, Build Check, and DCO.

## Verification

- actionlint and Prettier pass for the workflow; the changelog configuration test passes with the trimmed parser list; shfmt and ShellCheck are clean; markdownlint passes for the contributing guide and these notes.
- The action's source at the pinned commit reads the pull request with `pulls.get` and `pulls.listCommits` and writes only commit statuses, and its own README examples grant `pull-requests: read`.
