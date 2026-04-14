# API Contract: Radius.Core/environments — ProvidersKubernetes Extension

**Feature**: 003-external-k8s-deploy
**Date**: 2026-04-14
**API Version**: 2025-08-01-preview

## Resource Type

`Radius.Core/environments`

## Changed Model: ProvidersKubernetes

### Before (existing)

```json
{
  "providers": {
    "kubernetes": {
      "namespace": "my-namespace"
    }
  }
}
```

### After (extended)

```json
{
  "providers": {
    "kubernetes": {
      "namespace": "my-namespace",
      "target": "external",
      "clusterType": "eks",
      "clusterName": "my-eks-cluster"
    }
  }
}
```

### Field Definitions

| Field | Type | Required | Values | Description |
|-------|------|----------|--------|-------------|
| `namespace` | string | yes | any | Existing. Kubernetes namespace for workloads |
| `target` | string | no | `current` (default), `external` | New. Target cluster for recipe execution |
| `clusterType` | string | conditional | `aks`, `eks` | New. Required when `target = external` |
| `clusterName` | string | conditional | any | New. Required when `clusterType` is `aks` or `eks` |

## Example: EKS External Cluster

```json
{
  "properties": {
    "providers": {
      "aws": {
        "accountId": "123456789012",
        "region": "us-west-2"
      },
      "kubernetes": {
        "namespace": "my-app",
        "target": "external",
        "clusterType": "eks",
        "clusterName": "my-eks-cluster"
      }
    }
  }
}
```

## Example: AKS External Cluster

```json
{
  "properties": {
    "providers": {
      "azure": {
        "subscriptionId": "aaaa-bbbb-cccc-dddd",
        "resourceGroupName": "my-rg"
      },
      "kubernetes": {
        "namespace": "my-app",
        "target": "external",
        "clusterType": "aks",
        "clusterName": "my-aks-cluster"
      }
    }
  }
}
```

## Example: Current Cluster (backward compatible)

```json
{
  "properties": {
    "providers": {
      "kubernetes": {
        "namespace": "my-app"
      }
    }
  }
}
```

## Validation Error Responses

### Missing clusterType when target=external

```json
{
  "error": {
    "code": "BadRequest",
    "message": "providers.kubernetes.clusterType is required when providers.kubernetes.target is 'external'"
  }
}
```

### Missing clusterName when clusterType is set

```json
{
  "error": {
    "code": "BadRequest",
    "message": "providers.kubernetes.clusterName is required when providers.kubernetes.clusterType is 'aks' or 'eks'"
  }
}
```

### clusterType set with target=current

```json
{
  "error": {
    "code": "BadRequest",
    "message": "providers.kubernetes.clusterType and providers.kubernetes.clusterName are only valid when providers.kubernetes.target is 'external'"
  }
}
```

### Missing AWS provider for EKS

```json
{
  "error": {
    "code": "BadRequest",
    "message": "providers.aws configuration with region is required when providers.kubernetes.clusterType is 'eks'"
  }
}
```

### Missing Azure provider for AKS

```json
{
  "error": {
    "code": "BadRequest",
    "message": "providers.azure configuration with resourceGroupName is required when providers.kubernetes.clusterType is 'aks'"
  }
}
```
