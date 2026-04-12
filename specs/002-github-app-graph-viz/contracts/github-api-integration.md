# Contract: GitHub Contents API Integration

**Version**: 1.0.0
**Consumer**: Browser extension (`GitHubClient`)
**Provider**: GitHub REST API v3

## Endpoints Used

### Fetch Static Graph Artifact

```
GET /repos/{owner}/{repo}/contents/.radius/static/app.json?ref={branch}
Accept: application/vnd.github.v3.raw
Authorization: token {oauth_token}
```

**Parameters**:

| Parameter | Location | Required | Description |
|-----------|----------|----------|-------------|
| `owner` | path | Yes | Repository owner (user or org) |
| `repo` | path | Yes | Repository name |
| `branch` | query (`ref`) | Yes | Branch name (e.g., `main`, `feature/add-redis`) |
| `oauth_token` | header | Yes | GitHub OAuth token from device flow auth |

**Success Response** (200):
- Raw JSON content of the static graph artifact (see `static-graph-artifact.md`)

**Error Responses**:

| Status | Meaning | Extension Behavior |
|--------|---------|-------------------|
| 404 | Artifact not found (CI hasn't run yet) | Display: "Application graph not yet available — waiting for CI to build." |
| 401 | Authentication failure | Prompt re-authentication |
| 403 | Insufficient permissions or rate limit | Display appropriate error message |

### Check for App Definition File

```
GET /repos/{owner}/{repo}/contents/app.bicep?ref={branch}
Accept: application/vnd.github.v3+json
Authorization: token {oauth_token}
```

**Purpose**: Detect presence of `app.bicep` to determine whether to inject graph features.

**Success Response** (200): File metadata (existence confirmed)
**Error Response** (404): No app definition file; do not inject graph features.

### Fetch PR Details

```
GET /repos/{owner}/{repo}/pulls/{pull_number}
Accept: application/vnd.github.v3+json
Authorization: token {oauth_token}
```

**Purpose**: Get base repository/ref and head repository/ref for diff visualization.

**Key fields from response**:
- `base.ref`: Base branch name (for fetching base graph)
- `base.repo.owner.login` and `base.repo.name`: Base repository coordinates
- `head.ref`: PR head branch name (for fetching PR graph)
- `head.repo.owner.login` and `head.repo.name`: Head repository coordinates for forked PRs
- `changed_files`: Number of changed files (for optimization)

## Rate Limits

- Authenticated: 5,000 requests/hour
- The extension makes at most 4–5 API calls per page navigation (check file existence, fetch PR metadata, fetch base artifact, fetch head artifact)
- No caching layer needed for initial implementation; browser caching headers from GitHub API are sufficient

## Authentication Flow

The extension uses GitHub Device Flow OAuth (already implemented):
1. User enters GitHub App slug and Client ID
2. Extension requests device code from `https://github.com/login/device/code`
3. User authenticates at `https://github.com/login/device`
4. Extension polls for access token
5. Token stored in `chrome.storage.local`
