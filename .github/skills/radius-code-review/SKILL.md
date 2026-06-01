---
name: radius-code-review
description: 'Perform an automated code review for a GitHub pull request in the Radius repository. Produces a PR analysis document, a PR review document, and a shell script that posts the review comments via the GitHub API. Use when asked to review a PR, generate review feedback for a PR, or create a script that posts PR review comments.'
argument-hint: 'Provide the PR number (and optionally repository owner/name) to review'
user-invocable: true
---

# Radius Code Review

Use this skill to perform an automated, end-to-end code review for a Radius pull request. The skill produces three artifacts in the `.copilot-tracking/` folder at the repository root:

1. `pr-analysis-${prNumber}.md` — file-by-file analysis of the PR
2. `pr-review-${prNumber}.md` — review comments and overall assessment
3. `pr-review-${prNumber}.sh` — shell script that posts the review to GitHub via the REST API

**Important**: Follow the code review guidelines defined in [`.github/instructions/code-review.instructions.md`](../../instructions/code-review.instructions.md) for the review process, principles, code quality criteria, and validation steps. This skill defines only the file creation and script generation workflow specific to this automated review.

## When to Use

- Reviewing a specific pull request and producing structured feedback
- Generating per-file analysis and review comments for a PR
- Producing a runnable script that posts review comments back to the PR

Do not use this skill for:

- General code authoring or refactoring tasks
- Documentation-only changes (use `contributing-docs-updater` instead when contributor doc impact is the focus)
- Reviewing local uncommitted changes (use the built-in `code-review` agent)

## Inputs

Before starting, confirm:

- **PR number** (required)
- **Repository owner / name** (defaults to `radius-project/radius` if not specified)
- Access to PR metadata, file diffs, and the PR description

If the PR number is not provided and cannot be inferred, ask the user for it.

## Preparation

1. Read [`.github/instructions/code-review.instructions.md`](../../instructions/code-review.instructions.md) and follow its **Before Starting a Review** section.
2. Create the `.copilot-tracking/` folder at the repository root if it does not exist.
3. Fetch the PR (title, description, changed files, diffs, linked issues, prior discussion) using `gh` or the GitHub API.

## Step 1: Analyze the Changes

Follow the **Step 1: Analyze the Changes** section in `.github/instructions/code-review.instructions.md`.

Create `.copilot-tracking/pr-analysis-${prNumber}.md` containing:

- **PR Summary** — purpose, scope, and linked issues
- **File-by-file analysis** — for each changed file:
  - File purpose and role in the codebase
  - Specific changes made
  - Impact assessment

Consider both the PR author's description and the actual diff when writing the analysis.

## Step 2: Provide Review Feedback

Follow the **Step 2: Provide Review Feedback** section in `.github/instructions/code-review.instructions.md`, including all General Code Quality Criteria and Unit Test Review Criteria. Apply language-specific guidance from the relevant files in `.github/instructions/` (Go, Shell, Make, Docker, GitHub Workflows, Bicep).

Create `.copilot-tracking/pr-review-${prNumber}.md` containing:

- **PR title review** — flag vague or inaccurate titles and suggest improvements
- **Overall PR assessment** — summary of key findings and recommendations
- **Per-file review comments** in this format:

  ```text
  path/to/file.ext
      Line X: Specific issue description
      Line Y: Suggestion for improvement
  ```

Keep comments concise, actionable, and specific. Remove purely complimentary comments.

## Step 3: Validate Your Review

Follow the **Step 3: Validate Your Review** section in `.github/instructions/code-review.instructions.md`. Verify:

- File paths and line numbers are correct
- Comments are clear and actionable
- No purely complimentary noise remains
- Findings align with the actual diff

## Step 4: Assess Contributor Documentation Impact

Per the **Step 4** guidance in the code review instructions, use the [`contributing-docs-updater`](../contributing-docs-updater/SKILL.md) skill to determine whether contributor documentation in `docs/contributing/` or `docs/architecture/` needs updates. Record the assessment in the overall assessment section of `pr-review-${prNumber}.md`.

## Step 5: Generate the Review-Posting Script

Generate `.copilot-tracking/pr-review-${prNumber}.sh`. Do **not** execute the script.

Requirements:

- Use the GitHub REST API (`POST /repos/{owner}/{repo}/pulls/{pull_number}/reviews`) rather than `gh` CLI, because the API supports posting multiple inline comments in one review. See [GitHub docs](https://docs.github.com/en/rest/pulls/reviews?apiVersion=2022-11-28#create-a-review-for-a-pull-request).
- Iterate over each comment in `pr-review-${prNumber}.md` and include it in the review payload.
- Include an overall review body taken from the overall PR assessment section.
- Use `jq` for both response parsing and request payload construction. Never build JSON by string interpolation, and never parse GitHub API responses with `grep`/`sed` — the PR response contains multiple `"sha":` fields and arbitrary field ordering.
- Read `GITHUB_TOKEN` from the environment; fail fast with a clear error if it is unset. Fail fast with a clear error if `jq` is not installed.
- Accept `PR_NUMBER` (and optionally `REPO_OWNER`/`REPO_NAME`) from environment variables so the same script can be re-run against a different PR without editing the file. Default `REPO_OWNER`/`REPO_NAME` to `radius-project`/`radius`.
- Resolve the head commit SHA from the PR via `jq -r '.head.sha // empty'`, not `HEAD`.
- Print success or failure messages with the review URL or response body.

Reference script structure:

```bash
#!/bin/bash
#
# GitHub PR Review Script
# Posts a review with multiple inline comments via the GitHub REST API.
#
# Usage:
#   export GITHUB_TOKEN=<your_token>
#   PR_NUMBER=1234 ./pr-review-1234.sh
#
# Required tools: curl, jq

set -euo pipefail

REPO_OWNER="${REPO_OWNER:-radius-project}"
REPO_NAME="${REPO_NAME:-radius}"
PR_NUMBER="${PR_NUMBER:-}"   # Override at runtime, or hard-code per PR

if [ -z "${GITHUB_TOKEN:-}" ]; then
    echo "Error: GITHUB_TOKEN environment variable is not set" >&2
    echo "Please set it with: export GITHUB_TOKEN=your_token_here" >&2
    exit 1
fi

if [ -z "${PR_NUMBER}" ]; then
    echo "Error: PR_NUMBER is not set" >&2
    exit 1
fi

if ! command -v jq >/dev/null 2>&1; then
    echo "Error: jq is required but not installed" >&2
    exit 1
fi

echo "Creating GitHub review for PR #${PR_NUMBER}..."
echo "Repository: ${REPO_OWNER}/${REPO_NAME}"

# Resolve the head commit SHA from the PR.
PR_RESPONSE=$(curl -sS \
    -H "Accept: application/vnd.github+json" \
    -H "Authorization: Bearer ${GITHUB_TOKEN}" \
    -H "X-GitHub-Api-Version: 2022-11-28" \
    "https://api.github.com/repos/${REPO_OWNER}/${REPO_NAME}/pulls/${PR_NUMBER}")

COMMIT_SHA=$(echo "${PR_RESPONSE}" | jq -r '.head.sha // empty')

if [ -z "${COMMIT_SHA}" ]; then
    echo "❌ Could not resolve head commit SHA from PR response" >&2
    echo "Response: ${PR_RESPONSE}" >&2
    exit 1
fi

echo "Using commit SHA: ${COMMIT_SHA}"

REVIEW_BODY="Overall assessment goes here."

# One object per inline comment. Use --arg to escape body text safely.
COMMENTS_JSON=$(jq -n '
[
    {path: $p1, line: 1, side: "RIGHT", body: $b1}
]
' \
    --arg p1 "path/to/file.ext" \
    --arg b1 "Comment body (jq handles escaping).")

PAYLOAD=$(jq -n \
    --arg commit_id "${COMMIT_SHA}" \
    --arg body "${REVIEW_BODY}" \
    --arg event "COMMENT" \
    --argjson comments "${COMMENTS_JSON}" \
    '{commit_id: $commit_id, body: $body, event: $event, comments: $comments}')

RESPONSE=$(curl -sS -X POST \
    -H "Accept: application/vnd.github+json" \
    -H "Authorization: Bearer ${GITHUB_TOKEN}" \
    -H "X-GitHub-Api-Version: 2022-11-28" \
    "https://api.github.com/repos/${REPO_OWNER}/${REPO_NAME}/pulls/${PR_NUMBER}/reviews" \
    -d "${PAYLOAD}")

REVIEW_ID=$(echo "${RESPONSE}" | jq -r '.id // empty')

if [ -n "${REVIEW_ID}" ]; then
    echo "✅ Review created successfully!"
    echo "Review ID: ${REVIEW_ID}"
    echo "Review URL: https://github.com/${REPO_OWNER}/${REPO_NAME}/pull/${PR_NUMBER}"
else
    echo "❌ Failed to create review" >&2
    echo "Response: ${RESPONSE}" >&2
    exit 1
fi
```

Choose the review `event` value based on the overall assessment:

- `COMMENT` for informational reviews
- `REQUEST_CHANGES` when blocking issues exist
- `APPROVE` only when explicitly requested by the user

## Outputs

After the skill runs, the user has:

- `.copilot-tracking/pr-analysis-${prNumber}.md`
- `.copilot-tracking/pr-review-${prNumber}.md`
- `.copilot-tracking/pr-review-${prNumber}.sh` (not executed)

Report the three file paths and remind the user to set `GITHUB_TOKEN` and run the script themselves to post the review.

## Example Prompts

- `/radius-code-review Review PR #1234`
- `/radius-code-review Generate review artifacts for radius-project/radius#5678`
- `/radius-code-review Produce a review and posting script for the active PR`

## Quality Checklist

Before finishing:

- [ ] `.copilot-tracking/` exists and contains all three artifacts
- [ ] Analysis covers every changed file in the PR
- [ ] Review comments are specific, actionable, and free of generic praise
- [ ] All cited file paths and line numbers match the PR diff
- [ ] Generated script passes `shellcheck -x` with no errors (variables quoted, JSON built via `jq`)
- [ ] Script uses the resolved head commit SHA, not `HEAD`
- [ ] Script was created but not executed
- [ ] Contributor doc impact assessed via `contributing-docs-updater`
