# Quickstart: Git App Graph Preview

This guide walks you through generating and using app graphs from Bicep files.

## Prerequisites

- [Radius CLI](https://docs.radapp.io/getting-started/install/) (v0.35.0+)
- [Bicep CLI](https://learn.microsoft.com/en-us/azure/azure-resource-manager/bicep/install) (installed automatically by `rad bicep download`)
- A Bicep file defining a Radius application
- Git repository (optional, for git metadata enrichment)

## 1. Generate an App Graph

Given a Bicep file `app.bicep`:

```bicep
extension radius

@description('The application name')
param appName string = 'myapp'

resource app 'Applications.Core/applications@2023-10-01-preview' = {
  name: appName
  properties: {
    environment: environment
  }
}

resource frontend 'Applications.Core/containers@2023-10-01-preview' = {
  name: 'frontend'
  properties: {
    application: app.id
    container: {
      image: 'myapp/frontend:v1.0.0'
      ports: {
        web: { containerPort: 3000 }
      }
    }
    connections: {
      backend: { source: backend.id }
    }
  }
}

resource backend 'Applications.Core/containers@2023-10-01-preview' = {
  name: 'backend'
  properties: {
    application: app.id
    container: {
      image: 'myapp/backend:v1.0.0'
      ports: {
        api: { containerPort: 8080 }
      }
    }
    connections: {
      cache: { source: cache.id }
    }
  }
}

resource cache 'Applications.Datastores/redisCaches@2023-10-01-preview' = {
  name: 'cache'
  properties: {
    application: app.id
    environment: environment
  }
}
```

Generate the app graph:

```bash
rad app graph app.bicep
```

This creates `.radius/app-graph.json` with the full topology.

## 2. View the Output

The JSON output shows resources and their connections:

```json
{
  "metadata": {
    "generatedAt": "2026-02-04T00:58:00Z",
    "radiusCliVersion": "0.35.0",
    "sourceFiles": ["app.bicep"],
    "sourceHash": "sha256:abc123...",
    "gitCommit": "def456"
  },
  "resources": [
    {
      "id": ".../Applications.Core/containers/frontend",
      "name": "frontend",
      "type": "Applications.Core/containers",
      "gitInfo": {
        "commitSha": "def456...",
        "author": "you@example.com",
        "date": "2026-02-03T15:30:00Z",
        "message": "Add frontend container"
      }
    }
    // ... more resources
  ],
  "connections": [
    {
      "sourceId": ".../containers/frontend",
      "targetId": ".../containers/backend",
      "type": "connection"
    }
    // ... more connections
  ]
}
```

## 3. Generate Markdown Preview

For a human-readable preview with Mermaid diagram:

```bash
rad app graph app.bicep --format markdown
```

This creates both `.radius/app-graph.json` and `.radius/app-graph.md`:

```markdown
# App Graph: myapp

## Resources

| Name | Type | Source | Last Commit |
|------|------|--------|-------------|
| frontend | Applications.Core/containers | app.bicep:12 | [def456](../../commit/def456) |
| backend | Applications.Core/containers | app.bicep:25 | [def456](../../commit/def456) |
| cache | Applications.Datastores/redisCaches | app.bicep:40 | [abc123](../../commit/abc123) |

## Topology

\```mermaid
graph LR
    frontend[frontend]
    backend[backend]
    cache[(cache)]
    
    frontend --> backend
    backend --> cache
\```
```

## 4. Commit the Graph

The app graph is designed to be committed alongside your Bicep files:

```bash
git add app.bicep .radius/app-graph.json
git commit -m "Add Redis cache to application"
```

## 5. Set Up GitHub Action for PR Diffs

Add `.github/workflows/app-graph-diff.yml`:

```yaml
name: App Graph Diff

on:
  pull_request:
    paths:
      - '**/.radius/app-graph.json'
  push:
    branches:
      - main
    paths:
      - '**/.radius/app-graph.json'

permissions:
  pull-requests: write

jobs:
  diff:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - uses: radius-project/app-graph-diff-action@v1
        with:
          github-token: ${{ secrets.GITHUB_TOKEN }}
```

When a PR changes the app graph, the Action posts a comment showing:
- Added/removed/modified resources
- New/removed connections
- Before/after Mermaid diagrams

## 6. Validate Graph Freshness in CI

Add validation to catch stale graphs:

```yaml
name: Validate App Graph

on:
  pull_request:
    paths:
      - '**/*.bicep'
      - '**/.radius/app-graph.json'

jobs:
  validate:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      
      - name: Install Radius CLI
        run: |
          curl -fsSL https://get.radapp.io/install.sh | bash
          rad bicep download
      
      - name: Validate graph is current
        run: |
          rad app graph app.bicep --stdout > /tmp/expected.json
          diff .radius/app-graph.json /tmp/expected.json || {
            echo "::error::App graph is stale. Run 'rad app graph app.bicep' and commit."
            exit 1
          }
```

## Common Options

| Option | Description |
|--------|-------------|
| `--stdout` | Write JSON to stdout instead of file |
| `-o <file>` | Write to custom output path |
| `--format markdown` | Also generate Markdown preview |
| `--no-git` | Skip git metadata (faster) |
| `--parameters <file>` | Use parameter file for Bicep |
| `--at <commit>` | Generate graph at specific commit |

## Troubleshooting

### "Bicep CLI not found"

```bash
rad bicep download
```

### "Not a git repository"

Git metadata is optional. The graph generates successfully, but `gitInfo` fields show "not available".

### "Stale graph detected in CI"

Regenerate and commit:

```bash
rad app graph app.bicep
git add .radius/app-graph.json
git commit --amend --no-edit
git push --force-with-lease
```

## Next Steps

- [View graph history](./history.md) - Track architecture evolution over time
- [Compare environments](./environments.md) - See how portable types resolve differently
- [Customize the GitHub Action](./github-action.md) - Advanced configuration options
