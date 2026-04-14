# Feature Specification: External Kubernetes Cluster Deployment

**Feature Branch**: `003-external-k8s-deploy`
**Created**: 2026-04-14
**Status**: Draft
**Input**: User description: "Enhance Radius to deploy workloads and execute recipes against external AKS or EKS clusters, rather than only the cluster where Radius is installed."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Deploy to an External EKS Cluster (Priority: P1)

As a platform engineer, I want to configure a Radius environment to deploy workloads to an external Amazon EKS cluster so that Radius can manage applications on clusters other than the one it is installed on.

I configure my environment's Kubernetes provider with `target: external`, `clusterType: eks`, and the EKS cluster name. Radius uses the existing registered AWS credentials along with the AWS region from `providers.aws.region` to obtain a kubeconfig for the target EKS cluster before executing any recipe (Terraform or Bicep). All Kubernetes resources created by the recipe land on the external cluster.

**Why this priority**: EKS is a widely-adopted managed Kubernetes platform. Enabling external EKS deployment unlocks multi-cluster workflows and is the primary motivator for this feature.

**Independent Test**: Can be fully tested by creating an environment pointing at an external EKS cluster, deploying a simple Kubernetes resource via a recipe (Terraform or Bicep), and verifying the resource exists on the external cluster.

**Acceptance Scenarios**:

1. **Given** an environment with `providers.kubernetes.target = external`, `providers.kubernetes.clusterType = eks`, `providers.kubernetes.clusterName = my-eks-cluster`, and valid AWS credentials registered, **When** a recipe that creates a Kubernetes ConfigMap is executed, **Then** the ConfigMap is created in the specified namespace on the external EKS cluster regardless of recipe engine.
2. **Given** an environment with `providers.kubernetes.target = external` and `providers.kubernetes.clusterType = eks`, **When** the registered AWS credentials lack permission to describe the EKS cluster, **Then** the recipe execution fails with a clear error message indicating insufficient AWS permissions to obtain the kubeconfig.
3. **Given** an environment with `providers.kubernetes.clusterType = eks` and `providers.kubernetes.clusterName` set to a non-existent cluster, **When** a recipe is executed, **Then** the operation fails with a clear error indicating the cluster was not found.
4. **Given** `providers.kubernetes.target = external` and `clusterType = eks` without `clusterName`, **When** the environment is created, **Then** validation fails stating `clusterName` is required.
5. **Given** `providers.kubernetes.clusterType = eks` without a corresponding `providers.aws` configuration, **When** the environment is created, **Then** validation fails stating that AWS provider configuration is required for EKS clusters.

---

### User Story 2 - Deploy to an External AKS Cluster (Priority: P1)

As a platform engineer, I want to configure a Radius environment to deploy workloads to an external Azure AKS cluster so that I can manage applications across multiple Azure-hosted clusters from a single Radius installation.

I configure my environment's Kubernetes provider with `target: external`, `clusterType: aks`, and the AKS cluster name. Radius uses the existing registered Azure credentials along with `providers.azure.resourceGroupName` to obtain a kubeconfig for the target AKS cluster before executing any recipe.

**Why this priority**: AKS is equally important as EKS for enterprise multi-cluster deployments and uses a parallel credential flow.

**Independent Test**: Can be fully tested by creating an environment pointing at an external AKS cluster, deploying a Kubernetes resource via a recipe (Terraform or Bicep), and verifying the resource on the external cluster.

**Acceptance Scenarios**:

1. **Given** an environment with `providers.kubernetes.target = external`, `providers.kubernetes.clusterType = aks`, `providers.kubernetes.clusterName = my-aks-cluster`, valid Azure credentials registered, and `providers.azure.resourceGroupName` set, **When** a recipe that creates a Kubernetes ConfigMap is executed, **Then** the ConfigMap is created in the specified namespace on the external AKS cluster regardless of recipe engine.
2. **Given** an environment with `providers.kubernetes.clusterType = aks` and missing `providers.azure.resourceGroupName`, **When** the environment is created or updated, **Then** the operation fails with a validation error indicating that `providers.azure.resourceGroupName` is required when `clusterType` is `aks`.
3. **Given** an environment with `providers.kubernetes.clusterType = aks`, **When** the registered Azure credentials lack permission to list AKS cluster credentials, **Then** the recipe execution fails with a clear error message indicating insufficient Azure permissions.
4. **Given** `providers.kubernetes.clusterType = aks` without a corresponding `providers.azure` configuration, **When** the environment is created, **Then** validation fails stating that Azure provider configuration is required for AKS clusters.

---

### User Story 3 - Default Behavior Unchanged for Current Cluster (Priority: P1)

As an existing Radius user, I want my current environments to continue working without modification so that the external cluster feature does not break my existing workflows.

When `providers.kubernetes.target` is omitted or set to `current`, Radius behaves exactly as it does today: recipes execute against the local/in-cluster Kubernetes using the existing kubeconfig resolution.

**Why this priority**: Backward compatibility is non-negotiable. Existing users must not be affected.

**Independent Test**: Can be tested by deploying a recipe with an environment that does not set `target` and verifying the resource lands on the local cluster, identical to current behavior.

**Acceptance Scenarios**:

1. **Given** an existing environment with only `providers.kubernetes.namespace` set (no `target` property), **When** a recipe is executed, **Then** it deploys to the local cluster exactly as it does today.
2. **Given** an environment with `providers.kubernetes.target = current`, **When** a recipe is executed, **Then** it deploys to the local cluster.
3. **Given** an environment with `providers.kubernetes.target = current` and `clusterType` or `clusterName` also set, **When** the environment is created or updated, **Then** validation fails because `clusterType` and `clusterName` are only valid when `target = external`.

---

### Edge Cases

- What happens when the external cluster's API server is temporarily unreachable? Radius returns a clear connectivity error rather than a generic failure.
- What happens when the dynamically-obtained kubeconfig token expires mid-recipe-execution? A fresh kubeconfig is obtained per recipe execution. For EKS tokens (~15 min validity), this is sufficient for most recipes. Token refresh during execution is out of scope.
- What happens when the external cluster's namespace specified in `providers.kubernetes.namespace` does not exist? Radius reports a clear error about the missing namespace.
- What happens when both AWS and Azure providers are configured but `clusterType` is `eks`? Only the AWS credentials are used for kubeconfig acquisition; the Azure provider is used for any Azure-targeted resources in the recipe, not for Kubernetes access.

## Clarifications

### Session 2026-04-14

- Q: Should direct Kubernetes resource management by Radius (e.g., Applications.Core/containers) also target the external cluster, or only recipe execution? → A: Recipes only — direct resource management stays on the local cluster.
- Q: Should Radius cache the dynamically-obtained kubeconfig or obtain a fresh one per recipe execution? → A: Fresh kubeconfig per recipe execution (no caching).
- Q: Should the external kubeconfig be passed to Terraform via a temp file (`config_path`) or inline credentials (`host`, `token`, `cluster_ca_certificate`)? → A: Inline credentials in the Terraform provider block.
- Q: Should Radius use AKS admin credentials or user credentials to obtain the kubeconfig? → A: User credentials (`listClusterUserCredential`) with Entra ID (AAD) authentication using the registered Azure service principal or workload identity. Local admin accounts are disabled by default on AKS, so admin credentials are not reliable.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The `providers.kubernetes` model MUST support a new `target` property with allowed values `current` and `external`. When omitted, the default MUST be `current`.
- **FR-002**: The `providers.kubernetes` model MUST support a new `clusterType` property with allowed values `aks` and `eks`. This property MUST only be valid when `target` is `external`.
- **FR-003**: The `providers.kubernetes` model MUST support a new `clusterName` property of type string. This property MUST be required when `clusterType` is `aks` or `eks`.
- **FR-004**: When `target` is `current` or omitted, Radius MUST use the existing kubeconfig resolution logic (in-cluster config or local kubeconfig) with no behavioral change.
- **FR-005**: When `clusterType` is `eks`, Radius MUST use the registered AWS credentials and `providers.aws.region` to dynamically obtain a kubeconfig for the named EKS cluster before recipe execution.
- **FR-006**: When `clusterType` is `aks`, Radius MUST use the registered Azure credentials and `providers.azure.resourceGroupName` to obtain user credentials (`listClusterUserCredential`) for the named AKS cluster, then authenticate via Entra ID (AAD) using the registered service principal or workload identity to obtain an access token. The resulting `host`, `token`, and `cluster_ca_certificate` are used inline.
- **FR-007**: The dynamically-obtained kubeconfig MUST be passed to the Terraform Kubernetes provider as inline credentials (`host`, `token`, `cluster_ca_certificate`) rather than written to a temporary file. No kubeconfig files are written to disk.
- **FR-008**: Terraform state MUST continue to be stored on the local Radius cluster. The Terraform Kubernetes backend configuration MUST NOT use the external cluster's kubeconfig; only the Terraform Kubernetes provider (for deploying resources) uses the external kubeconfig.
- **FR-009**: The dynamically-obtained kubeconfig MUST be used by the Bicep/UCP deployment engine when creating Kubernetes resources.
- **FR-010**: Radius MUST validate environment configuration at create/update time, rejecting invalid property combinations (e.g., `target = external` without `clusterType`, `clusterType` without `clusterName`, `clusterType = eks` without `providers.aws`, `clusterType = aks` without `providers.azure`).
- **FR-011**: When kubeconfig acquisition fails (permissions, cluster not found, network), Radius MUST return a clear, actionable error message identifying the root cause.
- **FR-012**: The `clusterType` and `clusterName` properties MUST be rejected with a validation error if `target` is `current` or omitted.

### Key Entities

- **Environment**: The existing `Radius.Core/environments` resource, extended with new Kubernetes provider properties (`target`, `clusterType`, `clusterName`).
- **Kubernetes Provider Configuration** (`ProvidersKubernetes`): Extended model that determines whether the environment targets the local cluster or an external managed cluster.
- **Kubeconfig**: A dynamically-generated credential artifact obtained at recipe execution time using cloud provider credentials. Not persisted as a Radius resource; generated on-demand.
- **AWS Credential**: Existing UCP credential resource (`/planes/aws/aws/providers/System.AWS/credentials/default`) used to authenticate with AWS and obtain EKS cluster access.
- **Azure Credential**: Existing UCP credential resource (`/planes/azure/azurecloud/providers/System.Azure/credentials/default`) used to authenticate with Azure and obtain AKS cluster access.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Users can deploy a Terraform recipe to an external EKS cluster by configuring only the environment's Kubernetes provider properties — no manual kubeconfig management required.
- **SC-002**: Users can deploy a Terraform recipe to an external AKS cluster by configuring only the environment's Kubernetes provider properties.
- **SC-003**: Existing environments without external cluster properties continue to function identically with zero configuration changes.
- **SC-004**: Invalid environment configurations (missing required properties, incompatible property combinations) are rejected at create/update time with clear error messages within the normal request-response cycle.
- **SC-005**: Users can deploy a Bicep recipe to an external EKS or AKS cluster with the same environment configuration used for Terraform recipes.
- **SC-006**: When kubeconfig acquisition fails, 100% of failure modes produce an error message that identifies the specific cause (permissions, cluster not found, network error).

## Assumptions

- AWS and Azure credentials are already registered via `rad credential register` before configuring an external cluster environment. Radius does not auto-register credentials.
- Only one AWS credential and one Azure credential are supported (named `"default"` per the current design). Multi-credential support is a future enhancement.
- The EKS kubeconfig acquisition follows the same mechanism as `aws eks update-kubeconfig` — using AWS STS to generate a bearer token for the cluster's authentication endpoint.
- The AKS kubeconfig acquisition uses user credentials (`listClusterUserCredential`) combined with Entra ID (AAD) token acquisition using the registered Azure service principal or workload identity. This is equivalent to `az aks get-credentials` followed by `kubelogin convert-kubeconfig --login spn`.
- The target namespace (`providers.kubernetes.namespace`) is expected to already exist on the external cluster. Radius does not auto-create namespaces on external clusters.
- Terraform state is stored on the local Radius cluster, not on the external target cluster. This keeps state management centralized and avoids requiring external cluster credentials for the backend.

## Out of Scope

The following items are explicitly excluded from this specification and planned for future work:

- **Generic external Kubernetes clusters**: Support for non-managed clusters using direct kubeconfig/token/client-certificate authentication (`clusterType: generic`).
- **New Kubernetes credential type**: A UCP credential resource for storing Kubernetes authentication tokens or client certificates.
- **Kubeconfig import CLI command**: `rad credential register kubernetes --from-kubeconfig` for importing kubeconfig contexts.
- **Multi-credential support**: Ability to register multiple credentials per cloud provider (keyed by account ID, subscription ID, or cluster name).
- **Credential reference on environments**: A `credentialRef` property on environments to select which credential to use.
- **Cluster endpoint property**: `providers.kubernetes.clusterEndPoint` for specifying the API server URL directly.
- **Cross-cluster Terraform state management**: Advanced state storage strategies for multi-cluster scenarios.
- **Namespace auto-creation**: Automatically creating the target namespace on external clusters if it does not exist.
- **Direct resource management on external clusters**: Radius resource providers (e.g., `Applications.Core/containers`) continue to target the local cluster only. External cluster targeting applies exclusively to recipe execution.
