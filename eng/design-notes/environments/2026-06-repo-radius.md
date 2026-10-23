<!--
Structure based on the Radius feature specification template:
https://github.com/radius-project/radius/blob/main/eng/design-notes/templates/feature-spec-template.md
-->

# Topic: Repo Radius

- **Author**: Will Tsai (@willtsai)

## Topic Summary

Repo Radius lets teams deploy and manage applications through their existing CI/CD pipelines without operating a permanent Radius control plane. A job can start a temporary control plane or connect to a centralized service that the platform team already runs.

Radius and the application need not run in the same place. A control plane hosted on Kubernetes can deploy workloads to a separate Kubernetes cluster or a managed container environment. Teams should be able to use GitHub Actions or Azure DevOps to deploy to Azure Kubernetes Service (AKS), Amazon Elastic Kubernetes Service (EKS), Azure Container Apps (ACA), or Amazon Elastic Container Service (ECS). These are intended targets; not every combination is available today.

### Top level goals

- Run Radius operations unattended in existing CI/CD pipelines without requiring a particular repository host or frontend.
- Let teams choose a temporary control plane or an existing centralized control plane.
- Keep Radius configuration and deployment state after jobs end so later runs can reuse approved environments and recipes and manage existing applications.
- Deploy workloads to the team's chosen Azure or AWS platform, regardless of where the Radius control plane runs.
- Show developers deployment results and how to recover from failures without crossing the platform team's environment and access boundaries.

### Non-goals (out of scope)

- Selecting workflow files, packaging, storage technologies, authentication protocols, or result schemas. These belong in technical designs.
- Replacing CI/CD systems, their approval processes, or the tools used to monitor running applications.
- Defining release management or promotion between environments. Teams keep these workflows in their existing pipelines.
- Requiring a new graphical interface or coding assistant to complete a deployment.
- Installing, upgrading, or operating an organization's centralized Radius service.
- Migrating existing application state and resource ownership between independent control planes. Connecting to an existing control plane is not a migration.
- Adding a general-purpose deployment what-if engine, automatic rollback, or continuous cloud drift detection.

## User profile and challenges

### User persona(s)

**Platform engineer.** Sets deployment standards, controls cloud access, and provides environments for application teams. Some platform engineers run a shared Radius service; others want Radius to run only during a pipeline job. Both need Radius to work within their existing delivery practices.

**Application developer.** Maintains an application and its declared dependencies. Uses the team's pipeline to deploy, inspect, update, and remove the application without having to learn how to host Radius or configure each cloud service.

### Challenge(s) faced by the user

- Maintaining a permanent control plane adds work before a team can deploy its first application.
- A disposable job can lose its local state before the next deployment.
- A pipeline-specific integration can force teams to change CI/CD systems or duplicate environment configuration.
- Requiring workloads to run beside Radius prevents teams from choosing a different cluster or a managed container platform.
- Teams with a centralized Radius installation need to reuse it rather than create another control plane for every job.
- Failed or interrupted jobs can leave resources partly updated, and developers need to know what remains before retrying.

### Positive user outcome

A platform engineer configures the pipeline's connection to Radius and the environments it may manage. Developers use that pipeline to deliver applications without managing the control plane. Each job reads the application's state, targets the selected environment, and saves enough information for the next job or an operator to continue.

A fresh job can update an earlier deployment. A job using a shared control plane leaves that service running. Neither mode requires application workloads to share the control plane's cluster.

## Key scenarios

### Scenario 1: Run Radius inside an existing CI/CD pipeline

> As a platform engineer, I want to run Radius for the duration of a CI/CD pipeline job, so that I can automate application deployment without operating a permanent Radius control plane.

The platform engineer adds Radius operations to an existing pipeline, such as GitHub Actions or Azure DevOps, and chooses a temporary control plane. The job starts Radius, selects the environment and its saved state, and runs the requested operations without interactive prompts.

- The job reports when Radius is ready, which application and environment it targets, and whether the requested operations succeeded.
- When the job finishes normally, it releases the temporary control plane without deleting deployed workloads or their persistent state.
- A later job can run on a different agent without depending on files left on the previous agent.
- If Radius fails to start, the job stops before running the requested operations and tells the platform engineer what to correct. If the job is interrupted, the developer follows the recovery steps in Scenario 6.

### Scenario 2: Keep Radius configuration and deployment state independently of the pipeline

> As a platform engineer, I want each temporary Radius instance to use a centrally managed persistent state store, so that later jobs can reuse our environments and recipes and manage resources created by earlier runs.

The platform engineer configures persistent state for Radius. The store preserves environment definitions, registered recipe packs, their associations and settings, and application and resource records. Each new job recovers this configuration and deployment state instead of starting from scratch. The state outlives the agent and does not require storage from a particular CI/CD vendor.

- A fresh job can use the saved environments and recipe configuration without requiring the platform engineer to recreate them. Scenario 5 depends on this saved configuration.
- Radius saves recipe sources and configuration in recipe registrations. The Bicep or Terraform modules they reference can stay in external registries or repositories; the state store need not contain copies of those modules.
- Recovering saved configuration does not grant access. The job still needs credentials and network access and can use only the environments and resources the platform team authorizes.
- Later jobs can inspect, update, or remove the application's recorded resources after the original job and temporary control plane are gone.
- The job distinguishes an explicitly new environment from an existing environment whose state is missing or inaccessible. It does not silently replace missing state with an empty deployment.
- Jobs targeting the same state cannot silently overwrite one another's changes. A conflicting job waits or reports why it cannot proceed.
- If a job changes cloud resources but cannot save the resulting state, it reports failure and warns that the recorded state may no longer match the cloud resources.

When a pipeline connects to an existing centralized control plane, that service keeps managing its configuration and deployment state. The job uses the environments and recipes already registered there, subject to its permissions, rather than creating a separate copy to manage the same application.

### Scenario 3: Deploy to the team's chosen Azure or AWS container platform

> As an application developer, I want Radius to deploy my application to the Azure or AWS container platform selected by our platform team, so that adopting Radius does not force my project onto Kubernetes.

The developer selects an approved workload environment, separate from the location where Radius runs. The environment can target an external AKS or EKS cluster, or a managed container platform such as ACA or ECS. Kubernetes workloads still need a workload cluster. Managed container workloads do not need one just because Radius runs on Kubernetes.

- The job identifies the workload destination separately from the Radius control plane it uses.
- Radius deploys the application and its dependencies to the selected destination, not to a temporary control plane's cluster.
- Stopping a temporary control plane does not stop the application.
- If the target cannot support a requested application capability, the job identifies the unsupported capability rather than silently selecting another platform.

A centralized control plane also lets teams choose a separate workload destination. Settings and supported capabilities may differ across target platforms.

### Scenario 4: Connect a pipeline to an existing centralized Radius control plane

> As a platform engineer, I want a CI/CD job to connect to an existing external Radius control plane and run Radius commands against it, so that teams can use the Radius service we already operate.

The platform engineer selects the existing service and target Radius environment, then supplies the approved connection and access settings. The job connects to that service and runs the requested operations against the state the service already manages.

- The pipeline does not start another control plane or require the agent to host Kubernetes for Radius.
- The job identifies the connected service and target environment before making changes.
- If the connection or authorization fails, the job stops the operation and explains what to correct. It does not fall back to a new local control plane or a different environment.
- Whether the job completes, fails, or is cancelled, it leaves the centralized service, its state, and unrelated applications intact.
- The job can access only the applications and environments the platform team authorizes.

Connecting to this service does not transfer applications from another control plane. The service must already manage the application, or the job must create a new application there.

### Scenario 5: Use approved environments and existing infrastructure

> As a platform engineer, I want pipeline deployments to use our approved environments and deployment recipes, so that developers can deliver applications without recreating our infrastructure standards for each project.

The platform engineer configures approved environments and recipe packs in Radius and gives the pipeline the access it needs. Scenario 2 preserves that configuration across temporary jobs; jobs connected to a centralized control plane use the configuration that service already manages. The platform engineer does not recreate environments or register recipes for each run. Developers select an authorized environment and declare what their application needs. They do not configure control-plane hosting, handle deployment credentials manually, or replace the team's existing delivery approvals.

- Jobs using either a temporary or a centralized control plane can use the team's approved recipe configuration, including recipes backed by existing infrastructure modules.
- A deployment can reference existing foundation resources, such as a container hosting environment, storage account, or messaging service, and add application-owned resources within them.
- Teams can apply environment-specific settings and supported application settings, such as scaling configuration, without duplicating the application definition.
- Deployments preserve the approved networking and project isolation. If access or configuration is missing, the job explains what to correct instead of broadening access or choosing a different target.
- Referencing a shared resource does not make the application its owner. Removing the application leaves those shared resources intact.

The platform team keeps its existing environment and recipe management practices. It does not need a new organization-wide catalog or a fleet-wide update service.

### Scenario 6: Recover from failures and remove an application safely

> As an application developer, I want to understand a failed deployment and safely retry or remove my application, so that a failed job does not leave me guessing which resources still exist.

The developer reads the job result to see what succeeded, what failed, and whether Radius saved the application state. After correcting the problem, they can retry against the same environment. If an operator must reconcile the recorded state with cloud resources first, the result says so. It does not promise automatic rollback.

- Failure details remain available after the agent or temporary control plane disappears.
- The result never reports a cancelled job as a successful deployment. If an operation's outcome is unknown, the result says so and directs the developer to check it before retrying.
- The developer can explicitly request that Radius remove an application from a named environment, subject to the team's approval rules.
- Radius deletes application-owned resources while preserving referenced foundations, other applications, and any centralized control plane.
- If Radius removes only part of an application, the result identifies what remains so a later operation can continue. Stopping Radius alone does not remove an application.

## Key dependencies and risks

- **Workload platform support.** Each target needs recipes that provide the requested application capabilities. Users need a list of supported platforms and errors that identify unavailable capabilities.
- **Network and access configuration.** Jobs and control planes must be able to reach the services they need, including persistent state and deployment targets. Teams using private networks must not have to expose services publicly to adopt Repo Radius.
- **State and interrupted operations.** A failed job can change cloud resources without recording those changes. Developers need saved state and instructions for recovery before they can safely retry.
- **Shared-service boundaries.** Jobs using a centralized control plane must not disrupt other teams or exceed their authorized environments.
- **Compatibility.** A pipeline and its selected control plane or recipes may be incompatible. The error must identify the incompatibility and explain what the user can do about it.
- **Job startup time.** Starting a temporary control plane delays the requested operation. Test how much delay teams will accept in routine deployments.

## Key assumptions to test and questions to answer

- Which CI/CD and workload-platform combinations must the first release support? Test the GitHub Actions and Azure DevOps scenarios with AKS, EKS, ACA, and ECS before deciding.
- Which persistent-state services do prospective users already know how to operate? Confirm their access, retention, and recovery requirements with platform engineers before choosing a storage technology.
- What connection and permission setup will teams accept for a centralized control plane, especially on private networks? Test an unattended job with restricted access to one environment.
- Which recipe configurations and uses of existing resources must work with both temporary and centralized control planes? Test with an application that uses an existing hosting environment and shared backing services.
- What should users see when a job is cancelled while a centralized service is still executing an operation? Define when work can stop and how a later job discovers its outcome.
- What startup delay is acceptable, and how should a team learn that its pipeline, Radius service, or recipes need an update? Validate with representative deployment and inspection jobs.

## Current state

The earlier Repo Radius proposal called for running Radius temporarily through GitHub Actions, with GitHub-specific environment and state handling. Repo Radius must also work with other CI/CD systems and connect to an existing centralized control plane. These requirements do not imply that Radius supports those scenarios today.

The [external Kubernetes deployment proposal](./2026-05-external-kubernetes.md) covers workloads running on a cluster separate from Radius. It addresses the Kubernetes case; managed container targets also need suitable platform support.

## Details of user problem

As a platform engineer, I already have pipelines, environment boundaries, and deployment standards. I want Radius to work within them. My team may not want another permanent service to maintain. Or we may already run Radius and should not need a second instance just for CI/CD. Either way, replacing a disposable agent must not lose our deployment history or change where applications run.

As an application developer, I want to describe my application and deliver it through the same pipeline as the rest of my code. I need to know where it runs, whether the change succeeded, and how to recover if it failed. I should not need to know where the Radius control plane runs or how long it lives.

## Desired user experience outcome

I can use Radius from my existing pipeline by starting it for a job or connecting to the service my platform team provides. I can deploy to the approved Azure or AWS environment, inspect the result after the job ends, and update the application from a fresh job. If an operation fails, I can see what happened and what to do next without losing track of the application.

### Detailed user experience

1. **Configure the pipeline once.** The platform engineer chooses a temporary control plane or an existing Radius service, provides approved access, and configures the target environments and recipe packs in Radius. For a temporary control plane, the engineer also configures persistent state so later jobs recover this configuration along with application and resource records.
2. **Select the application and destination.** The developer supplies the application revision, operation, and environment through the normal pipeline. Existing review and approval rules still apply.
3. **Confirm readiness.** The job identifies the selected control plane and workload destination. If configuration is missing, required state is inaccessible, or a connection fails, the job stops the operation and explains why.
4. **Run the operation.** The pipeline runs existing Radius commands against the selected control plane and environment. The job reports progress and failures.
5. **Review the outcome.** The developer sees the revision, target, resource results, and any state or recovery warnings. The developer can still inspect the application after the job ends.
6. **Finish without affecting workloads.** The job releases a temporary control plane or leaves a centralized service running. Deployed applications and their state remain available for the next operation.
7. **Run later commands.** A later job uses existing Radius commands to update, inspect, or explicitly remove the application. The developer checks an interrupted or failed operation before retrying.

## Key investments

### Feature 1

**CI/CD execution choices.** Let teams run Radius for a job or connect to a centralized service. Jobs run unattended and do not require a particular CI/CD vendor or frontend.

### Feature 2

**Persistent Radius configuration, deployment state, and recovery.** Let fresh jobs reuse saved environments and recipe configuration and manage existing applications. Prevent conflicting updates from silently losing that state, and tell developers when an operation's outcome is unknown.

### Feature 3

**Independent workload targets and approved configuration.** Let teams deploy to supported Azure and AWS container platforms using their environment settings and recipes, regardless of where Radius runs.

### Feature 4

**Command results and recovery.** Show the outcome of existing Radius commands in the pipeline and help developers recover from failed or interrupted jobs. Keep this behavior consistent across both control-plane modes.
