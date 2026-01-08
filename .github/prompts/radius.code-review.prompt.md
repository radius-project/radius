---
tools: ['edit/createFile', 'edit/createDirectory', 'edit/editFiles', 'search/codebase', 'githubRepo', 'github.vscode-pull-request-github/issue_fetch', 'github.vscode-pull-request-github/searchSyntax', 'github.vscode-pull-request-github/doSearch', 'github.vscode-pull-request-github/activePullRequest', 'todos']
description: 'Perform a code review for a pull request (PR) in a GitHub repository.'
---

Context: ${workspaceFolder}

PR is an acronym for Pull Request.

**Important**: Follow the code review guidelines defined in `.github/instructions/code-review.instructions.md` for the review process, review principles, code quality criteria, and validation steps. This prompt file defines only the file creation and script generation requirements specific to this automated review workflow.

You will create three files as part of this process: two markdown documents and one shell script. The first markdown document will detail the changes made in the PR on a file-by-file basis. The second markdown document will contain your review comments for each file and an overall assessment of the PR. The shell script will use the GitHub API to post your review comments from the second markdown file to the PR.

All files you create as part of this PR review will go into a folder named `.copilot-tracking` at the root of the project. If this folder does not exist, create it.

**Before Starting:**
Follow the "Before Starting a Review" section in `.github/instructions/code-review.instructions.md`, and additionally:
1. Create the `.copilot-tracking` folder if it does not exist.

**Step 1: Analyze the Changes**
Follow the "Step 1: Analyze the Changes" section in `.github/instructions/code-review.instructions.md`.
For this pr, ${activePullRequest}, create a markdown document in the `.copilot-tracking` folder that describes in detail how each file has changed and what each file does, and what the changes are. Consider what the PR author wrote in the PR description as well as the changes that exist in each file.

Create `.copilot-tracking/pr-analysis-${prNumber}.md` with:
- PR Summary section
- File-by-file analysis with:
  - File purpose and role
  - Specific changes made
  - Impact assessment

**Step 2: Provide Review Feedback**
Follow the "Step 2: Provide Review Feedback" section in `.github/instructions/code-review.instructions.md`, including all General Code Quality Criteria and Unit Test Review Criteria.

Create `.copilot-tracking/pr-review-${prNumber}.md` with:
- Overall PR assessment
- Per-file review comments in this format:
    path/to/file.ext
        Line X: Specific issue description
        Line Y: Suggestion for improvement

**Step 3: Validate Your Review**
Follow the "Step 3: Validate Your Review" section in `.github/instructions/code-review.instructions.md` to ensure accuracy, clarity, value, and correctness of the review.

**Step 4: Generate a script for posting the review**

You are a shell scripting expert and your job is to generate a shell script named `pr-review-${prNumber}.sh`.
- Do not execute the script - just create it for me.
- Use the github API to add these comments to the review instead of the github CLI because the api supports adding multiple comments to a single review. GH CLI documentation is here: https://docs.github.com/en/rest/pulls/reviews?apiVersion=2022-11-28#create-a-review-for-a-pull-request.
- Iterate over each comment in the `pr-review-${prNumber}.md` file and add it to the review script. Add an overall review comment from the overall PR assessment section of the `pr-review-${prNumber}.md` file.
- When adding PR comments to the review script, make sure that characters are properly escaped to avoid json parsing errors.
- A sample script is below:

```bash
#!/bin/bash

# GitHub PR Review Script
# This script creates a GitHub review with multiple inline comments using the GitHub API

# Configuration
GITHUB_TOKEN="${GITHUB_TOKEN}"  # Set this environment variable with your GitHub token
REPO_OWNER="radius-project"
REPO_NAME="radius"
PR_NUMBER="" # Add PR number here
COMMIT_SHA="HEAD"  # You may want to replace this with the actual commit SHA

# Check if GitHub token is set
if [ -z "$GITHUB_TOKEN" ]; then
    echo "Error: GITHUB_TOKEN environment variable is not set"
    echo "Please set it with: export GITHUB_TOKEN=your_token_here"
    exit 1
fi

# Function to create the review with comments
create_review() {
    # shellcheck disable=SC2016
    curl -X POST \
        -H "Accept: application/vnd.github+json" \
        -H "Authorization: Bearer $GITHUB_TOKEN" \
        -H "X-GitHub-Api-Version: 2022-11-28" \
        "https://api.github.com/repos/$REPO_OWNER/$REPO_NAME/pulls/$PR_NUMBER/reviews" \
        -d '{
            "commit_id": "'"$COMMIT_SHA"'",
            "body": "Thank you for the contribution! There are a few comments below but I am tagging @ytimocin for review as well given his expertise in the command structure.",
            "event": "REQUEST_CHANGES",
            "comments": [
                {
                    "path": "pkg/cli/delete/types.go",
                    "line": 28,
                    "body": "The interface comment mentions \"Bicep deployments\" but this is for delete operations. Consider updating the comment to accurately reflect the delete functionality."
                },
                {
                    "path": "pkg/cli/cmd/app/delete/delete_test.go",
                    "line": 128,
                    "body": "The test setup is comprehensive. Consider adding tests for error scenarios in the progress reporting system."
                }
            ]
        }'
}

echo "Creating GitHub review for PR #$PR_NUMBER..."
echo "Repository: $REPO_OWNER/$REPO_NAME"
echo ""

# Get the latest commit SHA for the PR
echo "Getting latest commit SHA for PR..."
COMMIT_RESPONSE=$(curl -s \
    -H "Accept: application/vnd.github+json" \
    -H "Authorization: Bearer $GITHUB_TOKEN" \
    -H "X-GitHub-Api-Version: 2022-11-28" \
    "https://api.github.com/repos/$REPO_OWNER/$REPO_NAME/pulls/$PR_NUMBER")

# Extract the head commit SHA from the PR response
COMMIT_SHA=$(echo "$COMMIT_RESPONSE" | grep -o '"sha": *"[^"]*"' | head -1 | sed 's/.*"sha": *"\([^"]*\)".*/\1/')

if [ -z "$COMMIT_SHA" ]; then
    echo "❌ Could not get commit SHA from PR response"
    echo "Response: $COMMIT_RESPONSE"
    exit 1
else
    echo "Using commit SHA: $COMMIT_SHA"
fi

echo ""
echo "Creating review..."

# Create the review
RESPONSE=$(create_review)

# Check if the request was successful
if echo "$RESPONSE" | grep -q '"id"'; then
    echo "✅ Review created successfully!"
    REVIEW_ID=$(echo "$RESPONSE" | grep -o '"id":[0-9]*' | head -1 | cut -d':' -f2)
    echo "Review ID: $REVIEW_ID"
    echo "Review URL: https://github.com/$REPO_OWNER/$REPO_NAME/pull/$PR_NUMBER"
else
    echo "❌ Failed to create review"
    echo "Response: $RESPONSE"
    exit 1
fi

```
