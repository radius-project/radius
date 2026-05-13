# Feature Specification: Deploy to External AKS and EKS Clusters

* **Author**: Zach Casper (@zachcasper)
* **Tracking Issue**: [radius-project/radius#6934](https://github.com/radius-project/radius/issues/6934)

## Summary

Today, Radius can only deploy applications to the Kubernetes cluster on which Radius itself is installed. This feature specification covers removing that constraint for the two most common managed Kubernetes platforms — Amazon EKS and Azure AKS — so that a single Radius installation on a small management cluster can deploy and manage applications on many other clusters.

After this work, a Radius environment can name an external AKS or EKS cluster, and any recipe (Bicep or Terraform) executed against that environment will create its Kubernetes resources on the external cluster, using cloud credentials that Radius already has registered. Support for arbitrary, self-managed Kubernetes clusters is explicitly deferred; a forward-compatible path for it is sketched at the end of this document so the chosen model does not paint Radius into a corner.

### Top level goals

- Decouple "where the Radius control plane runs" from "where applications are deployed" for the most common managed-Kubernetes scenarios (AKS and EKS).
- Reuse the AWS and Azure credentials already registered with Radius — no new credential type, no new persisted resource.
- Make recipe behavior identical across Bicep and Terraform when targeting an external cluster.
- Preserve full backward compatibility for environments that do not name an external cluster.

### Non-goals (out of scope)

- Generic / self-managed / on-premises Kubernetes clusters (k3s, OpenShift, Rancher, GKE, other clouds).
- A new Kubernetes credential resource or `rad credential register kubernetes` command.
- Storing kubeconfig files, service-account tokens, or client certificates inside Radius.
- Auto-creating namespaces on external clusters.
- Direct (non-recipe) resource management on external clusters. Resource providers such as `Applications.Core/containers` continue to operate against the cluster Radius is installed on; only recipe execution is redirected.
- Cross-region or cross-account deployments within a single environment. Each environment remains scoped to a single AWS `{accountId, region}` or Azure `{subscriptionId, resourceGroupName}`.
- Multi-credential support (multiple registered AWS/Azure credentials per Radius installation).

## User profile and challenges

### User persona(s)

The primary user is a **platform engineer** at an organization adopting an internal developer platform. They operate Radius on behalf of multiple application teams. Their organization typically has:

- A small "management" or "tools" Kubernetes cluster operated by the platform team.
- Multiple workload clusters, often segmented by environment (dev, staging, prod), region, business unit, or tenant.
- Existing AWS or Azure cloud accounts with established credential and RBAC practices.

A secondary user is the **application developer** consuming the platform. They do not configure environments directly but rely on the platform engineer's environments to deploy their applications.

### Challenge(s) faced by the user

Radius's current "deploy to my own cluster" model conflates the control plane and the workload plane. This causes practical problems for the platform engineer:

- **Blast radius.** A control plane sharing a cluster with production workloads means an upgrade, a misbehaving recipe, or a noisy neighbor can destabilize both.
- **Multi-environment workflows.** A single Radius installation cannot manage `dev`, `staging`, and `prod` if each lives on its own cluster — Radius must be installed and operated in each one.
- **Multi-tenant platforms.** Internal developer platforms commonly hand each team its own cluster. The platform team cannot offer Radius as a shared service across those clusters today.
- **Operational overhead.** Every additional Radius installation is another control plane to upgrade, monitor, back up, and secure.

Existing offerings do not meet this need: deploying directly with `kubectl`, Helm, or Terraform forces developers to think in terms of clusters and credentials, which is exactly what Radius's environment abstraction is meant to hide.

### Positive user outcome

The platform engineer installs Radius once on a stable management cluster and points environments at the workload clusters they already operate. Application developers continue to deploy to a named environment without knowing or caring which cluster sits behind it. The platform team gets per-environment isolation and per-team cluster boundaries without paying the cost of running multiple Radius installations.

## Key scenarios

### Scenario 1: Deploy to an external EKS cluster

A platform engineer configures a Radius environment with an EKS cluster name on the existing `aws` provider block. Recipes deployed to that environment land on the named EKS cluster using the AWS credentials already registered with Radius.

### Scenario 2: Deploy to an external AKS cluster

A platform engineer configures a Radius environment with an AKS cluster name on the existing `azure` provider block. Recipes deployed to that environment land on the named AKS cluster using the Azure credentials already registered with Radius.

### Scenario 3: Existing single-cluster installations are unaffected

A user with an existing environment that does not name an external cluster continues to deploy to the cluster Radius is installed on, with no configuration changes and no behavioral difference.

### Scenario 4 (future): Deploy to a generic Kubernetes cluster

Out of scope for this feature specification, but the model chosen here must allow a future enhancement that targets self-managed clusters using a Kubernetes-native credential (a referenced Secret containing a token or client certificate). See _Key investments → Future direction_.

## Key dependencies and risks

- **Dependency: existing UCP credential model.** The feature reuses the existing AWS and Azure credentials registered via `rad credential register`. No risk in itself, but it inherits the current single-credential-per-cloud limitation.
- **Dependency: cluster RBAC.** The registered cloud principal must hold both the cloud-side permission to obtain cluster access (`eks:DescribeCluster` for EKS, _Azure Kubernetes Service Cluster User Role_ for AKS) and a Kubernetes RBAC role on the target cluster sufficient to create the resources the recipe defines. Radius cannot grant these on the user's behalf; documentation must make this prerequisite obvious.
- **Dependency: network reachability.** The Radius cluster must be able to reach the external cluster's Kubernetes API server. Private clusters and clusters behind firewalls may require additional networking work that is the user's responsibility.
- **Risk: bearer-token lifetime.** EKS bearer tokens are valid for ~15 minutes. A recipe that runs longer than that could see token expiry mid-execution. Mitigation: obtain a fresh credential per recipe execution; document the limit; treat mid-execution refresh as a future enhancement if real recipes ever exceed it.
- **Risk: future generic-cluster shape locks us in.** If the AKS/EKS shape chosen now is incompatible with a future generic-cluster shape, we will be forced into a breaking change. Mitigation: place cluster identity in the cloud provider blocks (`aws.eksClusterName`, `azure.aksClusterName`) so the `kubernetes` block remains free for a future generic credential reference.
- **Risk: state location.** Terraform state could in principle live on either cluster. Storing it on the external cluster would distribute state and require external credentials for the backend. Mitigation: state stays on the Radius cluster; only the Terraform Kubernetes _provider_ uses external credentials, not the backend.

## Key assumptions to test and questions to answer

- **Assumption:** placing `eksClusterName` under `aws` and `aksClusterName` under `azure` is more intuitive to users than an `external` flag on the `kubernetes` block. Validation: design review feedback on this PR (which has already pushed in this direction).
- **Assumption:** users are willing to grant the registered cloud principal both the cloud RBAC and the Kubernetes RBAC needed for the target cluster, rather than expecting Radius to manage a separate Kubernetes credential. Validation: quickstart user testing.
- **Assumption:** a per-recipe-execution credential acquisition (no caching) is fast enough not to matter in practice. Validation: measure the EKS `DescribeCluster` + STS presign latency and the AKS `ListClusterUserCredentials` + Entra ID token latency once implemented; revisit if either materially affects recipe runtime.

## Current state

Radius today supports configuring an environment with a `providers.kubernetes.namespace` and uses the in-cluster Kubernetes config (when running in-cluster) or the user's local kubeconfig (when running out-of-cluster) to deploy resources. There is no mechanism to redirect that deployment at a different cluster.

A previous iteration of this design proposed a `target` / `clusterType` / `clusterName` triplet under `providers.kubernetes`. Design review on PR [#11644](https://github.com/radius-project/radius/pull/11644) converged on a different shape (cluster identity belongs with the cloud that owns the cluster), which this document reflects.

The tracking issue is [#6934 — Manage applications in multiple environments on separate Kubernetes clusters](https://github.com/radius-project/radius/issues/6934).

## Details of user problem

> When I try to operate Radius for multiple application teams, I have to install Radius on every workload cluster I want to manage. My organization runs separate clusters for dev, staging, and prod, plus per-team clusters for tenant isolation. That means I am running and upgrading a Radius control plane on each of those clusters, even though they are otherwise identical. If a Radius upgrade goes wrong on one of those clusters, it can affect the production workloads running alongside it. I cannot offer "Radius" as a single shared service to my application teams — I can only offer "Radius on _your_ cluster," which defeats much of the value of having a control plane in the first place.

> The Kubernetes clusters I want to deploy to are AKS and EKS clusters that already exist. The cloud credentials needed to access them are credentials I have already registered with Radius for deploying cloud resources via recipes. I should not need a second copy of those credentials, in a different form, just to point Radius at a different cluster.

## Desired user experience outcome

> After this scenario is implemented, I can install Radius once on a small management cluster and configure each Radius environment to point at the AKS or EKS cluster where that environment's workloads should run. My application developers deploy to environments by name without knowing which cluster sits behind each one. I get per-environment cluster isolation, per-team cluster boundaries, and a single control plane to upgrade and operate. As a result, I can offer Radius as a real shared platform service rather than a per-cluster install, and my blast radius for control-plane changes shrinks dramatically.

### Detailed user experience

The mental model is:

> **Cluster identity belongs with the cloud that owns the cluster.**

EKS clusters are AWS resources, so the EKS cluster name lives in the `aws` provider block. AKS clusters are Azure resources, so the AKS cluster name lives in the `azure` provider block. The `kubernetes` block stays focused on the Kubernetes-specific concern: the namespace.

**Step 1 — Existing behavior is unchanged.** An environment that does not name an external cluster deploys to the cluster Radius runs on, exactly as today:

```bicep
resource env 'Radius.Core/environments@2025-08-01-preview' = {
  name: 'my-radius-env'
  properties: {
    providers: {
      kubernetes: {
        namespace: 'my-app'
      }
    }
  }
}
```

**Step 2 — Register cloud credentials with Radius.** This is the same step users perform today to deploy AWS or Azure resources via recipes; no new credential type is introduced.

```bash
# AWS — for an EKS target
rad credential register aws access-key \
  --access-key-id "$AWS_ACCESS_KEY_ID" \
  --secret-access-key "$AWS_SECRET_ACCESS_KEY"

# Azure — for an AKS target
rad credential register azure sp \
  --client-id "$AZURE_CLIENT_ID" \
  --client-secret "$AZURE_CLIENT_SECRET" \
  --tenant-id "$AZURE_TENANT_ID"
```

**Step 3 — Grant the credential access to the target cluster.**

- **EKS:** the AWS principal needs `eks:DescribeCluster` plus a corresponding entry in the cluster's access configuration (EKS access entries or `aws-auth` ConfigMap) mapped to a Kubernetes RBAC role.
- **AKS:** the Azure principal needs the **Azure Kubernetes Service Cluster User Role** plus a Kubernetes RBAC role on the cluster (e.g., **Azure Kubernetes Service RBAC Writer**).

**Step 4 — Create an environment that names the external cluster.** For EKS:

```bicep
resource env 'Radius.Core/environments@2025-08-01-preview' = {
  name: 'my-radius-env'
  properties: {
    providers: {
      aws: {
        accountId: '<AWS_ACCOUNT_ID>'
        region: '<AWS_REGION>'
        eksClusterName: '<EKS_CLUSTER_NAME>'
      }
      kubernetes: {
        namespace: '<KUBERNETES_NAMESPACE>'
      }
    }
  }
}
```

For AKS:

```bicep
resource env 'Radius.Core/environments@2025-08-01-preview' = {
  name: 'my-radius-env'
  properties: {
    providers: {
      azure: {
        subscriptionId: '<SUBSCRIPTION_ID>'
        resourceGroupName: '<RESOURCE_GROUP_NAME>'
        aksClusterName: '<AKS_CLUSTER_NAME>'
      }
      kubernetes: {
        namespace: '<KUBERNETES_NAMESPACE>'
      }
    }
  }
}
```

The equivalent CLI invocations follow the existing `--aws-<field>` / `--azure-<field>` flag convention. For EKS:

```bash
rad env create my-radius-env \
  --aws-account-id <AWS_ACCOUNT_ID> \
  --aws-region <AWS_REGION> \
  --aws-eks-cluster-name <EKS_CLUSTER_NAME> \
  --kubernetes-namespace <KUBERNETES_NAMESPACE>
```

For AKS:

```bash
rad env create my-radius-env \
  --azure-subscription-id <SUBSCRIPTION_ID> \
  --azure-resource-group <RESOURCE_GROUP_NAME> \
  --azure-aks-cluster-name <AKS_CLUSTER_NAME> \
  --kubernetes-namespace <KUBERNETES_NAMESPACE>
```

A single environment is scoped to a single cloud: it may specify a `providers.aws` block or a `providers.azure` block, but not both. As a corollary, at most one external cluster (`aws.eksClusterName` or `azure.aksClusterName`) can be named per environment.

**Step 5 — Deploy.** Recipes deploy to the named external cluster regardless of recipe engine:

```bash
rad deploy app.bicep --environment my-radius-env
```

**Step 6 — Verify.** The user can confirm with `kubectl` against the external cluster:

```bash
aws eks update-kubeconfig --name my-eks-cluster --region us-west-2
# or
az aks get-credentials --resource-group my-rg --name my-aks-cluster

kubectl get all -n my-app
```

**Acceptance scenarios** (expected behavior in success and failure cases):

1. With `aws.eksClusterName` set and valid AWS credentials registered, a recipe that creates a Kubernetes resource creates it on the external EKS cluster in the configured namespace, identically for Bicep and Terraform recipes.
2. With `azure.aksClusterName` set and valid Azure credentials registered, the equivalent holds for AKS, including on AKS clusters with local accounts disabled (the AKS default).
3. With neither external cluster name set, deployment behavior is identical to today.
4. Setting `eksClusterName` without `aws.region` or `aws.accountId` (or `aksClusterName` without `azure.subscriptionId` or `azure.resourceGroupName`) is rejected at environment create/update time with a message naming the missing field.
5. Specifying both an `aws` provider block and an `azure` provider block on the same environment is rejected at create/update time. An environment is scoped to a single cloud.
6. When credential acquisition fails — missing cloud RBAC, cluster not found, or network unreachable — the operation fails with an actionable error identifying the root cause.

**Edge cases:**

- **External cluster temporarily unreachable.** A clear connectivity error rather than a generic failure.
- **Bearer token expires mid-execution.** A fresh credential is obtained per recipe execution; mid-execution refresh is out of scope.
- **Target namespace missing.** A clear error; auto-creation of namespaces on external clusters is out of scope.
- **Both AWS and Azure providers configured.** Rejected at validation. An environment specifies one cloud provider block (`aws` or `azure`), not both.

## Key investments

### Feature 1 — Extend the environment data model

Add an optional `eksClusterName` field to `providers.aws` and an optional `aksClusterName` field to `providers.azure`. The `providers.kubernetes` block remains as it is today. No new credential type, no new persisted resource.

### Feature 2 — Validation and backward compatibility

At environment create/update time, reject:
- `aws.eksClusterName` set without `aws.region` or `aws.accountId`.
- `azure.aksClusterName` set without `azure.subscriptionId` or `azure.resourceGroupName`.
- Both `providers.aws` and `providers.azure` set on the same environment.

When neither external cluster name is set, all existing kubeconfig resolution behavior is preserved unchanged.

### Feature 3 — On-demand Kubernetes credential acquisition

Before each recipe execution against an environment that names an external cluster, Radius obtains Kubernetes API access for that cluster using the registered cloud credential. Credentials are held only in memory for the duration of the execution and are never persisted. The flow must work on AKS clusters with local accounts disabled.

### Feature 4 — Recipe engine integration

Both the Bicep and Terraform recipe engines must use the on-demand external credential when one is available, with identical user-visible behavior. Terraform state continues to live on the cluster Radius is installed on; only the Terraform Kubernetes _provider_ targets the external cluster.

### Feature 5 — Direct resource management remains local

Resource providers such as `Applications.Core/*` continue to target the cluster Radius is installed on. External-cluster targeting is scoped to recipe execution. This boundary keeps the change tractable and avoids changing every resource provider in this single feature.

### Feature 6 — Documentation and quickstart

A quickstart that walks a user from "I have an EKS or AKS cluster and AWS/Azure credentials" through registering the credential, granting the necessary cloud and Kubernetes RBAC, creating an environment, and deploying a recipe.

### Future direction — Generic Kubernetes clusters

Not part of this feature specification, but the model is forward-compatible with a future enhancement for self-managed clusters. That future shape extends the `kubernetes` provider block without altering the AKS/EKS shapes defined here:

```bicep
resource env 'Radius.Core/environments@2025-08-01-preview' = {
  name: 'my-radius-env'
  properties: {
    providers: {
      kubernetes: {
        apiServerUrl: 'https://kubernetes.default.svc.cluster.local'
        namespace: '<KUBERNETES_NAMESPACE>'
        secretName: '<SECRET_NAME>'
      }
    }
  }
}
```

In that future shape, `apiServerUrl` identifies the cluster's API endpoint and `secretName` references a Kubernetes Secret in the Radius installation's namespace containing the credential — typically a service-account token or a client certificate plus the cluster CA. This pattern keeps Kubernetes credentials in the place users already manage them (Kubernetes Secrets) rather than introducing a Radius-specific credential store. A subsequent enhancement may add a CLI affordance such as `rad credential register kubernetes --from-kubeconfig` to materialize that Secret from a user's kubeconfig context.

A further enhancement could elevate `secretName` to a first-class Radius credential resource by adding a new credential type, `kubernetes`, alongside the existing `aws` and `azure` types. The user would register a Kubernetes credential once with `rad credential register kubernetes ...` (backed by a Kubernetes Secret managed by Radius) and reference it from environments by name, the same way AWS and Azure credentials are registered and reused today. The `secretName` field shown above is the minimum-viable, Kubernetes-native form of that idea; promoting it to a Radius credential type is an additive evolution that does not require revisiting the environment shape.

The work in the present topic, placing `eksClusterName` under `aws` and `aksClusterName` under `azure` and leaving the `kubernetes` block focused on Kubernetes-specific concerns, is the minimum change required to support managed clusters today while leaving room for this generic-cluster shape later.
