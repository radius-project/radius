# Research: GitHub App Graph Visualization

**Feature Branch**: `002-github-app-graph-viz`
**Date**: 2026-04-12

## Research Task 1: ARM JSON Output Structure from Bicep Compilation

**Context**: The CI workflow must compile `.bicep` → ARM JSON and extract resources, connections, and dependency edges. Understanding the ARM JSON structure is essential for building the static graph constructor.

### Decision: Parse ARM JSON `resources` map with `dependsOn` and `properties.connections`

### Rationale

The `bicep build` command produces ARM JSON with `languageVersion: "1.9-experimental"`. In this format, resources are a **map** (keyed by symbolic name), not an array. Each resource has:

- `type`: Full resource type with API version (e.g., `Applications.Core/containers@2023-10-01-preview`)
- `properties`: Object containing `application`, `container`, `connections`, `routes`, etc.
- `dependsOn`: Array of symbolic resource names (strings, not resource IDs)

**Connection extraction**: `properties.connections` is a map where each value has a `source` string. The source may be a `resourceId()` expression or a URL. For graph construction, `resourceId()` sources identify connected resources; URL sources represent external connections.

**Dependency edges**: `dependsOn` arrays contain symbolic names that map to other resources in the same ARM template. These establish ordering relationships visible in the graph.

**Resource ID construction**: ARM JSON uses symbolic names; full resource IDs (e.g., `/planes/radius/local/resourcegroups/default/providers/Applications.Core/containers/frontend`) must be constructed from the resource type and name fields.

### Alternatives Considered

1. **Parse Bicep AST directly**: Rejected — Bicep AST is internal to the compiler and may change; ARM JSON is the stable output format.
2. **Use the live `getGraph` API**: Rejected for static graph — requires a running Radius control plane. The static approach must work offline from source files only.

### Key ARM JSON Structure (from test fixtures)

```json
{
  "$schema": "https://schema.management.azure.com/schemas/2019-04-01/deploymentTemplate.json#",
  "languageVersion": "1.9-experimental",
  "resources": {
    "app": {
      "type": "Applications.Core/applications@2023-10-01-preview",
      "properties": { "name": "myapp", "environment": "[parameters('environment')]" }
    },
    "frontend": {
      "type": "Applications.Core/containers@2023-10-01-preview",
      "properties": {
        "application": "[reference('app').id]",
        "container": { "image": "myregistry/frontend:latest" },
        "connections": {
          "backend": { "source": "[resourceId('Applications.Core/containers', 'backend')]" }
        }
      },
      "dependsOn": ["app"]
    }
  }
}
```

---

## Research Task 2: Cytoscape.js for DAG Graph Rendering

**Context**: FR-032a specifies Cytoscape.js as the graph rendering engine. Need to understand best practices for DAG layout, styling, and event handling.

### Decision: Use Cytoscape.js with `dagre` layout extension, bundled with esbuild for MV3

### Rationale

Cytoscape.js is a mature, canvas-based graph rendering library widely used for biological networks and dependency graphs. In this repository the current browser extension build is plain `tsc` + file copy, which does not package bare npm dependencies into MV3-ready output. The plan therefore needs a build-system update as part of the feature. Key technical decisions:

- **Layout**: Use `cytoscape-dagre` extension for hierarchical DAG layout. Dagre provides deterministic layout (FR-036) and handles varying graph sizes (FR-035).
- **Styling**: Cytoscape.js stylesheet system supports conditional styling based on data attributes — ideal for diff coloring (green/yellow/red borders per FR-034).
- **Events**: `cy.on('tap', 'node', ...)` handles node click for popup navigation (FR-017).
- **Performance**: Canvas rendering handles 20+ nodes efficiently. For very large graphs, enable `textureOnViewport` for smooth panning/zooming.
- **Primer integration**: Cytoscape renders on a `<canvas>` element. The popup/tooltip overlays are standard DOM elements styled with Primer CSS variables (`--color-accent-fg`, `--color-danger-fg`, etc.).
- **Build packaging**: Add `esbuild` so content scripts, background service worker, and shared npm dependencies are bundled into `dist/` for Manifest V3 loading.

### Alternatives Considered

1. **D3.js + dagre (SVG-based)**: Valid alternative. Better for DOM interactivity (tooltips are native). Worse for large graphs (SVG DOM scaling). The spec notes this as a future swap candidate via interface abstraction (FR-032b).
2. **React Flow**: Requires React runtime — too heavy for a content script injection. Extension doesn't use React.
3. **Mermaid.js**: Static rendering only — no interactive click handlers or dynamic styling.

### Dependencies Required

```json
{
  "cytoscape": "^3.29.0",
  "cytoscape-dagre": "^2.5.0",
  "esbuild": "^0.25.0"
}
```

---

## Research Task 3: Chrome Extension Content Script Injection on GitHub SPA

**Context**: The extension injects UI into GitHub pages (PRs, repo root). GitHub uses Turbo for SPA navigation, which means traditional `document_idle` injection only fires on full page loads.

### Decision: Use MutationObserver + `turbo:load` event for SPA-aware injection

### Rationale

The existing extension in `web/browser-extension/src/content/inject.ts` already handles this pattern:

- Uses `window.location.pathname` matching to determine page type
- Checks for existing injection (`document.getElementById()`) to prevent duplicates
- Runs at `document_idle` (per `manifest.json`)

For SPA navigation, the extension needs:

1. **`turbo:load` event listener**: GitHub fires this on SPA navigation. Register a listener that re-runs injection logic.
2. **MutationObserver on `<body>`**: Fallback for cases where Turbo events don't fire reliably. Observe `childList` changes on the main content area.
3. **Debounce**: Batch rapid SPA navigation events to avoid duplicate injections.
4. **Cleanup**: Remove previous injections when navigating away from a page type (e.g., PR → repo root).

### Page Detection Patterns

| Page Type | URL Pattern | Injection Target |
|-----------|-------------|-----------------|
| PR page | `/:owner/:repo/pull/:number` | Below PR description (`#discussion_bucket`) |
| Repo root | `/:owner/:repo` (no sub-path or `/tree/`) | Tab bar alongside README/License |
| Dedicated app page | `/:owner/:repo/radius/app/:name` | N/A (extension creates full page) |

### Existing Pattern (from inject.ts)

The current code already identifies repo root pages via regex and injects a Deploy button next to the Code button. The graph features follow the same injection pattern but target different page sections.

---

## Research Task 4: GitHub Contents API for Fetching Graph Artifacts

**Context**: The extension fetches the pre-built graph artifact from the `radius-graph` orphan branch via the GitHub API.

### Decision: Use GitHub Contents API (`GET /repos/{owner}/{repo}/contents/{path}?ref={branch}`)

### Rationale

The GitHub Contents API is the simplest way to fetch a single file from a specific branch:

```
GET /repos/{owner}/{repo}/contents/main/app.json?ref=radius-graph
Accept: application/vnd.github.v3.raw
Authorization: token {user_token}
```

Key considerations:

- **File size limit**: Contents API supports files up to 100 MB. Graph JSON will be well under this.
- **`ref` parameter**: Specify the branch name to fetch from base branch or PR branch.
- **Raw content**: Use `Accept: application/vnd.github.v3.raw` to get file contents directly without base64 encoding.
- **404 handling**: When the artifact doesn't exist yet (CI hasn't run), the API returns 404. Per FR-015a, display "Application graph not yet available — waiting for CI to build."
- **Authentication**: The extension already has OAuth token via device flow (existing `GitHubClient` class).

### For PR Diff: Two Fetches

1. Fetch `{base-branch}/app.json` from the `radius-graph` branch in the **base repository** (e.g., `radius-project/example-repo`, path `main/app.json`, ref `radius-graph`)
2. Fetch `{head-branch}/app.json` from the `radius-graph` branch in the **head repository** (e.g., `contributor/example-repo`, path `feature/add-redis/app.json`, ref `radius-graph`) for forked PRs, or from the same repo for same-repo PRs
3. Compute diff client-side in the extension

### Rate Limits

- Authenticated requests: 5,000/hour — sufficient for normal usage.
- The extension already handles auth via `GitHubClient.token`.

### Existing Client Integration

The `GitHubClient` class in `web/browser-extension/src/shared/github-client.ts` already has methods for GitHub API calls with authentication. Add a new method:

```typescript
async getFileContents(owner: string, repo: string, path: string, ref: string): Promise<string | null> {
  // Returns raw file contents or null if 404
}
```

---

## Research Task 5: Static Graph Construction from ARM JSON

**Context**: Need to convert compiled ARM JSON into a browser-consumable static artifact for the extension to consume.

### Decision: Implement as `rad graph build` in the existing CLI, reusing generated graph types and emitting extra static-only metadata

### Rationale

The existing `computeGraph()` in `graph_util.go` builds graphs from live API data (database queries). The static graph constructor builds graphs from ARM JSON plus the original `app.bicep` source text. The CLI distribution model matters here: consumer repositories cannot be expected to `go run` code from the Radius repo, but they can call a reusable workflow that builds `rad` from a specific Radius ref in CI and still run the same command locally after building the CLI themselves. Key differences:

| Aspect | Live Graph (`computeGraph`) | Static Graph (new) |
|--------|----------------------------|-------------------|
| Input | `[]GenericResource` from DB query | ARM JSON file from `bicep build` |
| Resource IDs | Full resource IDs from storage | Constructed from type + name |
| Connections | Resolved from live data | Parsed from `properties.connections` |
| Dependencies | Implicit (BFS from connections) | Explicit from `dependsOn` |
| Output | `ApplicationGraphResponse` | Static artifact with `ApplicationGraphResponse` plus `appDefinitionLine` and `diffHash` |

Implementation approach:
1. Parse ARM JSON `resources` map
2. Parse `app.bicep` source text to map resource symbolic names to declaration line numbers in the single supported source file
3. For each resource, extract `type`, `name`, `properties.connections`, `dependsOn`, and authorable `codeReference`
4. Construct deterministic Radius-style resource IDs using the same logical shape used by the live graph response
5. Build `ApplicationGraphConnection` entries from parsed connections
6. Compute a stable `diffHash` from a canonical subset of resource properties needed for FR-016 (connections, image, ports, and selected display-critical properties)
7. Emit static artifact JSON and publish it to `{source-branch}/app.json` on the `radius-graph` orphan branch in CI

This command is invoked locally as `rad graph build --bicep app.bicep --output .radius/static/app.json` and in CI as `rad graph build --bicep app.bicep --orphan-branch radius-graph --source-branch <branch>`.

### Alternatives Considered

1. **TypeScript/Node.js tool**: Could run in CI but would require separate toolchain. Go is the primary language and can reuse existing types.
2. **Shell script with jq**: Too fragile for complex ARM JSON parsing with expression resolution.

---

## Research Task 6: Graph Diff Computation

**Context**: PR visualization requires computing the diff between base-branch and PR-branch graphs.

### Decision: Client-side diff in TypeScript based on resource ID matching plus precomputed `diffHash`

### Rationale

The diff computation is lightweight and should happen in the browser extension (not CI), because:

- It requires both graphs to be available simultaneously
- It's fast (O(n) where n = number of resources)
- It avoids CI needing to know about the base branch

**Algorithm**:

```
baseResources = Map<resourceID, resource> from base branch graph
prResources = Map<resourceID, resource> from PR branch graph

For each resource in prResources:
  if not in baseResources → ADDED (green)
  if in baseResources and diffHash changed → MODIFIED (yellow)
  else → UNCHANGED (default)

For each resource in baseResources:
  if not in prResources → REMOVED (red)
```

**Modified classification for FR-016**: The static builder computes `diffHash` from a canonicalized subset of authorable properties that affect review-relevant behavior, including `connections`, container image, ports, routes, and other selected resource-specific properties. The extension compares hashes and does not need to rehydrate the full source property bag.

---

## Research Task 7: codeReference Property Schema Extension

**Context**: A new optional `codeReference` string property must be added to Radius resource schemas.

### Decision: Add `codeReference` to shared authorable resource-property bases, then propagate it into the graph read model

### Rationale

Looking at the existing TypeSpec patterns:

- Authorable Radius resources reuse `ApplicationScopedResource` and `GlobalScopedResource` in `typespec/radius/v1/resources.tsp`
- The `codeReference` property should be available on ALL resource types (containers, gateways, datastores, etc.)
- Adding it to shared authorable base models ensures Bicep authors can actually set it
- The graph read model also needs additive fields so the browser extension can navigate without reparsing source on the client

**TypeSpec additions**:

```typespec
// Authorable property on shared resource bases
@doc("Optional repo-root-relative file path to the source code for this resource. Format: path/to/file.ts or path/to/file.ts#L10")
codeReference?: string;

// Additive read-model fields on ApplicationGraphResource
codeReference?: string;
appDefinitionLine?: int32;
diffHash?: string;
```

After TypeSpec change, run `make generate` to regenerate:
- Go models in `pkg/corerp/api/v20231001preview/zz_generated_models.go`
- OpenAPI specs in `swagger/`

The `codeReference` property flows through the existing graph construction pipeline:
1. User sets it in `.bicep` file
2. `bicep build` preserves it in ARM JSON `properties`
3. `rad graph build` copies it into `ApplicationGraphResource` and adds `appDefinitionLine` + `diffHash`
4. Extension reads it from graph JSON for navigation links

### Security Validation (FR-009a, FR-009b)

The extension validates `codeReference` before rendering links:

```typescript
const CODE_REF_PATTERN = /^[a-zA-Z0-9_\-./]+(?:#L\d+)?$/;

function isValidCodeReference(ref: string): boolean {
  if (!CODE_REF_PATTERN.test(ref)) return false;
  if (ref.includes('..')) return false;  // Path traversal
  if (ref.startsWith('/')) return false; // Absolute path
  return true;
}
```

GitHub URLs are constructed programmatically:
```typescript
// SAFE: Programmatic URL construction
const url = `https://github.com/${owner}/${repo}/blob/${branch}/${encodeURIComponent(filePath)}`;
// NEVER: Direct interpolation of raw codeReference into href
```
