# Dependency Review workflow

This workflow runs GitHub's **Dependency Review** on pull requests to catch newly introduced risky dependency changes.

## When it runs

- **Pull Requests**: Runs on every PR targeting `main`.

## What it checks

- Uses `actions/dependency-review-action` to diff dependencies between the PR and the base branch.
- Reports newly introduced vulnerabilities (and license issues when available).

## Repo-specific behavior

- `comment-summary-in-pr: on-failure` posts/updates a PR comment only when the check fails.

## Where to see results

- GitHub Actions run -> **Dependency Review** job logs + job summary.
- On failure, a PR comment summary is also posted.

## References

- <https://github.com/actions/dependency-review-action?tab=readme-ov-file>
- <https://docs.github.com/en/code-security/supply-chain-security/understanding-your-software-supply-chain/about-dependency-review>
