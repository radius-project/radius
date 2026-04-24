# Data Model: External Kubernetes Cluster Deployment

**Feature**: 003-external-k8s-deploy
**Date**: 2026-04-14

## Entity Changes

### ProvidersKubernetes (Extended)

The existing `ProvidersKubernetes` model gains three new fields.

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `namespace` | string | yes | — | Kubernetes namespace (existing) |
| `target` | enum: `current`, `external` | no | `current` | Whether to target the local cluster or an external one |
| `clusterType` | enum: `aks`, `eks` | conditional | — | Required when `target = external`. Type of managed cluster |
| `clusterName` | string | conditional | — | Required when `clusterType` is `aks` or `eks`. Name of the cluster |

### Validation Rules

1. When `target` is omitted or `current`: `clusterType` and `clusterName` MUST NOT be set.
2. When `target` is `external`: `clusterType` is required.
3. When `clusterType` is `aks` or `eks`: `clusterName` is required.
4. When `clusterType` is `eks`: `providers.aws` MUST be configured with `region`.
5. When `clusterType` is `aks`: `providers.azure` MUST be configured (including `subscriptionId` and `resourceGroupName`).

### Cross-Entity Dependencies

```
Environment
├── providers.kubernetes (ProvidersKubernetes)
│   ├── namespace (existing)
│   ├── target → determines kubeconfig source
│   ├── clusterType → determines credential flow (EKS via STS, AKS via Entra ID)
│   └── clusterName → cluster identifier for API calls
├── providers.aws (ProvidersAws) — required for clusterType=eks
│   ├── accountId
│   └── region → used for eks.DescribeCluster and STS presign
└── providers.azure (ProvidersAzure) — required for clusterType=aks
    ├── subscriptionId → used for AKS API calls
    ├── resourceGroupName → used for ListClusterUserCredentials
    └── identity (IdentitySettings)
```

## New Internal Types

### KubeCredentials (internal, not persisted)

Ephemeral credentials obtained at recipe execution time. Never stored as a Radius resource.

| Field | Type | Description |
|-------|------|-------------|
| `Host` | string | Kubernetes API server URL |
| `Token` | string | Bearer token |
| `CACertificate` | []byte | PEM-encoded CA certificate |

Produced by: EKS token generator or AKS Entra ID token generator
Consumed by: Terraform provider config builder, Bicep driver, ResourceClient

## TypeSpec Changes

### environments.tsp — ProvidersKubernetes

```typespec
@doc("Target cluster for Kubernetes deployments.")
union KubernetesTarget {
  @doc("Deploy to the cluster where Radius is installed.")
  current: "current",

  @doc("Deploy to an external managed Kubernetes cluster.")
  external: "external",
}

@doc("Type of managed Kubernetes cluster.")
union KubernetesClusterType {
  @doc("Azure Kubernetes Service cluster.")
  aks: "aks",

  @doc("Amazon Elastic Kubernetes Service cluster.")
  eks: "eks",
}

model ProvidersKubernetes {
  @doc("Kubernetes namespace to deploy workloads into.")
  `namespace`: string;

  @doc("Target cluster. Defaults to 'current' (the cluster where Radius is installed).")
  target?: KubernetesTarget;

  @doc("Type of managed Kubernetes cluster. Required when target is 'external'.")
  clusterType?: KubernetesClusterType;

  @doc("Name of the managed Kubernetes cluster. Required when clusterType is 'aks' or 'eks'.")
  clusterName?: string;
}
```

### Go Data Model — environment_v20250801preview.go

```go
type ProvidersKubernetes_v20250801preview struct {
    Namespace   string `json:"namespace"`
    Target      string `json:"target,omitempty"`      // "current" or "external"
    ClusterType string `json:"clusterType,omitempty"` // "aks" or "eks"
    ClusterName string `json:"clusterName,omitempty"`
}
```

### Internal Data Model — environment.go

The `Providers` struct in `pkg/corerp/datamodel/environment.go` needs a Kubernetes field:

```go
type Providers struct {
    Azure      ProvidersAzure      `json:"azure"`
    AWS        ProvidersAWS        `json:"aws"`
    Kubernetes ProvidersKubernetes `json:"kubernetes"` // NEW
}
```

```go
type ProvidersKubernetes struct {
    Namespace   string `json:"namespace"`
    Target      string `json:"target,omitempty"`
    ClusterType string `json:"clusterType,omitempty"`
    ClusterName string `json:"clusterName,omitempty"`
}
```


### recipes.Configuration Extension

And `recipes.Configuration` in `pkg/recipes/types.go` — the `RuntimeConfiguration` already carries `Kubernetes.Namespace`. The external cluster info flows through `Configuration.Providers.Kubernetes`.

## State Transitions

```
Environment Create/Update
  │
  ├─ Validate property combinations (FR-010, FR-012)
  │   ├─ PASS → store environment
  │   └─ FAIL → return 400 with specific validation error
  │
  └─ Stored Environment (no kubeconfig generated yet)

Recipe Execution
  │
  ├─ Load Configuration (ConfigurationLoader)
  │   └─ Populate Providers.Kubernetes from Environment
  │
  ├─ target = current (or omitted)
  │   └─ Use existing kubeconfig resolution (no change)
  │
  └─ target = external
      ├─ clusterType = eks
      │   ├─ Fetch AWS credentials from UCP
      │   ├─ eks.DescribeCluster → endpoint + CA cert
      │   ├─ STS PresignGetCallerIdentity → bearer token
      │   └─ KubeCredentials{Host, Token, CACertificate}
      │
      └─ clusterType = aks
          ├─ Fetch Azure credentials from UCP
          ├─ ListClusterUserCredentials → endpoint + CA cert
          ├─ Entra ID GetToken(AKS_AAD_SCOPE) → bearer token
          └─ KubeCredentials{Host, Token, CACertificate}
```
