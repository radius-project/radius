# Investigating flaky tests

## Purpose

This guide explains how to investigate an issue labeled [`flaky-test`](https://github.com/radius-project/radius/issues?q=is%3Aissue+is%3Aopen+label%3Aflaky-test): how to measure how often the flake actually hits CI, how to decide whether the root cause is a product defect or a defect in the test itself, how to choose and implement a fix, and how to report findings back on the issue. It backs the [`radius-investigate-flaky-test`](../../../../.github/skills/radius-investigate-flaky-test/SKILL.md) skill, which any engineer (or the [automatic trigger](#automatic-triggering) described below) can run against a `flaky-test` issue.

## Prerequisites

- [`gh` CLI](https://cli.github.com/) authenticated against `radius-project/radius`.
- The [basic prerequisites](../contributing-code-prerequisites/) for building and running the test suite (see the [test matrix](./README.md#test-matrix)).
- Read access to GitHub Actions run history for the repository (needed to pull historical pass/fail data).

## Steps

### 1. Identify the failing test and its workflow

Read the issue body for the exact test name (for example `Test_CLI_Delete/Validate_rad_app_delete_with_non_empty_resources`), the suite it belongs to, and any linked CI run. Match the suite to its workflow file so you know where to pull history from:

| Suite                  | Workflow                                                                                       | Command                           |
|------------------------|------------------------------------------------------------------------------------------------|-----------------------------------|
| Unit / integration     | [`unit-tests.yaml`](../../../../.github/workflows/unit-tests.yaml)                             | `make test`                       |
| Functional (non-cloud) | [`functional-test-noncloud.yaml`](../../../../.github/workflows/functional-test-noncloud.yaml) | `make test-functional-*-noncloud` |
| Functional (cloud)     | [`functional-test-cloud.yaml`](../../../../.github/workflows/functional-test-cloud.yaml)       | `make test-functional-*-cloud`    |
| Nightly `rad` CLI      | [`nightly-rad-CLI-tests.yaml`](../../../../.github/workflows/nightly-rad-CLI-tests.yaml)       | n/a (scheduled)                   |

### 2. Measure how often the flake hits

Pull recent run history for the matching workflow and count failures that mention the test name:

```sh
# List recent runs for the workflow (adjust --limit for a longer lookback window)
gh run list --repo radius-project/radius --workflow unit-tests.yaml \
  --json databaseId,conclusion,createdAt,headBranch --limit 200

# For each failed run, search the failed-step logs for the test name
gh run view --repo radius-project/radius <run-id> --log-failed | grep -i '<test name>'
```

For a faster signal across many runs, search Actions logs (or job annotations) with GitHub search, and cross-reference any other open `flaky-test` issues or closed duplicates for the same test:

```sh
gh issue list --repo radius-project/radius --search '"<test name>" in:title,body label:flaky-test'
gh search issues --repo radius-project/radius '<test name>'
```

Report the flake rate as `<failures> / <runs inspected>` over the lookback window (for example "4 / 180 runs in the last 30 days ≈ 2.2%"), plus links to the specific failed runs you counted. State the lookback window and run count so the figure is reproducible — do not report a bare percentage without the sample size.

### 3. Determine product defect vs. test defect

Read the failure output and the test and product code it exercises, then classify the root cause:

- **Product defect** — the product code has a real race condition, incorrect ordering, resource leak, or timing bug that a user could also hit outside of CI. Evidence: the failure reproduces with `-race`, the error indicates an invariant violation (not a timeout), or `git log`/`git blame` on the affected product code shows a recent change that plausibly introduced the race.
- **Test defect** — the test itself is the problem: a fixed/short timeout racing a genuinely slower operation, an assumption about ordering between concurrent goroutines or async operations, a shared/global test fixture, reliance on an external flaky dependency (network, an external registry, a cloud provider quota), or insufficient wait/poll logic. Evidence: the failure is a context-deadline or timeout error, the same operation succeeds when given more time locally, or the flake correlates with CI load/concurrency rather than a code change.

Back the classification with the specific evidence found (stack trace, run logs, blame output, related issues), not a guess. If evidence is inconclusive, say so explicitly rather than picking a default.

### 4. Propose (and choose) a fix

List the realistic fix options with trade-offs, then recommend one:

- **Product defect** → fix the underlying product bug. A retry in the test only hides the defect and must not be the only change.
- **Test defect caused by a transient/external dependency or eventual consistency** → wrap the flaky operation with the repository's existing retry library, [`pkg/retry`](../../../../pkg/retry/retry.go), instead of a bespoke retry loop or `time.Sleep` poll:

  ```go
  import "github.com/radius-project/radius/pkg/retry"

  retryer := retry.NewDefaultRetryer() // exponential backoff, 10 max retries, 60s max duration
  err := retryer.RetryFunc(ctx, func(ctx context.Context) error {
      if err := doFlakyThing(ctx); err != nil {
          return retry.RetryableError(err) // only retryable errors are retried
      }
      return nil
  })
  ```

  Use `retry.NewRetryer(&retry.RetryConfig{BackoffStrategy: ...})` when the default backoff (1s initial interval, exponential, 10 retries, 60s max duration) does not fit the operation. Only wrap the specific flaky call in `retry.RetryableError`; do not swallow non-retryable assertion failures.
- **Test defect caused by a race/ordering assumption** → fix the test's synchronization (wait for the actual condition instead of a fixed sleep, use a channel/`sync.WaitGroup`, or serialize the dependent steps) rather than adding a retry around a logically incorrect assertion.
- **Known non-hermetic external dependency** (for example a shared cluster or third-party registry outage) → note it, link the umbrella tracking issue if one exists, and consider whether a retry or a more hermetic replacement (local fixture, mock) is the better long-term fix.

### 5. Implement and validate the fix

- Make the code change (product or test).
- Re-run the specific test enough times to gain real confidence the fix addresses the flake, **only when a pull request is being opened for the fix** (an investigation-only comment does not require this loop):
  - Run it a **minimum of 20 times**.
  - If the test is "fast" (a single run completes in well under a few seconds, so 100 iterations finish in a reasonable time), **prefer 100 runs** instead of 20 for stronger confidence.
  - If you cannot tell whether the test qualifies as "fast" enough for 100 runs, or any other reason makes the correct iteration count unclear, **explicitly ask the user** which count to use rather than guessing.

  ```sh
  # Minimum verification loop
  go test ./path/to/package/... -run TestName -count=20 -race

  # Preferred loop for fast tests
  go test ./path/to/package/... -run TestName -count=100 -race
  ```

  Report the exact count run and the pass/fail outcome of every iteration in the findings comment and the PR body — a fix is not validated until the full loop is clean.
- Run the normal tier command for the suite you touched (see the [test matrix](./README.md#test-matrix)) to confirm no regressions.
- If you changed product code, add or update a regression test that exercises the race/condition directly where feasible.

### 6. Report findings on the issue

Post a single comment on the issue with:

- **Frequency** — the flake rate and lookback window from step 2, with links to the runs counted.
- **Root cause** — product defect or test defect, with the supporting evidence from step 3.
- **Fix options considered** — the alternatives from step 4 and why they were rejected.
- **Recommended / applied fix** — what was changed (or proposed, if no PR was opened yet) and why it addresses the root cause rather than masking it.

```sh
gh issue comment --repo radius-project/radius <issue-number> --body-file findings.md
```

### 7. Open a pull request (only if requested)

Follow the [pull-request guide](../../../contributing-pull-requests/README.md) for commit message format, Conventional Commit title, and review process. Specifically for this workflow:

- Commit with both a DCO sign-off and a cryptographic signature:

  ```sh
  git commit -s -S -m "fix(test): <description>

  Fixes: #<issue-number>"
  ```

- Reference the issue being fixed in the commit message and PR description.
- Apply the required release-impact label:

  ```sh
  gh pr create --repo radius-project/radius --label pr:standard --title "fix: <description>" --body-file pr-body.md
  ```

  A flaky-test fix is routine maintenance, so it uses `pr:standard` unless the fix also changes user-facing behavior (in which case follow the [label guidance](../../../contributing-pull-requests/README.md#5-open-the-pull-request-and-fill-out-the-template) for `pr:important`).
- Fill out the PR template's test-instructions section with the exact verification-loop command run in step 5 (`-count=20` or `-count=100`), the number of iterations, and that every iteration passed.

## Automatic triggering

The [`flaky-test-investigation.yml`](../../../../.github/workflows/flaky-test-investigation.yml) workflow runs whenever the `flaky-test` label is applied to an issue. It assigns the issue to the Copilot coding agent so this investigation starts without manual intervention. Issue assignment to `@copilot` requires a fine-grained personal access token from a Copilot-licensed user (GitHub blocks the default `GITHUB_TOKEN` from triggering the coding agent); a repository maintainer must create that token with `Issues: Read and write` access on this repository and store it in the `COPILOT_ASSIGN_PAT` repository secret. Until that secret is configured, the workflow leaves a comment on the issue noting that automatic assignment is unavailable and that the investigation must be run manually (or via the skill) instead.

## Verification

- The issue has a new comment containing a reproducible frequency figure (with sample size and run links), an evidenced root-cause classification, and the fix options considered.
- Any implemented fix uses `pkg/retry` for retry-based mitigations rather than a bespoke retry loop, unless the root cause is a genuine race requiring correct synchronization instead of a retry.
- If a PR was opened, the fix was verified with a repeated test-run loop of **at least 20 runs** (**100 runs preferred for fast tests**), with every iteration passing, and the exact command and iteration count are recorded in the findings comment and PR body. If the correct iteration count was unclear, the user was asked rather than a count being assumed.
- If a PR was opened, its commits are signed off (`-s`) and cryptographically signed (`-S`), it carries exactly one of `pr:standard`/`pr:important`, and it links the originating issue.

## Troubleshooting

- **No CI history is available for the test.** It may be newly added or renamed; check `git log` on the test file and widen the lookback window, or note in the issue comment that insufficient history exists to compute a reliable frequency.
- **The failure never reproduces locally.** Flaky failures tied to CI concurrency or cluster load may not reproduce on a single local run; rely on the historical run evidence and `-count=N` repetition instead of a single local run.
- **Automatic assignment to `@copilot` did not happen.** Confirm the `COPILOT_ASSIGN_PAT` secret is configured per [Automatic triggering](#automatic-triggering); without it the workflow can only comment, not assign.
