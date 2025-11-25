---
agent: agent
description: Automates GitHub pull request creation from the current branch
name: radius.create-pr
model: Claude Opus 4.5 (Preview) (copilot)
tools:
  ['runCommands', 'edit', 'search', 'fetch', 'githubRepo', 'github.vscode-pull-request-github/issue_fetch', 'github.vscode-pull-request-github/suggest-fix', 'github.vscode-pull-request-github/searchSyntax', 'github.vscode-pull-request-github/doSearch', 'github.vscode-pull-request-github/renderIssues', 'github.vscode-pull-request-github/activePullRequest', 'github.vscode-pull-request-github/openPullRequest'
---

# Create GitHub Pull Request

Create a pull request from the current branch following the steps below.

## Determining the Default Branch

When working with forks, the default branch must come from the correct remote:

- If `upstream` exists in `git remote -v`: use `git remote show upstream | grep 'HEAD branch'`
- Otherwise: use `git remote show origin | grep 'HEAD branch'`

## Steps

### Step 1: Validate Branch State

Stop and inform the user if any of these conditions exist:

- Current branch has no remote tracking branch (`git rev-parse --abbrev-ref --symbolic-full-name @{u}`)
- Uncommitted changes exist (`git status --porcelain`)
- Unpushed commits exist (`git rev-list --count @{u}..HEAD` returns > 0)

### Step 2: Load PR Template

Check for a PR template at these locations (in order):

- `.github/PULL_REQUEST_TEMPLATE.md`
- `.github/pull_request_template.md`
- `docs/PULL_REQUEST_TEMPLATE.md`
- `PULL_REQUEST_TEMPLATE.md`

Use the first template found to structure the PR description.

### Step 3: Generate PR Content

Analyze the changes using `git diff` and `git log` against the default branch.

**PR Title Requirements:**

- Maximum 80 characters
- Use noun phrases, not imperative verbs (e.g., "Authentication improvements for API endpoints" not "Add authentication to API")
- No conventional commit prefixes (`feat:`, `fix:`, etc.)
- Capitalize the first word

**PR Description:**

- Follow the PR template structure if one exists
- Preserve checkboxes from the template (do not convert to bullets)
- If no template: summarize changes, list modified files, include relevant commit context

### Step 4: Create the Pull Request

Create the PR using the GitHub MCP tool, falling back to `gh pr create` if unavailable.

- **head**: current branch
- **base**: default branch (from Step 1)

### Step 5: Report Result

Display the PR URL:

```
✅ Pull request created successfully!
🔗 URL: [PR_URL]
```

## Error Handling

Stop immediately on any failure with a clear error message. Common errors:

- Branch not pushed to remote
- Uncommitted or unpushed changes
- PR already exists for this branch
- Insufficient GitHub permissions
