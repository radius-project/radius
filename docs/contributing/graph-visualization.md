# Application Graph Visualization

This document covers the graph visualization feature that renders interactive Radius application topology diagrams in GitHub PRs and repository pages.

## Components

### 1. `rad graph build` CLI Command

Compiles a Bicep application definition into a static graph JSON artifact.

```bash
# Basic usage
rad graph build --bicep app.bicep --output .radius/static/app.json

# Custom paths
rad graph build --bicep apps/myapp.bicep --output artifacts/graph.json
```

**Prerequisites**: `bicep` CLI or `az bicep` must be available in PATH.

**Output**: A JSON artifact at `.radius/static/app.json` containing the `ApplicationGraphResponse` with additional metadata (`codeReference`, `appDefinitionLine`, `diffHash`).

### 2. Browser Extension Graph Features

The Radius browser extension (Chrome/Edge, Manifest V3) adds three graph visualization features to GitHub:

#### PR Diff Visualization
- Automatically detects PRs that modify `app.bicep`
- Fetches pre-built graph artifacts from base and head branches
- Computes resource-level diff (added/removed/modified/unchanged)
- Renders an interactive color-coded graph below the PR description
  - Green borders = added resources
  - Yellow borders = modified resources
  - Red borders = removed resources

#### Repository Root Tab
- Injects an "Application graph" tab on repo root pages (when `app.bicep` exists)
- Renders the current-state graph from the default branch without diff coloring

#### Dedicated Application Page
- Full-page interactive graph at `/:owner/:repo/radius/app/:name`
- Sidebar "Applications" section link on repo root pages

### 3. CI Workflow

A reusable GitHub Actions workflow that automates graph artifact generation:

```yaml
# In your repository's .github/workflows/build-graph.yml
name: build-app-graph

on:
  push:
    paths:
      - app.bicep

jobs:
  build-graph:
    permissions:
      contents: write
    uses: radius-project/radius/.github/workflows/__build-app-graph.yml@main
    with:
      app_file: app.bicep
      artifact_path: .radius/static/app.json
```

## Schema Extension: `codeReference`

An optional `codeReference` string property is available on all Radius resource types. It allows Bicep authors to link resources to their source code:

```bicep
resource cache 'Applications.Datastores/redisCaches@2023-10-01-preview' = {
  name: 'cache'
  properties: {
    application: app.id
    codeReference: 'src/cache/redis.ts#L10'
  }
}
```

**Validation**: The browser extension validates `codeReference` at the rendering boundary using a strict allowlist regex. Invalid values (path traversal, URL schemes, etc.) result in the "Source code" link being silently omitted.

## Development

### Building the Browser Extension

```bash
cd web/browser-extension
npm install
npm run build      # Production build
npm run watch      # Development watch mode
npm run typecheck  # Type checking only
```

### Building the Go Graph Package

```bash
go build ./pkg/cli/graph/
go test ./pkg/cli/graph/ -v
```

### Loading the Extension

1. Build: `cd web/browser-extension && npm run build`
2. Chrome: `chrome://extensions/` → Enable Developer mode → Load unpacked → Select `dist/`
3. Edge: `edge://extensions/` → Enable Developer mode → Load unpacked → Select `dist/`

## Architecture

```
TypeSpec (schema) → Go Models (codegen) → rad CLI (graph build) → JSON Artifact
                                                                        ↓
                                              Browser Extension ← GitHub Contents API
                                                    ↓
                                          Cytoscape.js (render) → Interactive Graph
```

## File Structure

| Path | Purpose |
|------|---------|
| `typespec/radius/v1/resources.tsp` | `codeReference` on shared resource bases |
| `typespec/Applications.Core/applications.tsp` | Extended `ApplicationGraphResource` model |
| `pkg/cli/graph/build.go` | ARM JSON parser and static graph builder |
| `pkg/cli/graph/diffhash.go` | Canonical diff hash computation |
| `cmd/rad/cmd/graph.go` | `rad graph build` CLI command |
| `web/browser-extension/src/content/` | Content scripts for graph injection |
| `web/browser-extension/src/shared/graph-types.ts` | TypeScript type definitions |
| `web/browser-extension/src/shared/github-api.ts` | GitHub Contents API client |
| `.github/workflows/__build-app-graph.yml` | Reusable CI workflow |
