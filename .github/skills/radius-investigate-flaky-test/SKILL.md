---
name: radius-investigate-flaky-test
description: 'Investigate a GitHub issue labeled flaky-test in the Radius repository: measure how often it hits CI, determine whether the root cause is a product defect or a test defect, propose and optionally implement the best fix, and post the findings as an issue comment. Use when asked to investigate, triage, or fix a flaky test, or when a flaky-test issue is assigned automatically.'
argument-hint: 'Issue number or URL for the flaky-test issue (e.g. 12575)'
user-invocable: true
---

# Investigate a flaky test

Backing doc: [Investigating flaky tests](../../../docs/contributing/contributing-code/contributing-code-tests/investigating-flaky-tests.md). This is the source of truth for every step below — follow it exactly, including the exact `gh` commands and the `pkg/retry` usage pattern.

## When to Use

- An issue carries the `flaky-test` label and needs investigation.
- You were asked to estimate how often a specific test fails in CI.
- You were asked to fix a flaky test and the fix should prefer the repository's retry library over ad hoc retry loops.
- The [`flaky-test-investigation.yml`](../../../.github/workflows/flaky-test-investigation.yml) automation assigned you to a `flaky-test` issue.

Do not use this skill for:

- General issue investigation unrelated to test flakiness (use `issue-investigator`).
- Reviewing a pull request (use `radius-code-review`).

## Inputs

- **Issue number or URL** (required; ask if not provided and cannot be inferred from context).
- **Whether to open a pull request** (optional; default to investigation + issue comment only unless the user asks for a fix to be implemented and submitted).

## 1. Load the issue and confirm scope

```sh
gh issue view --repo radius-project/radius <issue-number> --json title,body,labels,state,assignees,comments
```

Confirm the `flaky-test` label is present. Read the issue body for the failing test name, the suite, and any linked CI run URL. If the issue does not name a specific test or suite, search recent CI failures for a match before proceeding, and ask the user for clarification if the target test still cannot be identified.

If you are an external contributor automation considering opening a pull request, re-check the [external automated contribution eligibility](../../../AGENTS.md#external-automated-contributions) rules before implementing anything — this skill's own investigation and comment steps are always allowed, but opening a PR is not, unless those conditions are met.

## 2. Measure the flake frequency

Follow [step 2 of the backing doc](../../../docs/contributing/contributing-code/contributing-code-tests/investigating-flaky-tests.md#2-measure-how-often-the-flake-hits) exactly: identify the workflow that runs the suite, pull recent run history with `gh run list`, and count failures mentioning the test name with `gh run view --log-failed`. Cross-reference other open or closed `flaky-test` issues for the same test. Produce a frequency figure with its sample size and lookback window, plus links to every failed run counted — never report a bare percentage without that evidence.

## 3. Classify the root cause

Follow [step 3 of the backing doc](../../../docs/contributing/contributing-code/contributing-code-tests/investigating-flaky-tests.md#3-determine-product-defect-vs-test-defect). Read the failure output and the product/test code involved, then classify as a **product defect** or a **test defect**, citing the specific evidence (stack trace, timeout vs. invariant-violation error, `git blame` on recently changed code, CI load correlation). If the evidence is inconclusive, say so rather than guessing.

## 4. Propose the best fix

Follow [step 4 of the backing doc](../../../docs/contributing/contributing-code/contributing-code-tests/investigating-flaky-tests.md#4-propose-and-choose-a-fix). List the realistic options and recommend one:

- A genuine product bug needs a product-code fix, not a retry.
- A transient/external-dependency or eventual-consistency test defect should be wrapped with [`pkg/retry`](../../../pkg/retry/retry.go) (`retry.NewDefaultRetryer()` for the standard exponential backoff, or `retry.NewRetryer` with a custom `RetryConfig` when the default does not fit) — never a bespoke retry loop or a fixed `time.Sleep` poll.
- A race/ordering test defect needs correct synchronization, not a retry around an incorrect assertion.

## 5. Implement and validate (when a fix is requested)

Make the change, then validate per [step 5 of the backing doc](../../../docs/contributing/contributing-code/contributing-code-tests/investigating-flaky-tests.md#5-implement-and-validate-the-fix). **If a PR is requested**, the fix must be verified by repeating the specific test (`go test ./path/... -run TestName -count=N -race`):

- A **minimum of 20 runs**, all passing.
- **100 runs preferred** when the test is "fast" (a single run completes in well under a few seconds).
- If the right count is in question (e.g. you cannot tell how fast the test is, or there's any other ambiguity), **ask the user** explicitly rather than guessing.

Also run the suite's normal tier command, and add regression coverage for a product-code fix where feasible.

## 6. Post findings to the issue

Compose one comment covering frequency (with evidence), root cause (with evidence), fix options considered, and the recommended or applied fix. Post it with:

```sh
gh issue comment --repo radius-project/radius <issue-number> --body-file findings.md
```

Do not skip this step even when no fix is implemented — the investigation itself is the deliverable unless a PR is also requested.

## 7. Open a pull request (only if explicitly requested)

Re-verify PR eligibility (see step 1) immediately before creating the PR. Then follow [step 7 of the backing doc](../../../docs/contributing/contributing-code/contributing-code-tests/investigating-flaky-tests.md#7-open-a-pull-request-only-if-requested):

- Commit with both flags: `git commit -s -S -m "..."`.
- Reference `Fixes: #<issue-number>` in the commit message.
- Create the PR with the required release-impact label:

  ```sh
  gh pr create --repo radius-project/radius --label pr:standard --title "fix: <description>" --body-file pr-body.md
  ```

- Fill in the PR template's test-instructions section with the exact commands from step 5 and their results.

## Example Prompts

- `/radius-investigate-flaky-test Investigate issue #12575`
- `/radius-investigate-flaky-test Investigate #12575 and open a PR with the fix`

## Checklist

- [ ] `flaky-test` label confirmed on the issue
- [ ] Flake frequency reported with sample size, lookback window, and run links
- [ ] Root cause classified as product defect or test defect, with evidence
- [ ] Fix options listed; recommended fix uses `pkg/retry` when retrying is appropriate
- [ ] Findings posted as an issue comment
- [ ] If a PR was opened: fix verified with a passing repeat-run loop (≥20 runs, 100 preferred for fast tests; iteration count confirmed with the user if unclear)
- [ ] If a PR was opened: commits signed off and signed (`-s -S`), `pr:standard` or `pr:important` label applied, issue linked, PR eligibility re-checked
