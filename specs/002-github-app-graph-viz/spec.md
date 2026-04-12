# Feature Specification: GitHub App Graph Visualization

**Feature Branch**: `002-github-app-graph-viz`
**Created**: 2026-04-12
**Status**: Draft
**Input**: User description: "Build the GitHub App Graph Visualization feature for Radius: interactive application graph visualization delivered via a Chrome/Edge browser extension that renders application topology diagrams in GitHub PRs, repository root pages, and dedicated application pages."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - PR Diff Visualization with Change Highlighting (Priority: P1)

A developer modifies the application code and definition (e.g., adds a new Redis cache and connects it to an existing container). When they open a pull request that includes changes to the Radius app definition file, the browser extension detects the change, constructs application graphs for both the base branch and the PR branch, computes the diff, and renders an interactive color-coded application graph visualization directly in the PR description area. Added resources and connections are highlighted in green, modified ones in yellow, removed ones in red, and unchanged ones in their default style.

**Why this priority**: This is the highest-value scenario because it directly addresses the core challenge: reviewers lack a visual representation of how application topology changes during code review. It enables faster, more confident reviews and reduces context switching. Without this, reviewers must mentally reconstruct the application graph from raw code diffs.

**Independent Test**: Can be fully tested by loading the browser extension in Chrome/Edge, navigating to a GitHub PR that modifies an `app.bicep` file, and verifying that the graph visualization appears in the PR description with correct diff coloring. Delivers immediate value to PR reviewers.

**Acceptance Scenarios**:

1. **Given** a PR that adds a new resource to the app definition, **When** the user views the PR page with the browser extension installed, **Then** the new resource appears in the graph visualization with a green highlight and the graph is rendered in the PR description area below the description text.
2. **Given** a PR that modifies an existing resource's connections, **When** the user views the PR page, **Then** the modified resource appears with a yellow highlight and new/changed connections are visible.
3. **Given** a PR that removes a resource from the app definition, **When** the user views the PR page, **Then** the removed resource appears with a red highlight.
4. **Given** a PR that includes changes to the app definition, **When** the graph is still being generated, **Then** a loading indicator ("Generating app graph...") is displayed in the PR description area.
5. **Given** a PR that does not modify any Radius app definition file, **When** the user views the PR page, **Then** no graph visualization is rendered.

---

### User Story 2 - Interactive Resource Navigation from Graph (Priority: P1)

When viewing the application graph visualization (in a PR or on the repo root), a user clicks on a resource node in the graph. A popup appears with navigation links: "Source code" (linking to the file specified in the `codeReference` property) and "App definition" (linking to the relevant line in the `.bicep` file). For modified resources in a PR, links navigate to the diff view. For unchanged resources, links navigate to the file on the current branch.

**Why this priority**: Interactivity is essential for the graph to be useful beyond a static image. Navigation links connect the visual representation to the actual code, enabling developers to quickly drill into specific resources without searching the codebase.

**Independent Test**: Can be tested by clicking resource nodes in the graph and verifying that popups appear with correct navigation links that resolve to the expected files and line numbers.

**Acceptance Scenarios**:

1. **Given** a graph visualization of a PR with a modified resource that has a `codeReference` property, **When** the user clicks that resource, **Then** a popup appears with "Source code" and "App definition" links, and the "Source code" link navigates to the diff view in the PR for that file.
2. **Given** a graph visualization with an unchanged resource that has a `codeReference` property, **When** the user clicks that resource, **Then** the "Source code" link navigates to the file on the current branch (not the diff view).
3. **Given** a resource without a `codeReference` property, **When** the user clicks that resource, **Then** the popup appears with only the "App definition" link (no "Source code" link).
4. **Given** a resource with an invalid `codeReference` value (e.g., absolute path, URL), **When** the popup is rendered, **Then** the "Source code" link is omitted rather than showing an error.

---

### User Story 3 - Static Application Graph Construction (Priority: P1)

The system constructs an application graph from Radius Bicep source files without requiring a live deployment or running Radius control plane. The app definition file is compiled to ARM JSON, and the resulting JSON is parsed to extract resources, connections, and dependency edges. The output is a graph data structure compatible with the existing `ApplicationGraphResponse` schema.

**Why this priority**: This is the foundational capability that enables all visualization features. Without static graph construction, no graph can be rendered in any context (PR, repo root, or dedicated pages).

**Independent Test**: Can be tested by providing a sample `.bicep` file, compiling it, running the static graph construction, and verifying the output graph JSON contains the correct resources, connections, and relationships.

**Acceptance Scenarios**:

1. **Given** a valid Radius app definition file with multiple resources and connections, **When** the static graph construction runs, **Then** the output contains all declared resources with their types, names, and connections matching the app definition.
2. **Given** an app definition file with `dependsOn` relationships, **When** the static graph is constructed, **Then** dependency edges are represented in the graph output.
3. **Given** a resource with a `codeReference` property, **When** the static graph is constructed, **Then** the `codeReference` value is preserved in the graph output for that resource.
4. **Given** an app definition that fails to compile, **When** static graph construction is attempted, **Then** a meaningful error is reported instead of rendering a broken graph.

---

### User Story 4 - Repository Root "Application Graph" Tab (Priority: P2)

On the repository root page (code view on the default branch), the browser extension detects the presence of a Radius app definition file and injects an "Application graph" tab alongside the existing README and License tabs. When the tab is active, the current-state application graph for the default branch is rendered. The graph is interactive with the same popup/navigation behavior as the PR view, but without diff coloring.

**Why this priority**: This provides a living architecture reference that replaces stale manual diagrams. It is the second most impactful scenario because it benefits everyone who visits the repository (team leads, new team members, external contributors), but it depends on the static graph construction (User Story 3) being complete first.

**Independent Test**: Can be tested by navigating to a GitHub repo root that contains an app definition file and verifying the "Application graph" tab appears and renders the graph correctly.

**Acceptance Scenarios**:

1. **Given** a repository with a Radius app definition file on the default branch, **When** the user visits the repo root page with the extension installed, **Then** an "Application graph" tab appears alongside README/License tabs.
2. **Given** the user clicks the "Application graph" tab, **When** the tab becomes active, **Then** the application graph for the default branch is rendered without diff coloring.
3. **Given** a PR is merged to `main` that modifies the app definition, **When** the user visits the repo root page after the merge, **Then** the "Application graph" tab shows the updated graph reflecting the merged changes.
4. **Given** a repository without any Radius app definition file, **When** the user visits the repo root page, **Then** no "Application graph" tab is injected.

---

### User Story 5 - `codeReference` Property on Resource Schemas (Priority: P1)

A new optional property called `codeReference` is added to Radius resource schemas. This property is a string containing a repo-root-relative file path with an optional `#L<number>` line anchor. Authors (human or AI) set this property in the app definition to link each resource to its source code location. The browser extension uses this value to provide "Source code" navigation links in the graph visualization.

**Why this priority**: This property is required for the interactive navigation in User Stories 1 and 2 to provide full value. Without it, graph nodes can only link to the app definition, not to the source code that implements each resource.

**Independent Test**: Can be tested by defining a resource with a `codeReference` property in a `.bicep` file, compiling it, and verifying the property appears in the compiled output and is used correctly by the graph visualization.

**Acceptance Scenarios**:

1. **Given** a resource definition with `codeReference: 'src/cache/redis.ts#L10'`, **When** the app definition is compiled, **Then** the `codeReference` value is preserved in the compiled output.
2. **Given** a resource definition without a `codeReference` property, **When** the graph is rendered, **Then** the resource node's popup omits the "Source code" link but still shows the "App definition" link.
3. **Given** a `codeReference` value containing a URL scheme (`https://...`), **When** the value is consumed, **Then** it is treated as invalid and the "Source code" link is omitted.
4. **Given** a `codeReference` value with path traversal (`../secret/file.ts`), **When** the value is consumed, **Then** it is treated as invalid and the "Source code" link is omitted.

---

### User Story 6 - Dedicated Modeled App Graph Page (Priority: P2)

A link in the repository root sidebar (under "Applications") navigates to a dedicated page showing the modeled application graph. This page shows the abstract representation of the application topology based on the app definition file. The graph is interactive with source code and app definition navigation links.

**Why this priority**: Provides a deeper exploration experience beyond the tab on the repo root. Useful for larger applications where the embedded tab view may be too small. Lower priority because the repo root tab (User Story 4) covers the primary use case.

**Independent Test**: Can be tested by clicking the application link in the sidebar and verifying the dedicated page renders the full modeled graph with interactive navigation.

**Acceptance Scenarios**:

1. **Given** a repository with a Radius app definition file, **When** the user views the repo root page, **Then** an "Applications" section appears in the sidebar with a link to the application.
2. **Given** the user clicks the application link, **When** the dedicated page loads, **Then** the modeled application graph is rendered with all resources and connections.
3. **Given** the user clicks a resource node on the dedicated page, **When** the popup appears, **Then** it includes "Source code" and "App definition" navigation links that work correctly.

---

### User Story 7 - Dedicated Deployed App Graph Page (Priority: P3)

After a deployment, a link is available to view the deployed application graph. This page shows actual infrastructure resources alongside modeled resources, with status indicators: grey (queued), yellow (in-progress), green (success), red (failed). Clicking a successfully deployed resource links to the cloud provider portal. Clicking a failed resource shows the deployment error in a popup modal.

**Why this priority**: This is the most complex scenario, requiring integration with the Radius control plane and live deployment state. It provides significant value for post-deployment troubleshooting but depends on all other stories being complete and requires a running Radius environment.

**Independent Test**: Can be tested by deploying a Radius application, navigating to the deployed graph page, and verifying resource status indicators and navigation links work correctly.

**Acceptance Scenarios**:

1. **Given** an active deployment of a Radius application, **When** the user navigates to the deployed app graph page, **Then** modeled resources are shown alongside actual infrastructure resources with status indicators.
2. **Given** a successfully deployed resource, **When** the user clicks on it, **Then** they are navigated to the relevant cloud provider portal page for that resource.
3. **Given** a failed resource, **When** the user clicks on it, **Then** a popup modal displays the Radius deployment error message for that resource.
4. **Given** resources in various deployment states (queued, in-progress, success, failed), **When** the deployed graph is rendered, **Then** each resource displays the correct color-coded status indicator (grey, yellow, green, red respectively).

---

### Edge Cases

- What happens when the app definition file contains syntax errors that prevent compilation? The system displays a user-friendly error message instead of a broken graph.
- What happens when the app definition file is empty or defines no resources? The system displays an empty graph state with a message indicating no resources are defined.
- What happens when the graph contains a very large number of resources (20+)? The graph rendering engine handles layout gracefully, potentially with zooming or scrolling capabilities.
- What happens when the GitHub API rate limit is exceeded while fetching app definition files? The system displays an appropriate error message and suggests the user try again later.
- What happens when a `codeReference` points to a file that does not exist in the repository? The "Source code" link is rendered but navigates to a GitHub 404 page; no special handling is needed from the extension.
- What happens when the browser extension is not installed? The GitHub UI functions normally with no graph visualizations, tabs, or sidebar links visible.
- What happens when the base branch and PR branch app definitions have no diff? The graph is rendered without any diff coloring (all resources shown in default style).
- What happens when multiple app definition files exist in the repository? The system detects and visualizes the primary app definition file (e.g., `.radius/app.bicep` or `app.bicep` at the repo root).

## Requirements *(mandatory)*

### Functional Requirements

#### Static Graph Construction

- **FR-001**: System MUST construct an application graph from Radius app definition files without requiring a live deployment or running Radius control plane.
- **FR-002**: System MUST compile `.bicep` files to ARM JSON and parse the output to extract resources, connections (from the `connections` property), and dependency edges (from `dependsOn` arrays).
- **FR-003**: System MUST resolve `resourceId()` expressions from connection source strings to identify connected resources in the graph.
- **FR-004**: System MUST output the graph in a data format compatible with (or extending) the existing `ApplicationGraphResponse` schema.
- **FR-005**: System MUST persist the static graph JSON to `.radius/static/<app>.json` on each branch so that CI can regenerate it on push and the browser extension can fetch it via the GitHub API.

#### `codeReference` Property

- **FR-006**: Radius resource schemas MUST support an optional `codeReference` property of type string.
- **FR-007**: The `codeReference` value MUST be a repository-root-relative file path using forward slashes.
- **FR-008**: The `codeReference` value MAY include a single-line anchor in the form `#L<number>`.
- **FR-009**: The `codeReference` value MUST NOT include a URL scheme or host, query string parameters, absolute paths, or path traversal segments (`.` or `..`).
- **FR-010**: Consumers MUST treat `codeReference` values that do not match the valid format as invalid and omit the source-code navigation link rather than attempting to interpret them.

#### PR Diff Visualization

- **FR-011**: The browser extension MUST detect when a PR includes changes to a Radius app definition file.
- **FR-012**: The browser extension MUST fetch the app definition from both the base branch and the PR branch via the GitHub API.
- **FR-013**: The browser extension MUST construct static graphs for both versions and compute the diff to identify added, modified, and removed resources.
- **FR-014**: The browser extension MUST render the diff visualization in the PR description area directly below the PR description text with color coding: green (added), yellow (modified), red (removed), default (unchanged).
- **FR-015**: The browser extension MUST display a loading state ("Generating app graph...") while the graph is being constructed and rendered.
- **FR-016**: A resource MUST be classified as "modified" if its properties (connections, image, ports, or other key attributes) changed between the base branch and the PR branch.

#### Interactive Navigation

- **FR-017**: Clicking a resource node in the graph MUST display a popup with navigation links.
- **FR-018**: The popup MUST include a "Source code" link when the resource has a valid `codeReference` property; otherwise, the link MUST be omitted.
- **FR-019**: The popup MUST include an "App definition" link that navigates to the relevant line in the `.bicep` file.
- **FR-020**: For modified resources in a PR, the "Source code" link MUST navigate to the diff view in the PR for the referenced file.
- **FR-021**: For unchanged resources in a PR, the "Source code" link MUST navigate to the file on the current branch.
- **FR-022**: For modified resources in a PR, the "App definition" link MUST navigate to the diff view for the relevant line of the app definition file.

#### Repository Root Tab

- **FR-023**: The browser extension MUST detect the presence of a Radius app definition file in the repository on the default branch.
- **FR-024**: The browser extension MUST inject an "Application graph" tab alongside the README and License tabs in the repo root UI when an app definition file is detected.
- **FR-025**: The "Application graph" tab MUST render the current-state application graph for the default branch without diff coloring.
- **FR-026**: The graph in the "Application graph" tab MUST update to reflect changes when a PR is merged to the default branch.

#### Dedicated Pages

- **FR-027**: The browser extension MUST render an "Applications" section in the repository root sidebar with a link to the dedicated application page when an app definition file is detected.
- **FR-028**: The dedicated modeled app graph page MUST render the full application graph based on the app definition file with interactive navigation.
- **FR-029**: The dedicated deployed app graph page MUST show actual infrastructure resources alongside modeled resources with status color indicators: grey (queued), yellow (in-progress), green (success), red (failed).
- **FR-030**: Clicking a successfully deployed resource MUST navigate to the relevant cloud provider portal page.
- **FR-031**: Clicking a failed resource MUST display the Radius deployment error message in a popup modal.

#### Graph Rendering

- **FR-032**: The graph rendering engine MUST layout the application graph as a directed acyclic graph with nodes and edges.
- **FR-033**: Resource nodes MUST display the resource name, resource type, and optionally an image tag or key property.
- **FR-034**: The graph rendering MUST support diff coloring (green borders for added, yellow for modified, red for removed).
- **FR-035**: The graph rendering MUST handle varying graph sizes from 3-4 resources to 20+ resources.
- **FR-036**: The graph rendering MUST use a deterministic layout algorithm so that the same graph always renders the same way.
- **FR-037**: The visual styling MUST match GitHub's Primer design system (colors, fonts, spacing).

### Key Entities

- **Application Graph**: A directed graph representing the topology of a Radius application. Contains a collection of resources and the connections between them. Can exist in static (modeled from source code) or runtime (from live deployment) forms. Key attributes: application name, list of resources, list of connections.
- **Graph Resource**: A node in the application graph representing a single Radius resource. Key attributes: resource ID, resource type, resource name, provisioning state, `codeReference` (optional source code pointer), source line information (line number in the app definition file), connections (inbound and outbound), output resources (underlying infrastructure).
- **Graph Connection**: An edge in the application graph representing a relationship between two resources. Key attributes: source resource, target resource, direction (inbound/outbound).
- **Graph Diff**: The computed difference between two application graphs (base branch vs. PR branch). Key attributes: added resources, removed resources, modified resources, unchanged resources.
- **codeReference**: An optional string property on a Radius resource that provides a repo-root-relative file path (with optional line anchor) pointing to the source code that implements or configures the resource.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: PR reviewers can see a visual diff of application topology changes within 5 seconds of viewing a PR that modifies a Radius app definition file.
- **SC-002**: 90% of users with the browser extension installed can identify which resources were added, modified, or removed in a PR on first glance at the graph visualization.
- **SC-003**: Users can navigate from a graph resource node to its source code or app definition within 2 clicks (click resource → click link in popup).
- **SC-004**: The "Application graph" tab in the repository root always reflects the current application topology on the default branch within one page refresh after a merge.
- **SC-005**: Static graph construction completes in under 2 seconds for typical applications with 5-15 resources.
- **SC-006**: The graph visualization correctly renders applications with up to 20 resources without layout degradation or overlapping nodes.
- **SC-007**: The browser extension operates entirely client-side with no backend server dependency for the static/modeled graph features.
- **SC-008**: The graph visualization matches GitHub's visual design language (Primer design system) such that injected elements feel native to the GitHub UI.

## Assumptions

- The browser extension is already scaffolded and functional (from PR #11566) with content script injection, GitHub API client, device flow authentication, and SPA navigation handling.
- Users will install the Chrome/Edge browser extension to access graph visualization features; the features are not visible to users without the extension.
- The `bicep build` command (or equivalent compilation step) is available and produces ARM JSON output that contains the necessary resource, connection, and dependency information.
- The GitHub REST API provides sufficient access to fetch file contents from specific branches and refs with the permissions already configured in the extension manifest.
- Graph visualization features for static/modeled graphs do not require a running Radius control plane or deployed application.
- The deployed app graph feature (P3) will require integration with the Radius control plane to obtain live deployment state and resource status.

## Developer Setup & Usage Guide

This section provides comprehensive instructions for a developer to build, load, run, and test the GitHub App Graph Visualization feature end-to-end.

### Prerequisites

- **Node.js 18+** and **npm** installed
- **Chrome** or **Microsoft Edge** browser (Manifest V3 support required)
- **A GitHub Account** with at least one repository containing a Radius app definition file (e.g., `app.bicep`)
- **A GitHub App** with Device Flow enabled (for authenticating the extension to the GitHub API). See the GitHub App Setup section below for instructions.
- **(Optional) Radius CLI** installed if you want to test the static graph construction locally via the CLI before the browser extension handles it client-side
- **(Optional) Bicep CLI** installed if you want to manually compile `.bicep` files to ARM JSON for testing graph construction

### Step 1: Clone and Install Dependencies

```
cd web/browser-extension
npm install
```

This installs all required dependencies including TypeScript, Chrome extension types, and the cryptographic library used for GitHub secret encryption.

### Step 2: Build the Extension

```
npm run build
```

This compiles TypeScript source from `src/` into JavaScript in the `dist/` directory, and copies static assets (HTML, CSS, manifest, icons) into the output. The `dist/` directory is the fully built, loadable extension.

For active development with automatic recompilation on file changes:

```
npm run watch
```

Note: `npm run watch` only recompiles TypeScript. If you change HTML, CSS, `manifest.json`, or icons, run `npm run build` again or manually copy the changed files to `dist/`.

To clean the build output:

```
npm run clean
```

### Step 3: Load the Extension in Your Browser

#### Chrome

1. Navigate to `chrome://extensions/`
2. Enable **Developer mode** (toggle in the top-right corner)
3. Click **Load unpacked**
4. Select the `web/browser-extension/dist/` folder
5. The extension icon appears in the toolbar. Pin it for easy access.

#### Microsoft Edge

1. Navigate to `edge://extensions/`
2. Enable **Developer mode** (toggle in the left sidebar)
3. Click **Load unpacked**
4. Select the `web/browser-extension/dist/` folder
5. The extension icon appears in the toolbar.

**After loading**: Any time you rebuild the extension, go back to the extensions page and click the **reload** button (circular arrow) on the extension card to pick up the new code. Content script changes also require refreshing the GitHub tab.

### Step 4: GitHub App Setup (One-Time)

The extension authenticates via GitHub's Device Flow (OAuth without a backend server). You need a GitHub App:

1. Go to **https://github.com/settings/apps/new**
2. Fill in:
   - **Name**: e.g., `Radius Graph Dev`
   - **Homepage URL**: `https://github.com/radius-project/radius`
   - **Webhook**: Uncheck "Active" (not needed)
   - **Enable Device Flow**: Check this box
3. Set **Repository permissions**:
   - Contents: **Read-only** (minimum required to fetch app definition files for graph construction)
   - For full deploy features (optional): Administration, Actions, Contents, Environments, Secrets, Variables all at Read & Write
4. Click **Create GitHub App**
5. Copy the **Client ID** (starts with `Iv23li...`)
6. **Install the app** on your account or specific repos: go to `https://github.com/apps/YOUR-APP-SLUG/installations/new`

### Step 5: Authenticate the Extension

1. Navigate to any GitHub repository with the extension loaded
2. Click the extension's **Deploy** button (injected next to the Code button)
3. Enter the **App slug** and **Client ID** from your GitHub App
4. Click **Sign in with GitHub**
5. A device code is displayed — enter it at `https://github.com/login/device`
6. Once authenticated, the extension can fetch repository contents via the GitHub API

### Step 6: Test the App Graph Features

#### Testing PR Diff Visualization

1. Create a test repository (or use an existing one) with a Radius app definition file (e.g., `app.bicep`) on the `main` branch. A minimal example:
   ```bicep
   resource app 'Applications.Core/applications@2023-10-01-preview' = {
     name: 'myapp'
   }

   resource frontend 'Applications.Core/containers@2023-10-01-preview' = {
     name: 'frontend'
     properties: {
       application: app.id
       container: {
         image: 'myregistry/frontend:latest'
       }
     }
   }
   ```
2. Create a new branch and modify the app definition (e.g., add a new resource, change a connection, or remove a resource)
3. Open a pull request from your branch to `main`
4. Navigate to the PR page in your browser (with the extension loaded)
5. **Expected result**: Below the PR description text, a loading indicator ("Generating app graph...") appears, followed by the rendered application graph with diff coloring:
   - Green borders/highlights for added resources
   - Yellow for modified resources
   - Red for removed resources
   - Default styling for unchanged resources
6. Click on a resource node — a popup should appear with "Source code" and/or "App definition" links
7. Verify that links navigate to the correct file or diff view

#### Testing the Repository Root "Application Graph" Tab

1. Navigate to the repository root page on the `main` branch
2. **Expected result**: An "Application graph" tab appears alongside the README and License tabs (below the file listing)
3. Click the "Application graph" tab
4. **Expected result**: The current-state application graph is rendered without diff coloring
5. Click on resource nodes to verify popup navigation works

#### Testing the `codeReference` Property

1. Add a `codeReference` property to a resource in your app definition:
   ```bicep
   resource cache 'Applications.Datastores/redisCaches@2023-10-01-preview' = {
     name: 'cache'
     properties: {
       application: app.id
       codeReference: 'src/cache/redis.ts#L10'
     }
   }
   ```
2. Create a PR with this change and view the graph visualization
3. Click the resource node for "cache"
4. **Expected result**: The popup includes a "Source code" link pointing to `src/cache/redis.ts` at line 10
5. Test with an invalid `codeReference` (e.g., `https://example.com/file.ts` or `../secret.ts`)
6. **Expected result**: The "Source code" link is omitted from the popup (no error displayed)

#### Testing the Applications Sidebar and Dedicated Page

1. Navigate to the repository root page
2. **Expected result**: An "Applications" section appears in the sidebar with a link to your application
3. Click the application link
4. **Expected result**: A dedicated page renders the full modeled application graph with interactive navigation

### Step 7: Testing with Different Graph Sizes

To validate the graph rendering scales correctly, test with app definitions of varying complexity:

- **Small** (3-4 resources): A simple app with a container, a database, and a gateway
- **Medium** (8-12 resources): A microservices app with multiple containers, datastores, and messaging components
- **Large** (20+ resources): A complex app to verify layout handles many nodes without overlapping

### Troubleshooting

| Symptom | Likely Cause | Resolution |
| ------- | ------------ | ---------- |
| No "Deploy" button or graph appears on GitHub | Extension not loaded or content script not injected | Check `chrome://extensions/` for errors. Reload the extension. Refresh the GitHub tab. |
| "Generating app graph..." spinner never resolves | GitHub API authentication failure or rate limiting | Open the browser DevTools console (F12) and check for errors. Re-authenticate the extension. Check GitHub API rate limits. |
| Graph renders but no diff coloring in PR | The PR does not modify a recognized app definition file | Ensure the PR changes a file matching the expected app definition file pattern (e.g., `app.bicep`, `.radius/app.bicep`). |
| "Source code" link missing from popup | Resource lacks a `codeReference` property, or the value is invalid | Add a valid `codeReference` to the resource in the app definition. |
| Extension popup shows authentication error | GitHub App not installed on the repository, or token expired | Re-install the GitHub App on the repo. Sign out and sign back in via the extension. |
| TypeScript compilation errors on `npm run build` | Missing or outdated dependencies | Run `npm install` again. Check that Node.js version is 18+. |
| Changes not reflected after rebuild | Browser caching the old extension code | Click "Reload" on the extension card in `chrome://extensions/`. Hard-refresh the GitHub tab (Ctrl+Shift+R). |

### Development Workflow Summary

1. **Edit** source files in `web/browser-extension/src/`
2. **Build** with `npm run build` (or use `npm run watch` for TypeScript auto-recompilation)
3. **Reload** the extension in `chrome://extensions/` or `edge://extensions/`
4. **Refresh** the GitHub tab to pick up content script changes
5. **Test** by navigating to PRs, repo root pages, and dedicated application pages
6. **Debug** using the browser DevTools console (F12) — content script logs appear in the page console; service worker logs appear in the extension's "Inspect views: service worker" link on the extensions page
