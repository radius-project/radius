# Feature Specification: Repo Radius

* **Author**: Zach Casper (@zachcasper)

## Summary

Today, Radius requires users to install Radius on a Kubernetes cluster. That Kubernetes cluster must have identity and access management configured, gateway controllers installed, storage configured, container image registries set up, and other configurations. Only then can Radius be installed. After these are complete, they must all be performed again for the production environment and Radius installed again.

This is a heavy requirement to place on any team. Running Radius as a Kubernetes service carries real install and maintenance complexity, and a team must take that on before they can build a single application. Teams that already have platform engineers, or their own Kubernetes expertise, may accept that cost. For everyone else, and especially for developers without Kubernetes expertise, it is too much to ask just to build an app. This makes Radius not a viable option for these users.

Repo Radius is a rethinking of how to deliver Radius to developers. Repo Radius transforms Radius from a heavyweight persistent control plane into a lightweight on-demand engine. Concretely, Repo Radius enables developers to run Radius within a GitHub Actions runner, with state stored externally in the same GitHub account.

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

### User personas

The primary user is a developer working on a cloud-native application hosted on GitHub. They may be an individual developer, a small team, or part of a larger organization. They do not have a dedicated platform engineering team and likely do not have deep Kubernetes expertise. They want to deploy their application to AWS or Azure without managing infrastructure.

A secondary user is a platform engineer evaluating Radius. Repo Radius provides a low-friction entry point: they can try Radius on a real application without provisioning a Kubernetes cluster and later migrate to a self-hosted Radius installation if needed.

### User problem description

I am a developer building a cloud-native application. My application is an online storefront made up of several parts that must work together: a containerized web frontend and API, a PostgreSQL database for orders and inventory, a Redis cache for sessions and catalog lookups, and a RabbitMQ message queue for asynchronous order fulfillment. The frontend and API run as containers, while the database, cache, and queue are managed cloud services that must be provisioned, connected to the workloads, and granted the right identity and network access. Developing the application is easy with today's AI coding agents. The hard part is not the code; it is getting all of it deployed and wired together correctly in the cloud.

What makes this hard is the depth of detail each piece demands. Every managed service has its own configuration to learn: the provisioning options for the database, the cache, and the queue; the managed identities and IAM roles that let my workloads talk to them; the connection strings, endpoints, and secrets that have to be generated and injected into the right containers; and the network rules that allow traffic between them. I have to do all this again for every environment (dev, test, and production), keep them consistent, and keep the dependencies and ordering straight in my head. Radius is appealing precisely because it models an application at this level and handles that wiring for me. But to use Radius today, I first have to provision a Kubernetes cluster and stand up a Radius control plane on it, then do it all over again for a separate production cluster, and keep all of it upgraded, monitored, and secured. That is on top of building my application, and I do not have a platform team to hand it to.

So in practice I never get to Radius at all. Instead I stitch everything together myself with infrastructure as code: I write Terraform, CloudFormation, or Helm charts against cloud-specific providers, learning each provider's resource types and arguments for every service I use; I wire the connections, managed identities, and network access between the workloads and the services by hand; and I duplicate and tweak it all for each environment while managing state and ordering myself. It is a large amount of low-level, provider-specific code to write and maintain, it is easy to get a detail wrong, and the effort grows with every service and every environment I add. I would rather use a tool that models my application for me, but not if the price of admission is managing a Kubernetes cluster and Radius control plane.

### Challenge(s) faced by the user

* **High barrier to entry**. Before deploying a single application with Radius today, the developer must provision a Kubernetes cluster, configure IAM, install gateway controllers, set up storage, configure a container registry, and install Radius. This takes hours to days and requires Kubernetes expertise.
* **Ongoing operational burden**. A persistent Radius control plane must be upgraded, monitored, and secured. For a developer who just wants to deploy an application, this overhead is disproportionate to the value.
* **Environment duplication**. Separating production from non-production workloads requires separate Kubernetes clusters, each with its own Radius control plane. This doubles the setup and operational cost.
* **No try-before-you-buy path**. There is no lightweight way to experience Radius's value proposition without committing to the full Kubernetes setup.

### Positive user outcome

A developer, using a solution built using Repo Radius, can deploy their application to AWS or Azure without having to install Kubernetes or Radius. They can create multiple environments and provide their cloud account details, and Repo Radius will provision the required infrastructure for their application. Radius runs on demand within GitHub Actions, starts quickly, deploys their application, and shuts down. The state is stored in the same GitHub account. If they outgrow Repo Radius, they can take their application definition to a self-hosted Radius installation without modification.

## Key scenarios

### Scenario 1: Set up a new environment

The developer creates an environment by providing cloud account details (e.g., AWS account ID and region, or Azure subscription and resource group). The environment is stored as a GitHub Environment and reused across subsequent deployments.

### Scenario 2: Authorize cloud access

The developer connects their AWS or Azure account to GitHub using OIDC identity federation so that deployments can provision cloud resources using short-lived tokens rather than stored credentials.

### Scenario 3: Deploy an application from a GitHub repository

The developer asks to deploy their application. Cloud resources are provisioned on AWS or Azure without any prior Kubernetes cluster setup, Radius installation, or infrastructure configuration.

### Scenario 4: Update a deployed application

The developer modifies their application and deploys the change to an existing environment. Repo Radius loads the persisted state from the previous deployment and applies changes incrementally.

### Scenario 5: Retrieve the application graph

The developer asks to see the application graph for a deployed application including the resources it comprises, their IDs, and the connections between them. Repo Radius loads the persisted state and returns the JSON of the deployed application graph view without deploying or modifying any resources.

### Scenario 6: Delete a deployed application

The developer asks to delete their application. Repo Radius loads the persisted state, deletes the application and its associated cloud resources, and updates the persisted state to reflect the removal.

### Scenario 7: Use an updated version of Repo Radius

Repo Radius is published as a set of versioned GitHub Actions (e.g., `radius-project/run-rad-commands@v1`). The developer (or the frontend on their behalf) references a major version tag such as `@v1` to receive backward-compatible updates automatically, and moves to the next major tag (for example, `@v2`) for breaking changes, following the standard GitHub Actions versioning convention. Because there is no persistent control plane, the developer never performs a manual upgrade; the next deployment simply runs the referenced version.

## Detailed user experience

Since Repo Radius is only the backend for a larger end-to-end developer experience, the scenarios described below are intentionally low-level. They describe how Repo Radius behaves for each operation: what it expects going in, what the action does, and what it returns when things succeed or fail. Since Repo Radius does not have a user interface, what the developer sees and clicks is defined by the frontend, which is a separate component covered in its own design.

### Scenario 1-2: Set up a new environment and authorize cloud access

#### Prerequisites

* A GitHub repository with GitHub Actions enabled.
* Permission to create GitHub Environments in the repository and to configure the cloud provider's identity federation (an Entra ID app registration for Azure, or an IAM OIDC identity provider and role for AWS).

#### Workflow

The frontend performs the following steps on the user's behalf.

##### Step 1: Enable Repo Radius for the repository

A workflow file in the repository's `.github/workflows/` directory is required for GitHub Actions to run; an action cannot be invoked without one. The frontend therefore writes the two lightweight workflows to the repository on the user's behalf: one to verify cloud authentication and one to run `rad` commands. These workflows are intentionally thin: they contain no Repo Radius logic and simply invoke the published Repo Radius GitHub Actions. All implementation detail lives within the published actions, so a committed workflow only needs to change when the repository adopts a new major version of an action (see Scenario 7).

Repo Radius is published as the following GitHub Actions, each pinned to a matching major version (e.g., `@v1`):

| Workflow | Action | Purpose |
| --- | --- | --- |
| `radius-verify-cloud-auth.yml` | `radius-project/verify-cloud-auth` | Verify that the GitHub Environment's OIDC configuration can authenticate to the cloud provider before attempting a deployment. |
| `radius-run-rad-commands.yml` | `radius-project/run-rad-commands` | Run one or more `rad` CLI commands against the environment. This single action performs every Radius operation (deploying, updating, deleting, and reading) by running the corresponding `rad` commands. |

##### Step 2: Environment setup

The frontend creates a GitHub Environment. The environment stores cloud provider details (e.g., AWS account and region, or Azure subscription and resource group) and Kubernetes cluster details (e.g., the EKS or AKS cluster name and Kubernetes namespace) for application workloads.

##### Step 3: OIDC setup

The frontend guides the user through configuring their cloud provider to trust the GitHub Actions OIDC identity provider as a federated identity source, so that the workflow can authenticate using short-lived tokens rather than stored credentials. For Azure, this means creating a federated credential on an Entra ID app registration that trusts tokens issued by GitHub Actions; the resulting client ID, tenant ID, and subscription ID are stored in the GitHub Environment. For AWS, this means creating an IAM OIDC identity provider and an IAM role with a trust policy that accepts GitHub Actions tokens; the role ARN and region are stored in the GitHub Environment.

##### Step 4: Verify cloud authentication

Before the environment is considered ready, the frontend dispatches the `radius-verify-cloud-auth.yml` workflow (which invokes the `radius-project/verify-cloud-auth` action) for the target GitHub Environment. This confirms that the environment's variables and OIDC configuration are complete and correct, so that misconfiguration is surfaced during setup rather than on the first deployment.

The verification performs an end-to-end authentication check: it requests a GitHub Actions OIDC token, exchanges it with the cloud provider, and confirms that the resulting short-lived credentials are valid. It does not provision, modify, or delete any resources.

When an EKS or AKS cluster name is provided in the environment, the verification additionally confirms Kubernetes connectivity: it uses the authenticated cloud credentials to retrieve the cluster's access configuration and confirms that the cluster's API server is reachable. This catches a misnamed cluster, a cluster in a different region or resource group, or an identity that lacks access to the cluster before the first deployment.

#### Workflow outcome and failure cases

The outcome is reported in two complementary ways, consistent with how all Repo Radius workflows return results:

* **Workflow run conclusion**. The GitHub Actions run concludes as success or failure, giving the frontend a coarse pass/fail signal it can read directly from the run status via the GitHub Actions API without downloading any files.
* **Result artifact**. The workflow uploads a structured result artifact (a small JSON document) containing the specific outcome from the table below, a human-readable message, and remediation guidance. The frontend downloads this artifact to distinguish between the failure modes (for example, "trust not established" versus "cluster unreachable") and to surface actionable guidance to the user.

The workflow reports one of the following outcomes:

| Outcome | Meaning | Example artifact contents |
| --- | --- | --- |
| Authentication succeeded (`success`) | Authentication succeeded and the credentials are valid. | The `verify-cloud-auth-result` artifact with a message such as "Authentication succeeded. Environment is ready for deployments." |
| Missing configuration (`missing_configuration`) | A required variable for the selected cloud provider is absent. | The `verify-cloud-auth-result` artifact with a message such as "A required cloud authentication variable is not set. Re-run environment and OIDC setup to populate the environment." |
| Trust not established (`trust_not_established`) | The cloud provider rejected the GitHub OIDC token. | The `verify-cloud-auth-result` artifact with a message such as "Cloud provider rejected the GitHub OIDC token. Re-run environment and OIDC setup to verify the IAM role trust policy or app registration federated credential matches the repository, environment, and subject claim." |
| Insufficient permissions (`insufficient_permissions`) | Authentication succeeded but the identity lacks permission to perform the check. | The `verify-cloud-auth-result` artifact with a message such as "Authenticated successfully, but the AWS IAM role or Azure app registration is missing the required permissions. Re-run the environment and OIDC setup or manually add the missing permission. See the Radius documentation for required permissions." |
| Cluster unreachable (`cluster_unreachable`) | A cluster name was provided, but the cluster could not be found or its API server could not be reached. | The `verify-cloud-auth-result` artifact with a message such as "Cluster my-eks-cluster could not be found or its API server is unreachable. Correct the EKS cluster name, or grant the identity access to the cluster." |
| Provider unreachable (`provider_unreachable`) | The cloud provider's authentication endpoint could not be reached. | The `verify-cloud-auth-result` artifact with a message such as "Cloud provider authentication endpoint could not be reached. Re-run the verification workflow to try again. If it persists, check the provider's service status." |

The result artifact is uploaded under a well-known, stable artifact name (for example, `verify-cloud-auth-result`) and is available to the frontend via the GitHub Actions API. The artifact name and the JSON schema form part of the stable contract between Repo Radius and any frontend, so they must remain backward-compatible within a major version.

### Scenario 3-4: Deploy a new or updated application

From the developer's point of view, this is the step that matters most: they ask to deploy the storefront to an environment and need to come away knowing it actually worked, or, if it did not, exactly what failed. The frontend turns that intent into a workflow dispatch and reports the result back.

#### Prerequisites

* The target GitHub Environment exists with its cloud and cluster variables populated (Scenario 1).
* Cloud authentication has been verified for the environment and `verify-cloud-auth-result` reported `success` (Scenario 2).
* The application definition is present in the repository.
* The `radius-run-rad-commands.yml` workflow is committed to the repository (Step 1).

#### Workflow

The frontend deploys (or updates) an application by dispatching the `radius-run-rad-commands.yml` workflow (which invokes the `radius-project/run-rad-commands` action) for the target GitHub Environment. Deployment is expressed as a `rad deploy` command in the `rad_commands` input; an update uses the same command against the same environment, and Radius reconciles the change incrementally from the persisted state. The workflow accepts the following inputs:

| Input | Value |
| --- | --- |
| `environment` | The GitHub Environment name |
| `rad_commands` | `['deploy .radius/app.bicep', 'app graph storefront -o json']` |

> [!IMPORTANT]
> Repo Radius does not accept arbitrary `rad` invocations. Each entry in `rad_commands` is validated against an enum of allowed commands so that commands which are not applicable to the ephemeral, per-run model (for example, commands that manage or upgrade a persistent control plane, switch workspaces, or change installation state) cannot run, and so the contract stays narrow and reviewable. Validation happens before the control plane is created or any command runs, so a disallowed command errors out and fails fast without provisioning anything. The specific set of allowed commands is defined in the technical design. The enum is part of the stable contract and is versioned with the action; additional commands may be introduced in backward-compatible releases.

To deploy the storefront and capture its application graph, the frontend dispatches the workflow for the target environment (`production`) with the two `rad` commands shown above in the `rad_commands` input.

The `radius-run-rad-commands.yml` workflow contains only one action calling the `radius-project/run-rad-commands@v1` GitHub Action which is published to the GitHub Marketplace. This is deliberate: the workflow file lives in the user's repository, where logic is difficult to update because each change requires a commit to every consuming repository. By keeping the workflow as a thin wrapper that simply invokes the published action, all of the real logic lives in the action itself and can be updated centrally on the GitHub Marketplace. Users automatically pick up new backward-compatible releases without any change to the workflow in their repository. The `radius-project/run-rad-commands` action performs the following:

1. **Authenticate with cloud providers**. The workflow uses the GitHub Environment's OIDC variables to authenticate. For Azure, it calls `azure/login` with the client ID, tenant ID, and subscription ID. For AWS, it calls `aws-actions/configure-aws-credentials` with the IAM role ARN and region.

1. **Load the Radius data store**. The workflow reads the persisted Radius data store from the configured storage location and loads it into the ephemeral control plane. On first run, the data store is empty.

1. **Create an ephemeral Radius control plane**. The workflow creates a lightweight k3d cluster on the runner, installs the `rad` CLI, and installs Radius on the k3d cluster. This ephemeral cluster hosts the Radius control plane only; application workloads deploy to the developer's target cluster.

1. **Create a Radius environment based on the GitHub Environment**. The workflow creates a Radius resource group and environment with the properties from the GitHub Environment passed as an input to the workflow.

1. **Register the default recipe pack**. The workflow registers the default AWS and Azure recipe pack that ships with Radius to the environment, so the application's resources resolve to recipes without the user having to author or register recipes themselves. Customizing the recipe pack is a future feature; the initial release uses the default recipe pack only.

1. **Execute the `rad` commands**. The workflow runs each command supplied in the `rad_commands` input, in order, prefixing each with `rad`, and captures each command's output. If a command fails, the action stops and does not run the remaining commands.

1. **Persist the Radius data store**. The workflow writes the updated Radius data store back to the configured storage location so that subsequent workflow runs can resume from the current state.

#### Workflow outcome and failure cases

If a `rad` command fails, the action stops before running the remaining commands, so `command_failed` reports a partial result whose final entry is the failed command. `state_persist_failed` is the other partial-state case: the commands ran, but the updated data store could not be written back, so the cloud may contain resources the persisted state does not reflect. The workflow reports one of the following outcomes:

| Outcome | Meaning | Example artifact contents |
| --- | --- | --- |
| All commands succeeded (`succeeded`) | Every command ran successfully. | The `run-rad-commands-result` artifact whose `commands` array has an entry per command, each with the command and its output (such as the `rad deploy` output and the JSON application graph). |
| A command failed (`command_failed`) | A command did not complete successfully; remaining commands are not run. | The `run-rad-commands-result` artifact whose last entry is the failed command including its exit code and error output. Entries for commands that did not run are absent. |
| Disallowed command (`disallowed_command`) | One or more commands are not in the allowed-command enum; the request is rejected and no command runs. | The `run-rad-commands-result` artifact identifying the disallowed command. |
| Environment not found (`environment_not_found`) | The named GitHub Environment does not exist or has no configuration. The request is rejected before authentication and no command runs. | The `run-rad-commands-result` artifact with an empty `commands` array and a message such as "GitHub Environment 'production' was not found or has no configuration. Create the environment and complete cloud authentication setup before deploying." |
| Authentication failed (`authentication_failed`) | Cloud authentication failed before any command ran; the GitHub OIDC token could not be exchanged for valid AWS or Azure credentials. | The `run-rad-commands-result` artifact with an empty `commands` array and a message such as "Failed to authenticate to AWS. The IAM role trust policy or permissions may have changed since the environment was verified. Re-run cloud authentication verification." |
| State load failed (`state_load_failed`) | The persisted Radius data store could not be read or restored into the control plane, so no command ran. | The `run-rad-commands-result` artifact with an empty `commands` array and a message indicating the stored state is missing or unreadable. |
| State persist failed (`state_persist_failed`) | Commands ran, but the updated data store could not be written back. The cloud may now contain resources that the persisted state does not reflect. | The `run-rad-commands-result` artifact including the command results plus a message warning that state was not saved, so a subsequent run may not reflect the resources that were just provisioned. |

#### The `run-rad-commands-result` artifact

The `run-rad-commands-result` artifact is the contract between Repo Radius and any frontend, so its name and schema are fixed and versioned with the action. It carries the overall `outcome`, a human-readable `message`, and an ordered `commands` array with one entry per command (the command, its outcome, and its captured output). For example, deploying the storefront and then reading its graph yields an artifact whose `commands` array has two succeeded entries: the `rad deploy` output listing the provisioned resources, and the JSON application graph.

```json
{
  "outcome": "succeeded",
  "message": "All 2 commands succeeded.",
  "commands": [
    { 
      "command": "deploy .radius/app.bicep", "outcome": "succeeded", 
      "output": "Deployment Complete ..."
    },
    { 
      "command": "app graph storefront -o json",
      "outcome": "succeeded", "output": "{ ... }" 
    }
  ]
}
```

The complete schema is defined in the technical design.

### Scenario 5: Retrieve the application graph

#### Prerequisites

* The target GitHub Environment exists with its cloud and cluster variables populated (Scenario 1).
* Cloud authentication has been verified for the environment (Scenario 2).
* The application has been deployed to the target environment (Scenario 3-4).

#### Workflow

The frontend retrieves the application graph for a deployed application by dispatching the same `radius-run-rad-commands.yml` workflow with a `rad app graph` command in the `rad_commands` input. This is a read-only operation: it loads the persisted state and returns the deployed application graph without provisioning, modifying, or deleting any resources, so the data store is unchanged.

| Input | Value |
| --- | --- |
| `environment` | The GitHub Environment name |
| `rad_commands` | `['app graph storefront -o json']` |

The action runs `rad app graph storefront -o json` and writes the result to the `run-rad-commands-result` artifact; on success, the artifact's single entry contains the JSON of the deployed application graph view. For the storefront, the graph shows the `frontend` and `api` containers, with the `api` connected to the `orders-db` database, the `cache`, and the `orders-queue`.

#### Workflow outcome and failure cases

The workflow reports the same outcomes as the deploy scenario. 

### Scenario 6: Delete a deployed application

#### Prerequisites

* The target GitHub Environment exists with its cloud and cluster variables populated (Scenario 1).
* Cloud authentication has been verified for the environment (Scenario 2).
* The application has been deployed to the target environment (Scenario 3-4).

#### Workflow

The frontend deletes an application by dispatching the same `radius-run-rad-commands.yml` workflow with a `rad app delete` command in the `rad_commands` input. The workflow accepts the same inputs as the deploy scenario:

| Input | Value |
| --- | --- |
| `environment` | The GitHub Environment name |
| `rad_commands` | `['app delete storefront --yes']` |

The action follows the same stages as the deploy scenario, running `rad app delete storefront --yes` to delete the application and its associated cloud resources (the `orders-db` PostgreSQL database, the `cache` Redis instance, and the `orders-queue` RabbitMQ queue). The command's output is included in the single `run-rad-commands-result` artifact.

#### Workflow outcome and failure cases

The workflow reports the same outcomes as the deploy scenario. 

### Scenario 7: Use an updated version of Repo Radius

The two Repo Radius actions are versioned independently and published to the GitHub Marketplace. The lightweight workflows committed to the user's repository reference each action by its major version tag (for example, `radius-project/run-rad-commands@v1`), so every dispatch automatically picks up the latest backward-compatible release of that major version without the user changing anything.

The `@v1` actions are expected to be long-lasting. Bug fixes and backward-compatible enhancements ship as new `v1.x` releases under the same `@v1` tag, so the user receives them transparently on the next workflow run. Because there is no persistent control plane, there is nothing for the user to upgrade, monitor, or migrate between releases.

When a change would break the established contract (for example, the workflow inputs, the allowed-command set, the result artifact names, or the JSON schema), a new major version of the affected action is published (`@v2`). The existing `@v1` action continues to work unchanged, so existing repositories are unaffected until they opt in. To adopt the new version, the frontend updates the workflow in the repository to reference the new major tag (for example, `radius-project/run-rad-commands@v2`). This is the only situation in which a committed workflow needs to change.

## Key investments

### Investment 1: Deployment to an external Kubernetes cluster

Enable Radius to deploy application workloads to an AKS or EKS cluster that is separate from the cluster hosting the Radius control plane. Repo Radius runs the control plane on an ephemeral k3d cluster within the GitHub Actions runner while deploying workloads to the developer's target cluster, so the control plane and workload clusters must be fully decoupled. This capability is delivered by the [Deploy to External AKS and EKS Clusters](../environments/2026-05-external-kubernetes.md) feature, which names the external cluster on the environment's cloud provider block. The method of acquiring Kubernetes API access is to be defined in the technical design.

### Investment 2: Externalized, portable Radius data store

Move the Radius data store out of the in-cluster database so that state persists across otherwise stateless runs. At the end of a run the store is exported to GitHub-native storage, and at the start of the next run it is re-imported into the ephemeral control plane. This covers the Radius resource data, application graphs, deployment history, and Terraform state.

### Investment 3: Repo Radius GitHub Actions with a stable contract

Build and publish the two Repo Radius actions (`verify-cloud-auth` and `run-rad-commands`), each versioned by major tag. Define and maintain a stable, well-documented contract: the action inputs, the workflow run conclusion signal, and result artifacts published under well-known names with a versioned JSON schema (see the `run-rad-commands-result` artifact in Scenario 3-4). This contract lets any frontend drive Repo Radius without coupling to its internals and must remain backward compatible within a major version.

### Investment 4: Lightweight workflows for each action

Provide the thin, ready-to-commit workflow file that pairs with each action (`radius-verify-cloud-auth.yml` and `radius-run-rad-commands.yml`). Each workflow declares its inputs and invokes only the corresponding action at its major version tag, with no Repo Radius logic of its own. The frontend writes these workflows into the user's repository so that GitHub Actions has a dispatchable entry point for each operation, and because the workflows contain no logic they only need to change when the referenced major version is updated.

### Investment 5: Fast ephemeral control plane startup

Provision the k3d cluster, install the `rad` CLI, and install Radius quickly enough that the per-run startup cost is acceptable within a GitHub Actions runner. Because the control plane is created and torn down on every operation, startup time is on the critical path for every user-facing action and is the primary determinant of perceived responsiveness. The approach to optimizing control plane startup time is to be defined in the technical design.

### Investment 6: Cloud credential integration via OIDC

Integrate with GitHub's OIDC federation to provide short-lived AWS and Azure credentials to the Radius control plane without storing long-lived secrets. This includes the per-run authentication steps, injection of session credentials into the Radius pods for Terraform provider access, and the `verify-cloud-auth` preflight check that validates the environment's configuration before the first deployment.

## Key dependencies and risks

* **Dependency: External UI**. Repo Radius is designed to be shipped as part of a developer solution which includes an application graph visualization, AI-based modeling of the application, and a user interface to configure environments and visualize deployments. These other components are a dependency and not part of the Repo Radius scope.

* **Risk: Radius startup time**. Radius must start quickly enough within a GitHub Actions runner that the developer experience feels responsive. If startup takes minutes, the experience degrades significantly.

* **Risk: Broad cloud permissions and blast radius**. The IAM role (AWS) or app registration (Azure) assumed via OIDC must hold a significant number of permissions to create and destroy the full range of resources an application may require. A role this broad has a high blast radius: a misbehaving recipe, a compromised workflow, or an over-scoped command could create, modify, or destroy a wide range of cloud resources. Scoping these permissions to the minimum necessary, and isolating them per environment, is an important mitigation to design for.

* **Risk: Recipe and recipe pack backward compatibility**. Because Repo Radius is stateless and provisions on demand, every deployment re-resolves the application's resources against whatever recipe pack version is in effect at that time. Once an application is deployed, it must be assumed that there are live cloud resources backed by a recipe. Recipe and recipe pack versions must therefore be tightly controlled so that a newer recipe version remains backward-compatible with resources provisioned by an earlier version; a breaking recipe change could fail to reconcile, orphan, or inadvertently destroy resources that are already deployed. Recipe and recipe pack versioning needs the same backward-compatibility discipline as the action contract itself.

## Alternatives considered

### Multiple operation-specific actions versus a single generic action

We considered publishing a separate action for each operation (for example, `radius-deploy`, `radius-destroy`, `radius-app-graph`) instead of the single `run-rad-commands` action.

* **Multiple operation-specific actions**. Each action exposes a narrow, strongly typed input surface (for example, `radius-deploy` takes an application path; `radius-destroy` takes an application name), so the contract is self-documenting and each action validates only the inputs relevant to it. The drawback is that every new operation requires a new action, a new workflow file committed to the repository, and a new major-version lifecycle to maintain. Composing several operations in one run (deploy, then read the graph) requires multiple workflow dispatches and multiple ephemeral control plane startups, multiplying the per-run startup cost that Investment 5 works to minimize.
* **Single generic action (recommended)**. One action runs an ordered list of allowed `rad` commands. A single ephemeral control plane is created once per run and reused across all commands, so multi-step flows (deploy, then `app graph`) pay the startup cost once. Adding a new operation usually means allowing a new command in the enum rather than publishing a new action and workflow. The trade-off is a less strongly typed input surface, which is mitigated by validating every command against the allowed-command enum before anything runs.

**Recommendation**: The single generic `run-rad-commands` action. It minimizes per-run startup cost for multi-step flows, keeps the committed workflow count small, and lets new operations ship as backward compatible additions to the allowed-command enum rather than as new actions. It also reduces the surface area in the user's repository to a single, highly flexible workflow that can accommodate future functionality: only the published action (which lives outside the user's repository) needs to be updated with new allowed commands, so the user's repository does not need to change.

### Per-command result artifacts versus a single combined artifact

We considered uploading a separate result artifact per command instead of one combined `run-rad-commands-result` artifact.

* **Per-command artifacts**. Each command's output is uploaded as its own artifact (for example, `rad-command-0`, `rad-command-1`), so the frontend can download a single command's output without retrieving the others. The drawback is that the artifact set varies with the number and order of commands, so the frontend must discover artifact names dynamically, correlating outputs back to the requested order is awkward, and a run produces many small artifacts.
* **Single combined artifact (recommended)**. One `run-rad-commands-result` artifact contains a `commands` array with an entry per command, in input order, each with the command, exit code, and output. The artifact name is stable and known in advance, the ordering is explicit, and a single download yields the full result of the run.

**Recommendation**: The single combined `run-rad-commands-result` artifact. A stable, well-known artifact name with an ordered `commands` array is simpler for the frontend to consume and keeps the versioned contract small.

### Environments defined in a repository file versus GitHub Environments

We considered letting the user define multiple environments in a checked-in file (for example, an `env.bicep` in the repository) instead of modeling each environment as a GitHub Environment.

* **Environments defined in a repository file**. A single `env.bicep` declares every environment (dev, test, production) and its cloud provider and cluster details in one place under version control, so the full set of environments is reviewable in a pull request and lives alongside the application. The drawback is that it duplicates a concept GitHub already provides. Cloud credentials must still be stored as GitHub Environment secrets, so an env.bicep would split related configuration across two locations: non-secret details in the file and secrets in the environment.

* **GitHub Environments (recommended)**. Each environment is a GitHub Environment whose variables hold the cloud provider and cluster details. This reuses GitHub's native environment model including scoped secrets. The downside is that GitHub Environments are not easily created in bulk or declaratively; each environment and its variables are configured imperatively through the GitHub UI, the `gh` CLI, or the REST API rather than from a single checked-in file. The offsetting upside is that users likely already know how to create GitHub Environments and may already have tooling or automation for managing them.

**Recommendation**: Model environments as GitHub Environments only. It reuses GitHub's native scoping, protection, and OIDC integration rather than reinventing them in a repository file, and it keeps a single source of truth for environment configuration that the workflows and the frontend already consume.
