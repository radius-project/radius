# Feature Specification: Repo Radius

* **Author**: Zach Casper (@zachcasper)

## Summary

Today, Radius requires users to install Radius on a Kubernetes cluster. That Kubernetes cluster must have identity and access management configured, gateway controllers installed, storage configured, container image registries set up, and other configurations. Only then can Radius be installed. After these are complete, they must all be performed again for the production environment and Radius installed again.

This limits Radius adoption to organizations that have a platform engineering team to perform these tasks. For developers without a platform engineering team, Radius is not a viable option.

Repo Radius is a rethinking of how to deliver Radius to developers. Repo Radius transforms Radius from a heavy-weight persistent control plane into a lightweight on-demand engine. Concretely, Repo Radius enables developers to run Radius within a GitHub Actions runner, with state stored externally in the same GitHub account.

### Top level goals

* Eliminate the prerequisite to have a Kubernetes cluster configured.
* Enable Radius to run ephemerally within a GitHub Actions runner, starting on demand and shutting down after each operation.
* Persist Radius state (application graphs, deployment history, environment configuration) in GitHub-native storage rather than in-cluster databases.
* Support the Copilot app integration in the near team and other UX surfaces in the future.
* Maintain compatibility with the existing Radius resource model and Bicep/Terraform recipe system.

### Non-goals (out of scope)

* Replacing the existing persistent control plane deployment model. Repo Radius is an alternative delivery mechanism, not a replacement.
* Building the frontend UI component such as the Copilot app. Repo Radius must ship with a separate frontend component.
* Supporting non-GitHub source control platforms (GitLab, Bitbucket, Azure DevOps).
* Running Repo Radius outside of GitHub Actions (e.g., locally on a developer's workstation as the primary mode).
* Multi-repository orchestration at the infrastructure layer. Multi-repo applications may use Repo Radius per-repo, but cross-repo state coordination is deferred.


## User profile and challenges

### User persona(s)

The primary user is a **developer** working on a cloud-native application hosted on GitHub. They may be an individual developer, a small team, or part of a larger organization. They do not have a dedicated platform engineering team and may not have deep Kubernetes expertise. They want to deploy their application to AWS or Azure without managing infrastructure.

A secondary user is a **platform engineer** evaluating Radius. Repo Radius provides a low-friction entry point: they can try Radius on a real application without provisioning a Kubernetes cluster, and later migrate to a self-hosted Radius installation if needed.

### Challenge(s) faced by the user

* **High barrier to entry**. Before deploying a single application with Radius today, the developer must provision a Kubernetes cluster, configure IAM, install gateway controllers, set up storage, configure a container registry, and install Radius. This takes hours to days and requires Kubernetes expertise.
* **Ongoing operational burden**. A persistent Radius control plane must be upgraded, monitored, and secured. For a developer who just wants to deploy an application, this overhead is disproportionate to the value.
* **Environment duplication**. Separating production from non-production workloads requires separate Kubernetes clusters, each with its own Radius control plane. This doubles the setup and operational cost.
* **No try-before-you-buy path**. There is no lightweight way to experience Radius's value proposition without committing to the full Kubernetes setup.

### Positive user outcome

The developer, or a solution built on Repo Radius, creates the Radius workflow in the GitHub repository, then deploys their application to AWS or Azure without provisioning or managing any infrastructure beyond their cloud account. Radius runs on demand within GitHub Actions, starts quickly, deploys their application, and shuts down. State is stored in the same GitHub account. If they outgrow Repo Radius, they can migrate to a self-hosted Radius installation.

## Key scenarios

### Scenario 1: Deploy an application from a GitHub repository

The developer asks to deploy their application. Cloud resources are provisioned on AWS or Azure without any prior Kubernetes cluster setup, Radius installation, or infrastructure configuration.

### Scenario 2: Set up a new environment

The developer creates an environment by providing cloud account details (e.g., AWS account ID and region, or Azure subscription and resource group). The environment is stored as a GitHub Environment and reused across subsequent deployments.

### Scenario 3: Authorize cloud access

The developer connects their AWS or Azure account to GitHub using OIDC identity federation so that deployments can provision cloud resources using short-lived tokens rather than stored credentials.

### Scenario 4: Update a deployed application

The developer modifies their application and deploys the change to an existing environment. Repo Radius loads the persisted state from the previous deployment and applies changes incrementally.

### Scenario 5: Migration to self-hosted Radius

A developer who has outgrown Repo Radius can export their application definitions and environment configurations and migrate to a persistent Radius installation on Kubernetes.

## Key dependencies and risks

* **Dependency: External UI**. Repo Radius is designed to be shipped as part of a developer solution which includes an application graph visualization, AI-based modeling of the application, and a user interface to configure environments and visualize deployments. These other components are a dependency and not part of the Repo Radius scope.

* **Risk: Radius startup time**. Radius must start quickly enough within a GitHub Actions runner that the developer experience feels responsive. If startup takes minutes, the experience degrades significantly.

## Key assumptions to test and questions to answer

* **Reading application graph data**. Application graph data must be readable by the frontend without the Radius control plane being available and separate from the Radius control plane state. In the current designs for the application graph, graph data is stored separately from Repo Radius.

* **Storage location**. What is the technical storage medium? The previous assumption used an orphan branch in the Git repository but that has security drawbacks. Are other options within GitHub, such as GHCR, sufficient for all state that Radius needs to persist, or will some state require a different storage mechanism?

* **Real-time deployment status**. How does the frontend receive real-time progress updates during a workflow run? The GitHub REST API does not support streaming job logs while a job is running. This feature spec includes the creation of artifacts after each Radius CLI command is executed, but GitHub Actions does not provide a public API for streaming updates (e.g., resource-by-resource updates from the `rad deploy` command). Is it possible, and if so, what mechanism should the workflow use to provide incremental status to the frontend?

* **Concurrency**. How do we handle two developers triggering deployments to the same environment simultaneously? Git-based locking, GitHub environment protection rules, or something else?

* **Credential lifecycle**. How are cloud provider credentials (OIDC tokens) scoped and rotated within the Repo Radius lifecycle?

## Current state

Work on many Repo Radius components or dependencies is already in flight. This includes:

* **Deploy to External AKS and EKS Clusters**: [Feature spec](https://github.com/radius-project/radius/blob/main/eng/design-notes/environments/2026-05-external-kubernetes.md) is reviewed and merged.

* **Externalize Radius state store**: The [technical design](https://github.com/radius-project/radius/pull/11457) is in draft but is expected to be superseded by a revised technical design.

* **Workflow including OIDC authentication**: A [prototype workflow](https://github.com/radius-project/github-extension/blob/0378b1e349d09fe0f7f09c978c0e7a32c214a72a/.copilot/extensions/radius/src/shared/github-client.ts#L1581) has been implemented which includes OIDC authentication to AWS and Azure. This spec extends the prototype with (1) standardized workflow inputs and outputs and (2) loading of the external Radius state store.

## Details of user problem

When I try to deploy my cloud-native application with Radius, I first have to provision a Kubernetes cluster, configure IAM policies, install ingress controllers, set up persistent storage, configure a container registry, and then install Radius. This process takes hours, requires Kubernetes expertise I may not have, and is error-prone. If I want to separate production from non-production workloads, I need a second cluster and a second Radius installation.

Once Radius is running, I am responsible for keeping the Kubernetes cluster and the Radius control plane operational: upgrading Radius, monitoring its health, backing up its state, and securing it. This is a significant ongoing cost for what I really want, which is just to deploy my application to AWS or Azure.

As a result, I fall back to my existing infrastructure as code solution and do not adopt Radius.

## Desired user experience outcome

After Repo Radius is implemented, a frontend can offer developers a complete deploy-to-cloud experience backed by Radius without requiring the developer to provision any infrastructure. The frontend handles user interaction and intent; Repo Radius handles execution.

### Detailed user experience

Repo Radius is a backend designed to be driven by a separate frontend component (Copilot app, CLI, browser extension, or other integration). The frontend interacts with Repo Radius entirely through GitHub APIs. The following steps describe the interaction model.

#### Step 1: Environment setup (prerequisite)

The frontend creates a GitHub Environment. The environment stores cloud provider details (e.g., AWS account and region, or Azure subscription and resource group) and Kubernetes cluster details (e.g., the EKS or AKS cluster name and Kubernetes namespace) for application workloads.

#### Step 2: OIDC setup (prerequisite)

The frontend guides the user through configuring their cloud provider to trust the GitHub Actions OIDC identity provider as a federated identity source, so that the workflow can authenticate using short-lived tokens rather than stored credentials. For Azure, this means creating a federated credential on an Entra ID app registration that trusts tokens issued by GitHub Actions; the resulting client ID, tenant ID, and subscription ID are stored in the GitHub Environment. For AWS, this means creating an IAM OIDC identity provider and an IAM role with a trust policy that accepts GitHub Actions tokens; the role ARN and region are stored in the GitHub Environment.

#### Step 3: Workflow dispatch

The frontend calls the GitHub API `workflow_dispatch` to initiate a Repo Radius workflow. The workflow accepts the following inputs:

| Input | Required | Description |
| --- | --- | --- |
| `environment` | Yes | The GitHub Environment name to use as the Radius environment. |
| `radius_commands` | Yes | A single `rad` CLI command string or a JSON-encoded array of strings. Each string is a `rad` CLI command, to be executed in order, with the `rad` prefix omitted. |

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

#### Step 4: Workflow execution

The single Repo Radius workflow is executed within a GitHub Actions runner. A prototype of this workflow has been validated. The workflow performs the following stages:

* **Authenticate with cloud providers**. The workflow uses the GitHub Environment's OIDC variables to authenticate. For Azure, it calls `azure/login` with the client ID, tenant ID, and subscription ID. For AWS, it calls `aws-actions/configure-aws-credentials` with the IAM role ARN and region.

* **Load the Radius data store**. The workflow reads the persisted Radius data store from the configured storage location and loads it into the ephemeral control plane. On first run, the data store is empty.

* **Create an ephemeral Radius control plane**. The workflow creates a lightweight k3d cluster on the runner, installs the `rad` CLI, and installs Radius on the k3d cluster. This ephemeral cluster hosts the Radius control plane only; application workloads deploy to the developer's target cluster.

* **Configure cloud provider credentials**. Cloud credentials from the OIDC exchange are injected into the Radius control plane via the standard `rad credential register` commands.

* **Create a Radius environment based on the GitHub Environment**. The workflow creates a Radius resource group and environment with the properties from the GitHub Environment passed as an input to the workflow.

* **Execute `rad` CLI commands**. The workflow iterates over the `radius_commands` input. For each command, the workflow executes the command, captures its output, and uploads the output as a workflow artifact. If a command fails, the workflow stops iteration and proceeds to persist the data store.

* **Persist the Radius data store**. The workflow writes the updated Radius data store back to the configured storage location so that subsequent workflow runs can resume from the current state.

#### Step 5: Read workflow output

The frontend reads the output of the workflow by polling the GitHub Actions API for the workflow run status and downloading artifacts as they become available. Artifacts are available via the API as soon as each upload step completes, so the frontend can retrieve results incrementally while the workflow is still running.

## Key investments

### Investment 1: Deployment to external Kubernetes cluster

Enable Radius to deploy application workloads to an AKS or EKS cluster that is separate from the cluster hosting the Radius control plane. This is required because Repo Radius runs the control plane on an ephemeral k3d cluster within the GitHub Actions runner while deploying to the developer's target cluster.

### Investment 2: Externalization of the Radius data store

Move the Radius data store out of the ephemeral Kubernetes cluster so that state persists across workflow runs. This includes the Radius resource data, application graphs, deployment history, and Terraform state.

### Investment 3: Repo Radius workflow with standardized inputs and outputs

Build the single reusable GitHub Actions workflow that orchestrates Repo Radius operations. The workflow accepts a GitHub Environment name and `rad` CLI commands as inputs, and produces artifacts as outputs. The input and output contract must be stable and well-defined so that multiple frontend components can drive Repo Radius without coupling to its internals.

### Investment 4: Cloud credential integration

Integrate with GitHub's OIDC federation to securely provide AWS and Azure credentials to the Radius control plane without storing long-lived secrets. This includes the workflow's authentication steps, `rad credential register` commands, and injection of session credentials into the Radius pods for Terraform provider access.
