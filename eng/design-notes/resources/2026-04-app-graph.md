# Application Graph Visualization

* **Author**: Nithya Subramanian (@nithyatsu)

## Overview

Radius provides an **application resource** that lets teams define and deploy their entire application — including compute, relationships, and infrastructure — as a single unit. Developers express the resources that make up an application (containers, databases, message queues, etc.) along with the relationships between them. Together, these form the **Radius application graph**: a directed graph of resources and their connections.

The application graph serves two key purposes:

1. **Deployment and configuration** — Radius uses the graph to understand resource dependencies, enabling it to orchestrate deployment and inject configuration automatically.
2. **Visualization** — The graph gives users an intuitive, topology-based view of their application rather than a flat list of resources.

This design extends the application graph from a runtime-only, CLI-only tool to a **multi-modality visualization system** which can be embedded directly in the GitHub developer workflow. The key additions are:

* A **modeled/planned application graphs** built from Bicep definitions (no deployment required). This is the core Radius enhancement.
* A **CI/CD pipeline** that automatically builds graph artifacts on every push and PR.
* A **diff visualization** that highlights added, removed, and modified resources when reviewing pull requests.
* A **browser extension** that injects interactive graph visualizations into GitHub repository pages and pull requests. 

> **Feature Spec Reference:** [2026-04-github-app-graph-visualization-feature-spec](https://github.com/willtsai/radius/blob/app-graph-viz-gh-feature-spec/eng/design-notes/app-graph/2026-04-github-app-graph-visualization-feature-spec.md) by Will Tsai (@willtsai)

### What exists today

Radius currently supports a single type of application graph — the **deployment graph** — via the `rad app graph` CLI command. This command calls a Radius API that queries the control plane for all deployed resources, constructs edges based on the `connections` property of each resource, and returns the serialized graph. See [Radius App Graph](2023-10-app-graph.md) for details on how the API builds this graph. Because it reflects live infrastructure, this graph is only available after an application has been deployed.

### Proposed graph types

This design proposes extending Radius to support three kinds of application graph:

#### 1. Modeled application graph

A graph constructed from application definitions authored in Bicep files (or their compiled JSON output), **without** deploying the application. This is useful for:

* Visualizing application architecture from source code checked into a repository.
* Highlighting infrastructure changes introduced by a Pull Request.

**Limitation:** Because the concrete infrastructure resources depend on the recipe bound to each resource type — which in turn depends on the target Radius environment — the modeled graph cannot include infrastructure-level details.

#### 2. Deployed application graph (deployment graph)

The graph of a **live, deployed** application, as described above. This is the only graph type supported today.

#### 3. Simulated deployment graph

A graph that shows what the concrete infrastructure resources and their dependencies **would be** if an application definition were deployed against a specific environment, without actually deploying it. 

## Terms and definitions



## Objectives

> **Issue Reference:** <!-- TODO: Link to tracking issue -->

### Goals

* Define a graph schema that is flexible and extensible enough to represent static, run-time, and simulated deployment graphs.
  * Review the server-side API (`getGraph` custom action on `Applications.Core/applications|Radius.Core/applications`) that returns the run-time application graph for a deployed application, based on schema decisions.
* Identify a persistence mechanism since the graph should be available irrespective of the ephemeral nature of Radius control plane. The graph construction is still an in-memory operation.
* Provide CLI commands that enables users to access these graphs

### Non-goals

* Graph display specifics
* Workflow specifics

While these are essential for repo-radius work, they are not part of core Radius. 

### User scenarios

All the  scenarios are UI-based. To enable these user experiences through visualizations, Radius will be enhanced to support commands that produce [the three graph command variations](#proposed-graph-types). However, the methods to invoke these commands, visualization libraries, and rendering effects are not part of core Radius.

#### Scenario 1: PR diff visualization with change highlighting

A developer modifies `app.bicep` to add a new Redis cache and connect it to an existing container. When they open a pull request, a color-coded diff graph appears below the PR description: added resources in **green**, removed resources in **red**, modified resources in **yellow**, and unchanged resources in **gray**. The reviewer can click any node to navigate to the source code or the `app.bicep` definition line.

#### Scenario 2: Repository root architecture diagram

When a developer navigates to the repository root on GitHub, an "Application graph" tab appears next to the README tab. Clicking it shows the current application topology for the `main` branch — always up to date because CI rebuilds it on every merge.

#### Scenario 3: Interactive navigation from graph to code

A developer clicks on a node in the graph (e.g., "cache") and sees a popup with links to: (1) the source code file referenced by the `codeReference` property, and (2) the `app.bicep` line where the resource is declared.

#### Scenario 4: Deployed graph

Once the user deploys an application, The repository should link to the deployed app graph, along with the details for each "concrete" resource.

## Design

### High Level Design for App Graph - Repo Radius

The system consists of four components that work together:

Users/Agents:

Developer authors/modifies app.bicep. This could be through UI Buttons and/or Agents that create the PR on user's behalf.                
1. Developer/Agent pushes branch and opens PR   
2. Developer/Agent merges the new app deifintion to main

Radius: 

Supports required rad graph commands. The commands are context aware and can determine whether they are being run in repo-radius mode. If so, 
they commit the graph artifact to an orphan branch as app-graph.json. If not, they output to a local file.

GH workflow:

1. Runs the appropriate rad graph command based on the event.
3. Handles concurrency. 

Browser extension:

1. Reads the app-graph.json commited to orphan branches 
2. Parse and render using Cytoscape.

We will merge Workflows,  Browser extension and Graph renderer cytoscape.js java scripts into the github-extension repository. Radius changes will be merged into the Radius repository.

![Component diagram](2026-04-app-graph-components.svg)

> Source files: [2026-04-app-graph-components.svg](2026-04-app-graph-components.svg) (rendered above) and [2026-04-app-graph-components.excalidraw](2026-04-app-graph-components.excalidraw) (editable in the Excalidraw VS Code plugin).

### Detailed Design

#### User Experience

Below command exist today in Radius to access the deployed application graph:

```
#Show graph for specified application. Default to default application
rad app graph [my-application]
```

We will enhance the above command to 
- commit the output to orphan branch, if [running for repo-radius](link to repo-radius doc). 
- support modeled and simulated graphs.

When a rad deploy is called as part of repo radius, there could also be a call to rad app graph [app-name] and persist that to radius-graph.

#####  Accessing modeled graph

```
#Show modeled graph for specified my-app.bicep. Default ./app.bicep
rad app graph --bicep /path/to/my-app.bicep [-o /path/to/output.json]

Compiling app.bicep → /tmp/app.json
Parsed 4 resources, 3 connections
[Committed main/app.json to orphan branch radius-graph]
```

The command:
1. Invokes `bicep build` to compile `app.bicep` to ARM JSON.
2. Parses resources, connections, `dependsOn`, and `codeReference` from the JSON.
3. Detects source line mappings by scanning the Bicep file for `resource` declarations.
4. Computes a `diffHash` for each resource based on relevant properties.
5. Commits the resulting `StaticGraphArtifact` JSON to `{source-branch}/deployments/app-graph.json` on the orphan `radius-graph` branch, if it is run in the context of a github runner (repo radius). Otherwise writes `StaticGraphArtifact` JSON to `app-graph.json` in current directory.

#####  Accessing planned graph

This is richer than modeled graph, main difference being recipe outputs are available.
These details are at a high level and require further research/experimentation.

```
#Show a dry-run of  app.bicep if deployed using recipes in environment env in group grp
rad app graph --bicep [app.bicep] -e env [-g grp]
```
The command 

1. Invokes `bicep build` to compile `app.bicep` to ARM JSON.
2. Parses resources, connections, `dependsOn`, and `codeReference` from the JSON.
3. Detects source line mappings by scanning the Bicep file for `resource` declarations.
4. Computes a `diffHash` for each resource based on relevant properties.
5. Resolves resources craeted by recipes.
6. Commits the resulting `StaticGraphArtifact` JSON to `{source-branch}/scopename-envname/app-graph.json` on the orphan `radius-graph` branch, if it is run in the context of a github runner (repo radius). Otherwise writes `StaticGraphArtifact` JSON to `app-graph.json` in current directory.

There are 2 potential approaches to how the recipe resources command can be resolved:

#### static inferences

1. Invokes `bicep build` to compile `app.bicep` to ARM JSON.
2. Parses resources, connections, `dependsOn`, and `codeReference` from the JSON.
3. For each of the resources, resolves recipe that will be used based on provided environment information
4. Run `bicep build` on recipe or [`terraform graph`](https://developer.hashicorp.com/terraform/cli/commands/graph) on recipe to gather as much detail as possible statically
5. Integrate back to the StaticGraphArtifact through "outputResources" field. 
6. Commit to orphan branch

#### simulated inferences [prefered]

Radius currently supports a simulated environment. At a high level, this makes an entry in Radius datastore for each reasource, identical to what a `rad deploy` does. But the deployment status is used to indicate the resources have not been deployed yet. The simulated environment also does not do a dry-run on the recipes. 
We could choose to reuse this idea and enhance it so that we populate outputResources using dry-run abilities of bicep and terraform.


##### Implementation approach

#### Git dependency

While it is ideal for Radius to not take an additional dependencies, Radius already has a git dependency because of Gitea. Further, If we use workflows to own orphan branch + graph data handling these funcrionalities will not be tested as part of core Radius. Therefore, we are handling git interactions 
through a new package in Radius (pkg/cli/gitstate/).  

##### Schema

The static graph artifact uses `StaticGraphArtifact` as its JSON envelope, wrapping the existing `ApplicationGraphResponse` API type extended with three optional fields (`diffHash`, `appDefinitionLine`, `codeReference`). The `Version` field exists on `StaticGraphArtifact` (not `ApplicationGraphResponse`) because the API response is versioned by its URL (`v20231001preview`), while the file artifact needs its own schema version. `StaticGraphArtifact` is a `cli-only` datamodel, since the new fields are all relevant for code/ static analysis and do not involve a API call. 

```go
/ StaticGraphArtifact is the JSON envelope for the static graph artifact.
// When using orphan branch storage, this is committed to {source-branch}/app.json
// on the radius-graph orphan branch.
type StaticGraphArtifact struct {
	Version     string                                          `json:"version"`
	GeneratedAt string                                          `json:"generatedAt"`
	SourceFile  string                                          `json:"sourceFile"`
	Application corerpv20231001preview.ApplicationGraphResponse `json:"application"`
}
```


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

#### Workflow 

The workflow will be responsible for installing rad cli, running the rad graph command on appropritate events (merge to main, PR against main from a fork). 

##### Concurrent PR handling

Multiple PRs can be open simultaneously, each writing to the same `radius-graph` orphan branch. Conflicts are avoided through:

**1. Directory-per-branch isolation.** Each PR writes to its own directory on the orphan branch (`feature-a/app.json`, `feature-b/app.json`). Since different PRs use different `--source-branch` values, their artifacts never overwrite each other — they're in separate directories within the same branch.

**2. GitHub Actions concurrency group.** The reusable workflow uses a concurrency group scoped to the triggering ref:

```yaml
concurrency:
  group: build-app-graph-${{ github.ref }}
  cancel-in-progress: true
```

This means: if a new push arrives on the same PR branch while a previous graph build is still running, the in-progress build is cancelled and replaced. 


##### Complete artifact example

A full example of a `StaticGraphArtifact` for an application with a frontend container connected to a Redis cache:

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
        "provisioningState": "NotSpecified", 
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
    ]
  }
}
```

Key observations:

- `frontend` has an **Outbound** connection to `cache`; `cache` has a corresponding **Inbound** connection from `frontend` (bidirectional edges).
- `outputResources` is empty for all static graph resources (populated only for planned and deployed graphs).
- Each resource has a unique `diffHash` computed from its review-relevant properties.


#### Graph persistence

The graph is constructed in-memory but must be persisted so it remains accessible when the Radius control plane is not running (e.g., in GitHub Actions CI/CD where the cluster is torn down after each run).

| Graph type | Persisted where | Written when |
|---|---|---|
| modeled graph | `{branch}/appgraph.json` on `radius-graph` orphan branch | CI runs `rad graph build` on push/PR |
| planned graph | `{branch}/scopename-envname/appgraph.json` on `radius-graph` orphan branch | TBD |
| deployed graph | `{branch}/deployments/appgraph.json` on `radius-graph` orphan branch | `rad shutdown` serializes after deploy |

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



### Error Handling

| Scenario | Behavior |
|----------|----------|
| `app.bicep` not found | CLI exits with error: "app.bicep not found at specified path" |
| `bicep build` fails | CLI exits with Bicep compiler error output |
| Not running in repo-radius context | CLI writes `app-graph.json` locally and skips orphan-branch commit |
| Orphan `radius-graph` branch doesn't exist | `pkg/cli/gitstate/` creates it automatically from the empty-tree SHA |
| Git identity not configured | `pkg/cli/gitstate/` falls back to a `github-actions[bot]` identity for the commit |
| `git push` rejected (non-fast-forward) | CLI fetches latest `radius-graph`, rebases the worktree, and retries |
| Recipe resolution fails (planned graph) | CLI emits the modeled graph and logs the unresolved recipes; `outputResources` left empty |
| Head artifact not built yet (PR) | Extension shows "waiting for CI" message |
| Both artifacts missing (PR) | Extension shows "waiting for CI" message |
| GitHub API rate limit | Extension logs warning, proceeds without auth |
| Invalid `codeReference` format | Extension omits the source-code navigation link |

## Test plan

### Unit tests

| Component | Test file | Coverage |
|-----------|----------|----------|
| DiffHash computation | `pkg/cli/graph/diffhash_test.go` | Determinism, stability across map iteration, different properties produce different hashes, dependsOn affects hash, empty properties |
| Modeled graph build | `pkg/cli/graph/build_test.go` | Resource extraction, connection parsing, `resourceId()` expression resolution, source line mapping |
| Planned graph build | `pkg/cli/graph/planned_test.go` | Recipe resolution, `outputResources` population from dry-run, fallback when recipe unresolved |
| Orphan branch primitives | `pkg/cli/gitstate/gitstate_test.go` | Worktree create/remove, orphan branch init from empty-tree SHA, fetch+rebase+push retry |
| Graph diff (TypeScript) | `web/browser-extension/src/content/graph-diff.test.ts` | Added/removed/modified/unchanged classification, null base, null head, empty graphs |

### Functional tests

| Test | Description |
|------|-------------|
| End-to-end modeled graph | Compile a test `app.bicep`, run `rad app graph --bicep`, verify output JSON matches expected artifact |
| End-to-end planned graph | Run `rad app graph --bicep -e simulated-env`, verify `outputResources` are populated for resources backed by recipes |
| Orphan branch lifecycle | Run the CLI in a fresh repo, verify `radius-graph` is created and `{branch}/app-graph.json` is committed |
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
| `rad app graph --bicep` | Logs: graph type (modeled/planned), resource count, connection count, compilation time, commit SHA. Errors: Bicep compilation failures, recipe resolution failures, git operations. |
| `pkg/cli/gitstate/` | Logs: branch fetch, worktree path, commit SHA, push retries. |
| CI workflow | Standard GitHub Actions logging. Step-level timing. |
| Browser extension | `console.debug` for page detection, artifact fetching, graph rendering. `console.error` for API failures. |

## Development plan

- Enhance `rad app graph [app-name]` to commit the deployed graph artifact to the `radius-graph` orphan branch when running in repo-radius context.
- Add `rad app graph --bicep` for the modeled graph; emit `appDefinitionLine` and `codeReference` so the UI can deep-link from a node to source code.
- Add `rad app graph --bicep -e env [-g grp]` for the planned graph; populate `outputResources` via recipe dry-run.
- Introduce `pkg/cli/gitstate/` to encapsulate orphan-branch fetch / worktree / commit / push.

| Phase | Scope | Priority |
|-------|-------|----------|
| **Phase 1: Modeled graph** | `rad app graph --bicep`, `pkg/cli/gitstate/`, orphan-branch persistence, diffHash | P0 |
| **Phase 2: CI integration** | Reusable workflow that installs `rad` and runs the modeled-graph command on push/PR | P0 |
| **Phase 3: Browser extension — repo tab** | "Application graph" tab on repo root, Cytoscape rendering | P0 |
| **Phase 4: Browser extension — PR diff** | Diff computation, color-coded PR graph, navigation popups | P0 |
| **Phase 5: Authentication** | GitHub App device flow, PAT support | P0 |
| **Phase 6: Deployed graph** | Persist `rad app graph [app]` output during `rad shutdown`; extension page showing live state | P1 |
| **Phase 7: Planned graph** | `rad app graph --bicep -e env`; recipe dry-run to populate `outputResources` | P2 |

## Open Questions

1. **Deployed graph persistence:** The `filesystem-state` branch implements `rad shutdown` with PostgreSQL backup to a `radius-state` orphan branch. Adding a `getGraph` call during shutdown to persist the deployed graph JSON to `{branch}/deployments/app-graph.json` on `radius-graph` would enable deployed-graph visualization after cluster teardown. Should this be integrated in this iteration or deferred?

2. **Cross-control-plane deployment tracking:** When the same `app.bicep` is deployed potentially by multiple Radius control planes (e.g., an ephemeral CI plane and a persistent staging plane), each control plane maintains its own independent view of the application in its own database. In addition, users can use cloud provider cli/ portals to change the configuration to suit their needs. If an instance of control plane or an  user modifies the resources of the  application, then Radius's stored state and `getGraph` output become stale.

Note that the modeled graph (`rad app graph --bicep`) is unaffected — it always reads from the Bicep source in the repository and is independent of any control plane. It depicts the app graph as inferred from the code.

Only the deployed graph (from `getGraph`) is affected by this problem.

Possible approaches to drift:

**Approach 1: Single-writer enforcement.** Add a constraint that only one control plane can deploy a given application at a time — essentially an ownership claim. A second control plane attempting to deploy the same application would receive an error. This avoids the stale-data problem entirely by preventing it, but limits flexibility for multi-environment workflows. The detection mechanism can reuse the same metadata fields proposed in Approach 2 (`lastModifiedBy`) to identify whether another control plane currently owns the application. However, this leads to a poor user experience: an operator who wants to apply a minor tweak to a resource property via the AWS or Azure portal would be forced to use `rad deploy` instead.

**Approach 2: Application-level "last modified" metadata.** Add `lastModifiedAt` (UTC timestamp) and `lastModifiedBy` (control plane identifier, e.g., cluster name) as properties on the Application resource itself. When `getGraph` is called, the control plane compares its stored `lastModifiedAt` with the value on the Application resource to detect whether another instance has made changes since the last deployment. This does not prevent staleness but makes it detectable. However, it requires support for synchronizing/ refreshing state.

**Drift detection and refresh** Today, comparable products already maintain state and offer drift detection:
- **Pulumi** — [Drift Detection and Remediation](https://www.pulumi.com/blog/drift-detection/) via `pulumi refresh` (manual) or scheduled drift detection in Pulumi Cloud (automated).
  
- **Terraform** — [Health Assessments with Drift Detection](https://developer.hashicorp.com/terraform/cloud-docs/workspaces/health#drift-detection) in HCP Terraform, plus `terraform plan -refresh-only` in the open-source CLI.

Drawing from these approaches, Radius could offer `rad` commands to detect drift and apply a refresh. At a high level, this would involve:

1. Query the Application resource for its `lastModifiedAt` (UTC) and `lastModifiedBy`. If a newer timestamp from a different control plane is found, offer a `rad` command to refresh the local state. This addresses concurrent updates by multiple Radius instances, but doesn not detect drifts induced by users.

2. Query each resource of the application against its actual cloud provider state. If the deployed properties differ from what Radius has recorded, enable updating the stored state to match the actual deployment. This addresses changes made directly through cloud provider consoles or CLIs.

## Alternatives considered

### Static graph: Parse Bicep directly vs compile to ARM JSON

**Option considered:** Parse `.bicep` files directly in Go to extract resources and connections.

**Rejected because:**

- Requires a Bicep parser in Go (none exists; Bicep is C#/.NET).
- Must handle Bicep's full expression language (interpolation, conditionals, loops).
- Cannot handle Bicep modules without recursive resolution.
- Ongoing maintenance burden as Bicep syntax evolves.

**Chosen approach:** Compile to ARM JSON via `bicep build`, then parse the stable JSON format. See [Accessing modeled graph](#accessing-modeled-graph) for details.

### Graph persistence: File in repo vs orphan branch vs external storage

| Option | Pros | Cons |
|--------|------|------|
| File in repo (e.g., `.radius/graph.json`) | Simple, visible in PRs | Clutters commit history, merge conflicts |
| Orphan branch | Clean separation, no history interference | Requires git operations, less discoverable |
| External storage (S3, Azure Blob) | Scalable | Extra infrastructure, auth complexity |
| GitHub Actions cache | No extra infra | Unpredictable eviction (7-day TTL, LRU) |
| GitHub Actions artifacts | Cross-run accessible | Retention limits, complex download logic |

**Chosen approach:** Orphan branch. Clean separation from application code, natural per-branch organization, accessible via GitHub API. This is consistent with the `filesystem-state` branch's choice of orphan branches for PostgreSQL state persistence, validated by the same analysis (see [GitHub Actions Workspace](../2026-03-github-workspace-design.md) alternatives considered).

The implemnentation should be decoupled from the persistence target as part of an extensible design.


## Design Review Notes

<!-- Update this section with the decisions made during the design review meeting. -->
