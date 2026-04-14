# External Cluster Deployment for Radius

* **Author**: Shruthi Kumar (@sk593)

## Overview

External cluster deployment enables Radius to deploy application output resources (Kubernetes Deployments, Services, ConfigMaps, etc.) to a cluster different from the one where the Radius control plane is installed. This supports the GitHub Actions deployment model where an ephemeral k3d cluster hosts Radius while the user's production EKS or AKS cluster receives the deployed application.

## Terms and definitions

| Term | Definition |
|------|-----------|
| **Control plane cluster** | The Kubernetes cluster where Radius is installed (e.g., ephemeral k3d in GitHub Actions). |
| **Target cluster** | The user's existing Kubernetes cluster (EKS, AKS, GKE) where application output resources are deployed. |
| **Output resources** | Kubernetes resources created by Radius during deployment — Deployments, Services, ConfigMaps, Secrets, etc. |
| **RADIUS_TARGET_KUBECONFIG** | Environment variable pointing to a kubeconfig file for the target cluster. |

## Objectives

### Goals

- Enable Radius to deploy output Kubernetes resources to an external target cluster while the control plane runs on a separate cluster.
- Support EKS and AKS as target clusters.
- Require no changes to the Radius CLI or Bicep templates — the configuration is transparent to the user.
- Work with the GitHub Actions ephemeral k3d deployment model.

### Non goals

- **Multi-cluster orchestration**: Deploying different resources to different clusters within a single deployment is out of scope.
- **GKE support**: Google Kubernetes Engine is not yet tested but should work with the same mechanism.
- **Persistent control plane**: This design assumes an ephemeral control plane. A persistent Radius installation with external target clusters is a future consideration.

## Design

### High Level Design

The Radius control plane runs on one Kubernetes cluster (k3d) and deploys output resources to another (EKS/AKS). This is achieved by providing a target cluster kubeconfig to the Radius components via a Kubernetes secret and an environment variable.

```
┌──────────────────────────┐     ┌──────────────────────────┐
│  k3d Cluster             │     │  Target Cluster          │
│  (Control Plane)         │     │  (EKS / AKS)             │
│                          │     │                          │
│  ┌─────────────────┐     │     │  ┌─────────────────┐     │
│  │ applications-rp │─────┼─────┼─▶│ Deployments     │     │
│  │ (async worker)  │     │     │  │ Services        │     │
│  └─────────────────┘     │     │  │ ConfigMaps      │     │
│  ┌─────────────────┐     │     │  │ Secrets         │     │
│  │ dynamic-rp      │─────┼─────┼─▶│ (output         │     │
│  │                 │     │     │  │  resources)     │     │
│  └─────────────────┘     │     │  └─────────────────┘     │
│  ┌─────────────────┐     │     │                          │
│  │ ucpd            │     │     └──────────────────────────┘
│  │ controller      │     │
│  │ bicep-de        │     │
│  └─────────────────┘     │
└──────────────────────────┘
```

### Detailed Design

#### Environment Variable

When `RADIUS_TARGET_KUBECONFIG` is set, Radius loads a separate Kubernetes client configuration for deploying output resources. The control plane continues to use its in-cluster configuration for internal operations (UCP, controller, etc.).

#### Code Changes

**`pkg/kubeutil/config.go`** — New function `NewTargetClientConfig()`:

```go
const TargetKubeconfigEnvVar = "RADIUS_TARGET_KUBECONFIG"

func NewTargetClientConfig(options *ConfigOptions) (*rest.Config, error) {
    targetPath := os.Getenv(TargetKubeconfigEnvVar)
    if targetPath == "" {
        return nil, nil // No target cluster configured
    }
    return NewClientConfigFromLocal(&ConfigOptions{ConfigFilePath: targetPath})
}
```

**`pkg/server/asyncworker.go`** — Modified `Run()` to create target cluster clients:

```go
// Check for external target cluster
outputK8s := k8s
targetConfig, _ := kubeutil.NewTargetClientConfig(nil)
if targetConfig != nil {
    outputK8s, _ = kubeutil.NewClients(targetConfig)
}
// Pass outputK8s to NewApplicationModel for output resource deployment
appModel, _ := model.NewApplicationModel(..., outputK8s.RuntimeClient, outputK8s.ClientSet, ...)
```

This means:
- `KubernetesHandler.Put()` deploys output resources using the target cluster client
- UCP, controller, and other internal components continue using the k3d in-cluster client
- If `RADIUS_TARGET_KUBECONFIG` is not set, behavior is unchanged (single cluster)

#### Workflow Integration

The deploy workflow:

1. Authenticates with the cloud provider via OIDC
2. Fetches the target cluster kubeconfig (static token for EKS, `az aks get-credentials` for AKS)
3. Creates an ephemeral k3d cluster and installs Radius
4. Creates a Kubernetes secret with the target kubeconfig
5. Patches `applications-rp` and `dynamic-rp` deployments to mount the secret and set `RADIUS_TARGET_KUBECONFIG`
6. Runs `rad deploy` which deploys cloud resources via registered credentials and Kubernetes resources to the target cluster

#### EKS Static Token Kubeconfig

EKS uses exec-based credential plugins by default (`aws eks update-kubeconfig`), which require the AWS CLI inside the container. Since Radius containers don't have AWS CLI, the workflow generates a static kubeconfig with a bearer token:

```yaml
apiVersion: v1
clusters:
- cluster:
    certificate-authority-data: <CA_DATA>
    server: <ENDPOINT>
  name: eks
contexts:
- context:
    cluster: eks
    user: eks-user
  name: eks
current-context: eks
kind: Config
users:
- name: eks-user
  user:
    token: <TOKEN>  # From aws eks get-token, valid ~15 minutes
```

The token is valid for approximately 15 minutes, which is sufficient for typical deployment workflows.

#### EKS Access Entry

The IAM role used by the workflow must have access to the EKS cluster's Kubernetes API. The verification workflow automatically creates an EKS access entry and polls until the policy propagates (up to 2 minutes):

```bash
aws eks create-access-entry \
  --cluster-name $CLUSTER \
  --principal-arn $ROLE_ARN \
  --type STANDARD

aws eks associate-access-policy \
  --cluster-name $CLUSTER \
  --principal-arn $ROLE_ARN \
  --policy-arn arn:aws:eks::aws:cluster-access-policy/AmazonEKSClusterAdminPolicy \
  --access-scope type=cluster
```

The workflow also checks the cluster's auth mode and upgrades from `CONFIG_MAP` to `API_AND_CONFIG_MAP` if needed. After creating the entry, it polls `kubectl get nodes` every 10 seconds until successful.

**Note:** If the IAM role is recreated (e.g., CloudFormation stack update), the existing access entry becomes stale and must be deleted and recreated. The verification workflow handles new entries automatically but cannot detect stale ones.

#### AKS Kubeconfig

AKS kubeconfigs from `az aks get-credentials` use Azure Identity tokens which work inside the container since the Azure Identity SDK is available.

#### Deployment Patching

The workflow patches `applications-rp` and `dynamic-rp` to mount the target kubeconfig:

```json
[
  {
    "op": "add",
    "path": "/spec/template/spec/volumes/-",
    "value": {
      "name": "target-kubeconfig",
      "secret": { "secretName": "target-kubeconfig" }
    }
  },
  {
    "op": "add",
    "path": "/spec/template/spec/containers/0/volumeMounts/-",
    "value": {
      "name": "target-kubeconfig",
      "mountPath": "/etc/radius/target-kubeconfig",
      "readOnly": true
    }
  },
  {
    "op": "add",
    "path": "/spec/template/spec/containers/0/env/-",
    "value": {
      "name": "RADIUS_TARGET_KUBECONFIG",
      "value": "/etc/radius/target-kubeconfig/config"
    }
  }
]
```

Only `applications-rp` and `dynamic-rp` are patched — they handle output resource deployment. `ucpd`, `controller`, and `bicep-de` continue using in-cluster config for internal operations.

#### Custom Images

The code change in `asyncworker.go` must be in the container images. The deploy workflow supports custom image registries:

```bash
rad install kubernetes \
  --set global.imageRegistry=ghcr.io/my-registry \
  --set global.imageTag=my-tag \
  --set de.image=ghcr.io/radius-project/deployment-engine \
  --set dashboard.image=ghcr.io/radius-project/dashboard
```

The `deployment-engine` and `dashboard` images are pinned to the public registry since they are not built from this repository.

### Error Handling

| Scenario | Handling |
|----------|---------|
| `RADIUS_TARGET_KUBECONFIG` not set | No-op — single cluster behavior, output resources deploy to control plane cluster |
| Target kubeconfig file not found | `applications-rp` fails to start with clear error message |
| Target cluster unreachable | Deployment fails with connection error — user sees it in `rad deploy` output |
| EKS token expired (>15 min) | Deploy step refreshes the token, recreates the kubeconfig secret, and restarts pods before running `rad deploy` |
| IAM role lacks EKS access | Verification workflow creates access entry automatically and polls until propagated |
| EKS access entry stale (role recreated) | Must be manually deleted and recreated — workflow cannot detect stale entries |

## Test plan

### Manual Testing

1. Create an EKS cluster and configure OIDC credentials via the browser extension
2. Set `RADIUS_IMAGE_REGISTRY` and `RADIUS_IMAGE_TAG` to point to custom images with the code change
3. Trigger deploy from the extension
4. Verify output resources (Deployment, Service) are created on the EKS cluster, not the k3d cluster

### Unit Testing

- Test `NewTargetClientConfig()` returns nil when env var is not set
- Test `NewTargetClientConfig()` loads kubeconfig from specified path
- Test `AsyncWorker.Run()` uses target cluster clients when env var is set

## Security

- **Target kubeconfig stored as Kubernetes secret** — mounted read-only into the container
- **EKS tokens are short-lived** — ~15 minute validity, fresh for each workflow run
- **EKS access entry scoped to the IAM role** — only the deploy workflow's OIDC role has cluster access
- **No credential persistence** — the k3d cluster and all secrets are destroyed after the workflow

## Compatibility

This change is fully backward compatible. When `RADIUS_TARGET_KUBECONFIG` is not set, Radius behaves exactly as before — all resources deploy to the cluster where Radius is installed.

## Open Questions

1. **Long-running deployments**: EKS tokens expire after ~15 minutes. The deploy step refreshes the token before `rad deploy`, but deployments taking longer than 15 minutes may still fail.
2. **bicep-de patching**: The deployment engine may also need target cluster access for recipes that use `extension kubernetes`. Currently only `applications-rp` and `dynamic-rp` are patched.
3. **Persistent installations**: How should this work for non-ephemeral Radius installations where the target cluster might change?
4. **Stale EKS access entries**: When an IAM role is recreated with the same name, the old access entry blocks the new one. The workflow should detect and handle this automatically.

## Alternatives considered

### Install Radius directly on the target cluster

Install Radius on the user's EKS/AKS cluster instead of k3d. Rejected because:
- Requires the user to grant Radius admin access to their production cluster
- Leaves Radius installed after deployment (cleanup burden)
- The goal is zero-install Radius for GitHub deployments

### Use Radius on the target cluster with Helm uninstall after deploy

Install Radius on the target cluster, deploy, then uninstall. Rejected because:
- Uninstall may leave resources behind
- CRDs and webhooks persist after Helm uninstall
- Risk of interfering with existing workloads during install/uninstall

## Design Review Notes

<!-- Update this section with the decisions made during the design review meeting. -->
