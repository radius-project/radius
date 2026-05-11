# Design Notes: Git App Graph Preview

**Feature Branch**: `001-git-app-graph-preview`  
**Last Updated**: February 5, 2026  
**Related**: [spec.md](spec.md)

---

## Conceptual Framework

### The Graph as a File Projection

A fundamental aspect of the app graph is that it serves as a **projection of the files associated with an application**. Rather than being a standalone artifact, the graph is computed from and represents the source files that define the application's structure.

### Runtime Environments

The graph is designed to operate in multiple contexts:

| Environment | Description | Use Case |
| --- | --- | --- |
| **Local workstation** | Graph generated from a local clone of a repository | Developer preview, local validation, pre-commit checks |
| **Without git** | Graph generated from standalone files (no git repository) | Quick prototyping, isolated testing, non-versioned projects |
| **Control plane server** | Graph managed by Radius UCP or similar orchestrator | Enterprise scenarios, centralized graph management, cross-repo analysis |
| **GitHub (CI/CD)** | Graph generated or read within GitHub Actions | PR visualization, automated validation, deployment pipelines |

This flexibility ensures the graph works across the full development lifecycle—from local experimentation through production deployment—without requiring a specific hosting model.

### Serialization and Multi-User Considerations

The graph's storage format varies by runtime environment, with different trade-offs for collaboration:

| Environment | Storage | Conflict Handling |
| --- | --- | --- |
| **Local/Git** | JSON file (`.radius/app-graph.json`) | Git branching and merging |
| **Server-hosted** | Graph database | Transactional updates, no merge conflicts |

## Local File Format Design Goals

For users without a server instance, the serialized graph file must be as friendly as possible to branching and merging:

- **Deterministic ordering**: Resources and connections sorted alphabetically by ID, ensuring identical inputs produce identical outputs across branches
- **One resource per logical block**: JSON structure organized so that adding/removing a resource affects minimal lines, reducing merge conflict surface area
- **Stable identifiers**: Resource IDs derived from source content rather than generation order, so the same resource in different branches has the same ID
- **Human-readable format**: Pretty-printed JSON with consistent indentation, making manual conflict resolution feasible when needed

## Server-Hosted Graph Database

When the graph is hosted on a control plane server, a proper graph database (e.g., Neo4j, Amazon Neptune, or an embedded solution) can be used:

- **Transactional updates**: Multiple users can modify the graph concurrently without file-level conflicts
- **Query capabilities**: Rich traversal and pattern matching for cross-repository analysis
- **Real-time collaboration**: Changes propagate immediately without commit/push cycles
- **Historical versioning**: Built-in temporal queries for graph state at any point in time

The graph schema remains consistent across both storage backends—only the serialization and conflict resolution mechanisms differ.

### Standard Schema for Universal Representation

The graph would have a **standard schema** that allows it to represent the app graph. While it could be extended to represent other deployment-related concepts, the app graph is the core component of this schema.

This schema-first approach enables:

- Consistent tooling regardless of source format
- Interoperability between different deployment technologies
- A stable foundation for visualization and diffing

### Multi-Format Source Support

Today, the app model is represented in an `app.bicep` file for Radius users. However, the app graph is designed to work with **any application representation format**:

- Radius `app.bicep` files
- Kustomize files
- GitOps repository configurations
- KRO (Kubernetes Resource Orchestrator) files
- Kubernetes CRDs
- Other declarative infrastructure formats

### Schema Mapping Architecture

The graph understands different source formats through a **two-layer approach**:

1. **Standard Schema**: A universal schema that defines how applications are represented in the graph (resources, connections, metadata)

2. **Schema Mappings**: Format-specific mappings that translate from each source type to the standard graph schema:
   - `radius` → graph (out-of-the-box, first-class support)
   - `kustomize` → graph
   - `kubernetes-crds` → graph
   - `kro` → graph
   - etc.

The Radius schema mapping would be provided out of the box, with mappings for other formats as an extensibility point.

### Lighting Up Experiences in Any Repository

This architecture allows Radius to **light up experiences in any repository**:

- Add Radius to a GitOps pipeline, and the graph understands the GitOps configurations
- The graph loads itself from whatever source format exists
- Visualization, diffing, and PR integration work regardless of the underlying technology

### Recipe Registration Against the Graph

If the app graph is loaded (from any source format), we can then **register recipes against the app graph**. This decouples recipe registration from the source format, allowing Radius deployments to work with any technology.

```text
┌─────────────────────────────────────────────────────────────────────────────┐
│                           SOURCE FORMATS                                    │
├─────────────┬─────────────┬─────────────┬─────────────┬─────────────────────┤
│  app.bicep  │  kustomize  │  kro files  │  k8s CRDs   │  gitops configs     │
└──────┬──────┴──────┬──────┴──────┬──────┴──────┬──────┴──────────┬──────────┘
       │             │             │             │                 │
       ▼             ▼             ▼             ▼                 ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                         SCHEMA MAPPINGS                                     │
│   (format-specific parsers that translate to standard graph schema)         │
└─────────────────────────────────┬───────────────────────────────────────────┘
                                  │
                                  ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                            APP GRAPH                                        │
│   ┌─────────────┐    ┌─────────────┐    ┌─────────────┐                     │
│   │  Foo        │──▶│  Bar        │───▶│ Baz         │                     │
│   └─────────────┘    └─────────────┘    └─────────────┘                     │
│                    (standard schema representation)                         │
└─────────────────────────────────┬───────────────────────────────────────────┘
                                  │
                                  ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                       RECIPE REGISTRATION                                   │
│   Recipes bound to types in the graph                                       │
└─────────────────────────────────┬───────────────────────────────────────────┘
                                  │
                                  ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                          DEPLOYMENT                                         │
│   Graph + Recipes → Concrete infrastructure in target environment           │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Migration Path: GitOps to Bicep

The graph facilitates **migration between deployment technologies**. For example, migrating from a GitOps workflow to a simpler `app.bicep` representation:

```text
┌─────────────────────────────────────────────────────────────────────────────┐
│                         GITOPS REPOSITORY                                   │
│  ├── clusters/                                                              │
│  │   └── prod/                                                              │
│  │       ├── flux-system/                                                   │
│  │       └── apps/                                                          │
│  │           ├── frontend-deployment.yaml                                   │
│  │           ├── backend-deployment.yaml                                    │
│  │           ├── redis-statefulset.yaml                                     │
│  │           └── kustomization.yaml                                         │
│  └── base/                                                                  │
│      └── ...                                                                │
└─────────────────────────────────┬───────────────────────────────────────────┘
                                  │
                                  │ (1) Parse GitOps configs
                                  │     via kustomize schema mapping
                                  ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                            APP GRAPH                                        │
│                                                                             │
│   Resources:                    Connections:                                │
│   ┌──────────────────┐          ┌──────────────────────────────────┐        │
│   │ frontend         │          │ frontend → backend (http:8080)   │        │
│   │ type: Container  │          │ backend  → redis   (tcp:6379)    │        │
│   │ image: nginx:1.2 │          └──────────────────────────────────┘        │
│   ├──────────────────┤                                                      │
│   │ backend          │          Metadata:                                   │
│   │ type: Container  │          • sourceFormat: kustomize                   │
│   │ image: api:3.1   │          • migrationDate: 2026-02-05                 │
│   ├──────────────────┤          • originalFiles: [list of yamls]            │
│   │ redis            │                                                      │
│   │ type: Cache      │                                                      │
│   └──────────────────┘                                                      │
└─────────────────────────────────┬───────────────────────────────────────────┘
                                  │
                                  │ (2) Serialize graph to Bicep
                                  │     via radius schema mapping
                                  ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                            app.bicep                                        │
│                                                                             │
│   resource frontend 'Applications.Core/containers@2023-10-01' = {           │
│     name: 'frontend'                                                        │
│     properties: {                                                           │
│       container: { image: 'nginx:1.2' }                                     │
│       connections: { backend: { source: backend.id } }                      │
│     }                                                                       │
│   }                                                                         │
│                                                                             │
│   resource backend 'Applications.Core/containers@2023-10-01' = { ... }      │
│   resource redis 'Applications.Datastores/redisCaches@2023-10-01' = { ... } │
└─────────────────────────────────────────────────────────────────────────────┘
                                  │
                                  │ (3) Validate equivalence
                                  │     regenerate graph from bicep,
                                  ▼     compare to original

                           ✓ Migration Complete
```

Because both formats map to the same intermediate graph representation, translation becomes straightforward:

1. Parse GitOps configs into the graph
2. Serialize the graph to `app.bicep` format
3. Validate equivalence

### Intelligent State (vs. Terraform State)

Another way of thinking about the graph is that it represents **intelligent state**—similar in purpose to Terraform state, but without the associated management problems:

| Terraform State | App Graph |
| ----------------- | ----------- |
| Stores resource mappings and metadata | Stores resource relationships and metadata |
| Requires remote backends, locking, conflict resolution | Committed to git, versioned naturally |
| State drift causes deployment failures | Graph is regenerated from source, no drift |
| Sensitive data in state files | No runtime secrets—purely structural |
| Single source of truth (fragile) | Derived artifact (resilient) |

The app graph is a **derived view** rather than a primary data store, which eliminates the operational burden of state management while providing the benefits of understanding what was deployed and how it relates.

---

## Implications for Implementation

These conceptual foundations inform several implementation decisions in the [spec](spec.md):

1. **Committed Artifact Model**: The graph is committed to version control (`.radius/app-graph.json`) because it's a derived projection, not primary state

2. **Deterministic Output**: Same input must produce identical output to support git-based versioning

3. **Schema-First Design**: The JSON graph format should be designed for extensibility to support future schema mappings

4. **Lightweight GitHub Action**: The Action reads committed JSON rather than generating graphs, keeping it source-format agnostic

5. **Staleness Detection**: The `sourceHash` field enables validation that the graph matches its source files

## Appendix 1: Radius Conceptual Graph

```mermaid
flowchart TD
  Workspace["Workspace"]
  Group["Group"]
  Environment["Environment"]
  RecipePack["Recipe Pack"]
  Recipe["Recipe"]
  Deployment["Deployment"]
  ResourceType["Resource Type"]
  DeployedResource["Deployed Resource<br/>(Resource Instance)"]
  Workspace --> Environment
  Workspace --> Group

  %% Environment relationships
  Environment --> RecipePack
  Environment --> Deployment

  %% Deployment selects group
  Group --> Deployment

  %% Recipes and resource types
  RecipePack --> Recipe
  Recipe --> ResourceType

  %% Deployment uses recipes and types
  Deployment --> ResourceType
  Deployment --> Recipe

  %% Deployment produces resource instances
  Deployment --> DeployedResource

  %% Resource instances are of a type and produced by a recipe
  DeployedResource --> ResourceType
  DeployedResource --> Recipe
```