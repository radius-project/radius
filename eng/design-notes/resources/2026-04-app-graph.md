# Application Graph Visualization

* **Author**: Nithya Subramanian (@nithyatsu)

## Overview

Radius provides an **application resource** that lets teams define and deploy their entire application — including compute, relationships, and infrastructure — as a single unit. Developers express the resources that make up an application (containers, databases, message queues, etc.) along with the relationships between them. Together, these form the **Radius application graph**: a directed graph of resources and their connections.

The application graph serves two key purposes:

1. **Deployment and configuration** — Radius uses the graph to understand resource dependencies, enabling it to orchestrate deployment and inject configuration automatically.
2. **Visualization** — The graph gives users an intuitive, topology-based view of their application rather than a flat list of resources.

This design extends the application graph from a runtime-only, CLI-only tool to a **multi-modality visualization system** embedded directly in the GitHub developer workflow. The key additions are:

* A **static application graph** built from Bicep definitions (no deployment required).
* A **CI/CD pipeline** that automatically builds graph artifacts on every push and PR.
* A **diff visualization** that highlights added, removed, and modified resources when reviewing pull requests.
* A **browser extension** that injects interactive graph visualizations into GitHub repository pages and pull requests.

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
| DiffHash | A stable hash of a resource's review-relevant properties, used to detect modifications between branches. |
| rootScope | The current UCP scope (e.g., `/planes/radius/local/resourceGroups/default`). |

## Objectives

> **Issue Reference:** <!-- TODO: Link to tracking issue -->

### Goals

* Define a graph schema that is flexible and extensible enough to represent static, run-time, and simulated deployment graphs.
  * Review the server-side API (`getGraph` custom action on `Applications.Core/applications|Radius.Core/applications`) that returns the run-time application graph for a deployed application, based on schema decisions.
* Identify a persistence mechanism since the graph should be available irrespective of the ephemeral nature of Radius control plane. The graph construction is still an in-memory operation.
* Provide a CLI command (`rad graph build`) that constructs and outputs a static application graph from Bicep or compiled JSON application definition files.
* Review the CLI command (`rad app graph`) that outputs the run-time graph of a deployed application by calling the `getGraph` API.

### Non-goals

* Simulated deployment graph (dry-run) — identified as a future capability but out of scope for this iteration. This also requires enhancing Radius to avail tf plan/ what-if to understand recipe's output resources without executing them.

### User scenarios

#### Scenario 1: PR diff visualization with change highlighting

A developer modifies `app.bicep` to add a new Redis cache and connect it to an existing container. When they open a pull request, a color-coded diff graph appears below the PR description: added resources in **green**, removed resources in **red**, modified resources in **yellow**, and unchanged resources in **gray**. The reviewer can click any node to navigate to the source code or the `app.bicep` definition line.

#### Scenario 2: Repository root architecture diagram

When a developer navigates to the repository root on GitHub, an "Application graph" tab appears next to the README tab (in the README file eventually; tab is because of using browser extensions, will change based on the UI stack we would choose). Clicking it shows the current application topology for the `main` branch — always up to date because CI rebuilds it on every merge.

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
**Sample Output**
```
Compiling app.bicep → /tmp/app.json
Parsed 4 resources, 3 connections
Committed main/app.json to orphan branch radius-graph
```

The command:
1. Invokes `bicep build` to compile `app.bicep` to ARM JSON.
2. Parses resources, connections, `dependsOn`, and `codeReference` from the JSON.
3. Detects source line mappings by scanning the Bicep file for `resource` declarations.
4. Computes a `diffHash` for each resource based on relevant properties.
5. Commits the resulting `StaticGraphArtifact` JSON to `{source-branch}/app.json` on the orphan branch.

##### CLI flags

| Flag | Description | Default |
|------|-------------|--------|
| `--bicep` | Path to Bicep application definition file | `app.bicep` |
| `--output` | Path for the output graph artifact (local file mode) | `.radius/static/app.json` |
| `--orphan-branch` | Commit the artifact to this git orphan branch instead of writing a local file | (none — local file mode) |
| `--source-branch` | Source branch name used as the directory prefix on the orphan branch (required with `--orphan-branch`) | (none — required) |

When `--orphan-branch` is omitted, the artifact is written locally to `--output`. When `--orphan-branch` is provided, `--source-branch` is required and the artifact is committed to `{source-branch}/app.json` on the orphan branch. This means each branch gets its own directory — for example, CI for a PR from `feature-add-redis` writes to `feature-add-redis/app.json`, while a merge to `main` writes to `main/app.json`. The browser extension/ UI layers above use these directory names to fetch the correct base and head artifacts for diff comparison.

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

We will NOT merge Workflows,  Browser extension and  Cytoscape.js  into main for now.

**Data Flow:**

1. **Build time:** `rad graph build` compiles Bicep → ARM JSON → `StaticGraphArtifact` → commits to `radius-graph` orphan branch.
2. **CI time:** The reusable workflow `__build-app-graph.yml` runs `rad graph build` on push/PR events.
3. **View time:** The browser extension fetches the artifact via GitHub Contents API and renders it client-side.
4. **Diff time:** On PRs, the extension fetches both base and head artifacts, diffs them by resource ID and `diffHash`, and color-codes nodes.

### Detailed Design

#### Static Graph Builder (`rad graph build`)

**Location:** `pkg/cli/graph/`

##### Implementation approach

The CLI compiles Bicep to ARM JSON via `bicep build`, then parses the JSON. This is preferred over parsing Bicep directly because:

1. No custom Bicep parser needed — ARM JSON is trivially parsed with `encoding/json`.
2. Handles all Bicep features (modules, conditionals, loops, parameters).
3. Provides two edge sources: `connections` (application-level) and `dependsOn` (deployment ordering).
4. The Bicep CLI is already a prerequisite for `rad deploy`.

##### Key types

The static graph artifact uses `StaticGraphArtifact` as its JSON envelope, wrapping the existing `ApplicationGraphResponse` API type extended with three optional fields (`diffHash`, `appDefinitionLine`, `codeReference`). The `Version` field exists on `StaticGraphArtifact` (not `ApplicationGraphResponse`) because the API response is versioned by its URL (`v20231001preview`), while the file artifact needs its own schema version. `StaticGraphArtifact` is a `cli-only` datamodel, since the new fields are all relevant for code/ static analysis and do not involve a API call. 

See [Full schema reference](#full-schema-reference) for the complete type definitions, field descriptions, and a [worked example](#complete-artifact-example).

##### Build algorithm

1. **Compile:** Invoke `bicep build app.bicep --stdout` → ARM JSON.
2. **Parse resources:** Iterate `resources` map in ARM JSON. For each resource:
   - Construct a full resource ID from type and name.
   - Extract `properties.connections` → resolve `resourceId()` expressions to target IDs.
   - Extract `dependsOn` → resolve symbolic references to target IDs.
   - Extract `properties.codeReference` if present.
3. **Map source lines:** Scan `app.bicep` for `resource <name> '<type>' =` declarations, recording line numbers. 
4. **Compute diffHash:** For each resource, extract only the review-relevant properties (`connections`, `container`, `ports`, `routes`, `resources`, `recipe`, `resourceProvisioning`) using an allowlist, canonicalize as sorted JSON, and hash with SHA-256 including `dependsOn`. Properties not in the allowlist (e.g., `application`, `environment`, `name`, `type`) are ignored — new properties added to resource types in the future won't affect diffs unless explicitly added to the allowlist.
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

The diffHash enables the browser extension(UI component) to classify resources as modified vs unchanged without comparing all properties.

##### Orphan branch commit

After building the artifact, the CLI:

1. Creates or checks out the orphan branch (default: `radius-graph`).
2. Writes `{source-branch}/app.json`.
3. Commits with author identity from git config.
4. Pushes to origin.

##### Concurrent PR handling

Multiple PRs can be open simultaneously, each writing to the same `radius-graph` orphan branch. Conflicts are avoided through:

**1. Directory-per-branch isolation.** Each PR writes to its own directory on the orphan branch (`feature-a/app.json`, `feature-b/app.json`). Since different PRs use different `--source-branch` values, their artifacts never overwrite each other — they're in separate directories within the same branch.

**2. GitHub Actions concurrency group.** The reusable workflow uses a concurrency group scoped to the triggering ref:

```yaml
concurrency:
  group: build-app-graph-${{ github.ref }}
  cancel-in-progress: true
```

This means: if a new push arrives on the same PR branch while a previous graph build is still running, the in-progress build is cancelled and replaced. Builds for *different* PR branches run in parallel since their `github.ref` values differ.

**3. Always start from latest state.** Before writing anything, the CLI downloads the most recent version of the `radius-graph` branch from GitHub. This ensures it has all artifacts written by other PRs. Then it:

```
1. Download the latest `radius-graph` branch from GitHub
2. Update the local copy to match
3. Open the branch in a temporary folder (so it doesn't interfere with the code files)
4. Write the new `{source-branch}/app.json` into that folder
5. Save and upload the changes back to GitHub
```

This fetch-then-push pattern means each build starts from the latest state. Since each PR writes to a different directory, the commits don't conflict — git can fast-forward.

**Race condition risk:** If two PRs push at exactly the same time, one `git push` could fail with a non-fast-forward error. The current code does **not** retry — the push error is returned to the caller and the CI job fails. In practice this is rare because the concurrency group serializes builds per-branch, and different branches write to different directories. A future improvement could add a fetch-rebase-push retry loop.

**Cleanup:** Stale directories for merged/closed PRs are not automatically cleaned up from the orphan branch. Over time, the orphan branch accumulates directories for old branches. This is a known gap — a periodic cleanup job or a post-merge hook could be added to prune stale directories.


---

#### Graph JSON Schema

The graph artifact uses a unified schema for both static and run-time graphs. See [Full schema reference](#full-schema-reference) and [Complete artifact example](#complete-artifact-example) below for the detailed schema and a worked example.

##### Full schema reference

The graph schema is defined in TypeSpec at `typespec/Applications.Core/applications.tsp` and generated into Go types at `pkg/corerp/api/v20231001preview/zz_generated_models.go`.

**`ApplicationGraphResponse`** — top-level response from `getGraph` API and the `application` field in `StaticGraphArtifact`:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `resources` | `[]ApplicationGraphResource` | Yes | The resources in the application graph |

**`ApplicationGraphResource`** — a single resource node in the graph:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | `string` | Yes | Full resource ID (e.g., `/planes/radius/local/resourcegroups/default/providers/Applications.Core/containers/frontend`) |
| `type` | `string` | Yes | Resource type (e.g., `Applications.Core/containers`) |
| `name` | `string` | Yes | Resource name (e.g., `frontend`) |
| `provisioningState` | `string` | Yes | Provisioning state. Always `"Succeeded"` for static graphs; actual state for run-time graphs |
| `connections` | `[]ApplicationGraphConnection` | Yes | Connections to/from this resource (outbound and inbound) |
| `outputResources` | `[]ApplicationGraphOutputResource` | Yes | Underlying infrastructure resources. Empty `[]` for static graphs; populated for run-time graphs |
| `codeReference` | `string` | No | Repo-relative path to source code (e.g., `src/frontend/index.ts` or `src/cache/redis.ts#L10`). Format defined in feature spec |
| `appDefinitionLine` | `int32` | No | 1-based line number of the `resource` declaration in `app.bicep`. Auto-detected by `rad graph build` |
| `diffHash` | `string` | No | SHA-256 hash of review-relevant properties for diff classification (e.g., `sha256:883755ad...`) |

**`ApplicationGraphConnection`** — a directed edge between resources:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | `string` | Yes | Resource ID of the connected resource |
| `direction` | `string` | Yes | `"Outbound"` (this resource connects to target) or `"Inbound"` (target connects to this resource) |

**`ApplicationGraphOutputResource`** — an underlying infrastructure resource (run-time only):

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | `string` | Yes | Resource ID of the infrastructure resource |
| `type` | `string` | Yes | Resource type (e.g., `kubernetes: apps/Deployment`) |
| `name` | `string` | Yes | Resource name |

**`StaticGraphArtifact`** — the CLI-only JSON envelope written to disk or orphan branch (defined in `pkg/cli/graph/build.go`):

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `version` | `string` | Yes | Schema version (currently `"1.0.0"`) |
| `generatedAt` | `string` | Yes | ISO 8601 timestamp of artifact generation |
| `sourceFile` | `string` | Yes | Path to the Bicep source file (e.g., `app.bicep`) |
| `application` | `ApplicationGraphResponse` | Yes | The graph data |

##### Complete artifact example

A full example of a `StaticGraphArtifact` for an application with a frontend container connected to a Redis cache, and an environment resource:

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
          {
            "id": "/planes/radius/local/resourcegroups/default/providers/Applications.Datastores/redisCaches/cache",
            "direction": "Outbound"
          }
        ],
        "outputResources": [],
        "diffHash": "sha256:883755ad2f9e...",
        "appDefinitionLine": 23,
        "codeReference": "src/frontend/index.ts"
      },
      {
        "id": "/planes/radius/local/resourcegroups/default/providers/Applications.Datastores/redisCaches/cache",
        "name": "cache",
        "type": "Applications.Datastores/redisCaches",
        "provisioningState": "Succeeded",
        "connections": [
          {
            "id": "/planes/radius/local/resourcegroups/default/providers/Applications.Core/containers/frontend",
            "direction": "Inbound"
          }
        ],
        "outputResources": [],
        "diffHash": "sha256:b4e91c3d7a01...",
        "appDefinitionLine": 45,
        "codeReference": "src/cache/redis.ts#L10"
      },
      {
        "id": "/planes/radius/local/resourcegroups/default/providers/Applications.Core/environments/default",
        "name": "default",
        "type": "Applications.Core/environments",
        "provisioningState": "Succeeded",
        "connections": [],
        "outputResources": [],
        "diffHash": "sha256:f7a20e8b6c55...",
        "appDefinitionLine": 1
      }
    ]
  }
}
```

Key observations:

- `frontend` has an **Outbound** connection to `cache`; `cache` has a corresponding **Inbound** connection from `frontend` (bidirectional edges).
- `environment` has no `codeReference` — the source-code navigation link is omitted in the UI.
- `outputResources` is empty for all static graph resources (populated only for run-time graphs).
- Each resource has a unique `diffHash` computed from its review-relevant properties.

##### Resource property selection

The graph JSON includes properties for each resource node. There are three approaches considered:

**Approach A: Include all properties (current behavior) [Preferred, current implementation]**

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

### API design and server-side support

No REST API changes or server-side changes are required for this iteration. The existing `getGraph` custom action on `Applications.Core/applications` continues to serve the run-time graph unchanged. Future work may add `diffHash`, `appDefinitionLine`, and `codeReference` to the server-side `ApplicationGraphResponse` TypeSpec definition.

The new `StaticGraphArtifact` type is CLI-only, defined in `pkg/cli/graph/build.go` and mirrored in `web/browser-extension/src/shared/graph-types.ts`. See [Full schema reference](#full-schema-reference) for the complete type definitions.

### CLI Design

See [User Experience > CLI: `rad graph build`](#cli-rad-graph-build) for the full command description, sample I/O, and flags. The existing `rad app graph` command is unchanged.

### Implementation Details

| Component | Changes |
|---|---|
| **UCP** | No changes |
| **Bicep** | Used as-is (`bicep build`). No custom Bicep parsing |
| **Core RP** | No changes. Future: add `diffHash`, `appDefinitionLine`, `codeReference` to `ApplicationGraphResource` TypeSpec model |
| **CLI** (`pkg/cli/graph/`) | New package — see [Static Graph Builder](#component-1-static-graph-builder-rad-graph-build) for file listing and algorithm |
| **Browser Extension** (`web/browser-extension/`) | New — see [Browser Extension](#component-3-browser-extension) for architecture |

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

The browser extension is tested with these [detailed instructions](https://github.com/radius-project/radius/blob/6ab75a070d0bd48b82d7d5cc7fb4af89e99c7ed9/web/browser-extension/graph-extension-quickstart.md)


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


1. **Run-time graph persistence:** The `filesystem-state` branch implements `rad shutdown` with PostgreSQL backup to a `radius-state` orphan branch. Adding a `getGraph` call during shutdown to persist the run-time graph JSON alongside the SQL dumps would enable deployed graph visualization after cluster teardown. Should this be integrated in this iteration or deferred?

2. **Cross-control-plane deployment tracking:** When the same `app.bicep` is deployed potentially by multiple Radius control planes (e.g., an ephemeral CI plane and a persistent staging plane), each control plane maintains its own independent view of the application in its own database. In addition, users can use cloud provider cli/ portals to change the configuration to suit their needs. If an instance of control plane or an  user modifies the resources of the  application, then Radius's stored state and `getGraph` output become stale.

Note that the static graph (`rad graph build`) is unaffected — it always reads from the Bicep source in the repository and is independent of any control plane. It depicts the app graph as inferred from the  code. 

Only the run-time graph (from `getGraph`) is affected by this problem.

Possible approaches to drift:

1. **Single-writer enforcement.** Add a constraint that an application can only be deployed by one control plane at a time — essentially an ownership claim. A second control plane attempting to deploy the same application would receive an error. This avoids the stale-data problem entirely by preventing it, but limits flexibility for multi-environment workflows. We can use the same fields that approach 2 proposes to detect if the app is managed by another control plane to report error. However, this will lead to bad user experience. For consider an Operator who wants to apply a minor tweak to a resource's property while navigating the AWS/ Azure portal. The user will be forced to use `rad deploy`` for it. 

2. **Application-level "last modified" metadata.** Add `lastModifiedAt` (timestamp) and `lastModifiedBy` (control plane identifier, e.g., cluster name) as properties on the Application resource itself. When `getGraph` is called, the control plane can compare its stored `lastModifiedAt` with the value on the Application resource to detect if another instance has made changes since it last deployed. This doesn't prevent the staleness but makes it detectable. However, 
we need support for synchronizing the states across these multiple radius instances. 
   
Today, comparable products like Terraform, Pulumi do have states and ability to detect drift. 
https://www.pulumi.com/blog/drift-detection/
https://developer.hashicorp.com/terraform/cloud-docs/workspaces/health#drift-detection

Similar to their approaches,we could offer rad commands that are able to detect drift and apply a refresh. At a high level, this would involve

1. querying the application resource and getting the last modified time (which should be in UTC) and by who and if that is newer, we supply another rad command to refresh the local state files. This convers only concurrent updates by mutiple radius instances.

2. querying each resource of the application and if the app definition has changed, enable updating the state based on the actual deployment's properties.

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


## Design Review Notes

<!-- Update this section with the decisions made during the design review meeting. -->
