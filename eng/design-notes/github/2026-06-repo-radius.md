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
* Support the Copilot app integration in the near term and other UX surfaces in the future.
* Reuse the existing self-hosted Radius implementation with minimal to no GitHub-specific modifications.

### Non-goals (out of scope)

* Replacing the existing persistent control plane deployment model. Repo Radius is an alternative delivery mechanism, not a replacement.
* Building the frontend UI component such as the Copilot app. Repo Radius must ship with a separate frontend component.
* Supporting non-GitHub source control platforms (GitLab, Bitbucket, Azure DevOps).
* Running Repo Radius outside of GitHub Actions (e.g., locally on a developer's workstation as the primary mode).
* Supporting multi-repo applications is deferred to a later date.
* Customizing the recipe pack is deferred to a later date. The initial release uses the default AWS and Azure recipe pack that ships with Radius.
* Creating mature, production-grade recipe packs for AWS and Azure. Repo Radius depends on the default recipe pack that ships with Radius; hardening and expanding that recipe pack is separate work outside the scope of Repo Radius.


## User profile and challenges

### User persona(s)

The primary user is a developer working on a cloud-native application hosted on GitHub. They may be an individual developer, a small team, or part of a larger organization. They do not have a dedicated platform engineering team and likely do not have deep Kubernetes expertise. They want to deploy their application to AWS or Azure without managing infrastructure.

A secondary user is a platform engineer evaluating Radius. Repo Radius provides a low-friction entry point: they can try Radius on a real application without provisioning a Kubernetes cluster and later migrate to a self-hosted Radius installation if needed.

### Challenge(s) faced by the user

* **High barrier to entry**. Before deploying a single application with Radius today, the developer must provision a Kubernetes cluster, configure IAM, install gateway controllers, set up storage, configure a container registry, and install Radius. This takes hours to days and requires Kubernetes expertise.
* **Ongoing operational burden**. A persistent Radius control plane must be upgraded, monitored, and secured. For a developer who just wants to deploy an application, this overhead is disproportionate to the value.
* **Environment duplication**. Separating production from non-production workloads requires separate Kubernetes clusters, each with its own Radius control plane. This doubles the setup and operational cost.
* **No try-before-you-buy path**. There is no lightweight way to experience Radius's value proposition without committing to the full Kubernetes setup.

### Positive user outcome

A developer, using a solution built using Repo Radius, can deploy their application to AWS or Azure without having to install Kubernetes or Radius. They can provide their cloud account details and Repo Radius will provision the required infrastructure for their application. Radius runs on demand within GitHub Actions, starts quickly, deploys their application, and shuts down. The state is stored in the same GitHub account. If they outgrow Repo Radius, they can migrate to a self-hosted Radius installation.

## Key scenarios

### Scenario 1: Set up a new environment

The developer creates an environment by providing cloud account details (e.g., AWS account ID and region, or Azure subscription and resource group). The environment is stored as a GitHub Environment and reused across subsequent deployments.

### Scenario 2: Authorize cloud access

The developer connects their AWS or Azure account to GitHub using OIDC identity federation so that deployments can provision cloud resources using short-lived tokens rather than stored credentials.

### Scenario 3: Deploy an application from a GitHub repository

The developer asks to deploy their application. Cloud resources are provisioned on AWS or Azure without any prior Kubernetes cluster setup, Radius installation, or infrastructure configuration.

### Scenario 4: Update a deployed application

The developer modifies their application and deploys the change to an existing environment. Repo Radius loads the persisted state from the previous deployment and applies changes incrementally.

### Scenario 5: Delete a deployed application

The developer asks to delete their application. Repo Radius loads the persisted state, deletes the application and its associated cloud resources, and updates the persisted state to reflect the removal.

### Scenario 6: Retrieve the application graph

The developer asks to see the application graph for a deployed application including the resources it comprises, their IDs, and the connections between them. Repo Radius loads the persisted state and returns the JSON of the deployed application graph view without deploying or modifying any resources.

### Scenario 7: Run a `rad` CLI command

The developer (or the frontend on their behalf) needs an operation that does not have a dedicated action, such as listing resources or showing application details. Repo Radius runs the requested `rad` CLI command against the persisted state and returns its output.

### Scenario 8: Use an updated version of Repo Radius

Repo Radius is published as a set of versioned GitHub Actions (e.g., `radius-project/deploy@v1`). The developer (or the frontend on their behalf) references a major version tag such as `@v1` to receive backward-compatible updates automatically, and moves to the next major tag for breaking changes (`@v1`, `@v2` is the standard GitHub Actions semantic versioning convention). Because there is no persistent control plane, the developer never performs a manual upgrade; the next deployment simply runs the referenced version.

## Key dependencies and risks

* **Dependency: External UI**. Repo Radius is designed to be shipped as part of a developer solution which includes an application graph visualization, AI-based modeling of the application, and a user interface to configure environments and visualize deployments. These other components are a dependency and not part of the Repo Radius scope.

* **Risk: Radius startup time**. Radius must start quickly enough within a GitHub Actions runner that the developer experience feels responsive. If startup takes minutes, the experience degrades significantly.

* **Risk: Broad cloud permissions and blast radius**. The IAM role (AWS) or app registration (Azure) assumed via OIDC must hold a significant number of permissions to create and destroy the full range of resources an application may require. A role this broad has a high blast radius: a misbehaving recipe, a compromised workflow, or an over-scoped command could create, modify, or destroy a wide range of cloud resources. Scoping these permissions to the minimum necessary, and isolating them per environment, is an important mitigation to design for.

* **Risk: Recipe and recipe pack backward compatibility**. Because Repo Radius is stateless and provisions on demand, every deployment re-resolves the application's resources against whatever recipe pack version is in effect at that time. Once an application is deployed, it must be assumed that there are live cloud resources backed by a recipe. Recipe and recipe pack versions must therefore be tightly controlled so that a newer recipe version remains backward-compatible with resources provisioned by an earlier version; a breaking recipe change could fail to reconcile, orphan, or inadvertently destroy resources that are already deployed. Recipe and recipe pack versioning needs the same backward-compatibility discipline as the action contract itself.

## Details of user problem

When I try to deploy my cloud-native application with Radius, I first have to provision a Kubernetes cluster, configure IAM policies, install ingress controllers, set up persistent storage, configure a container registry, and then install Radius. This process takes hours, requires Kubernetes expertise I may not have, and is error-prone. If I want to separate production from non-production workloads, I need a second cluster and a second Radius installation.

Once Radius is running, I am responsible for keeping the Kubernetes cluster and the Radius control plane operational: upgrading Radius, monitoring its health, backing up its state, and securing it. This is a significant ongoing cost for what I really want, which is just to deploy my application to AWS or Azure.

As a result, I fall back to my existing infrastructure as code solution and do not adopt Radius.

## Desired user experience outcome

After Repo Radius is implemented, a frontend can offer developers a complete deploy-to-cloud experience backed by Radius without requiring the developer to provision any infrastructure. The frontend handles user interaction and intent; Repo Radius handles execution.

## Detailed user experience

### Scenario 1-2: Set up a new environment and authorize cloud access

#### Step 1: Enable Repo Radius for the repository

A workflow file in the repository's `.github/workflows/` directory is required for GitHub Actions to run; an action cannot be invoked without one. The frontend therefore writes one or more lightweight workflows to the repository on the user's behalf. These workflows are intentionally thin: they contain no Repo Radius logic and simply invoke the published Repo Radius GitHub Actions. All implementation detail lives within the published actions, so a committed workflow only needs to change when the repository adopts a new major version of an action (see Scenario 8).

Repo Radius is published as the following GitHub Actions, each pinned to a matching major version (e.g., `@v1`):

| Workflow | Action | Purpose |
| --- | --- | --- |
| `radius-verify-cloud-auth.yml` | `radius-project/verify-cloud-auth` | Verify that the GitHub Environment's OIDC configuration can authenticate to the cloud provider before attempting a deployment. |
| `radius-deploy.yml` | `radius-project/deploy` | Deploy or update an application to the environment. |
| `radius-destroy.yml` | `radius-project/destroy` | Delete an application and its associated cloud resources from the environment. |
| `radius-app-graph.yml` | `radius-project/app-graph` | Retrieve the application graph, including the resources an application comprises and the connections between them. This is a read-only operation. |
| `radius-run-rad-command.yml` | `radius-project/run-rad-command` | Execute arbitrary `rad` CLI commands. This is the general-purpose primitive for any operation that does not have a dedicated action. |

#### Step 2: Environment setup

The frontend creates a GitHub Environment. The environment stores cloud provider details (e.g., AWS account and region, or Azure subscription and resource group) and Kubernetes cluster details (e.g., the EKS or AKS cluster name and Kubernetes namespace) for application workloads.

The environment stores the following cloud provider and cluster details:

| Variable | Cloud | Required | Description |
| --- | --- | --- | --- |
| `AWS_ACCOUNT_ID` | AWS | Yes (AWS) | AWS account ID where cloud resources are provisioned. |
| `AWS_EKS_CLUSTER_NAME` | AWS | Yes (AWS) | Name of the target EKS cluster for application workloads. |
| `AWS_REGION` | AWS | Yes (AWS) | AWS region where cloud resources are provisioned. |
| `AZURE_AKS_CLUSTER_NAME` | Azure | Yes (Azure) | Name of the target AKS cluster for application workloads. |
| `AZURE_RESOURCE_GROUP` | Azure | Yes (Azure) | Resource group for provisioned cloud resources. |
| `AZURE_SUBSCRIPTION_ID` | Azure | Yes (Azure) | Subscription ID where cloud resources are provisioned. |
| `KUBERNETES_NAMESPACE` | Both | No | Kubernetes namespace for application workloads. Defaults to `default` if not specified. |

While the frontend typically creates the GitHub Environment on the user's behalf, the user can also create it directly using the `gh` CLI or the GitHub REST API. For example, using the `gh` CLI to create `dev`, `test`, and `prod` environments and set their variables:

```bash
for env in dev test prod; do
  # Create the environment
  gh api --method PUT repos/{owner}/{repo}/environments/$env

  # Set the cloud provider and cluster variables
  gh variable set AWS_ACCOUNT_ID --env $env --body "123456789012"
  gh variable set AWS_REGION --env $env --body "us-west-2"
  gh variable set AWS_EKS_CLUSTER_NAME --env $env --body "my-eks-cluster"
  gh variable set KUBERNETES_NAMESPACE --env $env --body "my-app-$env"
done
```

If GitHub offers a more declarative approach to creating and configuring environments (for example, a configuration file checked into the repository), that approach can be used instead of the imperative `gh` CLI commands shown above.

#### Step 3: OIDC setup

The frontend guides the user through configuring their cloud provider to trust the GitHub Actions OIDC identity provider as a federated identity source, so that the workflow can authenticate using short-lived tokens rather than stored credentials. For Azure, this means creating a federated credential on an Entra ID app registration that trusts tokens issued by GitHub Actions; the resulting client ID, tenant ID, and subscription ID are stored in the GitHub Environment. For AWS, this means creating an IAM OIDC identity provider and an IAM role with a trust policy that accepts GitHub Actions tokens; the role ARN and region are stored in the GitHub Environment.

The environment stores the following authentication details:

| Variable | Cloud | Required | Description |
| --- | --- | --- | --- |
| `AZURE_CLIENT_ID` | Azure | Yes (Azure) | Client ID of the Entra ID app registration used for OIDC federation. |
| `AZURE_TENANT_ID` | Azure | Yes (Azure) | Entra ID tenant ID. |
| `AWS_ROLE_ARN` | AWS | Yes (AWS) | ARN of the IAM role assumed via OIDC federation. |

#### Step 4: Verify cloud authentication

Before the environment is considered ready, the frontend dispatches the `radius-verify-cloud-auth.yml` workflow (which invokes the `radius-project/verify-cloud-auth` action) for the target GitHub Environment. This confirms that the environment's variables and OIDC configuration are complete and correct, so that misconfiguration is surfaced during setup rather than on the first deployment.

The verification performs an end-to-end authentication check: it requests a GitHub Actions OIDC token, exchanges it with the cloud provider, and confirms that the resulting short-lived credentials are valid. It does not provision, modify, or delete any resources.

When an EKS or AKS cluster name is provided in the environment, the verification additionally confirms Kubernetes connectivity: it uses the authenticated cloud credentials to retrieve the cluster's access configuration and confirms that the cluster's API server is reachable. This catches a misnamed cluster, a cluster in a different region or resource group, or an identity that lacks access to the cluster before the first deployment.

The outcome is reported in two complementary ways, consistent with how all Repo Radius workflows return results:

* **Workflow run conclusion**. The GitHub Actions run concludes as success or failure, giving the frontend a coarse pass/fail signal it can read directly from the run status via the GitHub Actions API without downloading any files.
* **Result artifact**. The workflow uploads a structured result artifact (a small JSON document) containing the specific outcome from the table below, a human-readable message, and remediation guidance. The frontend downloads this artifact to distinguish between the failure modes (for example, "trust not established" versus "cluster unreachable") and to surface actionable guidance to the user.

The workflow reports one of the following outcomes:

| Outcome | Meaning | Action Exit Code | Example artifact contents |
| --- | --- | --- | --- |
| Success | Authentication succeeded and the credentials are valid. | 0 | `{"outcome": "success", "message": "Authentication succeeded. Environment is ready for deployments."}` |
| Missing configuration | A required variable for the selected cloud provider is absent. | 1 | `{"outcome": "missing_configuration", "message": "Required variable AWS_ROLE_ARN is not set. Re-run environment and OIDC setup to populate the environment."}` |
| Trust not established | The cloud provider rejected the GitHub OIDC token. | 2 | `{"outcome": "trust_not_established", "message": "Cloud provider rejected the GitHub OIDC token. Re-run environment and OIDC setup to correct or verify the IAM role trust policy or app registration federated credential matches the repository, environment, and subject claim."}` |
| Insufficient permissions | Authentication succeeded but the identity lacks permission to perform the check. | 3 | `{"outcome": "insufficient_permissions", "message": "Authenticated successfully, but the AWS IAM role or Azure app registration is missing the required permissions. Re-run the environment and OIDC setup or manually add the missing permission. See the Radius documentation for required permissions."}` |
| Cluster unreachable | A cluster name was provided, but the cluster could not be found or its API server could not be reached. | 4 | `{"outcome": "cluster_unreachable", "message": "Cluster my-eks-cluster could not be found or its API server is unreachable. Correct the EKS cluster name, or grant the identity access to the cluster."}` |
| Provider unreachable | The cloud provider's authentication endpoint could not be reached. | 5 | `{"outcome": "provider_unreachable", "message": "Cloud provider authentication endpoint could not be reached. Re-run the verification workflow to try again. If it persists, check the provider's service status."}` |

The result artifact is uploaded under a well-known, stable artifact name (for example, `verify-result`) and is available to the frontend via the GitHub Actions API. The artifact name and the JSON schema (the set of fields and the allowed `outcome` values) form part of the stable contract between Repo Radius and any frontend, so they must remain backward-compatible within a major version.

### Scenario 3-4: Deploy a new or updated application

The frontend deploys an application by dispatching the `radius-deploy.yml` workflow (which invokes the `radius-project/deploy` action) for the target GitHub Environment. The workflow accepts the following inputs:

| Input | Required | Description |
| --- | --- | --- |
| `environment` | Yes | The GitHub Environment name to use as the Radius environment. |
| `app_definition` | Yes | Path, relative to the repository root, to the application definition (for example, a Bicep file) to deploy. |

Example dispatch:

```json
POST /repos/{owner}/{repo}/actions/workflows/radius-deploy.yml/dispatches
{
  "ref": "main",
  "inputs": {
    "environment": "production",
    "app_definition": ".radius/app.bicep"
  }
}
```

The `radius-deploy.yml` workflow contains only one action calling the `radius-project/deploy@v1` GitHub Action which is published to the GitHub Marketplace. The `radius-project/deploy` action performs the following:

1. **Authenticate with cloud providers**. The workflow uses the GitHub Environment's OIDC variables to authenticate. For Azure, it calls `azure/login` with the client ID, tenant ID, and subscription ID. For AWS, it calls `aws-actions/configure-aws-credentials` with the IAM role ARN and region.

1. **Load the Radius data store**. The workflow reads the persisted Radius data store from the configured storage location and loads it into the ephemeral control plane. On first run, the data store is empty.

1. **Create an ephemeral Radius control plane**. The workflow creates a lightweight k3d cluster on the runner, installs the `rad` CLI, and installs Radius on the k3d cluster. This ephemeral cluster hosts the Radius control plane only; application workloads deploy to the developer's target cluster.

1. **Create a Radius environment based on the GitHub Environment**. The workflow creates a Radius resource group and environment with the properties from the GitHub Environment passed as an input to the workflow.

1. **Register the default recipe pack**. The workflow registers the default AWS and Azure recipe pack that ships with Radius to the environment, so the application's resources resolve to recipes without the user having to author or register recipes themselves. Customizing the recipe pack is a future feature; the initial release uses the default recipe pack only.

1. **Execute `rad deploy`**. The workflow runs `rad deploy "${{ inputs.app_definition }}"` then uploads the output as a workflow artifact. The workflow continues even if the deployment fails.

1. **Persist the Radius data store**. The workflow writes the updated Radius data store back to the configured storage location so that subsequent workflow runs can resume from the current state.

The workflow reports one of the following outcomes:

| Outcome | Meaning | Action Exit Code | Example artifact contents |
| --- | --- | --- | --- |
| Deployment succeeded | The deployment succeeded. | 0 | A JSON representation of the application graph similar to the output from `rad app graph -o json`. |
| Deployment failed | One or more resources failed to deploy. | 1 | The output of `rad deploy` including the error message. |

### Scenario 5: Delete a deployed application

The frontend deletes an application by dispatching the `radius-destroy.yml` workflow (which invokes the `radius-project/destroy` action) for the target GitHub Environment. The workflow accepts the following inputs:

| Input | Required | Description |
| --- | --- | --- |
| `environment` | Yes | The GitHub Environment name to use as the Radius environment. |
| `application` | Yes | Name of the application to delete. |

The `radius-destroy.yml` workflow contains only one action calling the `radius-project/destroy@v1` GitHub Action which is published to the GitHub Marketplace. The `radius-project/destroy` action follows the same stages as the `radius-project/deploy` action, except that instead of running `rad deploy` it runs `rad app delete "${{ inputs.application }}" --yes` to delete the application and its associated cloud resources. The updated data store is then persisted so the removal is reflected in subsequent runs.

The workflow reports one of the following outcomes:

| Outcome | Meaning | Action Exit Code | Example artifact contents |
| --- | --- | --- | --- |
| Deletion succeeded | The application and its associated cloud resources were deleted. | 0 | The output of `rad app delete` confirming the application was deleted. |
| Deletion failed | One or more resources failed to delete. | 1 | The output of `rad app delete` including the error message. |

### Scenario 6: Retrieve the application graph

The frontend retrieves the application graph for a deployed application by dispatching the `radius-app-graph.yml` workflow (which invokes the `radius-project/app-graph` action) for the target GitHub Environment. This is a read-only operation: it loads the persisted state and returns the deployed application graph without provisioning, modifying, or deleting any resources. The workflow accepts the following inputs:

| Input | Required | Description |
| --- | --- | --- |
| `environment` | Yes | The GitHub Environment name to use as the Radius environment. |
| `application` | Yes | Name of the application whose graph is retrieved. |

The `radius-app-graph.yml` workflow contains only one action calling the `radius-project/app-graph@v1` GitHub Action which is published to the GitHub Marketplace. The `radius-project/app-graph` action follows the same stages as the `radius-project/deploy` action, except that instead of running `rad deploy` it runs `rad app graph "${{ inputs.application }}" -o json` and uploads the result. Because the operation only reads data, nothing is created, modified, or deleted, and the data store is unchanged.

The workflow reports one of the following outcomes:

| Outcome | Meaning | Action Exit Code | Example artifact contents |
| --- | --- | --- | --- |
| Graph retrieved | The application graph was retrieved successfully. | 0 | The JSON of the deployed application graph view, as produced by `rad app graph -o json`. |
| Retrieval failed | The application graph could not be retrieved (for example, the application does not exist). | 1 | The output of `rad app graph` including the error message. |

### Scenario 7: Run a `rad` CLI command

Unlike the deploy, destroy, and app-graph scenarios, which use purpose-built actions, this scenario uses `radius-project/run-rad-command`, the catch-all action that can run any `rad` CLI command. Any operation Radius supports but does not have a dedicated action for, such as `rad resource list` or `rad app show`, is performed through the `radius-project/run-rad-command` action.

The frontend dispatches the `radius-run-rad-command.yml` workflow (which invokes the `radius-project/run-rad-command` action) for the target GitHub Environment. The workflow accepts the following inputs:

| Input | Required | Description |
| --- | --- | --- |
| `environment` | Yes | The GitHub Environment name to use as the Radius environment. |
| `rad_commands` | Yes | A single `rad` CLI command string or a JSON-encoded array of strings. Each string is a `rad` CLI command, run in order, with the `rad` prefix omitted (for example, `resource list -o json`). |

The `radius-run-rad-command.yml` workflow contains only one action calling the `radius-project/run-rad-command@v1` GitHub Action which is published to the GitHub Marketplace. The `radius-project/run-rad-command` action follows the same stages as the `radius-project/deploy` action, except that instead of running `rad deploy` it runs each command supplied in the `rad_commands` input. When the commands only read data, nothing is created, modified, or deleted, and the data store is unchanged; commands that change state persist the data store as usual.

Each command produces its own artifact, named by its zero-based position in the `rad_commands` array: `rad-command-0`, `rad-command-1`, and so on. The frontend correlates each artifact to the command at the same index. Commands run in order, and each command's artifact is uploaded as soon as that command completes, so the frontend can retrieve results incrementally while later commands are still running. If a command fails, the action stops and does not run the remaining commands; only the artifacts for the commands that ran are produced.

The workflow reports one of the following outcomes. The overall workflow run concludes as success only if every command succeeded; if any command fails, the run concludes as failure.

| Outcome | Meaning | Action Exit Code | Example artifact contents |
| --- | --- | --- | --- |
| Command succeeded | The command ran successfully. | 0 | Per command, an artifact named `rad-command-<index>` containing the command's output, such as the JSON resource list from `rad resource list -o json`. |
| Command failed | A command did not complete successfully; remaining commands are not run. | 1 | The `rad-command-<index>` artifact for the failed command containing its output including the error message. |

### Scenario 8: Use an updated version of Repo Radius

The five Repo Radius actions are versioned independently and published to the GitHub Marketplace. The lightweight workflows committed to the user's repository reference each action by its major version tag (for example, `radius-project/deploy@v1`), so every dispatch automatically picks up the latest backward-compatible release of that major version without the user changing anything.

The `@v1` actions are expected to be long-lasting. Bug fixes and backward-compatible enhancements ship as new `v1.x` releases under the same `@v1` tag, so the user receives them transparently on the next workflow run. Because there is no persistent control plane, there is nothing for the user to upgrade, monitor, or migrate between releases.

When a change would break the established contract (for example, the workflow inputs, the result artifact names, or the JSON schema), a new major version of the affected action is published (`@v2`). The existing `@v1` action continues to work unchanged, so existing repositories are unaffected until they opt in. To adopt the new version, the frontend updates the workflow in the repository to reference the new major tag (for example, `radius-project/deploy@v2`). This is the only situation in which a committed workflow needs to change.

## Key investments

### Investment 1: Deployment to an external Kubernetes cluster

Enable Radius to deploy application workloads to an AKS or EKS cluster that is separate from the cluster hosting the Radius control plane. Repo Radius runs the control plane on an ephemeral k3d cluster within the GitHub Actions runner while deploying workloads to the developer's target cluster, so the control plane and workload clusters must be fully decoupled. This capability is delivered by the [Deploy to External AKS and EKS Clusters](../environments/2026-05-external-kubernetes.md) feature, which names the external cluster on the environment's cloud provider block (`aws.eksClusterName` or `azure.aksClusterName`) and acquires Kubernetes API access on demand using the AWS or Azure credentials already registered with Radius. Repo Radius depends on that feature and reuses its environment shape and on-demand credential acquisition rather than introducing a separate mechanism.

### Investment 2: Externalized, portable Radius data store

Move the Radius data store out of the in-cluster database so that state persists across otherwise stateless runs. At the end of a run the store is exported to GitHub-native storage, and at the start of the next run it is re-imported into the ephemeral control plane. This covers the Radius resource data, application graphs, deployment history, and Terraform state.

### Investment 3: Repo Radius GitHub Actions with a stable contract

Build and publish the five Repo Radius actions (`verify-cloud-auth`, `deploy`, `destroy`, `app-graph`, and `run-rad-command`), each versioned by major tag. Define and maintain a stable, well-documented contract: the action inputs, the workflow run conclusion signal, and result artifacts published under well-known names with a versioned JSON schema. This contract lets any frontend drive Repo Radius without coupling to its internals and must remain backward-compatible within a major version.

### Investment 4: Lightweight workflows for each action

Provide the thin, ready-to-commit workflow file that pairs with each action (`radius-verify-cloud-auth.yml`, `radius-deploy.yml`, `radius-destroy.yml`, `radius-app-graph.yml`, and `radius-run-rad-command.yml`). Each workflow declares its inputs and invokes only the corresponding action at its major version tag, with no Repo Radius logic of its own. The frontend writes these workflows into the user's repository so that GitHub Actions has a dispatchable entry point for each operation, and because the workflows contain no logic they only need to change when the referenced major version is updated.

### Investment 5: Fast ephemeral control plane startup

Provision the k3d cluster, install the `rad` CLI, and install Radius quickly enough that the per-run startup cost is acceptable within a GitHub Actions runner. Because the control plane is created and torn down on every operation, startup time is on the critical path for every user-facing action and is the primary determinant of perceived responsiveness.

### Investment 6: Cloud credential integration via OIDC

Integrate with GitHub's OIDC federation to provide short-lived AWS and Azure credentials to the Radius control plane without storing long-lived secrets. This includes the per-run authentication steps, injection of session credentials into the Radius pods for Terraform provider access, and the `verify-cloud-auth` preflight check that validates the environment's configuration before the first deployment.