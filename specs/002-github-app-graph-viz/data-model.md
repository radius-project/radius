# Data Model: GitHub App Graph Visualization

**Feature Branch**: `002-github-app-graph-viz`
**Date**: 2026-04-12

## Entity: ApplicationGraphResponse

**Description**: Root response object for the application graph. Contains all resources in the graph. This is the existing schema that the static graph constructor outputs and the browser extension consumes.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `resources` | `ApplicationGraphResource[]` | Yes | All resources in the application graph |

**Relationships**: Contains → ApplicationGraphResource (1:many)

---

## Entity: ApplicationGraphResource

**Description**: A node in the application graph representing a single Radius resource. Extended with authoring and diff metadata needed by the browser extension.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | `string` | Yes | Full resource ID (e.g., `/planes/radius/local/resourcegroups/default/providers/Applications.Core/containers/frontend`) |
| `name` | `string` | Yes | Resource display name (e.g., `frontend`) |
| `type` | `string` | Yes | Full resource type (e.g., `Applications.Core/containers`) |
| `provisioningState` | `string` | Yes | Provisioning state (e.g., `Succeeded`, `Failed`, `InProgress`) |
| `connections` | `ApplicationGraphConnection[]` | Yes | Inbound and outbound connections to other resources |
| `outputResources` | `ApplicationGraphOutputResource[]` | Yes | Underlying infrastructure resources (Kubernetes Deployments, Services, etc.) |
| `codeReference` | `string` | No | **NEW** — Repo-root-relative file path with optional line anchor (e.g., `src/cache/redis.ts#L10`) |
| `appDefinitionLine` | `number` | No | **NEW** — 1-based line number of the resource declaration in `app.bicep` used for "App definition" links |
| `diffHash` | `string` | No | **NEW** — Stable hash of review-relevant resource properties used to classify modified resources |

**Relationships**:
- Contains → ApplicationGraphConnection (1:many)
- Contains → ApplicationGraphOutputResource (1:many)
- Belongs to → ApplicationGraphResponse (many:1)

**Validation Rules**:
- `id` must be a valid Radius resource ID
- `codeReference` (when present) must match `^[a-zA-Z0-9_\-./]+(?:#L\d+)?$`
- `codeReference` must not contain `..`, URL schemes, or absolute paths
- `appDefinitionLine` (when present) must be a positive integer
- `diffHash` (when present) must be generated from canonicalized resource properties and treated as opaque by clients

---

## Entity: ApplicationGraphConnection

**Description**: An edge in the application graph representing a relationship between two resources.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | `string` | Yes | Resource ID of the connected resource |
| `direction` | `Direction` | Yes | `Inbound` or `Outbound` relative to the owning resource |

**Relationships**: Belongs to → ApplicationGraphResource (many:1)

---

## Entity: ApplicationGraphOutputResource

**Description**: A leaf infrastructure resource generated from a Radius resource (e.g., Kubernetes Deployment, Service).

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | `string` | Yes | Infrastructure resource ID |
| `name` | `string` | Yes | Infrastructure resource name |
| `type` | `string` | Yes | Infrastructure resource type (e.g., `apps/Deployment`, `core/Service`) |

**Relationships**: Belongs to → ApplicationGraphResource (many:1)

---

## Entity: GraphDiff (browser extension only, not persisted)

**Description**: The computed difference between two application graphs (base branch vs. PR branch). Exists only in the browser extension's runtime memory for rendering diff-colored visualizations.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `added` | `ApplicationGraphResource[]` | Yes | Resources present in PR branch but not base branch |
| `removed` | `ApplicationGraphResource[]` | Yes | Resources present in base branch but not PR branch |
| `modified` | `ModifiedResource[]` | Yes | Resources present in both branches with changed `diffHash` values |
| `unchanged` | `ApplicationGraphResource[]` | Yes | Resources identical in both branches |

**Relationships**: References → ApplicationGraphResource (many:many)

---

## Entity: ModifiedResource (browser extension only, not persisted)

**Description**: A resource that exists in both branches but has changed comparison metadata.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `base` | `ApplicationGraphResource` | Yes | Resource from base branch |
| `current` | `ApplicationGraphResource` | Yes | Resource from PR branch |
| `baseDiffHash` | `string` | Yes | `diffHash` from the base branch artifact |
| `currentDiffHash` | `string` | Yes | `diffHash` from the PR branch artifact |

---

## Entity: StaticGraphArtifact (persisted as `.radius/static/app.json`)

**Description**: The CI-generated static graph JSON file stored in the repository. This is the `ApplicationGraphResponse` serialized to JSON with additional metadata.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `version` | `string` | Yes | Schema version (e.g., `1.0.0`) |
| `generatedAt` | `string` | Yes | ISO 8601 timestamp of generation |
| `sourceFile` | `string` | Yes | Path to the source Bicep file (e.g., `app.bicep`) |
| `application` | `ApplicationGraphResponse` | Yes | The graph data |

**State transitions**: Generated → Stale (when source changes) → Regenerated (on next CI run)

---

## Entity: Direction (enum)

| Value | Description |
|-------|-------------|
| `Inbound` | Connection comes from the referenced resource to this resource |
| `Outbound` | Connection goes from this resource to the referenced resource |

---

## Entity: DiffStatus (enum, browser extension only)

| Value | Color | Description |
|-------|-------|-------------|
| `added` | Green (`#2da44e` / `--color-success-fg`) | Resource exists in PR branch but not base branch |
| `removed` | Red (`#cf222e` / `--color-danger-fg`) | Resource exists in base branch but not PR branch |
| `modified` | Yellow (`#bf8700` / `--color-attention-fg`) | Resource properties differ between branches |
| `unchanged` | Default (`--color-fg-default`) | Resource identical in both branches |

---

## Entity: DeploymentStatus (enum, for deployed graph - P3)

| Value | Color | Description |
|-------|-------|-------------|
| `queued` | Grey (`#656d76` / `--color-neutral-emphasis`) | Resource deployment queued |
| `in-progress` | Yellow (`#bf8700` / `--color-attention-fg`) | Resource deployment in progress |
| `success` | Green (`#2da44e` / `--color-success-fg`) | Resource deployed successfully |
| `failed` | Red (`#cf222e` / `--color-danger-fg`) | Resource deployment failed |

---

## Entity Relationship Diagram

```
ApplicationGraphResponse
  └── resources[] ──→ ApplicationGraphResource
                         ├── connections[] ──→ ApplicationGraphConnection
                         ├── outputResources[] ──→ ApplicationGraphOutputResource
                         ├── codeReference (optional string)
                         ├── appDefinitionLine (optional number)
                         └── diffHash (optional string)

StaticGraphArtifact
  └── application ──→ ApplicationGraphResponse

GraphDiff (runtime only)
  ├── added[] ──→ ApplicationGraphResource
  ├── removed[] ──→ ApplicationGraphResource
  ├── modified[] ──→ ModifiedResource
  │                    ├── base ──→ ApplicationGraphResource
  │                    └── current ──→ ApplicationGraphResource
  └── unchanged[] ──→ ApplicationGraphResource
```
