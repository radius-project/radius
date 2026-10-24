<!--
Structure based on the Radius feature specification template:
https://github.com/radius-project/radius/blob/main/eng/design-notes/templates/feature-spec-template.md
-->

# Topic: Repo Radius

- **Author**: Will Tsai (@willtsai)

## Topic Summary

Repo Radius lets teams deploy and manage applications through their existing CI/CD pipelines without operating a permanent Radius control plane. A job can start a temporary control plane or connect to a centralized service that the platform team already runs.

A CI/CD runner executes pipeline jobs. Azure DevOps calls these runners agents.

Radius and the application need not run in the same place. A control plane hosted on Kubernetes can deploy workloads to a separate Kubernetes cluster or a managed container environment. Teams should be able to use GitHub Actions or Azure DevOps to deploy to Azure Kubernetes Service (AKS), Amazon Elastic Kubernetes Service (EKS), Azure Container Apps (ACA), or Amazon Elastic Container Service (ECS). These are intended targets; not every combination is available today.

### Top level goals

- Run Radius operations unattended in existing CI/CD pipelines without requiring a particular repository host or frontend.
- Let teams choose a temporary control plane or an existing centralized control plane.
- Keep Radius configuration and deployment state after jobs end so later runs can reuse approved environments and recipes and manage existing applications.
- Deploy workloads to the team's chosen Azure or AWS platform, regardless of where the Radius control plane runs.
- Show developers command outcomes, diagnostics, and whether state was saved, without crossing the platform team's environment and access boundaries.
- Provide runnable GitHub Actions reference samples that teams can adapt, from recipe-pack and environment setup through application deployment and deletion.
- Separate Radius's cloud access from the pipeline's permission to use Radius, with scoped identities and explicit credential requirements for each run.

### Non-goals (out of scope)

- Prescribing workflow-file structure, packaging, storage technologies, authentication protocols, or result schemas. These belong in technical designs; this specification defines what the reference samples must demonstrate.
- Replacing CI/CD systems, their approval processes, or the tools used to monitor running applications.
- Defining release management or promotion between environments. Teams keep these workflows in their existing pipelines.
- Requiring a new graphical interface or coding assistant to complete a deployment.
- Installing, upgrading, or operating an organization's centralized Radius service.
- Migrating existing application state and resource ownership between independent control planes. Connecting to an existing control plane is not a migration.
- Adding built-in automatic rollback, retry, or repair to Repo Radius. The pipeline or operator decides what to do next; reference samples can demonstrate retry and rollback policies in pipeline scripts.
- Adding a general-purpose deployment what-if engine or continuous cloud drift detection.

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
- A later job can run on a different CI/CD runner without depending on files left on the previous runner.
- If Radius fails to start, the job stops before running the requested operations and tells the platform engineer what to correct. An interrupted job reports its status and any unknown outcome as described in Scenario 6.

### Scenario 2: Keep Radius configuration and deployment state independently of the pipeline

> As a platform engineer, I want each temporary Radius instance to use a centrally managed persistent state store, so that later jobs can reuse our environments and recipes and manage resources created by earlier runs.

The platform engineer configures persistent state for Radius. The store preserves environment definitions, registered recipe packs, their associations and settings, and application and resource records. Each new job recovers this configuration and deployment state instead of starting from scratch. The state outlives the CI/CD runner and does not require storage from a particular CI/CD vendor.

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

- The pipeline does not start another control plane or require the CI/CD runner to host Kubernetes for Radius.
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

### Scenario 6: Understand the result of a pipeline operation

> As an application developer, I want a clear result and detailed diagnostics for each pipeline operation, so that I can tell what happened and decide what to do next.

The job identifies the requested command, control plane, application, and environment, then reports the outcome and whether Radius saved the resulting state. The developer can read these results after the job ends. Reporting an outcome does not retry, repair, or roll back the operation; the pipeline or operator decides the next action.

- The result distinguishes success, failure, cancellation, and an unknown outcome. A cancelled job is not reported as a successful operation.
- When an operation fails, the result identifies the failed step and includes available error details and diagnostics. It distinguishes completed work from work whose outcome is unknown.
- The result reports whether state was saved. If saving failed or cannot be confirmed, it warns that recorded state may not match cloud resources.
- Results and diagnostics remain available after the CI/CD runner or temporary control plane disappears.
- If a centralized service may still be running an operation after the job ends, the result makes that uncertainty explicit rather than claiming that cancellation stopped the operation.

### Scenario 7: Delete an application through the pipeline

> As an application developer, I want an authorized pipeline job to run existing Radius deletion commands, so that I can remove an application from the selected environment without managing the control plane.

The developer explicitly requests deletion through the team's pipeline, subject to its approval rules. The job uses the selected control plane and persisted state to run the existing Radius commands against the named application and environment. Repo Radius does not introduce new deletion behavior.

- A fresh job can delete an application recorded in the saved state after the original deployment job and temporary control plane are gone.
- The job confirms the target application and environment and requires the access needed for deletion.
- Deletion follows Radius's existing resource-ownership rules. It does not turn referenced foundations, unrelated applications, or the centralized control plane into deletion targets.
- The result reports completion or partial failure, including known remaining resources and any unknown outcomes. The pipeline or operator decides whether to run another command.
- Stopping or releasing a temporary control plane does not delete an application. Deletion requires an explicit Radius command.

### Scenario 8: Adapt GitHub Actions reference pipelines for my team

> As a platform engineer, I want runnable reference pipelines that cover Radius setup and application operations, so that I can adapt a working example instead of assembling my team's pipeline from isolated snippets.

The team starts from a maintained suite of GitHub Actions samples. Together, the samples cover registering and configuring recipe packs, creating and configuring environments, setting up persistent state and access, and deploying, inspecting, updating, and deleting an application. Platform setup is separate from routine application jobs, which reuse the saved environments and recipe registrations.

- The suite demonstrates both temporary control-plane execution and connection to an existing centralized service. Temporary jobs release their control plane without deleting workloads or saved state; connected jobs leave the shared service running.
- Later application jobs run on fresh GitHub Actions runners and use the saved configuration and deployment state. The samples do not recreate environments or register recipe packs on every deployment.
- Each sample documents prerequisites, required permissions, configuration inputs, supported workload targets, expected results, and cleanup steps. Maintainers validate the runnable samples against the Radius versions they document.
- The samples demonstrate cloud access for Radius, authentication and Radius RBAC assignments for the pipeline identity, and access to the state store or archive. They distinguish platform-setup permissions from application-job permissions and show which credential configuration persists and which credentials each run must obtain.
- Application deployment scripts demonstrate bounded retries for documented retryable failures and a clearly defined rollback procedure, such as redeploying a previous application revision. These are pipeline policies that users can inspect and adapt, not new Radius recovery capabilities.
- Each rollback example states what it restores and what it cannot reverse. Redeploying an earlier revision does not guarantee reversal of infrastructure or data changes, and restoring an older Radius state snapshot alone is not rollback.
- The scripts preserve command results and diagnostics, including failed retries or rollback attempts. Unknown outcomes or uncertain saved state require an operator check rather than a blind retry. Destructive cleanup requires an explicit request, not merely a failed deployment.
- The reference-sample requirement covers GitHub Actions only, not Azure DevOps samples. The broader CI/CD requirements remain unchanged. The suite identifies the workload platforms it demonstrates rather than promising every platform combination.

### Scenario 9: Provide cloud access for Radius operations

> As a platform engineer, I want to configure how Radius obtains cloud credentials, so that pipeline operations can reach approved deployment targets without exposing those credentials to application developers.

The platform engineer configures the identity Radius uses to call the cloud. This outbound identity is separate from the pipeline identity that calls Radius. Saved credential configuration identifies how Radius obtains access; it does not prove that usable credentials are available in a new run.

- Each temporary control-plane run establishes usable cloud credentials through a documented mechanism. Prefer short-lived workload credentials where supported; otherwise supply secrets from an approved CI/CD secret store or external secret manager.
- The mechanism makes credentials available to the Radius component that calls the cloud. A cloud login on the runner alone does not establish that component's identity.
- A centralized control plane can retain its platform-managed cloud identity. Application jobs do not need those cloud credentials merely to request deployment through that service.
- Documentation distinguishes saved credential metadata and references from secret material, states what is persisted, and identifies what each run must supply or renew. The deployment-state archive is not a general-purpose credential vault.
- Access to the persistent store or GHCR archive is established before restoring state. Credentials needed to retrieve the state cannot exist only inside that state.
- Missing or expired credentials and cloud authorization failures produce clear errors without exposing secrets. Saved state does not bypass credential rotation or revocation.

### Scenario 10: Authorize the pipeline's identity in Radius

> As a platform administrator, I want to assign Radius permissions to a dedicated pipeline workload identity, so that each job can perform only its approved operations.

The pipeline authenticates through the trusted access path supported by Radius. The administrator assigns roles to the workload identity Radius recognizes, following the [built-in RBAC specification](../security/2026-09-built-in-rbac-feature-spec.md). The identity remains recognizable across runs; a runner's machine name is not its authorization identity.

- Platform-setup jobs receive only the permissions needed to manage their recipe packs, environments, and credential configuration. Routine application jobs receive scoped application permissions, including deletion when authorized, and permission to use the approved environment.
- Permission to deploy through an environment does not grant permission to read its secrets or modify its platform configuration. Cloud permissions do not automatically grant Radius permissions, or vice versa.
- Temporary installations establish initial administrator access without an unprotected window. Saved role definitions and assignments remain tied to the intended principals across runs; routine jobs do not fall back to administrator access or disable enforcement.
- The first-release integration follows the RBAC specification's Kubernetes authentication path. A GitHub OIDC token is not assumed to be directly accepted by Radius; the supported authentication path must establish the pipeline identity.
- Authentication failures, Radius RBAC denials, and denials from the cloud, Kubernetes, or state registry remain distinguishable. Denied operations stop rather than retrying with broader privileges.
- Radius actions remain attributable to the pipeline workload identity without exposing secret values. Fresh runs honor revocation within the RBAC contract; restoring saved configuration must not silently revive revoked access.

## Key dependencies and risks

- **Recipes for the selected target.** The environment must have recipe packs registered that can deploy the application's requested resources to the chosen platform. If a required recipe is missing, the job reports what is missing rather than deploying to a different target.
- **Network and access configuration.** Jobs and control planes must be able to reach the services they need, including persistent state and deployment targets. Teams using private networks must not have to expose services publicly to adopt Repo Radius.
- **State and interrupted operations.** A failed job can change cloud resources without recording those changes. Developers need saved state, diagnostics, and explicit warnings about unknown outcomes to decide what to do next. Restoring an older state snapshot alone does not undo cloud changes.
- **Shared-service boundaries.** Jobs using a centralized control plane must not disrupt other teams or exceed their authorized environments.
- **Credential delivery and state access.** Restored credential metadata may refer to secrets or identities unavailable to a new control plane. Cloud credential delivery and access to the state store need separate setup; state archives can contain sensitive data and require appropriate protection.
- **Radius identity and RBAC.** Pipeline authorization depends on the built-in RBAC contract and a supported authentication path. Temporary bootstrap, stable workload identities, and restoration of authorization state must preserve least privilege and revocation.
- **Compatibility.** A pipeline and its selected control plane or recipes may be incompatible. The error must identify the incompatibility and explain what the user can do about it.
- **Job startup time.** Starting a temporary control plane delays the requested operation. Test how much delay teams will accept in routine deployments.
- **Sample maintenance.** Users will copy the reference pipelines. Outdated commands or unsafe recovery scripts can spread into real deployments, so the samples need ongoing validation and clear limits on retry and rollback.

## Key assumptions to test and questions to answer

- Which persistent-state services should we begin with first? We'll start with Postgres (options for either self-hosted or cloud service), but external state store should be as configurable as possible to easily support other persistent-state services in the future.
- What connection and permission setup will teams accept for a centralized control plane, especially on private networks? Test an unattended job with restricted access to one environment.
- Which supported mechanisms deliver cloud credentials to a temporary control plane, and what must each run obtain again? Validate short-lived credentials and secret-manager paths separately from runner-only cloud login.
- How will a fresh control plane recognize the same pipeline principal and restore authorization configuration without reviving revoked access? Validate bootstrap and separate setup, deployment, and deletion permissions against the RBAC specification.
- Which recipe configurations and uses of existing resources must work with both temporary and centralized control planes? Test with an application that uses an existing hosting environment and shared backing services.
- What should users see when a job is cancelled while a centralized service is still executing an operation? Define when work can stop and how a later job discovers its outcome.
- What startup delay is acceptable, and how should a team learn that its pipeline, Radius service, or recipes need an update? Validate with representative deployment and inspection jobs.
- Which workload targets and application changes should the GitHub Actions samples demonstrate first? Choose runnable examples for both control-plane modes and document where the sample rollback procedure cannot restore an earlier deployment.

## Current state

The [earlier Repo Radius proposal](https://github.com/radius-project/radius/pull/12078) called for running Radius temporarily through GitHub Actions, with GitHub-specific environment and state handling. Repo Radius must also work with other CI/CD systems and connect to an existing centralized control plane. These requirements do not imply that Radius supports those scenarios today.

The [OCI/GHCR state archive design](../architecture/2026-07-oci-ghcr-state-archive.md) defines an initial implementation of externalized state storage. It uses `rad shutdown` to archive control-plane database dumps and Terraform state in GitHub Container Registry, and `rad startup` to restore them for a later run. GHCR holds the saved archive, not the live control-plane database. This provides a starting point for preserving state across temporary jobs in Scenario 2 without making GHCR a requirement for every CI/CD system or future state-storage implementation.

The [credentials architecture](../../../docs/architecture/credentials.md) separates cloud credential metadata in the database from secret material in the default Kubernetes Secret backend. The current database and Terraform-state archive path does not imply that all cloud credential Secrets are preserved. Fresh control planes need a supported way to obtain usable credentials.

The [built-in RBAC specification](../security/2026-09-built-in-rbac-feature-spec.md) defines scoped access for workload identities, including CI/CD automation, and safe bootstrap for ephemeral installations. Its first release uses identities established through the Kubernetes authentication path. Repo Radius must integrate with that contract; these requirements do not claim that the RBAC integration is already available.

The [external Kubernetes deployment proposal](./2026-05-external-kubernetes.md) covers workloads running on a cluster separate from Radius. It addresses the Kubernetes case; managed container targets also need suitable platform support.

## Details of user problem

As a platform engineer, I already have pipelines, environment boundaries, and deployment standards. I want Radius to work within them. My team may not want another permanent service to maintain. Or we may already run Radius and should not need a second instance just for CI/CD. Either way, replacing a disposable CI/CD runner must not lose our deployment history or change where applications run.

As an application developer, I want to describe my application and deliver it through the same pipeline as the rest of my code. I need to know where it runs, whether the change succeeded, and what failed so I can decide what to do next. I should not need to know where the Radius control plane runs or how long it lives.

## Desired user experience outcome

I can use Radius from my existing pipeline by starting it for a job or connecting to the service my platform team provides. I can deploy to the approved Azure or AWS environment, inspect the result after the job ends, and update or explicitly delete the application from a fresh job. If an operation fails, I can see what happened, what remains unknown, and whether Radius saved the state.

### Detailed user experience

1. **Configure the pipeline once.** The platform engineer chooses a temporary control plane or an existing Radius service and configures the target environments and recipe packs in Radius. The engineer separately configures Radius's cloud identity, the pipeline workload identity and its Radius role assignments, and access to the state store. For a temporary control plane, persistent state lets later jobs recover the configuration and application records.
2. **Select the application and destination.** The developer supplies the application revision, operation, and environment through the normal pipeline. Existing review and approval rules still apply.
3. **Confirm readiness.** The job identifies the selected control plane, workload destination, and pipeline identity recognized by Radius. It establishes state-store access and the credentials needed for the run. Missing configuration, inaccessible state, authentication failures, and authorization denials stop the affected operation with a clear explanation.
4. **Run the operation.** The pipeline runs existing Radius commands against the selected control plane and environment. The job reports progress and failures.
5. **Review the outcome.** The developer sees the revision, target, command status, diagnostics, and whether state was saved. These results remain available after the job ends.
6. **Release the control plane.** The job releases a temporary control plane or leaves a centralized service running. This cleanup does not delete workloads or discard saved state; application deletion requires an explicit Radius command.
7. **Run later commands.** A later job uses existing Radius commands to update, inspect, or explicitly remove the application. The developer checks an interrupted or failed operation before retrying.

## Key investments

### Feature 1

**CI/CD execution choices.** Let teams run Radius for a job or connect to a centralized service. Jobs run unattended and do not require a particular CI/CD vendor or frontend.

### Feature 2

**Persistent Radius configuration and deployment state.** Let fresh jobs reuse saved environments and recipe configuration and manage existing applications. Prevent conflicting updates from silently losing that state, and report when saving state fails or cannot be confirmed.

### Feature 3

**Independent workload targets and approved configuration.** Let teams deploy to supported Azure and AWS container platforms using their environment settings and recipes, regardless of where Radius runs.

### Feature 4

**Command results and explicit deletion.** Preserve status and diagnostics for existing Radius commands, including deletion commands, across both control-plane modes. Leave rollback, retry, and repair decisions to the pipeline or operator.

### Feature 5

**GitHub Actions reference samples.** Maintain runnable pipelines that separate platform setup from application operations and cover the full path from recipe packs and environments to deployment and deletion. Demonstrate retry and rollback in pipeline scripts, with documented limits and no new built-in recovery behavior.

### Feature 6

**Cloud credentials and pipeline authorization.** Establish Radius's outbound cloud access separately from the pipeline's scoped Radius permissions. Document credential persistence and renewal, protect state-store access, and integrate temporary and centralized jobs with the built-in RBAC contract.
