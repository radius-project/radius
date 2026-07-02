# Feature Specification: Repo Radius

* **Author**: Zach Casper (@zachcasper)

## Summary

Today, Radius requires users to install Radius on a Kubernetes cluster. That Kubernetes cluster must have identity and access management configured, gateway controllers installed, storage configured, container image registries set up, and other configurations. Only then can Radius be installed. After these are complete, they must all be performed again for the production environment and Radius installed again.

This is a heavy requirement to place on any team. Running Radius as a Kubernetes service carries real install and maintenance complexity, and a team must take that on before they can build a single application. Teams that already have platform engineers, or their own Kubernetes expertise, may accept that cost. For everyone else, and especially for developers without Kubernetes expertise, it is too much to ask just to build an app. This makes Radius not a viable option for these users.

Repo Radius is a rethinking of how to deliver Radius to developers. It transforms Radius from a heavyweight persistent control plane into a lightweight on-demand engine. Concretely, Repo Radius enables developers to run Radius within a GitHub Actions runner, with state stored externally in the same GitHub account.

### Top level goals

* Eliminate the prerequisite to have a Kubernetes cluster configured.
* Enable Radius to run ephemerally within a GitHub Actions runner, starting on demand and shutting down after each operation.
* Persist Radius state (application graphs, deployment history, environment configuration) in GitHub-native storage rather than in-cluster databases.
* Support the Copilot app integration in the near term and other UX surfaces in the future.
* Reuse the existing self-hosted Radius implementation with minimal to no GitHub-specific modifications.

### Non-goals (out of scope)

* Replacing the existing persistent control plane deployment model. Repo Radius is an alternative delivery mechanism, not a replacement.
* Building the frontend UI component such as the Radius extension for the Copilot app. Repo Radius must ship with a separate frontend component.
* Supporting non-GitHub source control platforms (GitLab, Bitbucket, Azure DevOps).
* Running Repo Radius outside of GitHub Actions (e.g., locally on a developer's workstation as the primary mode).
* Supporting multi-repo applications is deferred to a later date.
* Customizing, hardening, or expanding the recipe pack. The initial release uses the default AWS and Azure recipe pack that ships with Radius as-is.

## Definition of terms

Throughout this document, several terms are used with a specific meaning in the context of Repo Radius.

**Repo Radius** – The on-demand delivery model described in this specification, in which Radius runs ephemerally inside a GitHub Actions runner with state stored in the same GitHub account.

**Self-hosted Radius** – The existing deployment model in which Radius runs as a persistent control plane installed on a Kubernetes cluster.

**Frontend** – The separate user-facing component (such as the Copilot app) that drives Repo Radius by dispatching GitHub Action workflows and presenting their results.

**Ephemeral control plane** – A Radius control plane created on demand inside a GitHub Actions runner for the duration of a single operation and torn down when it completes.

**k3d cluster** – A lightweight Kubernetes cluster which hosts the ephemeral Radius control plane inside a GitHub Actions runner.

**Workload cluster** – The developer's own EKS or AKS cluster where application workloads are deployed, separate from the ephemeral control plane.

**Radius data store** – The persisted Radius state (resource data, application graphs, deployment history, and Terraform state). In self-hosted Radius this is held in etcd.

**GitHub Environment** – A native GitHub feature used to store a deployment environment's cloud provider details, workload cluster details, and scoped secrets.

**GitHub Deployment** – A native GitHub record that associates a deployment with a GitHub Environment and a specific git commit, used to track which version of the application is currently deployed to each environment.

**OIDC identity federation** – A mechanism for exchanging a short-lived GitHub Actions token for cloud provider credentials without storing long-lived secrets.

## User profile and challenges

### User personas

The primary user is a developer building a cloud-native application with the source code hosted on GitHub. They may be an individual developer, a small team, or part of a larger organization. They likely do not have a dedicated platform engineering team or have deep Kubernetes expertise. They want to deploy their application to AWS or Azure without managing infrastructure.

A secondary user is a platform engineer who tailors Repo Radius for their teams, customizing recipes and predefining governed environments (User Journey 5) so that developers deploy within approved organizational standards.

### The developer's problem

I am a developer building a cloud-native application, an online storefront: a web frontend and API as containers, backed by a PostgreSQL database, a Redis cache, and a Kafka queue that each must be provisioned, connected to the workloads, and granted the right identity and network access. With today's AI coding agents the code is easy; the hard part is getting all of it deployed and wired together correctly in the cloud. Radius models an application at exactly this level and handles that wiring, but using it today first means provisioning a Kubernetes cluster and standing up a control plane, then repeating that for every environment and keeping it upgraded, monitored, and secured. Without a platform team, that price of admission is too high, so I fall back to traditional IaC tools like Terraform, CloudFormation, or Helm.

* **High barrier to entry**. Before deploying a single application with Radius today, the developer must provision a Kubernetes cluster, configure IAM, install gateway controllers, set up storage, configure a container registry, and install Radius. This takes hours to days and requires Kubernetes expertise.
* **Low-level, provider-specific wiring**. Without Radius, the developer owns a large body of provider-specific Terraform, CloudFormation, or Helm, even when an LLM generates the first draft. They still have to understand each provider's resource types, wire the managed identities, connection strings, endpoints, secrets, and network rules between workloads and services, and review and maintain it all. It is easy to get a detail wrong, and the effort grows with every service added.
* **Environment duplication**. The developer repeats this work for every environment (dev, test, and production), keeping them consistent and the dependencies and ordering straight by hand. With self-hosted Radius, separating production from non-production also requires separate Kubernetes clusters, each with its own Radius control plane.
* **Ongoing operational burden**. A persistent Radius control plane must be upgraded, monitored, and secured. For a developer who just wants to deploy an application, this overhead is disproportionate to the value.

### The platform engineer's problem

I am a platform engineer supporting several teams. I want my developers to deploy on their own, but I am accountable for what lands in our cloud accounts. I need to keep the consistency, security, and governance I am accountable for without taking away the fast, self-service experience my developers depend on. I do this today by publishing standard internally developed Terraform modules that all projects are expected to use and programmatically provisioning cloud environments via a self-service developer portal such as Backstage.

* **Predefined environments and managed credentials**. The platform engineer needs a way to programmatically create logical environments teams deploy to (dev, test, production), each bound to an approved cloud account, region, and cluster, and to manage the cloud credentials centrally so developers never handle long-lived secrets and production stays separated from non-production.
* **Standard, enforceable deployment modules**. The platform engineer needs to define the organization's standard IaC deployment modules once, encoding naming, tagging, network and security baselines, approved services and SKUs, and cost controls, and enforce them so every team's resources are provisioned to those standards rather than remediated after the fact.
* **Scale across a large estate**. The platform engineer needs both of these to work at scale across many teams and repositories, so governance is defined once and inherited everywhere without per-team configuration or after-the-fact enforcement.

### Positive user outcome

A developer can deploy to AWS or Azure without installing Kubernetes or self-hosted Radius. They create environments with their cloud account details and ask for their application to be deployed and Repo Radius provisions the exact infrastructure needed for the application in a secure and cost-efficient manner. As they modify their application, Repo Radius updates the deployed infrastructure to match the updated application. If they outgrow Repo Radius, the developer can migrate to self-hosted Radius without changing their application definition.

A platform engineer can define organization-wide recipes once and every team inherits them, and is able to programmatically create environments with cloud credentials so developers keep a fast, self-service experience while the platform engineer keeps the consistency, security, and governance they are accountable for.

## User journeys

The user experience is organized into five journeys. The first four follow the developer's lifecycle of an application: getting it running, iterating on it, operating it across environments, and evolving it over time. The fifth covers the platform engineer who tailors Repo Radius for a team. This fifth user journey is largely deferred beyond the initial release. Each journey is made up of user stories that describe a single thing the user wants to accomplish. The journeys and their stories are detailed in the sections that follow.

Repo Radius is the backend for these journeys, not a user interface. A separate frontend (such as the Copilot app) turns the user's intent into GitHub Action workflow runs and shows the results. The user stories describe what the user wants to accomplish and what Repo Radius does on each operation; what the user sees and clicks is defined by the frontend.

## User Journey 1: Get started

User Journey 1 covers a developer taking a cloud-native application from a GitHub repository to a running, reachable deployment in their own cloud account for the first time. It spans connecting the repository to a cloud environment and then deploying the application and viewing what was created and how to reach it. The online storefront introduced above is the running example throughout these journeys.

This journey assumes a GitHub repository with GitHub Actions enabled, a well-formed application definition (`app.bicep`) already created and committed to the repository, and that the developer can create GitHub Environments and configure their cloud provider's identity federation (an Entra ID app registration for Azure, or an IAM OIDC identity provider and role for AWS).

### User Story 1.1: Connect my repository to a cloud environment

> As a developer, I want to connect my repository to a cloud account once, so that every deployment can provision resources securely without me managing credentials.

#### Summary

The developer chooses where the application should run (an AWS account and region, or an Azure subscription and resource group) and names the environment (for example, `dev`). Rather than storing long-lived cloud credentials, they set up OIDC identity federation so that every operation authenticates with short-lived tokens. The connection is verified end to end before any deployment, so misconfiguration surfaces during setup rather than on the first deployment.

#### User Experience

The developer works entirely through the frontend, which walks them through three steps:

1. **Enable Repo Radius for the repository.** The frontend asks the developer for permission to enable Radius for the repository. With the developer's acknowledgement, it commits the required GitHub Action workflows to the repository so that GitHub Actions has a dispatchable entry point for each Radius operation.

2. **Create an environment.** The frontend guides the developer through creating a GitHub Environment, prompting for the environment name (for example, `dev`), whether it targets AWS or Azure, the cloud account details (AWS account and region, or Azure subscription and resource group), and the workload cluster details (the EKS or AKS cluster name and Kubernetes namespace).

3. **Set up cloud access.** The frontend guides the developer through establishing the OIDC trust connection between GitHub and their cloud provider. The developer follows the prompts to create the trust on their cloud account, and the frontend captures the OIDC identifiers and stores them in the GitHub Environment.

Once these steps are complete, the frontend asks Repo Radius to verify the connection and reports to the user whether the environment is ready.

#### Result

Completing these steps produces the following, all stored in the developer's own GitHub account:

* **Two committed workflows** in `.github/workflows/`. Each is a thin wrapper that invokes a published Repo Radius GitHub Action pinned to a major version tag (for example, `@v1`), so a committed workflow only changes when the repository adopts a new major version.

   | Workflow | Action | Purpose |
   | --- | --- | --- |
   | `radius-verify-cloud-auth.yml` | `radius-project/verify-cloud-auth` | Verify the environment's OIDC configuration can authenticate to the cloud provider before a deployment is attempted. |
   | `radius-run-rad-commands.yml` | `radius-project/run-rad-commands` | Run one or more allowed `rad` commands against the environment. This single action performs every Radius operation (deploying, updating, deleting, and reading). |

* **A GitHub Environment** holding the environment's configuration as variables: the cloud provider details (AWS account and region, or Azure subscription and resource group), the workload cluster details (the EKS or AKS cluster name and Kubernetes namespace), and the OIDC identifiers (for Azure, the client ID, tenant ID, and subscription ID; for AWS, the IAM role ARN and region). Modeling each environment as a GitHub Environment reuses GitHub's native scoping, protection rules, and secret storage, and provides a single source of truth that both the workflows and the frontend consume.

* **An OIDC trust configuration** on the cloud provider: for Azure, a federated credential on an Entra ID app registration; for AWS, an IAM OIDC identity provider and an IAM role whose trust policy accepts GitHub Actions tokens. The trust is scoped to the repository (matching a subject such as `repo:my-org/my-repo:*`) rather than to an individual environment, so every environment in the repository that targets this cloud account shares it.

To confirm the environment is ready, the frontend dispatches `radius-verify-cloud-auth.yml`, which runs the `radius-project/verify-cloud-auth` action. The action requests a GitHub Actions OIDC token, exchanges it with the cloud provider, confirms the resulting short-lived credentials are valid, and verifies the identity holds the required permissions. When a Kubernetes cluster name is present in the environment, it also confirms the cluster's API server is reachable. It does not provision, modify, or delete anything.

The action creates a `verify-cloud-auth-result` artifact within GitHub Actions containing the verification result. This artifact is a small JSON document with the outcome of the action and a human-readable message with helpful remediation guidance when there is an error.

The frontend informs the developer of the results by reading the workflow's success or failure conclusion for a coarse signal, then calling the GitHub Actions API and examining the artifact for specifics. The artifact name and schema are a stable contract that the frontend can rely on. On `success`, the frontend reports that the environment is ready, and it is reused by every later deployment.

Because this trust is scoped to the repository and stores no secret, adding or renaming an environment that targets the same cloud account needs no cloud-side change, and there are no credentials to rotate. It changes in only three cases: the repository is renamed or moved, a new cloud account or subscription is targeted, or the identity's permissions must change. Authentication problems from later drift are handled in the troubleshooting flow (User Story 2.4).

#### Exceptions

The verify workflow reports one of the following failure outcomes in the `verify-cloud-auth-result` artifact, each with a human-readable message and remediation guidance:

| Outcome | Meaning |
| --- | --- |
| Required setting missing | A required variable for the selected cloud provider is absent such as AWS region or Azure resource group. |
| Cloud provider does not trust GitHub | The cloud provider rejected the GitHub OIDC token (trust policy or federated credential mismatch). |
| Identity lacks required permissions | Authentication succeeded, but the identity lacks the required permissions. Note that the required permissions will need to be documented for the user. |
| Kubernetes cluster cannot be reached | A Kubernetes workload cluster name was given, but the cluster was not found, or its API server is unreachable. |
| Cloud provider cannot be reached | The cloud provider's API endpoint could not be reached. |

In every case, the frontend is responsible for surfacing the outcome and its remediation guidance to the developer so they can correct the problem.

### User Story 1.2: Deploy my application and view the result

> As a developer, I want to deploy my application and see detailed information about what was deployed, so that I can trust Radius provisioned and wired everything together correctly.

#### Summary

With the application already modeled in `app.bicep`, the developer asks the frontend to deploy it. Repo Radius runs on demand inside a GitHub Actions runner, provisions the resources defined in `app.bicep` and shuts down. The developer never installs or configures Radius or learns each provider's resource types and arguments.

#### User Experience

The developer asks the frontend to deploy an application:

1. **Request the deployment.** The developer asks for the application to be deployed, selecting the environment by name. For example, the developer requests the storefront application to be deployed to the `dev` environment.

2. **Preview what will be created.** Before anything is provisioned, the frontend shows the cloud resources that will be created and asks the developer for confirmation. For example, since the `dev` environment is configured to deploy to Azure, the frontend shows the developer:

    | Application resource       | Cloud resource                                  |
    | -------------------------- | ----------------------------------------------- |
    | `frontend` container       | Kubernetes service, deployment, service account |
    | `api` container            | Kubernetes service, deployment, service account |
    | `orders-db` database       | Azure Database for PostgreSQL Flexible Server   |
    | `cache` Redis cache        | Azure Cache for Redis                           |
    | `orders-queue` Kafka queue | Azure Event Hubs                                |

    It is the frontend's responsibility to examine the application's resources and their recipes to produce this initial deployment plan; previewing changes to an already-deployed application is covered in User Story 2.2.

3. **Watch the deployment progress.** As the deployment runs, the frontend shows the status of each individual cloud resource. The developer sees the Azure Database for PostgreSQL Flexible Server move from pending to provisioning to succeeded (or failed), so progress is visible resource by resource rather than as a single opaque step.

4. **See each resource's identity.** When the deployment completes, the frontend shows the cloud provider resource ID for each provisioned resource. For the storefront's database, this is the Azure resource ID of the Flexible Server, for example `/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/storefront-dev/providers/Microsoft.DBforPostgreSQL/flexibleServers/storefront-orders-db`.

#### Result

The frontend deploys the application by dispatching the `radius-run-rad-commands.yml` workflow passing in the target environment name and the `rad deploy` command.

| Input | Value |
| --- | --- |
| `environment` | The GitHub Environment name |
| `ref` | The git commit or tag to deploy (defaults to the latest commit on the default branch). Not used in this user story but is used in User Story 3.1. |
| `rad_commands` | `['deploy .radius/app.bicep', 'app graph storefront -o json']` |

Only commands from an allowed set are accepted; each entry in `rad_commands` is validated before anything runs, so commands that do not fit the ephemeral, per-run model (for example, managing Radius resource groups or workspaces) are rejected up front rather than executing the workflow.

On dispatch, the action checks out the repository at the requested `ref` (or the latest commit if not specified) to obtain the application definition, authenticates to the cloud provider using the environment's OIDC variables, then creates a fresh ephemeral Radius control plane on the runner and loads the persisted data store into it. The control plane hosts Radius only; the application is deployed to the environment's own EKS or AKS cluster. The action uses the default recipe pack so the application's resources resolve without the developer authoring recipes, runs the requested `rad` commands in order (stopping at the first failure), and writes the updated data store back so the next operation resumes from it. This per-run control plane lifecycle is common to every `run-rad-commands` operation.

The run performs two commands: `rad deploy` provisions the resources, and `rad app graph` returns the application graph. The `rad app graph` output provides each resource's cloud provider resource ID, the connections between components, and the reachable endpoints, which the frontend surfaces so the developer can confirm everything was wired as modeled (User Experience step 4). On success, the action uploads a single `run-rad-commands-result` artifact whose `commands` array holds the output of the deployment and the application graph JSON (see Appendix 4 for an example). On a successful deployment, the action also records a GitHub Deployment for the environment at the deployed commit, so the version running in each environment is tracked in GitHub and can later be promoted to another environment (User Story 3.1). The frontend uses the streaming workflow logs to report incremental progress and the `run-rad-commands-result` artifact to report the final outcome to the developer.

This is the same standard deployment Radius performs today; Repo Radius does not change how Radius deploys applications. The only potential change to core Radius is adjusting the output of the `rad deploy` command so the frontend can collect incremental deployment progress updates.

#### Exceptions

The action reports one of the following failure outcomes in the `run-rad-commands-result` artifact:

| Outcome | Meaning |
| --- | --- |
| Command failed | A command did not complete; the remaining commands did not run, and the failed command's error output is included. |
| Command not allowed | A requested command is not in the allowed set; nothing ran. |
| Environment not found | The named GitHub Environment does not exist or has no configuration; nothing ran. |
| Cloud authentication failed | Cloud authentication failed before any command ran. |
| State could not be loaded | The persisted data store could not be read or restored; nothing ran. Note that this does not apply if this is the first deployment (no data store is expected then). Repo Radius will need a method of determining if a data store should be expected or not. |
| State could not be saved | The commands ran, but the updated data store could not be written back, so the cloud may contain resources the persisted state does not reflect. |

## User Journey 2: Iterate

User Journey 2 covers the developer's day-to-day inner loop once their application is running in an environment: inspecting what is deployed, previewing a change before applying it, redeploying the application as it evolves, and recovering when a deployment fails. Every story builds on the environment and application established in User Journey 1.

Except where noted, each operation below dispatches `radius-run-rad-commands.yml` through the same per-run lifecycle and failure outcomes described in User Story 1.2, so each story covers only its own `rad_commands` and any unique behavior.

### User Story 2.1: View my application's graph

> As a developer, I want to view the graph of my deployed application, so that I can see what cloud resources are deployed, how they are connected, and how to reach them.

#### Summary

The developer asks the frontend to show the application graph for an application in an environment. Repo Radius returns the graph in the `run-rad-commands-result` artifact.

#### User Experience

The developer asks to view the graph for a deployed application, selecting the environment and application by name. For example, the developer requests the graph for the storefront application in the `dev` environment, and the frontend shows:

* The resources that make up the application (the `frontend` and `api` containers, and the `orders-db` database, `cache`, and `orders-queue` backing services)
* The connections between them (the `api` connected to `orders-db`, `cache`, and `orders-queue`)
* Each resource's cloud provider resource ID and reachable endpoints

The graph reflects the state Radius recorded during the most recent operation on that environment; it is not a live query of the cloud and does not detect changes made outside Radius.

#### Result

The frontend retrieves the graph from the target environment with a single read-only command: `rad_commands: ['app graph storefront -o json']`. The frontend then shows the graph to the developer.

#### Exceptions

If the named application has not been deployed to the environment, the `rad app graph` command fails (the Command failed outcome), which the frontend surfaces as nothing to show.

### User Story 2.2: Review planned changes before deployment

> As a developer, I want to review the changes a deployment will make to an already deployed application before I apply them, so that I can catch unintended or destructive changes before they reach my cloud environment.

#### Summary

Before redeploying (User Story 2.3), the developer wants to see what a deployment will change: which resources it will add, change, or remove. This is analogous to `terraform plan` or `az deployment group what-if`. Producing a trustworthy plan means comparing the application's desired state (the edited `app.bicep`) against its actual deployed state and predicting how the deployment engine and recipes would resolve the difference between the two, including provider-level effects such as whether a change forces a resource to be replaced. The frontend cannot do this on its own; only Radius, which owns the deployment engine and recipe resolution, can produce an accurate what-if.

This preview-before-apply capability is central to deploying with confidence, and it is a significant new feature that is not specific to Repo Radius—self-hosted Radius needs the same what-if, and both versions of Radius would use it. Because it is a generalized feature, a standalone feature specification is required that defines the user experience and functionality. It is called out here so the iterate journey reflects where it belongs, and left undetailed pending that dedicated spec.

### User Story 2.3: Change my application and redeploy

> As a developer, I want to change my application and redeploy it, so that my running environment reflects my latest definition without me managing the individual resource changes.

#### Summary

The developer, working through the frontend, edits app.bicep and commits it to the repository (for example, they may add a resource or change a setting), then requests the application to be redeployed to an existing environment. Repo Radius runs the same deploy operation as the initial deployment (User Story 1.2). As with Radius today, deployment is incremental and does not delete resources removed from the application definition. The frontend is responsible for identifying when resources are removed and prompting the developer if they want to delete the individual resource. Note that Terraform automatically deletes resources after being removed from a definition and the `az` CLI offers the `--mode Complete` to accomplish the same. This is a limitation of Radius today.

#### User Experience

The redeploy experience is the same as the initial deployment (User Story 1.2). The difference is that only changed resources show activity. For example, if the developer added a full-text search service to the storefront application and changed the `orders-db` size, the redeploy provisions the new Azure AI Search resource and updates the database, while the `frontend` and `api` containers, `cache`, and `orders-queue` are reported as unchanged.

#### Exceptions

Because a redeploy may change several resources, a mid-deployment failure (the Command failed outcome) can leave the environment partially updated, with some changes applied and others not. The failed command's error output identifies where the redeployment stopped, and the developer can correct the definition and redeploy to converge or troubleshoot the failure (User Story 2.4).

### User Story 2.4: Troubleshoot or recover a failed deployment

> As a developer, I want to understand why a deployment failed and get back to a good state, so that a failed run doesn't leave me stuck or with half-provisioned infrastructure.

#### Summary

When a deployment fails, the developer needs to know what went wrong and how to get back to a good state. Repo Radius adopts core Radius's existing troubleshooting and failure recovery capabilities, along with its limitations. The same errors core Radius reports during a deployment are surfaced to the developer, and recovery follows the pattern Radius uses today: correct the problem and redeploy or delete and redeploy as a last resort. Repo Radius introduces one new failure mode, discussed below.

#### User Experience

Repo Radius communicates the existing Radius deployment errors straight through to the developer rather than interpreting or replacing them. When a command fails, its error output is captured and written into the `run-rad-commands-result` artifact showing the exact Radius error message. The developer therefore diagnoses from the same information they would see running `rad` locally, and routes to the appropriate recovery path:

* **Authentication or permissions drift.** If the failure is a cloud authentication or permissions error (for example, the trust or role changed since setup), the developer re-verifies the environment (User Story 1.1) which describes OIDC authentication and authorization errors in more detail, then redeploys.
* **Definition or recipe error.** If a resource failed because of the application definition or a recipe (for example, an invalid setting or an unsupported configuration), the developer corrects `app.bicep` and redeploys (User Story 2.3).
* **Partial or unsaved state.** If a run failed partway through, the developer redeploys to converge the environment or, when redeploying cannot bring it back to a good state, deletes the application and redeploys from scratch (User Story 3.2).

Diagnosing an application that deployed successfully but is not behaving correctly at runtime is done with the workload cluster's own tooling and is out of scope for Repo Radius.

#### New failure mode in Repo Radius

Because its control plane is ephemeral and its state is loaded at the start of a run and saved at the end rather than living in a persistent store, Repo Radius introduces one failure mode that self-hosted Radius does not have: the recorded state and the cloud can fall out of sync when a run fails to persist its data store. This has two consequences:

* **Orphaned resources.** If a run provisions resources but then fails to save the updated data store (the State could not be saved outcome), those resources exist in the cloud with no record in the persisted state. The control plane is torn down after the run, so nothing retains knowledge of them.
* **Stale starting state.** Because every run begins from whatever was last persisted, a run that ended without saving leaves the next run's view of the application behind reality before it starts, so it may try to recreate resources that already exist.

Mitigating this is a Repo Radius responsibility: the load and save around each run must be as reliable as possible, and a failed save must be surfaced clearly so the developer knows the cloud may hold resources the recorded state does not reflect. The design of the externalized data store and its load and save guarantees is covered in the technical design.

## User Journey 3: Operate across environments

User Journey 3 covers managing an application beyond a single environment: promoting a validated version from one environment to another and removing an application and its infrastructure when it is no longer needed. Both stories use the same `run-rad-commands` mechanism and shared convention introduced in User Journey 2.

### User Story 3.1: Promote my application to another environment

> As a developer, I want to promote the exact version of my application running in one environment to another, so that what I validated in a lower environment is what runs in production.

#### Summary

Promotion deploys the exact version of the application already running in a source environment (for example, `dev`) to a target environment (for example, `production`), so the target runs the identical definition the source was validated with. There is no dedicated promotion operation in Repo Radius. However, Repo Radius enables the frontend to offer promotion functionality by providing two primitives:

* Repo Radius can deploy a specific git commit passed as the `ref` (User Story 1.2).
* After each successful deployment, Repo Radius records a GitHub Deployment so each environment's deployed version is tracked.

The frontend performs the promotion by reading the commit currently deployed to the source environment from its GitHub Deployment and passing that commit as the `ref` for the target. Repo Radius itself does not look up what is deployed where. Because a version is identified by its git commit and tracked as a GitHub Deployment, the developer promotes without managing branches, pull requests, or re-pointing files, and the target environment's own configuration is applied so the same definition adapts to each environment it is promoted into.

#### User Experience

The developer asks the frontend to promote an application from a source environment to a target environment. For example, the developer promotes the storefront from `dev` to `production`. From there the experience is the same as an initial deployment (User Story 1.2): the frontend previews what will be deployed, the developer confirms, and progress is shown resource by resource. The difference is that the developer chooses a source environment to promote from rather than a version to deploy. Because `production` carries its own configuration, the promoted definition provisions production-scoped resources without any change to the application definition.

#### Result

The frontend reads the source environment's current GitHub Deployment to obtain the commit deployed there, then dispatches `radius-run-rad-commands.yml` for the target environment with that commit as the `ref` and the same deploy command as an initial deployment (User Story 1.2). The action checks out the promoted commit, deploys it against the target environment using that environment's own configuration, and returns the `run-rad-commands-result` artifact. On success, the action records a new GitHub Deployment for the target environment at the promoted commit, so every environment's deployed version is tracked, and promotions can chain across a pipeline (for example, `dev` to `test` to `production`).

#### Exceptions

Promotion reports the same failure outcomes as an initial deployment (User Story 1.2). In addition, if the source environment has no current GitHub Deployment, nothing has been successfully deployed to it yet, so there is no version to promote and the frontend reports that the source environment is empty.

### User Story 3.2: Remove my application and its infrastructure

> As a developer, I want to remove my application and its infrastructure from an environment, so that I can tear down what I no longer need and stop paying for it.

#### Summary

The developer asks the frontend to delete an application from an environment. The frontend dispatches the delete command `rad_commands: ['app delete storefront --yes']`, which deletes the application and its provisioned cloud resources. The frontend also marks the environment's GitHub Deployment inactive, so the environment is no longer recorded as running the application. Removal reports the same failure outcomes as User Story 1.2; a mid-deletion failure can leave some resources deleted and others still present, which the developer can retry or troubleshoot (User Story 2.4).

## User Journey 4: Evolve

### User Story 4.1: Upgrade Repo Radius

> As a developer, I want to keep Repo Radius up to date, so that I get fixes and improvements without performing manual upgrades.

Repo Radius ships as the two GitHub Actions (User Story 1.1), published to the GitHub Marketplace and versioned by major tag. The committed workflows reference each action by its major tag (for example, `radius-project/run-rad-commands@v1`), so every run automatically picks up the latest backward-compatible release of that major version without the developer changing anything.

A breaking change (to the workflow inputs, the allowed command set, or the result artifact schema) ships as a new major version (`@v2`). The existing `@v1` continues to work, so repositories are unaffected until they opt in. Adopting a new major version requires the user, or the frontend to update the tag in the workflow stored in the repository.

### User Story 4.2: Migrate to self-hosted Radius

> As a developer, I want to move my application from Repo Radius to a self-hosted Radius installation, so that I can grow beyond Repo Radius without rewriting my application.

Repo Radius runs the same Radius as a self-hosted installation and uses only the standard resource types that ship with Radius. Therefore an application definition is fully portable. A team that outgrows Repo Radius can deploy the same definition to a self-hosted Radius installation without modification. Limiting Repo Radius to only using the standard resource types is a limitation that is accepted to guarantee portability (this is a two-way door that can be revisited in the future).

## User Journey 5: Tailor Repo Radius for a team

User Journey 5 covers the platform engineer, not the developer. Both stories are largely out of scope for the initial release and are described here only to frame the intended direction.

### User Story 5.1: Customize which recipes are used by Repo Radius

> As a platform engineer, I want to customize the recipes Repo Radius uses, so that developer requests for databases, caches, and queues are provisioned according to my organization's standards.

The platform engineer wants a developer asking for a database, cache, or queue to get infrastructure that meets organizational standards for naming, tagging, network and security baselines, approved services and SKUs, and cost controls, rather than the out-of-the-box defaults. This requires replacing or extending the recipe pack Repo Radius registers on each run.

Customizing the recipe pack is out of scope for the initial release (see Non-goals). Every run uses the default AWS and Azure recipe pack that ships with Radius. In the future we imagine a platform engineer specifying an organization-wide recipe pack that Repo Radius applies in place of the default, so a developer's resource request is provisioned to the organization's standards without the developer doing anything differently. Because self-hosted Radius already supports custom recipes, this is mainly a matter of giving Repo Radius a place to reference the platform engineer's recipe pack.

### User Story 5.2: Predefine environments for developers

> As a platform engineer, I want to predefine the environments my developers deploy to, so that they inherit approved cloud targets and settings without configuring cloud access themselves.

The platform engineer wants to define the environments teams are allowed to deploy to once, with the correct cloud accounts, clusters, and guardrails, and have developers simply select from them rather than each configuring their own identity federation and cloud targets (User Story 1.1).

Because Repo Radius models each environment as a GitHub Environment (User Story 1.1), a platform engineer can already create and govern these environments using GitHub's native environment management and protection rules, and developers deploy to the ones that exist. A first-class experience for predefining a catalog of approved environments that developers select from, with organization-wide governance, is deferred and out of scope for this specification.

## Appendix 1: Requirements

The requirements below are a first pass at what the initial release must deliver, grouped by component. This is a starting point for planning, not a final list.

### Functional requirements

| ID | Component | Functional requirement |
| --- | --- | --- |
| FR1 | `verify-cloud-auth` action | Shall be published to the Marketplace, versioned by major tag. |
| FR2 | `verify-cloud-auth` action | Shall request a GitHub OIDC token, exchange it with AWS or Azure, confirm the short-lived credentials are valid, and verify the identity's permissions. |
| FR3 | `verify-cloud-auth` action | Shall confirm the workload cluster's API server is reachable when a cluster name is present. |
| FR4 | `verify-cloud-auth` action | Shall publish a `verify-cloud-auth-result` artifact with a versioned JSON schema (outcome, message, remediation) under a stable artifact name. |
| FR5 | `run-rad-commands` action | Shall be published to the Marketplace, versioned by major tag. |
| FR6 | `run-rad-commands` action | Shall check out the repository at the requested `ref` commit or tag before running commands. |
| FR7 | `run-rad-commands` action | Shall validate each entry in `rad_commands` against an allowed-command enum before executing anything. |
| FR8 | `run-rad-commands` action | Shall authenticate to AWS or Azure using the environment's OIDC variables. |
| FR9 | `run-rad-commands` action | Shall create an ephemeral Radius control plane (k3d cluster, `rad`, Radius) and tear it down after the run. |
| FR10 | `run-rad-commands` action | Shall deploy application workloads to the environment's external EKS or AKS cluster while the ephemeral control plane, which hosts Radius only, runs on the runner. |
| FR11 | `run-rad-commands` action | Shall create the Radius resource group and environment from the GitHub Environment variables and register the default recipe pack. |
| FR12 | `run-rad-commands` action | Shall run the requested `rad` commands in order, stop at the first failure, and capture each command's output. |
| FR13 | `run-rad-commands` action | Shall record a GitHub Deployment on a successful deploy and mark it inactive on delete. |
| FR14 | `run-rad-commands` action | Shall publish a `run-rad-commands-result` artifact with a versioned JSON schema and an ordered `commands` array under a stable artifact name. |
| FR15 | `run-rad-commands` action | Shall report a failed data-store save distinctly (the "State could not be saved" outcome) so the developer knows the cloud may hold resources the persisted state does not reflect. |
| FR16 | Slim workflows | Shall be provided as ready-to-commit `radius-verify-cloud-auth.yml` and `radius-run-rad-commands.yml` that invoke the actions at their major tag. |
| FR17 | Radius data store | Shall be externalized to GitHub-native storage, loaded at the start of each run and saved at the end (resource data, application graphs, deployment history, Terraform state). |
| FR18 | Radius CLI | `rad deploy` output shall be adjusted so the frontend can report resource-by-resource progress. |

**Deferred to a later release:** preview planned changes / what-if (User Story 2.2), custom recipe packs (User Story 5.1), a predefined environment catalog (User Story 5.2), and state migration to self-hosted Radius (User Story 4.2).

### Non-functional requirements

| ID | Category | Non-functional requirement |
| --- | --- | --- |
| NFR1 | Performance | Control plane startup shall be fast enough that per-run latency is acceptable, since startup is on the critical path for every operation. |
| NFR2 | Security | Deployments shall authenticate with short-lived, OIDC-issued cloud credentials and shall not store long-lived cloud secrets. |
| NFR3 | Security | The cloud identity shall be scoped to the least privilege required and isolated per repository (and per cloud account or subscription to separate production from non-production) to limit blast radius. |
| NFR4 | Reliability | The data store load and save around each run shall be reliable and atomic, so a failed run does not silently corrupt or desync state. |

## Appendix 2: Dependencies and risks

**Dependency: Frontend.** Repo Radius is a backend; it requires a separate frontend (such as the Copilot app) to turn user intent into workflow dispatches and present results. The frontend is out of scope here but is a hard dependency for an end-to-end experience.

**Dependency: External workload cluster support.** Deploying workloads to a cluster separate from the ephemeral control plane is delivered by the [Deploy to External AKS and EKS Clusters](../environments/2026-05-external-kubernetes.md) feature, which Repo Radius depends on.

**Risk: Ephemeral startup time.** The control plane is created and torn down on every operation, so startup is on the critical path for every user-facing action. If it takes minutes, the experience degrades badly, making startup time the primary determinant of perceived responsiveness.

**Risk: Broad cloud permissions and blast radius.** The OIDC-assumed IAM role (AWS) or app registration (Azure) must hold enough permission to create and destroy the full range of resources an application may need. A role this broad has a high blast radius: a misbehaving recipe, a compromised workflow, or an over-scoped command could affect a wide range of cloud resources. Scoping permissions to the minimum and isolating them per environment is an important mitigation.

**Risk: Recipe and recipe pack backward compatibility.** Because Repo Radius is stateless and re-resolves resources against whatever recipe pack is in effect at each run, a newer recipe version must remain backward compatible with resources provisioned by an earlier version; a breaking recipe change could fail to apply, orphan, or destroy already-deployed resources. Recipe versioning needs the same discipline as the action contract.

## Appendix 3: Alternatives considered

### Multiple operation-specific actions versus a single generic action

We considered publishing a separate action for each operation (for example, `radius-deploy`, `radius-destroy`, `radius-app-graph`) instead of the single `run-rad-commands` action.

* **Multiple operation-specific actions**. Each action exposes a narrow, strongly typed input surface (for example, `radius-deploy` takes an application path; `radius-destroy` takes an application name), so the contract is self-documenting and each action validates only the inputs relevant to it. The drawback is that every new operation requires a new action, a new workflow file committed to the repository, and a new major-version lifecycle to maintain. Composing several operations in one run (deploy, then read the graph) requires multiple workflow dispatches and multiple ephemeral control plane startups, multiplying the per-run startup cost.
* **Single generic action (recommended)**. One action runs an ordered list of allowed `rad` commands. A single ephemeral control plane is created once per run and reused across all commands, so multi-step flows (deploy, then `app graph`) pay the startup cost once. Adding a new operation usually means allowing a new command in the enum rather than publishing a new action and workflow. The trade-off is a less strongly typed input surface, which is mitigated by validating every command against the allowed-command enum before anything runs.

**Recommendation**: The single generic `run-rad-commands` action. It minimizes per-run startup cost for multi-step flows, keeps the committed workflow count small, and lets new operations ship as backward compatible additions to the allowed-command enum rather than as new actions. It also reduces the surface area in the user's repository to a single, highly flexible workflow that can accommodate future functionality: only the published action (which lives outside the user's repository) needs to be updated with new allowed commands, so the user's repository does not need to change.

### Per-command result artifacts versus a single combined artifact

We considered uploading a separate result artifact per command instead of one combined `run-rad-commands-result` artifact.

* **Per-command artifacts**. Each command's output is uploaded as its own artifact (for example, `rad-command-0`, `rad-command-1`), so the frontend can download a single command's output without retrieving the others. The drawback is that the artifact set varies with the number and order of commands, so the frontend must discover artifact names dynamically, correlating outputs back to the requested order is awkward, and a run produces many small artifacts.
* **Single combined artifact (recommended)**. One `run-rad-commands-result` artifact contains a `commands` array with an entry per command, in input order, each with the command, exit code, and output. The artifact name is stable and known in advance, the ordering is explicit, and a single download yields the full result of the run.

**Recommendation**: The single combined `run-rad-commands-result` artifact. A stable, well-known artifact name with an ordered `commands` array is simpler for the frontend to consume and keeps the contract small.

### Environments defined in a repository file versus GitHub Environments

We considered letting the user define multiple environments in a checked-in file (for example, an `env.bicep` in the repository) instead of modeling each environment as a GitHub Environment.

* **Environments defined in a repository file**. A single `env.bicep` declares every environment (dev, test, production) and its cloud provider and Kubernetes cluster details in one place under version control, so the full set of environments is reviewable in a pull request and lives alongside the application. The drawback is that it duplicates a concept GitHub already provides. Cloud credentials must still be stored as GitHub Environment secrets, so an env.bicep would split related configuration across two locations: non-secret details in the file and secrets in the environment.
* **GitHub Environments (recommended)**. Each environment is a GitHub Environment whose variables hold the cloud provider and Kubernetes cluster details. This reuses GitHub's native environment model including scoped secrets. The downside is that GitHub Environments are not easily created in bulk or declaratively; each environment and its variables are configured imperatively through the GitHub UI, the `gh` CLI, or the REST API rather than from a single checked-in file. The offsetting upside is that users likely already know how to create GitHub Environments and may already have tooling or automation for managing them.

**Recommendation**: Model environments as GitHub Environments only. It reuses GitHub's native scoping, protection, and OIDC integration rather than reinventing them in a repository file, and it keeps a single source of truth for environment configuration that the workflows and the frontend already consume.

### Repository-scoped versus per-environment OIDC trust

We considered scoping the cloud trust relationship to each individual GitHub Environment (matching the environment in the token's subject claim) instead of scoping it to the repository.

* **Per-environment scope**. The AWS IAM role trust policy or Azure federated credential matches an exact subject such as `repo:my-org/my-repo:environment:production`, giving each environment its own trust and the tightest isolation between environments. The drawback is that every new environment requires a corresponding cloud-side change (a new trust condition or federated credential), which does not scale and, on Azure, quickly reaches the limit on federated credentials per app registration. It also pushes cloud configuration back onto the developer for a routine action (adding an environment) that should be self-service in GitHub.
* **Repository scope (recommended)**. The trust condition matches any subject from the repository (for example, `repo:my-org/my-repo:*`), so a single trust relationship and role cover every environment in the repository. Adding or renaming an environment that targets the same cloud account is purely a GitHub-side action and requires no cloud change. The trade-off is coarser isolation: all environments in the repository can assume the same cloud identity, so separation between non-production and production is achieved by targeting a different cloud account or subscription (each with its own trust and role) rather than by the trust condition alone.

**Recommendation**: Repository-scoped trust. It keeps adding environments a self-service, GitHub-only action, avoids per-environment cloud configuration and Azure's federated-credential limits, and matches how teams already separate production from non-production, by using a distinct cloud account or subscription. Where stronger isolation between environments in the same account is required, a team can still create a separate role and repository-scoped trust per account.

## Appendix 4: Example Result Artifact

Below is an example run-rad-commands-result artifact from the prototype. This example shows the workflow being run with three commands: rad resource list -o json, rad app show todo-list -o json, and rad app graph todo-list -o json. Note that the second commands failed and the third did not execute.

```json
{
  "schemaVersion": "1.0",
  "outcome": "command_failed",
  "exitCode": 1,
  "environment": "production",
  "commandsRequested": 3,
  "commandsRan": 2,
  "startedAt": "2026-06-29T18:42:01Z",
  "completedAt": "2026-06-29T18:42:37Z",
  "durationSeconds": 36,
  "message": "1 of 3 commands failed. Remaining commands were not run.",
  "commands": [
    {
      "index": 0,
      "command": "resource list -o json",
      "artifact": "rad-command-0",
      "outcome": "succeeded",
      "exitCode": 0,
      "startedAt": "2026-06-29T18:42:05Z",
      "completedAt": "2026-06-29T18:42:11Z",
      "durationSeconds": 6,
      "stdout": "[{\"id\":\"/planes/radius/local/resourceGroups/default/providers/Applications.Core/containers/frontend\",\"name\":\"frontend\",\"type\":\"Applications.Core/containers\"}]",
      "stderr": "",
      "errorMessage": null
    },
    {
      "index": 1,
      "command": "app show todo-list -o json",
      "artifact": "rad-command-1",
      "outcome": "failed",
      "exitCode": 1,
      "startedAt": "2026-06-29T18:42:11Z",
      "completedAt": "2026-06-29T18:42:37Z",
      "durationSeconds": 26,
      "stdout": "",
      "stderr": "Error: application \"todo-list\" not found in environment \"production\"\nTraceId: 5ec67312f45304c527560e39433eff84",
      "errorMessage": "application \"todo-list\" not found in environment \"production\""
    },
    {
      "index": 2,
      "command": "app graph todo-list -o json",
      "artifact": null,
      "outcome": "skipped",
      "exitCode": null,
      "startedAt": null,
      "completedAt": null,
      "durationSeconds": null,
      "stdout": null,
      "stderr": null,
      "errorMessage": "Not run: a prior command failed."
    }
  ]
}
```