# Research: External Kubernetes Cluster Deployment

**Feature**: 003-external-k8s-deploy
**Date**: 2026-04-14

## EKS Token Generation

### Decision
Use the AWS SDK v2 directly to generate EKS bearer tokens via STS presigned URL. Do not import `aws-iam-authenticator/pkg/token`.

### Rationale
The core token generation logic is ~15 lines using `sts.NewPresignClient` and `PresignGetCallerIdentity`. The `aws-iam-authenticator` library pulls in heavy transitive dependencies (metrics, file caching, IMDS) that Radius doesn't need.

### Alternatives Considered
- **`sigs.k8s.io/aws-iam-authenticator/pkg/token`**: Canonical implementation but heavy dependency. Rejected for simplicity.

### Technical Details
- **Token format**: `k8s-aws-v1.` + `base64.RawURLEncoding(presigned_STS_GetCallerIdentity_URL)`
- **Token lifetime**: 15 minutes (hardcoded by STS regardless of `X-Amz-Expires` value)
- **Required header**: `x-k8s-aws-id` set to cluster name (must be signed)
- **Cluster info**: `eks.DescribeCluster` returns endpoint + CA certificate (base64)
- **Required IAM permissions**: `sts:GetCallerIdentity` (implicit), `eks:DescribeCluster` (explicit)

### Dependencies
- `github.com/aws/aws-sdk-go-v2/service/eks` — **new, needs `go get`**
- `github.com/aws/aws-sdk-go-v2/service/sts` — already in go.mod
- `github.com/aws/aws-sdk-go-v2/config` — already in go.mod
- `github.com/aws/aws-sdk-go-v2/credentials` — already in go.mod
- `github.com/aws/smithy-go` — already indirect in go.mod

---

## AKS Credential Acquisition with Entra ID

### Decision
Use `ListClusterUserCredentials` with exec format to extract cluster endpoint and CA cert, then acquire an Entra ID access token using the registered Azure service principal or workload identity credentials. Do not use `ListClusterAdminCredentials` since local accounts are disabled by default on AKS.

### Rationale
Admin credentials (`ListClusterAdminCredentials`) fail on AKS clusters with local accounts disabled (the default). The user credential + Entra ID token flow is the standard production approach equivalent to `az aks get-credentials` + `kubelogin convert-kubeconfig --login spn`.

### Alternatives Considered
- **Admin credentials**: Simpler but fails on default AKS configurations. Rejected.
- **Direct ManagedClusters.Get()**: Could get FQDN but misses the fully-formed kubeconfig template. Using `ListClusterUserCredentials` is more complete.

### Technical Details
- **AKS AAD Server App ID**: `6dae42f8-4368-4678-94ff-3960e28e3630` (well-known, same across all public Azure tenants)
- **Token scope**: `6dae42f8-4368-4678-94ff-3960e28e3630/.default`
- **Credential types supported**: `ClientSecretCredential` (service principal) and `WorkloadIdentityCredential` — both already implemented in `pkg/azure/credential/ucpcredentials.go` via `UCPCredential`
- **Required Azure RBAC**: `Azure Kubernetes Service Cluster User Role` + a Kubernetes RBAC role (e.g., `Azure Kubernetes Service RBAC Writer`)

### Dependencies
- `github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerservice/armcontainerservice/v6` — **new, needs `go get`**
- `github.com/Azure/azure-sdk-for-go/sdk/azidentity` v1.13.1 — already in go.mod
- `github.com/Azure/azure-sdk-for-go/sdk/azcore` v1.21.0 — already in go.mod

---

## Terraform Kubernetes Provider Config

### Decision
Pass external cluster credentials inline (`host`, `token`, `cluster_ca_certificate`) in the `BuildConfig()` return map. No temp files.

### Rationale
The Terraform Kubernetes provider natively supports inline credential fields. The `map[string]any` returned by `BuildConfig()` maps 1:1 to Terraform provider JSON keys. The Azure and AWS providers already use this exact pattern for their credentials. Inline avoids temp file lifecycle management and security risks.

### Technical Details
- **Current `kubernetesProvider.BuildConfig()`**: Returns `{"config_path": "~/.kube/config"}` (not in cluster) or `nil` (in cluster)
- **New behavior**: When `envConfig` has external cluster target, return `{"host": ..., "token": ..., "cluster_ca_certificate": ...}`
- **Data flow**: `BuildConfig()` → `getProviderConfigs()` → `cfg.Provider["kubernetes"]` → `main.tf.json` → `terraform init/apply`
- **Signature change needed**: `BuildConfig()` currently receives `envConfig *recipes.Configuration` which will contain the new Kubernetes provider fields after data model changes.

---

## Bicep/UCP Deployment Engine

### Decision
For Bicep templates, pass the external kubeconfig through the `kubeConfig` parameter of the `extension kubernetes` block. This is the mechanism the deployment engine already supports.

### Rationale
The deployment engine (DE) is a separate C# service that creates Kubernetes resources directly. When `kubeConfig: ''` (empty), it uses in-cluster credentials. When a non-empty kubeconfig is provided, the DE uses it. This is the intended extensibility point.

### Technical Details
- **Deployment engine**: External C# service (`ghcr.io/radius-project/deployment-engine`) — not in this Go repo
- **Three injection points required**:
  1. **Bicep DE**: Pass kubeconfig via `extension kubernetes { kubeConfig: ... }` — requires the Bicep driver to inject the kubeconfig into the deployment context
  2. **Terraform**: Inline credentials in provider block (covered above)
  3. **ResourceClient** (garbage collection): The `KubernetesClientProvider` used for recipe resource deletion needs per-environment client creation instead of the current singleton
- **`recipes.Configuration` change**: The `Providers` struct currently has `Azure` and `AWS` but no `Kubernetes`. A new `Kubernetes` field is needed to carry `target`, `clusterType`, `clusterName` through to drivers.
- **`ConfigurationLoader`**: Must be updated to populate the new Kubernetes provider config from the Environment resource.

### Complexity Note
The Bicep path is more complex than Terraform because:
- The DE is a separate C# service that may need changes
- The kubeconfig injection happens at the Bicep template parameter level
- Recipe templates may need to accept a kubeconfig parameter

---

## Terraform State Backend

### Decision
Keep Terraform state on the local Radius cluster. The Kubernetes backend configuration continues to use in-cluster or local kubeconfig. Only the Terraform Kubernetes provider uses external credentials.

### Rationale
Centralizing state on the local cluster is simpler, avoids requiring external credentials for the backend, and keeps state management in one place. The `backends/kubernetes.go` code path is untouched.

### Alternatives Considered
- **State on external cluster**: State lives alongside resources but distributes state management and adds complexity. Rejected for v1.
- **Configurable**: Maximum flexibility but adds model complexity. Deferred.
