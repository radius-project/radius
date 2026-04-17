# Application Graph Visualization

* **Author**: Nithya Subramanian (@nithyatsu)

## Overview

Radius provides an **application resource** that lets teams define and deploy their entire application — including compute, relationships, and infrastructure — as a single unit. Developers express the resources that make up an application (containers, databases, message queues, etc.) along with the relationships between them. Together, these form the **Radius application graph**: a directed graph of resources and their connections.

The application graph serves two key purposes:

1. **Deployment and configuration** — Radius uses the graph to understand resource dependencies, enabling it to orchestrate deployment and inject configuration automatically.
2. **Visualization** — The graph gives users an intuitive, topology-based view of their application rather than a flat list of resources.

This design extends the application graph from a runtime-only, CLI-only tool to a **multi-modality visualization system** embedded directly in the GitHub developer workflow. The key additions are:

* A **static application graph** built from Bicep definitions (no deployment required).
* A **browser extension** that injects interactive graph visualizations into GitHub repository pages and pull requests.
* A **CI/CD pipeline** that automatically builds graph artifacts on every push and PR.
* A **diff visualization** that highlights added, removed, and modified resources when reviewing pull requests.

> **Feature Spec Reference:** [2026-04-github-app-graph-visualization-feature-spec](https://github.com/willtsai/radius/blob/app-graph-viz-gh-feature-spec/eng/design-notes/app-graph/2026-04-github-app-graph-visualization-feature-spec.md) by Will Tsai (@willtsai)

### What exists today

Radius currently supports a single type of application graph — the **run-time deployment graph** — via the `rad app graph` CLI command. This command calls a Radius API that queries the control plane for all deployed resources, constructs edges based on the `connections` property of each resource, and returns the serialized graph. See [Radius App Graph](2023-10-app-graph.md) for details on how the API builds this graph. Because it reflects live infrastructure, this graph is only available after an application has been deployed.

### Proposed graph types

This design proposes extending Radius to support three kinds of application graph:

#### 1. Static application graph

A graph constructed from application definitions authored in Bicep files (or their compiled JSON output), **without** deploying the application. This is useful for:

* Visualizing application architecture from source code checked into a repository.
* Highlighting infrastructure changes introduced by a Pull Request.

**Limitation:** Because the concrete infrastructure resources depend on the recipe bound to each resource type — which in turn depends on the target Radius environment — the static graph cannot include infrastructure-level details.

#### 2. Run-time application graph (deployment graph)

The graph of a **live, deployed** application, as described above. This is the only graph type supported today.

#### 3. Simulated deployment graph

A graph that shows what the concrete infrastructure resources and their dependencies **would be** if an application definition were deployed against a specific environment, without actually deploying it. This could be surfaced via a command such as:

```sh
rad app graph -e env-id --dry-run
```

Radius should provide a way to access all three kinds of graph.

## Terms and definitions

| Term | Definition |
|---|---|
| Application Graph | A directed graph representing an application as its constituent resources and the relationships between them. |
| Static Application Graph | An application graph inferred from a Bicep template or its compiled JSON output, without deploying the application. |
| Deployment Graph | An application graph constructed by querying the Radius control plane for the live resources of a deployed application. |
| Simulated Deployment Graph | An application graph that represents what would be deployed if an application definition were applied to a specific environment. |
| Graph Artifact | The serialized JSON file (`app.json`) containing a `StaticGraphArtifact` stored on the orphan branch. |
| Orphan Branch | A Git branch with no common history to `main`, used to store graph artifacts (default: `radius-graph`). The same pattern is used by the [GitHub Actions Workspace](../2026-03-github-workspace-design.md) feature for state persistence (`radius-state` orphan branch). |
| DiffHash | A stable BLAKE2b hash of a resource's review-relevant properties, used to detect modifications between branches. |
| rootScope | The current UCP scope (e.g., `/planes/radius/local/resourceGroups/default`). |
| codeReference | An optional resource property providing a repo-relative file path to the source code implementing the resource. |
| appDefinitionLine | The 1-based line number of the resource declaration in `app.bicep`, auto-detected during graph build. |

## Objectives

> **Issue Reference:** <!-- TODO: Link to tracking issue -->

### Goals

* Define a graph schema that is flexible and extensible enough to represent static, run-time, and simulated deployment graphs.
  * Review the server-side API (`getGraph` custom action on `Applications.Core/applications|Radius.Core/applications`) that returns the run-time application graph for a deployed application, based on schema decisions.
* Identify a persistence mechanism since the graph should be available irrespective of the ephemeral nature of Radius control plane. The graph construction is still an in-memory operation.
* Provide a CLI command (`rad graph build`) that constructs and outputs a static application graph from Bicep or compiled JSON application definition files.
* Provide a CLI command (`rad app graph`) that outputs the run-time graph of a deployed application by calling the `getGraph` API.
* Provide a browser extension that renders interactive application graph visualizations on GitHub repository pages and pull requests.
* Provide a reusable CI workflow that automatically builds graph artifacts on push and PR events.

### Non-goals

* Authorization / RBAC for viewing the graph — identified as a future capability dependent on Radius RBAC feature.
* Simulated deployment graph (dry-run) — identified as a future capability but out of scope for this iteration.
* Deployed application graph visualization in the browser extension (P1 — see feature spec).
* Planned application graph visualization in the browser extension (P2 — see feature spec).

### User scenarios

#### Scenario 1: PR diff visualization with change highlighting

A developer modifies `app.bicep` to add a new Redis cache and connect it to an existing container. When they open a pull request, a color-coded diff graph appears below the PR description: added resources in **green**, removed resources in **red**, modified resources in **yellow**, and unchanged resources in **gray**. The reviewer can click any node to navigate to the source code or the `app.bicep` definition line.

#### Scenario 2: Repository root architecture diagram

When a developer navigates to the repository root on GitHub, an "Application graph" tab appears next to the README tab. Clicking it shows the current application topology for the `main` branch — always up to date because CI rebuilds it on every merge.

#### Scenario 3: Interactive navigation from graph to code

A developer clicks on a node in the graph (e.g., "cache") and sees a popup with links to: (1) the source code file referenced by the `codeReference` property, and (2) the `app.bicep` line where the resource is declared.

## User Experience

### CLI: `rad graph build`

**Sample Input:**
```bash
rad graph build \
  --bicep app.bicep \
  --orphan-branch radius-graph \
  --source-branch main
```

**Sample Output:**
```
Compiling app.bicep → /tmp/app.json
Parsed 4 resources, 3 connections
Committed main/app.json to orphan branch radius-graph
```

The command:
1. Invokes `bicep build` to compile `app.bicep` to ARM JSON.
2. Parses resources, connections, `dependsOn`, and `codeReference` from the JSON.
3. Detects source line mappings by scanning the Bicep file for `resource` declarations.
4. Computes a `diffHash` for each resource based on review-relevant properties.
5. Commits the resulting `StaticGraphArtifact` JSON to `{source-branch}/app.json` on the orphan branch.

### CLI: `rad app graph` (existing)

**Sample Input:**
```bash
rad app graph my-app --output json
```

**Sample Output:**
```json
{
  "resources": [
    {
      "id": "/planes/radius/local/resourcegroups/default/providers/Applications.Core/containers/frontend",
      "name": "frontend",
      "type": "Applications.Core/containers",
      "provisioningState": "Succeeded",
      "connections": [
        { "id": "...", "direction": "Outbound" }
      ],
      "outputResources": [
        { "id": "...", "type": "kubernetes: apps/Deployment", "name": "frontend" }
      ]
    }
  ]
}
```

### Browser Extension: Repository root

When a user with the extension installed navigates to `https://github.com/{owner}/{repo}`, the extension:
1. Checks for `app.bicep` at the repository root.
2. Fetches `main/app.json` from the `radius-graph` orphan branch.
3. Injects an "Application graph" tab beside the README tab.
4. Renders an interactive DAG using Cytoscape.js with dagre layout.

### Browser Extension: Pull request view

When a user navigates to a PR that modifies `app.bicep`, the extension:
1. Fetches both `{base-branch}/app.json` and `{head-branch}/app.json` from the orphan branch.
2. Computes the diff using `computeGraphDiff()`.
3. Renders a color-coded graph below the PR description with a legend.
4. Shows a "waiting for CI" message if the head artifact hasn't been built yet.

## Design

### High Level Design

The system consists of four components that work together:

```
┌─────────────────────────────────────────────────────────────────────────┐
│                        Developer Workflow                              │
│                                                                        │
│   1. Developer authors/modifies app.bicep                              │
│   2. Pushes branch / opens PR                                          │
│   3. CI runs rad graph build → commits artifact to orphan branch       │
│   4. Browser extension reads artifact → renders interactive graph      │
└─────────────────────────────────────────────────────────────────────────┘

┌──────────┐    ┌──────────────┐    ┌───────────────┐    ┌──────────────┐
│ app.bicep│───>│ rad graph    │───>│ radius-graph  │<───│  Browser     │
│ (source) │    │ build (CLI)  │    │ orphan branch │    │  Extension   │
└──────────┘    └──────────────┘    └───────────────┘    └──────────────┘
                      │                                        │
                      │  bicep build                           │ GitHub API
                      ▼                                        │ (Contents)
                ┌──────────┐                                   ▼
                │ ARM JSON │                            ┌──────────────┐
                │ (parsed) │                            │ Cytoscape.js │
                └──────────┘                            │ (rendered)   │
                                                        └──────────────┘
```

**Data Flow:**

1. **Build time:** `rad graph build` compiles Bicep → ARM JSON → `StaticGraphArtifact` → commits to `radius-graph` orphan branch.
2. **CI time:** The reusable workflow `__build-app-graph.yml` runs `rad graph build` on push/PR events.
3. **View time:** The browser extension fetches the artifact via GitHub Contents API and renders it client-side.
4. **Diff time:** On PRs, the extension fetches both base and head artifacts, diffs them by resource ID and `diffHash`, and color-codes nodes.

### Detailed Design

#### Component 1: Static Graph Builder (`rad graph build`)

**Location:** `pkg/cli/graph/`

##### Implementation approach

The CLI compiles Bicep to ARM JSON via `bicep build`, then parses the JSON. This is preferred over parsing Bicep directly because:

1. No custom Bicep parser needed — ARM JSON is trivially parsed with `encoding/json`.
2. Handles all Bicep features (modules, conditionals, loops, parameters).
3. Provides two edge sources: `connections` (application-level) and `dependsOn` (deployment ordering).
4. The Bicep CLI is already a prerequisite for `rad deploy`.

##### Key types

```go
// StaticGraphArtifact is the JSON envelope for the graph artifact.
type StaticGraphArtifact struct {
    Version     string                         `json:"version"`
    GeneratedAt string                         `json:"generatedAt"`
    SourceFile  string                         `json:"sourceFile"`
    Application ApplicationGraphResponse       `json:"application"`
}
```

The `ApplicationGraphResponse` reuses the existing API schema from `pkg/corerp/api/v20231001preview/`, extended with:
- `diffHash` (`*string`) — BLAKE2b hash of review-relevant properties for diff classification.
- `appDefinitionLine` (`*int32`) — 1-based line number in `app.bicep`.
- `codeReference` (`*string`) — repo-relative path to source code.

##### Build algorithm

1. **Compile:** Invoke `bicep build app.bicep --stdout` → ARM JSON.
2. **Parse resources:** Iterate `resources` map in ARM JSON. For each resource:
   - Construct a full resource ID from type and name.
   - Extract `properties.connections` → resolve `resourceId()` expressions to target IDs.
   - Extract `dependsOn` → resolve symbolic references to target IDs.
   - Extract `properties.codeReference` if present.
3. **Map source lines:** Scan `app.bicep` for `resource <name> '<type>' =` declarations, recording line numbers.
4. **Compute diffHash:** For each resource, extract "authorable" properties (excluding `application`, `environment`, internal IDs), canonicalize as JSON, and hash with BLAKE2b including `dependsOn`.
5. **Add inbound connections:** For each outbound connection A→B, add a corresponding inbound connection B←A.
6. **Sort:** Sort resources by ID for deterministic output.

##### DiffHash computation

```go
func ComputeDiffHash(properties map[string]interface{}, dependsOn ...string) string {
    // 1. Remove non-authorable keys (application, environment)
    // 2. Canonicalize to sorted JSON
    // 3. Append sorted dependsOn
    // 4. Return "sha256:<hex>" of canonical form
}
```

The diffHash enables the browser extension to classify resources as modified vs unchanged without comparing all properties.

##### Orphan branch commit

After building the artifact, the CLI:
1. Creates or checks out the orphan branch (default: `radius-graph`).
2. Writes `{source-branch}/app.json`.
3. Commits with author identity from git config.
4. Pushes to origin.

##### CLI flags

| Flag | Description | Default |
|------|-------------|---------|
| `--bicep` | Path to Bicep application definition file | `app.bicep` |
| `--orphan-branch` | Orphan branch name for storing artifacts | `radius-graph` |
| `--source-branch` | Branch name to use as the artifact directory | Current branch |

---

#### Component 2: CI/CD Integration (Reusable Workflow)

**Location:** `.github/workflows/__build-app-graph.yml`

A `workflow_call` workflow that consumer repos invoke via a small wrapper workflow. It:

1. Checks out the consumer repo.
2. Checks out the Radius repo (for building `rad` and installing Bicep).
3. Builds `rad` from source with `go build ./cmd/rad`.
4. Installs the Bicep CLI via `build/install-bicep.sh`.
5. Runs `rad graph build` with the configured flags.

##### Consumer workflow example

```yaml
# .github/workflows/build-app-graph.yml
name: Build Application Graph
on:
  push:
    branches: [main]
    paths: [app.bicep]
  pull_request:
    paths: [app.bicep]
permissions:
  contents: write
jobs:
  build-graph:
    uses: radius-project/radius/.github/workflows/__build-app-graph.yml@main
    with:
      app_file: app.bicep
      orphan_branch: radius-graph
      workflow_source_ref: main
```

##### Workflow inputs

| Input | Description | Default |
|-------|-------------|---------|
| `app_file` | Path to Bicep file | `app.bicep` |
| `orphan_branch` | Orphan branch name | `radius-graph` |
| `workflow_source_ref` | Ref in radius-project/radius to build rad from | `main` |

---

#### Component 3: Browser Extension

**Location:** `web/browser-extension/`

A Chrome/Edge browser extension that injects graph visualizations into GitHub pages. Built with TypeScript and bundled with esbuild.

##### Architecture

```
web/browser-extension/
├── src/
│   ├── background/          # Service worker for extension lifecycle
│   │   └── service-worker.ts
│   ├── content/             # Content scripts injected into GitHub pages
│   │   ├── inject.ts        # Entry point — detects page type, dispatches
│   │   ├── repo-tab.ts      # Injects "Application graph" tab on repo root
│   │   ├── pr-graph.ts      # PR diff graph orchestrator
│   │   ├── app-page.ts      # Dedicated app graph page
│   │   ├── graph-renderer.ts # Cytoscape.js rendering with dagre layout
│   │   ├── graph-diff.ts    # Diff computation (added/removed/modified/unchanged)
│   │   └── graph-navigation.ts # Node click popups with code links
│   ├── popup/               # Extension popup UI (setup, auth, settings)
│   │   ├── popup.html
│   │   ├── popup.ts
│   │   └── popup.css
│   ├── shared/              # Shared utilities
│   │   ├── api.ts           # Token storage, GitHub client factory
│   │   ├── github-api.ts    # GraphGitHubAPI class (fetch artifacts, PR details)
│   │   ├── graph-types.ts   # TypeScript types mirroring Go schema
│   │   ├── device-flow.ts   # GitHub App device flow authentication
│   │   └── types.ts         # General types
│   └── styles/
│       └── graph.css        # Primer-aligned styling for graphs, popups, legend
├── dist/                    # Built extension (load unpacked from here)
├── manifest.json            # Extension manifest (permissions, content scripts)
├── esbuild.config.mjs       # Build configuration
└── package.json
```

##### Content script: Page detection (`inject.ts`)

The content script runs on all `github.com` pages. It:
1. Parses the URL to extract `owner`, `repo`, `pullNumber`.
2. Checks if `app.bicep` exists at the repo root (via GitHub API).
3. Dispatches to the appropriate handler:
   - Repo root → `repo-tab.ts` (Application graph tab)
   - PR page → `pr-graph.ts` (diff graph)
   - Dedicated app page → `app-page.ts`

##### Graph rendering (`graph-renderer.ts`)

Uses **Cytoscape.js** with the **cytoscape-dagre** layout plugin for directed acyclic graph rendering.

**Diff color scheme (Primer-aligned):**

| Status | Border color | Background color | Border width |
|--------|-------------|-----------------|-------------|
| Added | `#1a7f37` (success) | `#dafbe1` (success-subtle) | 3px |
| Removed | `#cf222e` (danger) | `#ffebe9` (danger-subtle) | 3px |
| Modified | `#9a6700` (attention) | `#fff8c5` (attention-subtle) | 3px |
| Unchanged | `#57606a` (muted) | `#f6f8fa` (canvas-subtle) | 2px |

Each node displays the resource name and short type. Edges represent outbound connections, rendered as directed bezier curves with arrow markers.

##### Diff computation (`graph-diff.ts`)

```typescript
function computeGraphDiff(
  base: ApplicationGraphResponse | null,
  head: ApplicationGraphResponse | null
): GraphDiff {
  // Build ID→resource maps for both base and head
  // For each head resource:
  //   - Not in base → added
  //   - In base but diffHash differs → modified
  //   - In base with same diffHash → unchanged
  // For each base resource:
  //   - Not in head → removed
}
```

The PR graph displays all head resources plus removed resources from the base, applying diff status colors to each node.

**Edge case:** If `headArtifact` is null (PR CI hasn't completed yet), the extension shows a "waiting for CI" message instead of rendering a misleading all-red graph.

##### Node interaction (`graph-navigation.ts`)

Clicking a node shows a popup with:
- Resource name and type
- Diff status badge
- Link to `app.bicep` at the resource's `appDefinitionLine`
- Link to source code via `codeReference` (if provided)

On PRs, links point to the PR diff view for modified resources and to the repo file view for unchanged resources.

##### Authentication

The extension supports two authentication methods:
1. **GitHub App Device Flow** — User creates a GitHub App, enters Client ID and app slug, then authorizes via device flow. Token stored in `chrome.storage.local`.
2. **Personal Access Token** — User pastes a PAT directly in settings. Simpler but less secure.

Authentication is optional for public repos but required for private repos and to avoid API rate limits.

---

#### Component 4: Graph JSON Schema

The graph artifact uses a unified schema that represents both static and run-time graphs:

```json
{
  "version": "1.0.0",
  "generatedAt": "2026-04-16T00:57:29Z",
  "sourceFile": "app.bicep",
  "application": {
    "resources": [
      {
        "id": "/planes/radius/local/resourcegroups/default/providers/Applications.Core/containers/frontend",
        "name": "frontend",
        "type": "Applications.Core/containers",
        "provisioningState": "Succeeded",
        "connections": [
          { "id": "...", "direction": "Outbound" },
          { "id": "...", "direction": "Inbound" }
        ],
        "outputResources": [],
        "diffHash": "sha256:883755ad...",
        "appDefinitionLine": 23,
        "codeReference": "src/frontend/index.ts"
      }
    ]
  }
}
```

**Schema extensions over the existing `ApplicationGraphResponse`:**

| Field | Type | Graph type | Description |
|-------|------|-----------|-------------|
| `diffHash` | `*string` | Static | BLAKE2b hash for diff classification |
| `appDefinitionLine` | `*int32` | Static | Line number in `app.bicep` |
| `codeReference` | `*string` | Static | Repo-relative path to source code |
| `provisioningState` | `string` | Both | Always `"Succeeded"` for static graphs |
| `outputResources` | `[]OutputResource` | Run-time | Empty for static graphs |

##### Resource property selection

The graph JSON includes properties for each resource node. There are three approaches considered:

**Approach A: Include all properties (current behavior)**

Dump every property from the resource's stored state into the graph node. All properties are read from the Radius control plane datastore.

| Pros | Cons |
|---|---|
| Simple — no schema changes needed | Graph JSON can be large |
| Consumers have full data | May include noisy or irrelevant fields |
| Forward-compatible | Harder to guarantee stable rendering contract |

**Approach B: Schema-driven property selection**

Extend the resource type YAML manifest with a `graphProperties` list declaring which properties to include.

| Pros | Cons |
|---|---|
| Compact graph JSON | Requires annotations on every resource type |
| Stable rendering contract | New properties hidden by default |

**Approach C: Hybrid — full dump with display hints**

Include all properties but add a `displayProperties` list for recommended rendering.

| Pros | Cons |
|---|---|
| Full data always available | Graph JSON size not reduced |
| Display hints guide UI | Two sources of truth |

**Proposed option:** For static graphs, Approach A is used with a filtered set: the build algorithm extracts only "authorable" properties (excluding `application`, `environment`, internal IDs) for the diffHash, while the graph JSON includes the fields needed for rendering (name, type, connections, diffHash, line number, codeReference).

---

#### Graph persistence

The graph is constructed in-memory but must be persisted so it remains accessible when the Radius control plane is not running (e.g., in GitHub Actions CI/CD where the cluster is torn down after each run).

| Graph type | Persisted where | Written when |
|---|---|---|
| Static graph | `{branch}/app.json` on `radius-graph` orphan branch | CI runs `rad graph build` on push/PR |
| Run-time graph | `graphs/{app}.json` on `radius-state` orphan branch | `rad shutdown` serializes after deploy |

**Why orphan branches?**
- No interference with application code history.
- GitHub Contents API provides easy access without local checkout.
- Natural per-branch organization (`main/app.json`, `feature-branch/app.json`).
- PR diff visualization naturally flows from comparing base vs head artifacts.
- Zero additional infrastructure — git is already available with `actions/checkout` credentials.
- Atomic commit+push semantics for consistent state.

**Relationship to the GitHub Actions Workspace feature:**

The orphan branch persistence pattern is shared with the [GitHub Actions Workspace](../2026-03-github-workspace-design.md) feature (`filesystem-state` branch). That feature uses a `radius-state` orphan branch to persist PostgreSQL database backups across ephemeral CI runs. The app graph feature uses a separate `radius-graph` orphan branch for graph artifacts. Both use the same underlying technique:

- `git worktree add` in `/tmp/` for isolated operations
- Sentinel files for lifecycle state (the workspace feature uses `.lock`, `.backup-ok`, `.deploy-lock`)
- Atomic commit+push with the `GITHUB_TOKEN` from `actions/checkout`

The `gitstate` package from the workspace feature (`pkg/cli/gitstate/`) provides reusable primitives for orphan branch management (create, checkout, commit, push) that the graph builder's orphan branch logic could adopt to avoid duplicating git plumbing code.

**Run-time graph persistence via `rad shutdown`:**

The `rad shutdown` command (from the `filesystem-state` branch) backs up PostgreSQL state and tears down the k3d cluster. A natural extension is to call `getGraph` for each deployed application during shutdown and write the graph JSON to the `radius-state` orphan branch alongside the SQL dumps. This would make run-time graphs available for visualization even after the cluster is destroyed — enabling the browser extension to show deployed infrastructure topology from the last known state.

---

### Server-side support

No server-side changes are required for the static graph feature. The existing `getGraph` API on `Applications.Core/applications` continues to serve the run-time graph.

Future work may add the `diffHash`, `appDefinitionLine`, and `codeReference` fields to the server-side `ApplicationGraphResponse` TypeSpec definition to enable a unified schema.

### API design

#### REST API changes

No REST API changes for this iteration. The `getGraph` custom action remains unchanged.

#### Graph artifact schema (new)

The `StaticGraphArtifact` type is defined in `pkg/cli/graph/build.go` and mirrored in `web/browser-extension/src/shared/graph-types.ts`. See [Component 4: Graph JSON Schema](#component-4-graph-json-schema) above for the full schema.

### CLI Design

#### New command: `rad graph build`

```
rad graph build [flags]

Flags:
  --bicep string           Path to the Bicep application definition file (default "app.bicep")
  --orphan-branch string   Orphan branch name for storing graph artifacts (default "radius-graph")
  --source-branch string   Branch name to use as the artifact directory (default: current branch)
```

#### Existing command: `rad app graph`

No changes. Continues to display the run-time graph via the `getGraph` API.

### Implementation Details

#### UCP

No changes.

#### Bicep

The Bicep CLI is used as-is (`bicep build`). No custom Bicep parsing.

#### Core RP

No changes for this iteration. Future work: add `diffHash`, `appDefinitionLine`, `codeReference` to the `ApplicationGraphResource` TypeSpec model.

#### CLI (`pkg/cli/graph/`)

New package with the following files:

| File | Purpose |
|------|---------|
| `build.go` | `BuildStaticGraph()` — main algorithm, ARM JSON parsing, resource/connection extraction |
| `diffhash.go` | `ComputeDiffHash()` — BLAKE2b hashing of authorable properties |
| `orphan.go` | Git operations for creating/updating the orphan branch |
| `cmd.go` | Cobra command setup for `rad graph build` |
| `build_test.go` | Unit tests for graph building |
| `diffhash_test.go` | Unit tests for hash stability, determinism |

#### Browser Extension (`web/browser-extension/`)

See [Component 3: Browser Extension](#component-3-browser-extension) for full architecture.

### Error Handling

| Scenario | Behavior |
|----------|----------|
| `app.bicep` not found | CLI exits with error: "app.bicep not found at specified path" |
| `bicep build` fails | CLI exits with Bicep compiler error output |
| Orphan branch doesn't exist | CLI creates it automatically |
| Git identity not configured | CLI fails with error suggesting `git config` |
| Head artifact not built yet (PR) | Extension shows "waiting for CI" message |
| Both artifacts missing (PR) | Extension shows "waiting for CI" message |
| GitHub API rate limit | Extension logs warning, proceeds without auth |
| Invalid `codeReference` format | Extension omits the source-code navigation link |

## Test plan

### Unit tests

| Component | Test file | Coverage |
|-----------|----------|----------|
| DiffHash computation | `pkg/cli/graph/diffhash_test.go` | Determinism, stability across map iteration, different properties produce different hashes, dependsOn affects hash, empty properties |
| Static graph building | `pkg/cli/graph/build_test.go` | Resource extraction, connection parsing, `resourceId()` expression resolution, source line mapping |
| Graph diff (TypeScript) | `web/browser-extension/src/content/graph-diff.test.ts` | Added/removed/modified/unchanged classification, null base, null head, empty graphs |

### Functional tests

| Test | Description |
|------|-------------|
| End-to-end graph build | Compile a test `app.bicep`, run `rad graph build`, verify output JSON matches expected artifact |
| Orphan branch creation | Run `rad graph build` in a fresh repo, verify orphan branch is created with correct structure |
| CI workflow | Push `app.bicep` change to test repo, verify workflow runs and artifact appears on orphan branch |
| Browser extension rendering | Manual verification: load extension, navigate to test repo, verify graph tab and PR diff graph |

### Extension testing

The browser extension is tested manually:
1. Build extension with `npm run build`.
2. Load unpacked from `web/browser-extension/dist/`.
3. Navigate to a repo with `app.bicep` and verify the "Application graph" tab renders.
4. Open a PR modifying `app.bicep` and verify diff graph with correct colors.

## Security

| Concern | Mitigation |
|---------|-----------|
| GitHub token storage | Stored in `chrome.storage.local` (extension-only storage, not accessible to web pages). No tokens in graph artifacts. |
| Orphan branch permissions | Requires `contents: write` permission in CI. Graph artifacts contain no secrets — only resource names, types, and connections. |
| Extension permissions | Minimal permissions: `activeTab`, `storage`. Content scripts scoped to `github.com`. |
| Token in auth flow | Device flow uses short-lived user codes. PATs entered manually by user. No client secrets stored in extension. |
| Graph artifact content | Contains only application topology (resource names, types, connections). No credentials, secrets, or infrastructure details. |

## Compatibility

| Concern | Impact |
|---------|--------|
| Existing `rad app graph` | No breaking changes. The existing command continues to work unchanged. |
| `ApplicationGraphResponse` schema | New fields (`diffHash`, `appDefinitionLine`, `codeReference`) are optional. Existing consumers are unaffected. |
| Browser support | Extension uses Chrome Extension Manifest V3. Compatible with Chrome 88+ and Edge 88+. |
| GitHub API | Uses public REST API v3 (Contents API, Pull Requests API). No dependency on preview features. |

## Monitoring and Logging

| Component | Instrumentation |
|-----------|----------------|
| `rad graph build` | Logs: resource count, connection count, compilation time, commit SHA. Errors: Bicep compilation failures, git operations. |
| CI workflow | Standard GitHub Actions logging. Step-level timing. |
| Browser extension | `console.debug` for page detection, artifact fetching, graph rendering. `console.error` for API failures. |

## Development plan

| Phase | Scope | Priority |
|-------|-------|----------|
| **Phase 1: Static graph builder** | `rad graph build` CLI command, orphan branch persistence, diffHash | P0 |
| **Phase 2: CI integration** | Reusable workflow `__build-app-graph.yml` | P0 |
| **Phase 3: Browser extension — repo tab** | "Application graph" tab on repo root, Cytoscape rendering | P0 |
| **Phase 4: Browser extension — PR diff** | Diff computation, color-coded PR graph, navigation popups | P0 |
| **Phase 5: Authentication** | GitHub App device flow, PAT support | P0 |
| **Phase 6: Deployed graph visualization** | Browser extension page showing live deployment state | P1 |
| **Phase 7: Planned graph visualization** | Dry-run graph showing expected deployment | P2 |

## Open Questions

1. **Should `app.bicep` be required at the repo root?** The current implementation looks for `app.bicep` at the root. Should it support a configurable path (e.g., `.radius/app.bicep`)?

2. **Polling for head artifact:** When a PR is opened and the head artifact hasn't been built yet, should the extension poll periodically and auto-render when the artifact appears, or require a manual page refresh?

3. **Multiple applications per repo:** The current design assumes one `app.bicep` per repo. How should multi-application repos be handled?

4. **`codeReference` adoption:** Who sets the `codeReference` property — the developer manually, or should tooling (e.g., Copilot skills) populate it automatically?

5. **Run-time graph persistence:** The `filesystem-state` branch implements `rad shutdown` with PostgreSQL backup to a `radius-state` orphan branch. Adding a `getGraph` call during shutdown to persist the run-time graph JSON alongside the SQL dumps would enable deployed graph visualization after cluster teardown. Should this be integrated in this iteration or deferred?

6. **Cloud provider navigation:** For deployed graphs, clicking a resource should navigate to the cloud provider console (AWS, Azure). How should provider-specific URLs be constructed?

## Alternatives considered

### Static graph: Parse Bicep directly vs compile to ARM JSON

**Option considered:** Parse `.bicep` files directly in Go to extract resources and connections.

**Rejected because:**
- Requires a Bicep parser in Go (none exists; Bicep is C#/.NET).
- Must handle Bicep's full expression language (interpolation, conditionals, loops).
- Cannot handle Bicep modules without recursive resolution.
- Ongoing maintenance burden as Bicep syntax evolves.

**Chosen approach:** Compile to ARM JSON via `bicep build`, then parse the stable JSON format. See [Static graph builder](#component-1-static-graph-builder-rad-graph-build) for details.

### Graph persistence: File in repo vs orphan branch vs external storage

| Option | Pros | Cons |
|--------|------|------|
| File in repo (e.g., `.radius/graph.json`) | Simple, visible in PRs | Clutters commit history, merge conflicts |
| Orphan branch | Clean separation, no history interference | Requires git operations, less discoverable |
| External storage (S3, Azure Blob) | Scalable | Extra infrastructure, auth complexity |
| GitHub Actions cache | No extra infra | Unpredictable eviction (7-day TTL, LRU) |
| GitHub Actions artifacts | Cross-run accessible | Retention limits, complex download logic |

**Chosen approach:** Orphan branch. Clean separation from application code, natural per-branch organization, accessible via GitHub API. This is consistent with the `filesystem-state` branch's choice of orphan branches for PostgreSQL state persistence, validated by the same analysis (see [GitHub Actions Workspace](../2026-03-github-workspace-design.md) alternatives considered).

### Browser extension vs GitHub App/Action with Markdown

| Option | Pros | Cons |
|--------|------|------|
| Browser extension | Rich interactive UI, real-time rendering | Requires user installation |
| GitHub Action generating Markdown/SVG | No installation needed | Static images, no interactivity, PR comment clutter |
| GitHub App with checks API | Native GitHub integration | Limited rendering capabilities |

**Chosen approach:** Browser extension with Cytoscape.js for rich, interactive visualizations with click-to-navigate capabilities.

## Design Review Notes

<!-- Update this section with the decisions made during the design review meeting. -->
