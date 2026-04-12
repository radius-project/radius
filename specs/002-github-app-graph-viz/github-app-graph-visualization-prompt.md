# GitHub App Graph Visualization

## Feature Summary

Implement interactive application graph visualization features for GitHub, delivered via a Chrome/Edge browser extension. The feature has three main capabilities:

1. **PR diff visualization** — When a PR modifies an `app.bicep` file, the browser extension detects the change and renders an interactive application graph directly in the PR description, with added/modified/removed resources color-coded (green/yellow/red).
2. **Repository root "Application graph" tab** — On the `main` branch, the browser extension injects an "Application graph" tab alongside README/License in the repo root UI, showing the current-state graph derived from the app definition file on the default branch.
3. **Dedicated application pages** — A link in the repo root sidebar navigates to a dedicated page for the application, showing the modeled graph (P0) and deployed graph (P1).

All visualization is client-side, rendered by the browser extension. The graph data is derived by fetching the Bicep app definition file from the GitHub API, compiling it (or using a pre-compiled JSON), and constructing a static application graph. No Radius control plane is needed for the static/modeled graph.

## Scope Analysis

### Primary repository: `radius/`

All work lives in the `radius` repository, primarily in:

| Area | Path(s) | Changes needed |
|------|---------|----------------|
| **Browser extension** | `web/browser-extension/` | Major — new content script modules for PR injection, repo root tab injection, dedicated pages, graph rendering engine, diff computation |
| **Bicep analysis / CLI** | `pkg/cli/cmd/app/graph/`, potentially new package | New static graph construction from compiled ARM JSON (Approach 2 from design doc) |
| **Graph schema** | `typespec/Applications.Core/applications.tsp`, `pkg/corerp/api/` | Potentially extend `ApplicationGraphResponse` to support static graph fields (e.g. `codeReference`, source line info) |
| **Bicep resource schema** | `typespec/` resource definitions | Add optional `codeReference` property to resource schemas |

### Secondary repositories (awareness, not primary implementation)

- **`design-notes/`** — Specification output will be stored here
- **`docs/`** — User documentation for the browser extension and graph features
- **`resource-types-contrib/`** — Resource type definitions may need `codeReference` support

## Existing Patterns

### Browser Extension (existing code from PR #11566)

The browser extension at `web/browser-extension/` is a Manifest V3 Chrome extension with:

- **Content script** (`src/content/inject.ts`): Injects a "Deploy" button on GitHub repo pages. Detects `{owner}/{repo}` from the URL, finds the Code button, and inserts UI elements. Handles GitHub SPA navigation via MutationObserver. Also injects a "Radius Applications" sidebar widget.
- **Service worker** (`src/background/service-worker.ts`): Message bus between content script and popup. Caches environment status (60s TTL). Handles tab management.
- **GitHub client** (`src/shared/github-client.ts`): Full GitHub REST API client using Bearer token auth. Methods for: repo info, environment CRUD, variables/secrets, file operations (`fileExists`, `commitFile`), app file management (`createAppFile`, `checkAppFile`), deployment workflow dispatch, deployment listing.
- **Device flow auth** (`src/shared/device-flow.ts`): GitHub App Device Flow (RFC 8628) for OAuth without a backend server.
- **Styling** (`src/content/styles.css`): GitHub Primer-compatible styling for injected buttons, dropdowns, sidebar widgets.

Key patterns to follow:
- GitHub DOM injection via `querySelector` and `insertAdjacentElement`
- Chrome extension message passing (`chrome.runtime.sendMessage`)
- Status caching in the service worker
- GitHub API calls direct from the extension (no backend server)
- SPA-aware re-injection via MutationObserver

### Existing Application Graph (runtime graph)

- **CLI**: `rad app graph` command in `pkg/cli/cmd/app/graph/` calls `client.GetApplicationGraph()` which hits the `getGraph` custom action on `Applications.Core/applications`.
- **API response** (`ApplicationGraphResponse`): Array of `ApplicationGraphResource` objects, each with `id`, `type`, `name`, `connections` (with `Direction: Outbound|Inbound`), `outputResources`, and `provisioningState`.
- **Graph computation** (`pkg/corerp/frontend/controller/applications/graph_util.go`): BFS traversal over live API data. Resolves connections from `/properties/connections` and `/properties/routes`. Builds bidirectional adjacency maps for inbound/outbound edges.
- **Connection resolution**: `findSourceResource()` resolves connection targets by parsing as resource ID or matching URL hostnames against known resources.

### Bicep Tools

- `bicep-tools/` converts YAML resource provider manifests to Bicep extension files (`types.json`, `index.json`). Pipeline: YAML → `ParseManifest()` → `Validate()` → `Convert()`. Uses `github.com/Azure/bicep-types-go`.
- The design document recommends **Approach 2**: Compile Bicep to ARM JSON via `bicep build`, then parse the JSON to extract resources, `connections`, and `dependsOn` edges. The CLI already invokes `bicep build` for deployment via `pkg/cli/bicep`.

### Static Graph Construction (from design document)

The design document defines the approach for building a static graph:
1. Invoke `bicep build` to compile `.bicep` to ARM JSON
2. Parse the JSON to extract resources, connections (from `connections` property), and `dependsOn` edges
3. Parse `resourceId()` expressions from connection source strings to resolve references
4. Output a graph JSON compatible with `ApplicationGraphResponse`

### Graph Persistence (from design document)

| Graph type | Persisted where | Written when |
|---|---|---|
| Static graph | `.radius/static/<app>.json` on each branch | CI generates from Bicep on push and PR |
| Run-time graph | `graphs/<app>.json` on `radius-state` orphan branch | `rad shutdown` serializes after deploy |

### codeReference Property (from requirements spec)

A new optional `codeReference` property on resource schemas:
- Format: repo-root-relative file path with optional `#L<number>` line anchor
- Example: `src/cache/redis.ts#L10`
- Must not include URL scheme/host, query params, absolute paths, or path traversal
- If not provided, graph renders without source code navigation link

## Constitution Alignment

Key principles that apply:

- **Principle I (API-First Design)**: The static graph JSON schema should be designed as a well-defined API, potentially extensible to serve both CLI and browser extension consumers.
- **Principle V (Collaboration-Centric Design)**: The PR diff visualization directly serves the collaboration between developers (authoring) and reviewers (understanding impact). The README graph serves team leads and new members.
- **Principle VII (Simplicity Over Cleverness)**: Start with the modeled (static) graph visualization. Don't over-engineer the graph rendering — use straightforward SVG/Canvas rendering or a proven graph layout library.
- **Principle VIII (Separation of Concerns)**: Graph data construction (parsing Bicep/JSON, building the graph model) should be separated from graph rendering (SVG/DOM manipulation in the browser extension).
- **Principle IX (Incremental Adoption)**: The feature should work without requiring a Radius control plane or deployed application. The browser extension is opt-in. Prioritize P0 (modeled graph) before P1 (deployed graph).
- **Principle X (TypeScript Standards)**: The browser extension is TypeScript. Follow strict mode, explicit types for public APIs.
- **Principle XVII (Polyglot Coherence)**: The static graph JSON schema must be consistent whether produced by the Go CLI or consumed by the TypeScript browser extension.

## Design Decisions Already Made (Source of Truth: Requirements Spec)

The requirements specification document is the primary source of truth. Key decisions:

1. **Visualization location**: Graph is injected into the PR description area by the browser extension, directly below the PR description text.
2. **Diff coloring**: Added = green, modified = yellow, removed = red, unchanged = default styling.
3. **Interactive popups**: Clicking a resource shows a popup with "Source code" and "App definition" links. Source code link comes from `codeReference` property. App definition link uses line number tracked by Radius during Bicep compilation.
4. **Navigation behavior**: For modified resources in a PR, links go to the diff view. For unchanged resources, links go to the file on the current branch.
5. **"Application graph" tab**: Injected as a tab alongside README/License in the repo root, implemented by the browser extension (client-side only, visible only to users with the extension installed).
6. **Dedicated pages**: Dedicated application page shows modeled graph (P0). Deployed graph page shows live deployment status with resource states (queued=grey, in-progress=yellow, success=green, failed=red) (P1). Planned graph is P2/TBD.
7. **Applications sidebar**: A sidebar link in the repo root shows available Radius applications.
8. **Graph rendering on merge**: When a PR is merged to `main`, the graph in the "Application graph" tab is updated to reflect the latest application code.

## Priority Breakdown

| Priority | Feature | Scope |
|----------|---------|-------|
| **P0** | Static graph generation from Bicep/ARM JSON | Go CLI + browser extension |
| **P0** | PR diff visualization with change highlighting | Browser extension content script |
| **P0** | "Application graph" tab on repo root | Browser extension content script |
| **P0** | Interactive resource popups with source/definition links | Browser extension |
| **P0** | `codeReference` property on resource schemas | TypeSpec + API types |
| **P1** | Dedicated modeled app graph page | Browser extension |
| **P1** | Deployed app graph visualization | Browser extension + Radius API |
| **P2** | Planned (simulated) app graph | Future |

## Spec Kit Prompt

The following prompt is ready to be copy-pasted into `/speckit.specify`:

---

Build the **GitHub App Graph Visualization** feature for Radius: an interactive application graph visualization delivered via the existing Chrome/Edge browser extension that lives in `web/browser-extension/`. This feature enables developers and reviewers to see auto-generated visual representations of the Radius application graph directly in the GitHub UI — in pull requests, on the repository root page, and on dedicated application pages.

### Problem Statement

Today, developers have no visual way to understand the impact of application code and infrastructure changes during code review. Architecture diagrams are maintained manually and drift out of date. To see the application graph, developers must deploy the application and run `rad app graph` from the CLI, which requires a running Radius control plane. There is no centralized, interactive visualization of the application topology that is accessible from GitHub.

### Feature Requirements

**The primary requirements document is `eng/design-notes/app-graph/2026-04-github-app-graph-visualization-feature-spec.md` in the `radius` repository. If there are any conflicts between it and the design document at `eng/design-notes/app-graph/2026-04-app-graph-ephemeral-radius-design.md`, the requirements spec is the source of truth.**

#### 1. Static Application Graph Construction (P0)

Build a mechanism to construct an application graph from Bicep source files without deploying the application. The recommended approach (from the design document) is:

- Compile `.bicep` files to ARM JSON using `bicep build` (the Radius CLI already does this for `rad deploy` via `pkg/cli/bicep`)
- Parse the compiled ARM JSON to extract resources, `connections` properties, and `dependsOn` arrays
- Resolve `resourceId()` expressions from connection source strings to identify connected resources
- Output the graph in a JSON format compatible with (or extending) the existing `ApplicationGraphResponse` schema

The static graph JSON should be persisted to `.radius/static/<app>.json` on each branch so that CI can regenerate it on push and the browser extension can fetch it via the GitHub API.

Key implementation references:
- Existing runtime graph computation: `pkg/corerp/frontend/controller/applications/graph_util.go`
- Existing CLI graph command: `pkg/cli/cmd/app/graph/`
- Bicep compilation: `pkg/cli/bicep/`
- TypeSpec API models: `typespec/Applications.Core/applications.tsp` (defines `ApplicationGraphResponse`, `ApplicationGraphResource`, `ApplicationGraphConnection`)

#### 2. `codeReference` Property (P0)

Add an optional `codeReference` property to Radius resource schemas. This property is a string with the format of a repo-root-relative file path, optionally with a `#L<number>` line anchor. For example: `src/cache/redis.ts#L10`. The browser extension uses this to provide "Source code" navigation links from the graph visualization. Format requirements:
- Must be a repo-root-relative path using forward slashes
- May include `#L<number>` single-line anchor
- Must not include URL schemes, query strings, absolute paths, or path traversal segments (`.`, `..`)
- Invalid values should cause the source-code navigation link to be omitted (not error)

#### 3. PR Diff Visualization (P0)

When a PR modifies an `app.bicep` (or equivalent Radius application definition file), the browser extension should:
- Detect that the PR includes changes to the app definition
- Fetch the app definition from both the base branch and the PR branch via the GitHub API
- Construct static graphs for both versions
- Compute the diff: which resources were added, modified, or removed
- Render the diff visualization in the PR description area with color coding: green (added), yellow (modified), red (removed), default (unchanged)
- Show a loading state ("Generating app graph...") while processing

Interactive behaviors:
- Clicking a resource shows a popup with navigation links
- "Source code" link (from `codeReference`): for modified resources, links to the diff view in the PR; for unchanged resources, links to the file on the branch
- "App definition" link: links to the relevant line in the `.bicep` file; for modified resources, links to the diff view

#### 4. Repository Root "Application Graph" Tab (P0)

On the repository root page (code view on the default branch), the browser extension should:
- Detect the presence of a Radius app definition file (e.g., `.radius/app.bicep`, `app.bicep`)
- Inject an "Application graph" tab alongside the README and License tabs below the file listing
- When the tab is active, render the application graph for the current branch
- The graph should be interactive with the same popup/navigation behavior as in the PR view (but without diff coloring since this shows the current state)
- When a PR is merged to `main`, the graph should reflect the updated application definition

#### 5. Dedicated Application Pages (P1)

##### Modeled App Graph Page
- A link in the repository root sidebar (under "Applications") navigates to a dedicated page
- Shows the modeled application graph based on the app definition file
- Interactive with source code and app definition navigation links

##### Deployed App Graph Page
- After a deployment, a link is available to view the deployed application graph
- Shows actual infrastructure resources alongside modeled resources
- Resource status indicators: grey (queued), yellow (in-progress), green (success), red (failed)
- Clicking a successfully deployed resource links to the cloud provider portal
- Clicking a failed resource shows the Radius deployment error in a popup modal
- Deployment status is determined by Radius based on live deployment state

#### 6. Graph Rendering Engine

The browser extension needs a graph rendering engine that can:
- Layout an application graph as a directed acyclic graph (DAG) with nodes and edges
- Render resource nodes with: name, resource type (e.g., "Compute/containers"), optional image tag or key property
- Render connection edges between resources
- Support diff coloring (green borders/outlines for added, yellow for modified, red for removed)
- Support interactive click handlers on nodes for popup display
- Handle varying graph sizes (from 3-4 resources to potentially 20+)
- Match GitHub's visual style (Primer design system colors, fonts, spacing)

### Existing Code to Build Upon

The browser extension at `web/browser-extension/` from PR #11566 (commits cherry-picked onto this branch) provides the foundation:

- **Content script** (`src/content/inject.ts`): DOM injection pattern for GitHub pages. Currently injects a "Deploy" button and sidebar widget. Handles SPA navigation via MutationObserver. This should be extended with new modules for PR graph injection and repo root tab injection.
- **Service worker** (`src/background/service-worker.ts`): Message bus with caching. Extend with messages for fetching app definition files and graph data.
- **GitHub client** (`src/shared/github-client.ts`): Full GitHub REST API client. Already has `fileExists()`, `checkAppFile()`, and file retrieval capabilities. Extend with methods to fetch file content from specific branches/refs.
- **Types** (`src/shared/types.ts`): Shared type definitions. Extend with graph-related types.
- **Styling** (`src/content/styles.css`): GitHub Primer-compatible CSS. Extend with graph visualization styles.
- **Auth** (`src/shared/device-flow.ts`, `src/shared/api.ts`): GitHub App Device Flow authentication. Reuse as-is.
- **Manifest** (`manifest.json`): Manifest V3 with content script matching `https://github.com/*/*`. Already includes permissions for `storage`, `activeTab`, `tabs`. May need additional `host_permissions` if fetching from other domains.

### Context Documents

These documents contain the full requirements and design details. Read them carefully:

1. **Requirements specification** (PRIMARY SOURCE OF TRUTH): `eng/design-notes/app-graph/2026-04-github-app-graph-visualization-feature-spec.md` — Contains detailed user experience flows, mockup screenshots, feature priorities, `codeReference` property spec, and all three key scenarios (PR diff, README graph, dedicated pages).

2. **Design document** (supplementary, take with a grain of salt as it was written before the requirements spec was finalized and is incomplete): `eng/design-notes/app-graph/2026-04-app-graph-ephemeral-radius-design.md` — Contains the static graph construction approach (Approach 2: compile Bicep to ARM JSON), graph schema design (unified schema for static + runtime), and persistence strategy.

3. **PR #11566** (cherry-picked into this branch): Contains the partially implemented browser extension code at `web/browser-extension/`. This is the codebase to build upon.

If there are any conflicting ideas between these documents, the requirements specification is the source of truth and should override everything else.

### Scope Boundaries

**In scope:**
- Static graph construction from Bicep/ARM JSON (Go or TypeScript implementation)
- Browser extension features: PR diff visualization, repo root tab, dedicated pages
- `codeReference` property addition to resource schemas
- Graph rendering engine (SVG or Canvas-based)
- Diff computation between base and PR branch graphs
- Interactive popups with source/definition navigation
- CSS styling matching GitHub Primer

**Out of scope (for this spec):**
- Simulated/planned deployment graph (P2)
- Authorization/RBAC for viewing the graph
- Non-GitHub CI/CD systems
- Copilot AI agent tooling for code changes (referenced in requirements spec for context only)
- Changes to the Radius control plane or server-side APIs for the static graph
- `rad app graph` CLI changes (separate effort, may share the static graph construction code)

### Architecture Considerations

- **Client-side only**: All graph construction and rendering happens in the browser extension. No backend server. The extension fetches Bicep/JSON files via the GitHub REST API and processes them locally.
- **Graph data format**: Define a `StaticApplicationGraph` TypeScript interface that is compatible with (or a subset of) the existing `ApplicationGraphResponse` Go type. Include `codeReference` and source line information for each resource.
- **Diff algorithm**: Compare graphs by resource `type` + `name` to identify added/removed/modified resources. A resource is "modified" if its properties (connections, image, ports, etc.) changed between base and PR branch.
- **Graph layout**: Use a proven DAG layout algorithm. Consider libraries like dagre, elkjs, or a custom topological sort + layer assignment. The layout should be deterministic so the same graph always renders the same way.
- **Performance**: Graph construction should be fast (<2s for typical applications with 5-15 resources). Cache graph data in the service worker to avoid re-fetching on tab switches.

### Testing Strategy

- **Unit tests**: Graph construction logic (parsing ARM JSON, building graph, computing diff). Graph rendering components.
- **Integration tests**: GitHub API interactions (mocked). Content script injection into DOM fixtures matching GitHub's HTML structure.
- **Manual testing**: Load the extension in Chrome/Edge and test against real GitHub repositories with Radius app definitions.

---

> **IMPORTANT**: At the very end of the `/speckit.specify` process, move this prompt file from `.copilot-tracking/github-app-graph-visualization.md` into your new `specs/<NNN-feature-name>/` folder. Keep the original filename. This preserves the original research and reasoning alongside your specification.
