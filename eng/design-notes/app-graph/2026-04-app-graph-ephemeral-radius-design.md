# Ephemeral Radius Design

* **Author**: Nithya Subramanian (@nithyatsu)

## Overview

Radius provides an **application resource** that lets teams define and deploy their entire application — including compute, relationships, and infrastructure — as a single unit. Developers express the resources that make up an application (containers, databases, message queues, etc.) along with the relationships between them. Together, these form the **Radius application graph**: a directed graph of resources and their connections.

The application graph serves two key purposes:

1. **Deployment and configuration** — Radius uses the graph to understand resource dependencies, enabling it to orchestrate deployment and inject configuration automatically.
2. **Visualization** — The graph gives users an intuitive, topology-based view of their application rather than a flat list of resources.

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
| rootScope | The current UCP scope (e.g., `/planes/radius/local/resourceGroups/default`). |

## Objectives

> **Issue Reference:**

### Goals

* Define a graph schema that is flexible and extensible enough to represent static, run-time, and simulated deployment graphs.
  * Review the server-side API (`getGraph` custom action on `Applications.Core/applications|Radius.Core/applications`) that returns the run-time application graph for a deployed application, based on schema decisions.
* Identify a persistence mechanism since the graph should be available irrespective of the epheremal nature of Radius control plane. The graph construction is still an in-memory operation.
* Provide a CLI command that constructs and outputs a static application graph from Bicep or compiled JSON application definition files.
* Provide a CLI command that outputs the run-time graph of a deployed application by calling the `getGraph` API.

### Non-goals

* Authorization / RBAC for viewing the graph - identified as a future capability dependent on Radius RBAC feature.
* Simulated deployment graph (dry-run) — identified as a future capability but out of scope for this iteration.

### User scenarios (optional)

## Design

### Schema

#### Design principles

* **Unified schema.** A single graph schema should represent both static and run-time graphs. Fields that only apply to one graph type (e.g., `provisioningState` for run-time graphs) are optionally populated. See [Resource property selection](#resource-property-selection)
* **DAG structure.** The graph is a directed acyclic graph (DAG). Each node is a Radius resource; each edge is a connection (inbound or outbound) between resources.
* **Query capabilities.** Beyond depicticing the dependency between resources, the graph should support queries such as: "what does the frontend container depend on?" or "is this resource owned by the application or shared via the environment?"
* **In-memory construction.** The graph is built in-memory on each request. This mirrors how ARM's Deployment Engine constructs a dependency graph from an ARM JSON template (or Bicep) using `dependsOn`. Radius applications have a comparable number of nodes, so the same approach is viable. `Cytoscape`, `jq`,`gonum\graph` can be used for visualization and query.
* **Persistence of serialized output.** When Radius is used with ephemeral infrastructure (e.g., GitHub Actions workspaces), the control plane is torn down after each workflow run. GitHub READMEs and PR views need access to the graph without spinning up the control plane. This requires persisting the serialized graph JSON independently of the control plane state. See [Graph persistence](#graph-persistence) below.

#### Graph persistence

The graph is constructed in-memory but must be persisted so it remains accessible when the Radius control plane is not running (e.g., the [GitHub Actions workspace](https://github.com/radius-project/radius/blob/b9999a12d965f659bb5c943f336c88781b883832/eng/design-notes/2026-03-github-workspace-design.md) tears down the cluster after each run). The persistence strategy differs by graph type:

| Graph type | Persisted where | Written when |
|---|---|---|
| Static graph | `.radius/static/<app>.json` on each branch | CI generates from Bicep on push and PR |
| Run-time graph | `graphs/<app>.json` on `radius-state` orphan branch | `rad shutdown` serializes after deploy |

* **Static graph per branch**: The CLI command that builds a static graph from Bicep writes the JSON to `.radius/static/`. CI runs this on every push, committing the result to the current branch. On a PR, GitHub's diff view naturally shows the architecture changes between the `main` branch graph and the PR branch graph, giving reviewers a clear picture of what the PR introduces.
* **Run-time graph on orphan branch**: `rad shutdown` calls `getGraph` for each deployed application and writes the JSON to `graphs/` on the `radius-state` orphan branch. This is a natural extension of the existing shutdown backup flow, which already persists SQL dumps to the same branch.

The Radius graphs do not have any sensitive data, therefore this should be OK.

#### Resource property selection

The graph JSON includes properties for each resource node. A key schema design decision is **which** properties of a resource appear in the graph. There are two broad approaches:

##### Approach A: Include all properties (current behavior)

Dump every property from the resource's stored state into the graph node. This is what the `getGraph` API does today. All properties are read from the Radius control plane datastore and serialized into the response, potentially traveling over the network to CLI clients/consumers. "View"/ upper layers have the business logic to filter  data as needed.

| Pros | Cons |
|---|---|
| Simple — no schema changes needed | Graph JSON can be large (especially with output resources populated) , increasing network bandwidth between the control plane and consumers |
| Consumers have full data; no second API call needed | May include noisy or irrelevant fields (internal IDs, timestamps) |
| Forward-compatible — new properties appear automatically | Harder to guarantee a stable rendering contract for UI/visualization |

##### Approach B: Schema-driven property selection

Extend the [resource type YAML manifest](https://docs.radapp.io/concepts/resource-types/) with a top-level `graphProperties` list that declares which properties should be included in the graph. Only listed properties are projected into the graph JSON. This reduces the data read from the control plane datastore and serialized over the network. It allows a default view to rely on data filtering applied at control plane.

Example manifest addition:

```yaml
# In the resource type definition YAML
graphProperties:
  - port
  - container.image
  - container.ports.web.containerPort
```

Alternatively, if most properties are display-worthy, an exclude list can be used:

```yaml
graphExclude:
  - container.readinessProbe
  - container.livenessProbe
```

| Pros | Cons |
|---|---|
| Compact graph JSON — only display-relevant fields | Requires schema markers to every resource type definition |
| Resource type authors control what's meaningful for visualization | New properties are hidden by default until annotated |
| Stable rendering contract for UI components | More complex graph construction logic (filter by annotation) |

##### Approach C: Hybrid — full dump with display hints (Claude suggested, retaining for discussion)

Include all properties in the graph JSON, but add a `displayProperties` list to each `ApplicationGraphResource` node that identifies the subset of properties recommended for rendering. Consumers can use the hints for default views and fall back to the full property set for advanced/detail views.

```json
{
  "id": "...",
  "type": "Applications.Core/containers",
  "name": "frontend",
  "properties": { /* all properties */ },
  "displayProperties": ["container.image", "container.ports.web.containerPort"]
}
```

| Pros | Cons |
|---|---|
| Full data always available — no information loss | Graph JSON size is not reduced |
| Display hints guide UI without restricting it | Two sources of truth (all props + display list) to maintain |
| Backward-compatible — existing consumers unaffected | Still requires resource type authors to specify display hints |

### Static graph

### Server side support

None

### Cli side support

There are two primary ways `rad app graph --path-to-bicep` (specific cli command is TBD)` can construct the static graph.

```bicep
extension radius

@description('The Radius application ID. Set automatically by the rad CLI.')
param application string

@description('The Radius environment ID. Set automatically by the rad CLI.')
param environment string

resource app 'Radius.Core/applications@2025-08-01-preview' = {
  name: 'my-app'
  properties: {
    environment: environment
  }
}

resource frontend 'Radius.Compute/containers@2025-08-01-preview' = {
  name: 'frontend'
  properties: {
    application: app.id
    environment: environment
    containers: {
      frontend: {
        image: 'ghcr.io/my-org/frontend:latest'
        ports: {
          web: {
            containerPort: 3000
          }
        }
      }
    }
    connections: {
      backend: {
        source: 'http://backend:8080'
      }
    }
  }
}

resource backend 'Radius.Compute/containers@2025-08-01-preview' = {
  name: 'backend'
  properties: {
    application: app.id
    environment: environment
    containers: {
      backend: {
        image: 'ghcr.io/my-org/backend:latest'
        ports: {
          api: {
            containerPort: 8080
          }
        }
      }
    }
    connections: {
      mongodb: {
        source: db.id
      }
    }
  }
}

resource db 'Radius.Data/mongoDatabases@2025-08-01-preview' = {
  name: 'my-db'
  properties: {
    application: app.id
    environment: environment
  }
}
```

```json
{
  "$schema": "https://schema.management.azure.com/schemas/2019-04-01/deploymentTemplate.json#",
  "contentVersion": "1.0.0.0",
  "metadata": {
    "_generator": {
      "name": "bicep",
      "version": "0.38.33.27573",
      "templateHash": "7342800074738175601"
    }
  },
  "parameters": {
    "application": {
      "type": "string",
      "metadata": {
        "description": "The Radius application ID. Set automatically by the rad CLI."
      }
    },
    "environment": {
      "type": "string",
      "metadata": {
        "description": "The Radius environment ID. Set automatically by the rad CLI."
      }
    }
  },
  "resources": [
    {
      "type": "Radius.Core/applications",
      "apiVersion": "2025-08-01-preview",
      "name": "my-app",
      "properties": {
        "environment": "[parameters('environment')]"
      }
    },
    {
      "type": "Radius.Compute/containers",
      "apiVersion": "2025-08-01-preview",
      "name": "frontend",
      "properties": {
        "application": "[resourceId('Radius.Core/applications', 'my-app')]",
        "environment": "[parameters('environment')]",
        "containers": {
          "frontend": {
            "image": "ghcr.io/my-org/frontend:latest",
            "ports": {
              "web": {
                "containerPort": 3000
              }
            }
          }
        },
        "connections": {
          "backend": {
            "source": "http://backend:8080"
          }
        }
      },
      "dependsOn": [
        "[resourceId('Radius.Core/applications', 'my-app')]"
      ]
    },
    {
      "type": "Radius.Compute/containers",
      "apiVersion": "2025-08-01-preview",
      "name": "backend",
      "properties": {
        "application": "[resourceId('Radius.Core/applications', 'my-app')]",
        "environment": "[parameters('environment')]",
        "containers": {
          "backend": {
            "image": "ghcr.io/my-org/backend:latest",
            "ports": {
              "api": {
                "containerPort": 8080
              }
            }
          }
        },
        "connections": {
          "mongodb": {
            "source": "[resourceId('Radius.Data/mongoDatabases', 'my-db')]"
          }
        }
      },
      "dependsOn": [
        "[resourceId('Radius.Core/applications', 'my-app')]",
        "[resourceId('Radius.Data/mongoDatabases', 'my-db')]"
      ]
    },
    {
      "type": "Radius.Data/mongoDatabases",
      "apiVersion": "2025-08-01-preview",
      "name": "my-db",
      "properties": {
        "application": "[resourceId('Radius.Core/applications', 'my-app')]",
        "environment": "[parameters('environment')]"
      },
      "dependsOn": [
        "[resourceId('Radius.Core/applications', 'my-app')]"
      ]
    }
  ]
}
```

#### Approach 1: Parse Bicep source files directly

The CLI would parse `.bicep` files, locate `connections` blocks, and resolve symbolic resource references (like `db.id`) to build graph edges.

| Pros | Cons |
|---|---|
| Works directly on the source files developers author — no compilation step | Requires a Bicep parser in Go. Bicep's compiler is C#/.NET; no Go parser exists for Bicep syntax. |
| Symbolic names like `db.id` are human-readable and map directly to resource declarations | Must handle Bicep's full expression language: string interpolation, conditionals (`if`), loops (`for`), ternaries, built-in functions |
| No dependency on the Bicep CLI binary being installed | Cannot handle Bicep **modules** — resources split across files via `module` declarations would require recursive resolution |
| Faster execution — no subprocess spawn | Bicep syntax evolves across versions; a custom parser would need ongoing maintenance |
| | Does not handle `.bicepparam` files or parameter defaults that affect which resources are included |

#### Approach 2: Compile Bicep to ARM JSON, then parse the JSON

The CLI invokes `bicep build` (which it already does for `rad deploy` via `pkg/cli/bicep`), then parses the compiled ARM JSON to extract resources, `connections`, and `dependsOn`.

| Pros | Cons |
|---|---|
| Radius CLI already invokes `bicep build` for deployment — the compilation pipeline exists in `pkg/cli/bicep` | Requires the Bicep CLI to be installed (already a prerequisite for `rad deploy`) |
| ARM JSON is a stable, well-documented format with standard JSON parsing in Go | Connection sources become ARM expressions like `"[resourceId('Radius.Data/mongoDatabases', 'my-db')]"` — need to parse `resourceId()` calls to extract type + name |
| Handles modules automatically — Bicep flattens module references into nested deployments in the JSON output | Parameters remain unresolved (e.g., `"[parameters('environment')]"`) — acceptable for static graphs since runtime values aren't needed |
| Handles conditionals, loops, and all Bicep features — the compiler resolves them | Conditional resources appear with a `"condition"` field that can't be evaluated without parameter values — the graph may include resources that wouldn't actually be deployed |
| `dependsOn` arrays are auto-computed — gives deployment ordering edges for free in addition to `connections` edges | Slight overhead of spawning a subprocess, but already acceptable for `rad deploy` |
| Also supports pre-compiled JSON as input — users can pass `.json` files directly, skipping the Bicep step | |
| Literal URL sources (like `"http://backend:8080"`) pass through as-is — easy to distinguish from `resourceId()` expressions | |

#### Recommendation

**Approach 2** (compile to JSON) is the preferred choice:

1. **No custom parser needed.** Parsing ARM JSON in Go is trivial (`encoding/json`). Parsing Bicep syntax in Go would be a significant, ongoing maintenance burden.
2. **Handles all Bicep features.** Modules, conditionals, loops, parameter defaults — the Bicep compiler deals with all of this.
3. **Two edge sources.** The JSON provides both `connections` (application-level relationships) and `dependsOn` (deployment ordering), enabling a richer graph.

The main implementation work is parsing `resourceId()` expressions from connection source strings — a straightforward regex/string-split on a well-known format like `[resourceId('Radius.Data/mongoDatabases', 'my-db')]`.
