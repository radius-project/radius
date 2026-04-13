# Quickstart: GitHub App Graph Visualization

**Feature Branch**: `002-github-app-graph-viz`

## Overview

This quickstart walks through setting up and testing the application graph visualization feature end-to-end: from shared schema extension to `rad graph build` output to browser extension rendering.

## Prerequisites

- Node.js 18+ and npm
- Go (version per `go.mod`)
- Chrome or Microsoft Edge
- A GitHub account with a test repository
- Radius dev container or local tooling (see `CONTRIBUTING.md`)

## Step 1: Build the Schema Extension

Add the `codeReference` property to `ApplicationScopedResource` and `GlobalScopedResource`, and extend the graph read model with `codeReference`, `appDefinitionLine`, and `diffHash`, then regenerate:

```bash
# From repo root
make generate
```

Verify the `CodeReference` field appears in the generated Go models:

```bash
grep -n "CodeReference" pkg/corerp/api/v20231001preview/zz_generated_models.go
```

## Step 2: Build and Test `rad graph build`

```bash
# Build the Radius CLI
go build -o bin/rad ./cmd/rad

# Test with a sample Bicep file
bin/rad graph build --bicep app.bicep --output /tmp/graph.json

# Inspect the output
cat /tmp/graph.json | jq '.application.resources[] | {name, type, codeReference, appDefinitionLine, diffHash}'
```

## Step 3: Build and Load the Browser Extension

```bash
cd web/browser-extension
npm install
npm run build
```

Load the extension:
1. Chrome: `chrome://extensions/` → Enable Developer mode → Load unpacked → Select `dist/`
2. Edge: `edge://extensions/` → Enable Developer mode → Load unpacked → Select `dist/`

## Step 4: Test PR Diff Visualization

1. In a test repo, create `app.bicep` on `main`:

```bicep
resource app 'Applications.Core/applications@2023-10-01-preview' = {
  name: 'myapp'
}

resource frontend 'Applications.Core/containers@2023-10-01-preview' = {
  name: 'frontend'
  properties: {
    application: app.id
    container: { image: 'myregistry/frontend:latest' }
    codeReference: 'src/frontend/index.ts'
  }
}
```

2. Set up a thin consumer workflow that calls the Radius reusable workflow contract
3. Push to `main` — CI generates `main/app.json` on the `radius-graph` orphan branch
4. Create a branch, add a new resource, open a PR
5. CI generates updated `{branch}/app.json` on the `radius-graph` orphan branch for the PR branch or fork branch
6. View the PR page with the extension loaded — diff graph appears below the PR description

## Step 5: Test Repository Root Tab

1. Navigate to the test repo's root page on `main`
2. Verify the "Application graph" tab appears alongside README/License
3. Click the tab — the current-state graph renders without diff coloring
4. Click a resource node — popup with navigation links appears

## Step 6: Verify codeReference Navigation

1. Click a resource with `codeReference` set
2. Popup shows "Source code" and "App definition" links
3. "Source code" navigates to the correct file/line
4. "App definition" navigates to the correct line in `app.bicep` when `appDefinitionLine` is present
5. Click a resource without `codeReference` — only "App definition" link appears

## Development Workflow

```bash
# Watch mode for TypeScript changes
cd web/browser-extension && npm run watch

# After rebuilding:
# 1. Reload extension in chrome://extensions/
# 2. Refresh the GitHub tab
# 3. Check DevTools console for logs
```

## Troubleshooting

| Issue | Solution |
|-------|----------|
| No graph appears | Check extension is loaded, authenticated, and CI has run |
| "Waiting for CI to build" | Push a change to `app.bicep` to trigger CI |
| Graph renders without diff colors | Ensure both base and head branches have artifacts on the `radius-graph` orphan branch and that `diffHash` changes for modified resources |
| "Source code" link missing | Add `codeReference` to the resource in `app.bicep` |
