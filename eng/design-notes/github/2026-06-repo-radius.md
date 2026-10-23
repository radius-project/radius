<!--
Structure based on the Radius feature specification template:
https://github.com/radius-project/radius/blob/main/eng/design-notes/templates/feature-spec-template.md
-->

# Topic: Repo Radius

- **Author**: Zach Casper (@zachcasper)

## Topic Summary

Repo Radius lets teams use Radius from their existing CI/CD pipelines without requiring every team to operate a permanent Radius control plane. A pipeline can start a temporary control plane for a job or connect to a centralized control plane that the platform team already operates. Both paths let developers deploy and manage applications through the delivery process they already use.

Where Radius runs does not determine where the application runs. A control plane hosted on Kubernetes can deploy workloads to a separate Kubernetes cluster or a managed container environment. Teams should be able to use GitHub Actions or Azure DevOps to deploy to Azure Kubernetes Service (AKS), Amazon Elastic Kubernetes Service (EKS), Azure Container Apps (ACA), or Amazon Elastic Container Service (ECS). These are intended targets; not every combination is available today.

### Top level goals

- Run Radius operations unattended in existing CI/CD pipelines without requiring a particular repository host or frontend.
- Let teams choose a temporary control plane or an existing centralized control plane.
- Preserve deployment state independently of disposable jobs so later runs can manage existing applications.
- Deploy workloads to the Azure or AWS platform selected by the team, independently of where the Radius control plane runs.
- Give developers clear deployment results and recovery guidance while preserving the platform team's environment and access boundaries.

### Non-goals (out of scope)

- Selecting workflow files, packaging, storage technologies, authentication protocols, or result schemas. These belong in technical designs.
- Replacing CI/CD systems, their approval processes, or the tools used to monitor running applications.
- Requiring a new graphical interface or coding assistant to complete a deployment.
- Installing, upgrading, or operating an organization's centralized Radius service.
- Migrating existing application state and resource ownership between independent control planes. Connecting to an existing control plane is not a migration.
- Adding a general-purpose deployment what-if engine, automatic rollback, or continuous cloud drift detection.

## User profile and challenges

### User persona(s)

**Platform engineer.** Owns deployment standards, cloud access, and the environments available to application teams. Some platform engineers operate a shared Radius service; others want Radius to exist only while a pipeline job runs. Both need to fit Radius into established delivery practices.

**Application developer.** Maintains an application and its declared dependencies. Wants to deploy, inspect, update, and remove it through the team's pipeline without learning how to host Radius or configure each cloud service.

### Challenge(s) faced by the user

- Operating a permanent control plane adds maintenance before a team can deploy its first application.
- Disposable jobs cannot rely on local state surviving until the next deployment.
- A pipeline-specific integration can force teams to change CI/CD systems or duplicate environment configuration.
- Requiring workloads to run beside Radius prevents teams from choosing a different cluster or a managed container platform.
- Teams with a centralized Radius installation need to reuse it rather than create another control plane for every job.
- Failed or interrupted jobs can leave resources partly updated, and developers need to know what remains before retrying.

### Positive user outcome

A platform engineer configures how a pipeline reaches Radius and which environments it may manage. Developers then use that pipeline to deliver applications without managing the control plane. Each job finds the right application state, targets the intended workload environment, and leaves enough information for the next job or an operator to continue.

A fresh job can update an earlier deployment. A job using a shared control plane leaves that service running. Neither mode requires application workloads to share the control plane's cluster.

## Key scenarios

### Scenario 1: Run Radius inside an existing CI/CD pipeline

> As a platform engineer, I want to run Radius for the duration of a CI/CD pipeline job, so that I can automate application deployment without operating a permanent Radius control plane.

The platform engineer adds Radius operations to an existing pipeline, such as GitHub Actions or Azure DevOps, and chooses to run Radius temporarily. The job starts Radius, selects the environment and its saved state, and runs the requested operations without interactive prompts.

- The job reports when Radius is ready, which application and environment it is targeting, and whether the requested operations succeeded.
- Normal job completion releases the temporary control plane without deleting deployed workloads or their persistent state.
- A later job can run on a different agent without depending on files left on the previous agent.
- Startup failure stops the requested operations and identifies what the platform engineer needs to correct. An interrupted job follows the recovery experience in Scenario 7.

### Scenario 2: Keep deployment state independently of the pipeline

> As a platform engineer, I want each temporary Radius instance to use a centrally managed persistent state store, so that later deployments can find and update resources created by earlier runs.

The platform engineer configures persistent state for the project and environment. A fresh job resumes from the recorded deployment rather than treating each run as a first deployment. The state does not depend on the lifetime of an agent or on storage supplied by a particular CI/CD vendor.

- Subsequent jobs can inspect, update, or remove the application's recorded resources after the original job and temporary control plane are gone.
- The job distinguishes an explicitly new environment from an existing environment whose state is missing or inaccessible. It does not silently replace missing state with an empty deployment.
- Jobs targeting the same state cannot silently overwrite one another's changes. A conflicting job waits or reports why it cannot proceed.
- If a job changes cloud resources but cannot preserve the resulting state, it reports failure and warns that recorded state may not reflect the cloud.

When a pipeline connects to an existing centralized control plane, that service remains responsible for its state. The job does not create a separate copy to manage the same application.

### Scenario 3: Deploy to the team's chosen Azure or AWS container platform

> As an application developer, I want Radius to deploy my application to the Azure or AWS container platform selected by our platform team, so that adopting Radius does not force my project onto Kubernetes.

The developer selects an approved environment rather than the location where Radius happens to run. That environment can target an external AKS or EKS cluster, or a managed container platform such as ACA or ECS. Kubernetes-based workloads still need a workload cluster; managed container workloads do not require a Kubernetes workload cluster merely because Radius uses one.

- The job identifies the workload destination separately from the Radius control plane it uses.
- The application and its dependencies deploy to the selected destination, not to a temporary control plane's cluster.
- Stopping a temporary control plane does not stop the application.
- If the target cannot support a requested application capability, the job identifies the unsupported capability rather than silently selecting another platform.

The same separation applies when the pipeline uses a centralized control plane. Platform-specific settings may differ; the experience does not promise identical capabilities across all targets.

### Scenario 4: Connect a pipeline to an existing centralized Radius control plane

> As a platform engineer, I want a CI/CD job to connect to an existing external Radius control plane and execute Radius commands against it, so that teams can use the Radius service we already operate.

The platform engineer selects the existing service, supplies the approved connection and access configuration, and identifies the target Radius environment. The job connects to that service and runs the requested operations against the state it already manages.

- The pipeline does not start another control plane or require the agent to host Kubernetes for Radius.
- The job identifies the connected service and target environment before making changes.
- A connection or authorization failure stops the operation with guidance. The pipeline does not fall back to a new local control plane or a different environment.
- Job completion, failure, or cancellation does not shut down the centralized service, discard its state, or delete unrelated applications.
- The job's access remains limited to the applications and environments authorized by the platform team.

Selecting this mode does not transfer applications from another control plane. The target service must already manage the application, or the job must be creating a new application there.

### Scenario 5: Use approved environments and existing infrastructure

> As a platform engineer, I want pipeline deployments to use our approved environments and deployment recipes, so that developers can deliver applications without rebuilding our infrastructure standards for each project.

The platform engineer makes approved targets, access settings, and recipes available to the pipeline. Developers select an environment and declare what their application needs. They do not configure control-plane hosting, handle deployment credentials manually, or replace the team's existing delivery approvals.

- Both execution modes can use the team's approved recipe configuration, including recipes backed by existing infrastructure modules.
- Application deployments can reference existing foundation resources, such as a container hosting environment, storage account, or messaging service, and add application-owned resources within them.
- Environment-specific settings and supported application settings, such as scaling configuration, apply without duplicating the application definition.
- Deployments preserve the approved networking and project isolation. Missing access or configuration produces an actionable failure rather than broader access or a different target.
- Referenced resources remain distinct from resources owned by the application, so removing an application does not remove shared foundations.

The platform team keeps its existing environment and recipe management practices. It does not need a new organization-wide catalog or a fleet-wide update service.

### Scenario 6: Deploy, inspect, and promote an application

> As an application developer, I want to deploy my application and its dependencies through our pipeline, inspect the result, and promote a tested revision to another environment, so that I can deliver changes without managing each cloud resource separately.

The developer commits an application change and runs the normal delivery pipeline. The job identifies the application revision and target environment, then reports progress and the result. The developer can inspect the recorded application resources, their connections, and available endpoints after the job ends.

- A later deployment updates the existing application using the state associated with the selected environment.
- Inspection distinguishes Radius's last recorded state from a live view of the cloud; it does not claim to detect changes made outside Radius.
- Promotion selects an explicit application revision and applies the destination environment's settings and approvals. It does not substitute the latest revision or promise identical infrastructure across environments.
- Removing a resource from the application definition does not imply that Radius has deleted it. Resources that require explicit removal remain visible until removed.
- Developers can perform these operations through the pipeline without a separate graphical frontend.

### Scenario 7: Recover from failures and remove an application safely

> As an application developer, I want to understand a failed deployment and safely retry or remove my application, so that a failed job does not leave me guessing which resources still exist.

The developer reviews the job result to see what succeeded, what failed, and whether Radius preserved the application state. After correcting the problem, they can retry against the same environment. The result explains when an operator must reconcile recorded state with cloud resources before a retry; it does not promise automatic rollback.

- Failure details remain available after the agent or temporary control plane disappears.
- A cancelled job is not reported as a successful deployment. If the outcome of an in-progress operation is unknown, the result says so and directs the developer to check it before retrying.
- The developer can explicitly request removal of an application from a named environment, subject to the team's approval rules.
- Removal deletes application-owned resources while preserving referenced foundations, other applications, and any centralized control plane.
- Partial removal identifies what remains and allows a later operation to continue. Stopping Radius alone is never treated as application removal.

## Key dependencies and risks

- **Workload platform support.** Each target needs recipes that provide the requested application capabilities. Users need a clear supported-platform list and understandable errors where a capability is unavailable.
- **Network and access configuration.** The selected execution mode must be able to reach the control plane, persistent state, and deployment targets it needs. Teams using private networks must not have to expose services publicly to adopt Repo Radius.
- **State and interrupted operations.** A failed job can change cloud resources without recording those changes. Developers need preserved state and explicit recovery guidance before they can safely retry.
- **Shared-service boundaries.** Jobs using a centralized control plane must not disrupt other teams or exceed their authorized environments.
- **Compatibility.** A pipeline and its selected control plane or recipes may be incompatible. The user needs an actionable compatibility error rather than an unexplained deployment failure.
- **Job startup time.** Temporary execution adds time before an operation starts. The acceptable delay must be tested with teams using the feature in routine deployments.

## Key assumptions to test and questions to answer

- Which CI/CD and workload-platform combinations must be available together in the first release? Validate GitHub Actions and Azure DevOps against the AKS, EKS, ACA, and ECS journeys before setting release coverage.
- Which persistent-state services can target teams already operate, and what access, retention, and recovery requirements must Repo Radius satisfy? Validate with platform engineers without prescribing a storage technology here.
- What connection and permission setup will teams accept for a centralized control plane, especially on private networks? Test an unattended job with restricted access to one environment.
- Which recipe configurations and existing-resource patterns must work across both execution modes? Validate with an application that uses an existing hosting environment and shared backing services.
- What should users see when a job is cancelled while a centralized service is still executing an operation? Define when work can stop and how a later job discovers its outcome.
- What startup delay is acceptable, and how should a team learn that its pipeline, Radius service, or recipes need an update? Validate with representative deployment and inspection jobs.

## Current state

The earlier Repo Radius proposal described temporary execution through GitHub Actions, with GitHub-specific environment and state handling. The scope now includes other CI/CD systems and connections to an existing centralized Radius control plane. These are product requirements, not claims of implemented support.

The [external Kubernetes deployment proposal](../environments/2026-05-external-kubernetes.md) addresses workloads running on a cluster separate from Radius. That separation is one part of this experience; managed container targets also need suitable platform support.

## Details of user problem

As a platform engineer, I already have pipelines, environment boundaries, and deployment standards. I want Radius to work within them. For some teams, running another permanent service is too much overhead. For others, we already operate Radius and should not have to create a second instance just to use it from CI/CD. In either case, a disposable agent must not lose our deployment history or change where applications run.

As an application developer, I want to describe my application and deliver it through the same pipeline as the rest of my code. I need to know where it was deployed, whether the change succeeded, and how to recover when it did not. I should not have to understand the location or lifetime of the Radius control plane to do that.

## Desired user experience outcome

I can use Radius from my existing pipeline, either temporarily or through the service my platform team provides. I can deploy to the approved Azure or AWS environment, inspect the result after the job ends, and update it from a fresh job. If an operation fails, I can see what happened and what to do next without losing track of the application.

### Detailed user experience

1. **Configure the pipeline once.** The platform engineer chooses temporary execution or an existing Radius service, provides approved access, and makes the target environments and recipes available. Temporary execution also has persistent state configured.
2. **Select the application and destination.** The developer supplies the application revision, operation, and environment through the normal pipeline. Existing review and approval rules still apply.
3. **Confirm readiness.** The job identifies the selected control plane and workload destination. Missing configuration, inaccessible required state, or a failed connection stops the operation with an explanation.
4. **Run the operation.** The job reports progress and failures while Radius manages the application and its dependencies in the selected environment.
5. **Review the outcome.** The developer sees the revision, target, resource results, and any state or recovery warnings. Inspection remains available after the job ends.
6. **Finish without affecting workloads.** Temporary execution releases its control plane; connected execution leaves the centralized service running. Deployed applications and their state remain available for the next operation.
7. **Continue delivery.** A later job updates, inspects, promotes, or explicitly removes the application. An interrupted or failed operation is checked before a retry.

## Key investments

### Feature 1

**CI/CD execution choices.** Let teams run Radius temporarily or connect to a centralized service through unattended jobs, without requiring a particular CI/CD vendor or frontend.

### Feature 2

**Persistent application state and recovery.** Let fresh jobs resume management of existing applications, prevent conflicting updates from silently losing state, and make uncertain outcomes visible.

### Feature 3

**Independent workload targets and approved configuration.** Let teams deploy to supported Azure and AWS container platforms using their environment settings and recipes, regardless of where Radius runs.

### Feature 4

**Application lifecycle experience.** Give developers consistent progress, inspection, promotion, failure recovery, and explicit removal through their existing pipelines.
