# Feature Specification: Repo Radius

* **Author**: Zach Casper (@zachcasper)

## Summary

Today, Radius requires users to install Radius on a Kubernetes cluster. That Kubernetes cluster must have identity and access management configured, gateway controllers installed, storage configured, container image registries set up, and other configurations. Only then can Radius be installed. After these are complete, they must all be performed again for the production environment and Radius installed again.

This limits Radius adoption to organizations that have a platform engineering team to perform these tasks. For developers without a platform engineering team, Radius is not a viable option.

Repo Radius is a rethinking of how to deliver Radius to developers. Repo Radius transforms Radius from a heavy-weight persistent control plane into a lightweight on-demand engine. Concretely, Repo Radius enables developers to run Radius within a GitHub Actions runner with the state stored externally, within the same GitHub account.

### Top level goals

* Eliminate the prerequisite to have a Kubernetes cluster configured.
* Enable Radius to run ephemerally within a GitHub Actions runner, starting on demand and shutting down after each operation.
* Persist Radius state (application graphs, deployment history, environment configuration) in GitHub-native storage rather than in-cluster databases.
* Support multiple UX surfaces (Copilot app, CLI, browser extension, third-party integrations), with Copilot app integration as the top priority.
* Maintain compatibility with the existing Radius resource model and Bicep/Terraform recipe system.

### Non-goals (out of scope)

* Replacing the existing persistent control plane deployment model. Repo Radius is an alternative delivery mechanism, not a replacement.
* Supporting non-GitHub source control platforms (GitLab, Bitbucket, Azure DevOps).
* Running Repo Radius outside of GitHub Actions (e.g., locally on a developer's workstation as the primary mode).
* Multi-repository orchestration at the infrastructure layer. Multi-repo applications (User Story #14) may use Repo Radius per-repo, but cross-repo state coordination is deferred.
* Building the Copilot agent, browser extension, or Deployment panel UI. These are separate components that consume Repo Radius capabilities.

## User profile and challenges

### User persona(s)

The primary user is a **developer** working on a cloud-native application hosted on GitHub. They may be an individual developer, a small team, or part of a larger organization. They do not have a dedicated platform engineering team and may not have deep Kubernetes expertise. They want to deploy their application to AWS or Azure without managing infrastructure.

A secondary user is a **platform engineer** evaluating Radius. Repo Radius provides a low-friction entry point: they can try Radius on a real application without provisioning a Kubernetes cluster, and later migrate to a self-hosted Radius installation if needed (User Story #38).

### Challenge(s) faced by the user

* **High barrier to entry.** Before deploying a single application with Radius today, the developer must provision a Kubernetes cluster, configure IAM, install gateway controllers, set up storage, configure a container registry, and install Radius. This takes hours to days and requires Kubernetes expertise.
* **Ongoing operational burden.** A persistent Radius control plane must be upgraded, monitored, and secured. For a developer who just wants to deploy an application, this overhead is disproportionate to the value.
* **Environment duplication.** Separating production from non-production workloads requires separate Kubernetes clusters, each with its own Radius control plane. This doubles the setup and operational cost.
* **No try-before-you-buy path.** There is no lightweight way to experience Radius's value proposition without committing to the full Kubernetes setup.

### Positive user outcome

The developer enables Lattice on their GitHub repository and deploys their application to AWS or Azure without provisioning or managing any infrastructure beyond their cloud account. Radius runs on demand within GitHub Actions, starts quickly, deploys their application, and shuts down. State is stored alongside their code in the same GitHub repository. If they outgrow Repo Radius, they can migrate to a self-hosted Radius installation.

## Key scenarios

### Scenario 1: Deploy an application from a GitHub repository

The developer asks to deploy their application. Cloud resources are provisioned on AWS or Azure without any prior Kubernetes cluster setup, Radius installation, or infrastructure configuration.

### Scenario 2: Set up a new environment

The developer creates an environment by providing cloud account details (e.g., AWS account ID and region, or Azure subscription and resource group). The environment is stored as a GitHub Environment and reused across subsequent deployments.

### Scenario 3: Authorize cloud access

The developer connects their AWS or Azure account to GitHub using OIDC identity federation so that deployments can provision cloud resources using short-lived tokens rather than stored credentials.

### Scenario 4: Resume after a previous deployment

The developer triggers a new deployment days or weeks after the last one. Repo Radius picks up where it left off, loading previously persisted state, without requiring the developer to reconfigure anything.

### Scenario 5: Migration to self-hosted Radius

A developer who has outgrown Repo Radius can export their application definitions and environment configurations and migrate to a persistent Radius installation on Kubernetes.

## Key dependencies

* **External UI** - Repo Radius is designed to be embedded in a developer solution which includes an application graph visualization, AI-based modeling of the application, and a user interface to configure environments and visualize deployments. Repo Radius must ship with a separate UX component.
* **Customizable Terraform backend** - Radius currently stores Terraform state in a Kubernetes secret on the host cluster. Since the Repo Radius cluster is ephemeral, Terraform state must be stored in a persistent external backend. This depends on the Terraform and Bicep Settings feature specification.

## Key risks

* **Radius startup time** - Radius must start quickly enough within a GitHub Actions runner that the developer experience feels responsive. If startup takes minutes, the experience degrades significantly.
* **State consistency** - Persisting state to orphan branches and repository variables introduces eventual consistency challenges. Concurrent deployments to the same environment could cause conflicts.

## Key assumptions to test and questions to answer

* **Reading the Radius data store.** The UX component needs to read application graph data from the Radius data store (e.g., to render a graph visualization or show resource properties). The persisted data store format may not be easily consumable by a Copilot skill or browser extension. The data store format may need to be adapted to support direct reads by UX components.
* **Storage location.** What is the technical storage medium? The previous assumption was used an orphan branch in the Git repository but that has security drawbacks. Are other options within GitHub, such as GHCR, sufficient for all state that Radius needs to persist, or will some state require a different storage mechanism?
* **Concurrency.** How do we handle two developers triggering deployments to the same environment simultaneously? Git-based locking, GitHub environment protection rules, or something else?
* **Credential lifecycle.** How are cloud provider credentials (OIDC tokens) scoped and rotated within the Repo Radius lifecycle?

## Current state

Radius currently requires a Kubernetes cluster with a persistent installation. The following related work provides building blocks for Repo Radius:

* **[Feature Specification: Deploy to External AKS and EKS Clusters](https://github.com/radius-project/radius/blob/main/eng/design-notes/environments/2026-05-external-kubernetes.md)**: Describes the ability to deploy to an AKS or EKS cluster not hosting Radius. Required by Repo Radius since the Kubernetes cluster is now ephemeral.
* **[Terraform and Bicep Settings Feature Specification](https://github.com/radius-project/design-notes/blob/main/features/2025-08-14-terraform-bicep-settings.md)**: Describes the ability to store the Terraform backend (state store) outside of Kubernetes. Required by Repo Radius since the Kubernetes cluster is now ephemeral.
  * Many of this feature spec's functionality has been implemented in Radius, however, the customizable Terraform backend **has not**. This must be implemented as part of Repo Radius.
* **[GitHub Actions Workspace with Git-Backed State Persistence](https://github.com/radius-project/radius/pull/11457)**: Draft technical design for storing the Radius data store within an orphaned Git branch. Design is likely being modified due to security drawbacks identified.

## Details of user problem

When I try to deploy my cloud-native application with Radius, I first have to provision a Kubernetes cluster, configure IAM policies, install ingress controllers, set up persistent storage, configure a container registry, and then install Radius. This process takes hours, requires Kubernetes expertise I may not have, and is error-prone. If I want to separate production from non-production workloads, I need a second cluster and a second Radius installation.

Once Radius is running, I am responsible for keeping the Kubernetes cluster and the Radius control plane operational: upgrading Radius, monitoring its health, backing up its state, and securing it. This is a significant ongoing cost for what I really want, which is just to deploy my application to AWS or Azure.

As a result, I either give up on Radius entirely and go back to manually configuring cloud resources, or I wait until my organization hires a platform engineering team, which may never happen.

## Desired user experience outcome

After Repo Radius is implemented, a UX component can offer developers a complete deploy-to-cloud experience backed by Radius without requiring the developer to provision any infrastructure. The UX component handles user interaction and intent; Repo Radius handles execution.

### Detailed user experience

Repo Radius is a backend designed to be driven by an external UX component (Copilot app, CLI, browser extension, or third-party integration). The UX component interacts with Repo Radius entirely through GitHub APIs. The following steps describe the interaction model.

#### Step 1: Environment setup (prerequisite)

The UX component uses the GitHub API to create a GitHub Environment. This GitHub Environment becomes the Radius environment when running in Repo Radius mode. The GitHub Environment must specify the following properties as environment variables, corresponding to the properties on `Radius.Core/environments`:

**For AWS:**

| Variable | Description |
| --- | --- |
| `AWS_ACCOUNT_ID` | The AWS account ID |
| `AWS_REGION` | The AWS region (e.g., `us-east-1`) |
| `EKS_CLUSTER_NAME` | The EKS cluster name |
| `KUBERNETES_NAMESPACE` | The Kubernetes namespace for application workloads |

**For Azure:**

| Variable | Description |
| --- | --- |
| `AZURE_SUBSCRIPTION_ID` | The Azure subscription ID |
| `AZURE_RESOURCE_GROUP` | The Azure resource group |
| `AKS_CLUSTER_NAME` | The AKS cluster name |
| `KUBERNETES_NAMESPACE` | The Kubernetes namespace for application workloads |

#### Step 2: OIDC setup (prerequisite)

The UX component guides the user through configuring OIDC federated identity with their cloud provider and updates the GitHub Environment with the required environment variables.

**For Azure:**

| Variable | Description |
| --- | --- |
| `AZURE_CLIENT_ID` | The Azure AD application (client) ID |
| `AZURE_TENANT_ID` | The Azure AD tenant ID |

**For AWS:**

| Variable | Description |
| --- | --- |
| `AWS_IAM_ROLE_ARN` | The IAM role ARN for OIDC federation |

#### Step 3: Workflow dispatch

The UX component calls the GitHub API `workflow_dispatch` to initiate a Repo Radius workflow. The workflow accepts a GitHub Environment name and one or more `rad` CLI commands.

**Single command:**

```json
POST /repos/{owner}/{repo}/actions/workflows/radius/dispatches
{
  "ref": "main",
  "inputs": {
    "environment": "production",
    "radius_commands": "deploy app.bicep"
  }
}
```

**Multiple commands:**

```json
POST /repos/{owner}/{repo}/actions/workflows/radius/dispatches
{
  "ref": "main",
  "inputs": {
    "environment": "production",
    "radius_commands": "[\"deploy app.bicep\", \"app graph\"]"
  }
}
```

The `radius_commands` input accepts either a single command string or a JSON-encoded array of strings. Each string is a `rad` CLI command with the `rad` prefix omitted.

#### Step 4: Workflow execution

The single Repo Radius workflow is executed within a GitHub Actions runner. A prototype of this workflow has been validated. The workflow performs the following stages:

* **Authenticate with cloud providers.** The workflow uses the GitHub Environment's OIDC variables to authenticate. For Azure, it calls `azure/login` with the client ID, tenant ID, and subscription ID. For AWS, it calls `aws-actions/configure-aws-credentials` with the IAM role ARN and region.

* **Load the Radius data store.** The workflow reads the persisted Radius data store from the configured storage location and loads it into the ephemeral control plane. On first run, the data store is empty.

* **Create an ephemeral Radius control plane.** The workflow creates a lightweight k3d cluster on the runner, installs the `rad` CLI, and installs Radius on the k3d cluster. This ephemeral cluster hosts the Radius control plane only; application workloads deploy to the developer's target cluster.

* **Configure cloud provider credentials.** Cloud credentials from the OIDC exchange are injected into the Radius control plane via the standard `rad credential register` commands.

* **Configure the Terraform backend.** The workflow configures a persistent Terraform backend outside of the ephemeral cluster so that Terraform state survives across workflow runs. The backend is defined in a `terraform.bicep` file in the `.radius` directory and applied via a `rad deploy terraform.bicep` command included in the `radius_commands` input.

* **Create a Radius environment based on the GitHub Environment.** The workflow creates a Radius resource group and environment with the properties from the GitHub Environment passed as an input to the workflow.

* **Execute `rad` CLI commands.** The workflow runs each command from the `radius_commands` input (e.g., `rad deploy app.bicep`). The workflow provides structured updates for each step in the workflow log. The output of each command is captured as a GitHub Actions workflow artifact for post-execution consumption by the UX component.

* **Persist the Radius data store.** The workflow writes the updated Radius data store back to the configured storage location so that subsequent workflow runs can resume from the current state. 

## Key investments

### Investment 1: Deployment to external Kubernetes cluster

Enable Radius to deploy application workloads to an AKS or EKS cluster that is separate from the cluster hosting the Radius control plane. This is required because Repo Radius runs the control plane on an ephemeral k3d cluster within the GitHub Actions runner while deploying to the developer's target cluster.

### Investment 2: Externalization of the Radius data store

Move the Radius data store out of the ephemeral Kubernetes cluster so that state persists across workflow runs. This includes the Radius resource data, application graphs, and deployment history. The storage mechanism must be accessible from GitHub Actions and readable by UX components.

### Investment 3: Repo Radius workflow with standardized inputs and outputs

Build the single reusable GitHub Actions workflow that orchestrates Repo Radius operations. The workflow accepts a GitHub Environment name and `rad` CLI commands as inputs, and produces structured workflow logs and artifacts as outputs. The input and output contract must be stable and well-defined so that multiple UX components can drive Repo Radius without coupling to its internals.

### Investment 4: Cloud credential integration

Integrate with GitHub's OIDC federation to securely provide AWS and Azure credentials to the Radius control plane without storing long-lived secrets. This includes the workflow's authentication steps, `rad credential register` commands, and injection of session credentials into the Radius pods for Terraform provider access.

### Investment 5: Customizable Terraform backend

Enable Radius to store Terraform state in a persistent backend outside of the ephemeral Kubernetes cluster. The backend configuration is defined in a `terraform.bicep` file in the `.radius` directory and applied as part of the workflow's `rad` CLI commands. This depends on the Terraform and Bicep Settings feature specification.
