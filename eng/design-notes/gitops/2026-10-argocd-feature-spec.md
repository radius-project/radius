# Topic: Radius Integration with Argo CD

- **Author**: Will Tsai (@willtsai)

## Topic Summary

This specification proposes an Argo CD integration that builds on the shared Radius deployment capabilities used by the existing Flux integration. Argo CD users should be able to deploy Radius definitions, navigate their components and dependencies, observe provisioning convergence, and diagnose failures without seeing only an opaque top-level custom resource.

The [original GitOps feature specification](2024-06-gitops-feature-spec.md) remains the foundation for installation, deployment, and parameter-editing scenarios. The [DeploymentTemplate controller design](2024-10-deploymenttemplate-controller.md) and [Flux integration design](2025-01-gitops-technical-design.md) describe the underlying deployment model. This document defines the incremental Argo CD source and UI experience; it does not replace those documents or redesign the Flux workflow. All Argo CD-specific behavior below is proposed, not a statement of released capability.

### Top level goals

- Bring Radius definitions and parameter-only changes into Argo CD using the shared `DeploymentTemplate` deployment model already used by Flux, rather than creating a separate Radius deployment engine.
- Let Argo CD users inspect Radius-managed applications without seeing only an opaque top-level custom resource.
- Distinguish Git synchronization, Radius provisioning, and observed workload health so a successfully applied manifest cannot conceal a failed Recipe or an unobserved deployment.
- Make a failed Bicep or Terraform operation traceable from the Argo CD application to the affected Radius resource, attempted revision, and actionable diagnostic.
- Preserve the separation of operator, developer, and SRE responsibilities, with application and environment scope enforced throughout navigation and deployment.
- Let operators adopt Argo CD without requiring Flux, while leaving existing Flux installations and configuration unchanged. Success means users can complete Scenarios 2-5 on a representative application containing a Kubernetes workload, an external dependency, and a failed Recipe without requiring cluster-admin access.

### Non-goals (out of scope)

- Replacing Flux, Argo CD, their reconciliation policies, or Git as the source of desired configuration.
- Redefining the original specification's Radius installation and authoring workflows, migrating existing Flux applications, or introducing a Flux UI integration (Flux doesn't offer a native GUI like ArgoCD does).
- Building a new GitOps dashboard in Radius. The proposed UI work augments Argo CD; an existing Radius view may be linked for additional context.
- Making Argo CD directly reconcile every cloud resource created by a Recipe, or representing a dependency relationship as Kubernetes ownership solely to draw a graph.
- Building or containerizing application code, publishing Recipe artifacts, or automatically updating image references. These remain CI responsibilities; GitOps consumes the resulting configuration changes.
- Providing a complete Terraform plan, cloud drift detector, database rollback, or disaster-recovery system through Argo CD's manifest diff and history.
- Adding arbitrary cloud mutations, terminal access, or a second deployment engine to a Radius UI extension.
- Requiring all future GitOps tools to offer Argo CD's UI features. Common Radius deployment semantics should remain reusable, but UI integration is tool-specific.

## User profile and challenges

### User persona(s)

**Infrastructure operators / administrators** maintain Radius installations, environments, Recipe registrations, and Argo CD repository access, projects, destinations, and policies. They decide which integrations users can install and which resources they may manage.

**Application developers** define applications and dependencies, publish code through CI, and submit application-definition changes through Git. They need to understand which component or dependency is preventing a deployment from becoming usable.

**Site Reliability Engineers (SREs)** adjust approved parameters and diagnose provisioning and runtime failures. They need application-scoped visibility without permission to rewrite application definitions, inspect unrelated environments, or administer the entire cluster.

### Challenge(s) faced by the user

Applying a top-level Radius resource is not enough to explain what happened beneath it. A developer can see that GitOps accepted a configuration while a Recipe is still provisioning, Terraform has failed, or a workload cannot reach a dependency. An opaque resource prevents the developer from using the graph and diagnostic workflows that make Argo CD valuable.

The Kubernetes ownership tree also differs from the Radius application graph. Shared services, external infrastructure, and application dependencies do not necessarily exist as child Kubernetes objects. Users need these relationships without a misleading claim that Argo CD owns or continuously monitors every graph node.

At enterprise scale, an unfiltered resource-group dropdown or global application inventory is not a usable entry point. Users need to find the applications and environments relevant to their responsibilities, with access restrictions that remain effective when they follow graph edges or diagnostic links.

### Positive user outcome

I can keep Argo CD as my GitOps interface and use Radius's existing application and deployment model. I can answer which revision was applied, what Radius is provisioning, which dependency is blocked, and where to investigate without maintaining a separate application catalog or acquiring broad administrative access.

## Key scenarios

### Scenario 1: Connect Radius definitions to Argo CD

As an operator with Radius installed, I configure an Argo CD Application to consume generated `DeploymentTemplate` manifests or the proposed Radius Bicep source integration. Definition and parameter changes use the same Radius deployment semantics as the Flux path. I can identify the input revision and destination, and compilation or input errors prevent deployment with an actionable message. I do not need to install Flux.

### Scenario 2: Navigate Radius resources and dependencies inside Argo CD

As a developer, I open an Argo CD application's resource tree and inspect its Radius deployment and discoverable Kubernetes resources. From that context, I open the proposed Radius graph view to follow application dependencies, including external or shared resources and custom resource types. The view identifies each relationship and its source; resources with no live observation are explicitly shown as unobserved rather than healthy.

### Scenario 3: Diagnose convergence and Recipe failures through Argo CD

As an SRE, I open an application that is synchronized but still provisioning or degraded. I locate the blocked dependency and see a safe error summary, attempted revision, operation context, and last-observed state. A Terraform failure is visible even if it occurs before any child resource is created. After an authorized correction and reconciliation, the view shows the new attempt and outcome without presenting the previous attempt's success as current.

### Scenario 4: Review changes and recover through Argo CD

As a developer or SRE, I inspect the desired-input diff and deployment history for an application or parameter change. I can connect a failed attempt to its input revision and correct or revert that input in Git. Requested logical resource removals and pending cleanup are visible; Argo CD's manifest diff and rollback history do not imply a complete cloud-resource plan or restoration of deleted data.

### Scenario 5: Find and inspect only the applications within my scope

As a developer or SRE, I enter through my existing Argo CD project and select an application and environment without browsing the organization's entire resource inventory. Searches, graph expansion, diagnostics, and direct links enforce the same effective access boundaries. Missing permission, unavailable data, and an empty authorized result have distinct, actionable states without revealing protected resource details.

## Key dependencies and risks

- **Shared Radius deployment foundation:** Argo CD must submit desired deployment inputs through the same `DeploymentTemplate` model and Radius controllers used by Flux. Reuse includes asynchronous operation handling and resource cleanup, not just manifest shape. Argo CD-specific source handling and presentation must not require a second provisioning path or change existing Flux users' behavior.
- **Argo CD compatibility:** The proposed integration depends on documented [config management plugins](https://argo-cd.readthedocs.io/en/stable/operator-manual/config-management-plugins/), [resource tracking](https://argo-cd.readthedocs.io/en/stable/user-guide/resource_tracking/), [custom health checks](https://argo-cd.readthedocs.io/en/stable/operator-manual/health/), and optional [UI extensions](https://argo-cd.readthedocs.io/en/stable/developer-guide/extensions/ui-extensions/). A supported Radius/Argo CD version range and the required administrator configuration are not yet established. Users must receive explicit prerequisites rather than a claim that installing a chart provides full integration.
- **Deployment status is not workload health:** Radius deployment completion does not establish continuing health for every dependent service. Argo CD does not automatically inherit health through all descendants, and a custom health check can inspect only its own object's state. The proposed experience must report supported dependency failures in the enclosing deployment's health and label unavailable runtime observations separately.
- **Resource identity and ownership:** A Radius deployment can produce multiple resources or reference shared infrastructure. Visibility must not give Flux and Argo CD concurrent reconciliation authority over the same desired resource, change Kubernetes garbage-collection relationships, or make shared resources prune targets. Cross-namespace and external relationships may require the Radius graph view rather than native tree placement.
- **Authorization across systems:** Argo CD project permissions, Kubernetes namespace access, and Radius resource-group permissions are not interchangeable. A deployment target or graph URL must not bypass the user's permitted Radius scope. If an extension queries a backend, the [proxy extension contract](https://argo-cd.readthedocs.io/en/stable/developer-guide/extensions/proxy-extensions/) requires backend validation in addition to Argo CD authorization; a permitted application must not become a route to arbitrary Radius resources.
- **Sensitive output and trusted extensions:** Recipe diagnostics, rendered templates, parameters, and cloud identifiers may contain sensitive information. Status summaries and links must exclude credentials and secret values. Detailed diagnostics require appropriate authorization and redaction; installing a plugin or UI extension is an administrator-controlled trust decision.
- **Deletion and recovery:** Removing a resource inside a deployment template can delete infrastructure even when Argo CD is not pruning the template object. Operators need a Radius-level deletion policy and visibility of requested removals, not reliance on Argo CD's prune toggle alone. Reapplying an earlier configuration cannot undo data loss or every provider-side change.
- **Freshness and scale:** A graph can become misleading when observations are stale, incomplete, or too large to navigate. Display observation scope and freshness, support scoped search and incremental expansion, and validate the experience against an agreed representative application inventory.
- **Flux compatibility:** Shared status or resource-identity improvements must not require Flux users to install Argo CD, replace `radius-gitops-config.yaml`, or migrate existing resources. Flux and Argo CD may serve different applications, but must not reconcile the same deployment inputs concurrently. Migration between tools is outside this proposal.

## Key assumptions to test and questions to answer

- **Native tree versus graph extension:** Is a Radius graph view within Argo CD acceptable for dependencies that cannot appear in its native Kubernetes tree, or is native-tree placement essential? Validate with operators using a prototype containing a workload, a shared database, and an external dependency before fixing the UI scope.
- **Minimum useful visibility:** The motivating need is dependency navigation and downstream convergence/error feedback. Manifest diffs, history, log links, and filtering are proposed supporting experiences, not a confirmed checklist of customer requests. Confirm their relative priority and whether authorized deep links are sufficient for detailed logs.
- **Source integration:** Can the proposed Bicep source experience fit administrators' plugin policies? Validate deterministic, side-effect-free manifest generation, parameter handling, compiler configuration changes, and repository authentication. Keep a generated-manifest path available for organizations that cannot install a config management plugin.
- **Split repositories:** The original specification includes separately permissioned definition and parameter repositories. How will the Argo CD source integration select and authorize both revisions? Validate this separately; neither a local Bicep `using` path nor the existing Flux source configuration proves cross-repository support.
- **Status coverage and freshness:** Which failures and readiness signals can Radius reliably report for Bicep and Terraform Recipes, custom types, and shared dependencies? Prototype a failure before resource creation, partial provisioning, delayed cleanup, controller unavailability, and a later workload failure. Define the observation interval and stale-data threshold before promising runtime-health behavior.
- **Application and environment mapping:** Can one Argo CD Application represent several Radius applications or environments? Validate how users select the intended context and how the mapping avoids duplicate resources, ambiguous source revisions, or cross-environment access.
- **Identity and access:** Which Argo CD identity and project context can be securely mapped to Radius permissions? Confirm the enforceable authorization path with the operators responsible for both systems. Do not offer the backend-backed graph or diagnostics where that mapping cannot be enforced.
- **Deletion and retry semantics:** What operator-configured approval or retention policy governs removals within a template, shared resources, and failed partial deployments? Which failures retry automatically, and how can users request an authorized retry without changing intent? Define these behaviors before enabling destructive or retry actions; a UI refresh must not imply a new deployment.
- **Compatibility and support:** Which Radius and Argo CD versions, extension mechanisms, and Recipe types will be supported? Who maintains the source integration, health customization, and graph extension? Confirm ownership and supported combinations through an integration prototype; this proposal does not establish a release date or support commitment.

## Current state

The [original GitOps specification](2024-06-gitops-feature-spec.md) establishes tool-neutral user scenarios. The [Radius Flux Controller design](2025-01-gitops-technical-design.md) describes the implemented Flux-specific source integration and explicitly excludes Argo CD. The [Flux controller](../../../pkg/controller/reconciler/flux_controller.go) reads Flux `GitRepository` artifacts, uses `radius-gitops-config.yaml` to select Bicep files and parameters, and creates or updates Radius `DeploymentTemplate` resources.

The reusable foundation is the deployment model beneath that Flux-specific source handling:

- **Deployment inputs:** The [DeploymentTemplate API](../../../pkg/controller/api/radapp.io/v1alpha3/deploymenttemplate_types.go) represents compiled template content, parameters, and provider scope. Argo CD can target this shared input model instead of requiring users to adopt a separate Radius definition format.
- **Execution and lifecycle:** The [DeploymentTemplate reconciler](../../../pkg/controller/reconciler/deploymenttemplate_reconciler.go) drives asynchronous Radius deployments, emits failure events, and creates owned `DeploymentResource` records for successful outputs. The proposed Argo CD integration reuses this execution and cleanup behavior; the source plugin and UI do not execute Recipes.
- **Status:** The existing API exposes an observed generation, deployment phrase, operation state, and output resource IDs. Phrases include `Updating`, `Ready`, `ReadyPendingCleanup`, `Failed`, `Deleting`, and `Deleted`. These are the starting point for Argo CD health reporting, not proof of complete dependency-health aggregation or continuous runtime monitoring.
- **Manifest generation:** The [`rad bicep generate-kubernetes-manifest` command](../../../pkg/cli/cmd/bicep/generatekubernetesmanifest/generatekubernetesmanifest.go) already generates a `DeploymentTemplate` manifest. It provides a declarative entry point for the proposed Argo CD path without requiring the Flux source controller.

Separately, the [`rad app graph` command](../../../pkg/cli/cmd/app/graph/graph.go) distinguishes modeled graphs from queries for deployed resources. This broader Radius capability is a foundation for graph presentation, not a graph feature already delivered by the Flux integration.

The proposed additions are Argo CD source configuration, native resource visibility, health customization, diagnostic correlation, and an optional Radius graph view. The Flux `GitRepository` adapter remains Flux-specific; Argo CD does not need it or `radius-gitops-config.yaml` merely to consume a generated manifest. Existing deployment status and output records do not by themselves expose every Recipe-created component or application dependency in Argo CD.

Argo CD's documented UI extensions can add resource/application tabs and application views, and [external URL links](https://argo-cd.readthedocs.io/en/stable/user-guide/external-url/) can connect a resource to an existing diagnostic view. These are available integration surfaces, not evidence that a Radius extension is installed or supported today.

## Details of user problem

I already use Argo CD to manage application configuration. Radius has a deployment foundation used by Flux, but I need to use it through Argo CD without installing another GitOps tool or losing the UI workflows my team relies on. My SRE colleagues must still be able to change approved inputs without gaining permission to change the whole application definition.

When I inspect a Radius deployment in Argo CD, a single custom resource cannot tell me whether its dependencies exist, whether Terraform converged, or which component is blocking the application. I need to navigate the actual application relationships and observed resources, not maintain an independently authored diagram. When data is missing or I lack permission, I need to know what I can do next without being given misleading health information or access to another team's resources.

## Desired user experience outcome

I can connect Radius definitions or parameter updates to Argo CD while retaining Radius's shared deployment behavior. In Argo CD, I can distinguish the accepted Git revision from Radius provisioning progress, navigate from an application to an affected dependency, and inspect authorized diagnostics. I can recover through an approved Git change while preserving my team's ownership and access policies.

### Detailed user experience

**Configure Argo CD to use the Radius foundation.** As an operator, I start with Radius installed and ready through the installation workflow in the original GitOps specification. I configure Argo CD repository access, permitted destinations, and the proposed integration prerequisites. I can select a directory of generated Radius Kubernetes manifests or the proposed Radius Bicep source integration, which accepts a definition path, parameter inputs, and deployment scope without requiring hand-maintained compiled manifests. Both paths submit `DeploymentTemplate` inputs to the shared Radius controllers; neither requires Flux to be installed.

The following existing command illustrates generating a manifest from a Bicep definition. It creates a file; it does not install the proposed Argo CD integration or deploy the application. In this example, `generated/` already exists and the selected template requires no parameters.

```bash
rad bicep generate-kubernetes-manifest app.bicep --group team-dev --destination-file generated/app.yaml
```

An illustrative Argo CD Application consuming that directory is shown below. The repository URL, project, namespace, and destination are examples; the `team-dev` project, destination namespace, Radius installation, CRDs, and permitted Radius resource group must already be configured. Synchronization is manual in this example.

```yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: orders-dev
  namespace: argocd
spec:
  project: team-dev
  source:
    repoURL: https://git.example.com/team/orders.git
    targetRevision: main
    path: generated
  destination:
    server: https://kubernetes.default.svc
    namespace: orders-dev
```

I review and commit the generated manifest using the same policies as other desired configuration. When source Bicep or parameter files change, my generation process must update that manifest; Argo CD watching `generated/` alone will not compile the source files. The proposed direct Bicep source experience removes that manual generation step and must surface compilation or input errors before any deployment. Viewing a diff or refreshing the source must never provision infrastructure.

**Submit existing Radius definitions and parameter changes.** I use the Environment, Recipe registration, application-definition, and approved-parameter workflows from the original specification. In Argo CD, a definition or parameter-only commit identifies the input revisions and destination for the resulting Radius deployment. Missing or inaccessible inputs cause a visible error rather than a deployment with implicit defaults. Changes to supported Bicep compiler configuration also participate in source generation. Split-repository resolution remains an explicit question rather than an assumed capability inherited from Flux.

Credentials remain in the approved credential-management system, not plaintext Git files or generated manifests. After CI publishes an application image or Recipe artifact, I update its immutable reference in the desired configuration; publishing the artifact alone does not trigger this workflow.

**Inspect the application and graph.** I select my application in Argo CD and see the Git revision, sync status, and health as distinct information. The native resource tree identifies the Radius deployment and Kubernetes descendants that can actually be discovered through valid relationships and permissions. Selecting a resource preserves the current application, environment, and revision context.

For richer relationships, I open the proposed **Radius** application/resource tab or graph view. Placement and names are illustrative pending version and usability validation. I can find a component by name or type, follow a connection to a shared or external dependency, and distinguish desired relationships from observed deployed resources. Nodes identify the logical Radius resource, its type, its environment, and any authorized deployed-resource reference. An external node is not presented as a Kubernetes object or as directly managed by Argo CD.

The graph derives from Radius definitions and observed resources rather than a second manually maintained catalog. It labels ownership separately from dependency links and distinguishes resources in this deployment from references managed elsewhere. An unsupported resource, missing observation, loading failure, or restricted dependency cannot silently disappear or appear healthy. Authorized external links preserve resource context but do not grant access to their destinations. A link-only experience can supplement the graph; it does not satisfy the requirement to navigate dependencies within Argo CD.

**Read convergence and health correctly.** Argo CD's native sync status continues to describe its comparison of desired and live Kubernetes manifests. A deployment may therefore be `Synced` while its Radius health is `Progressing` or `Degraded`. The proposed health customization and Radius view must communicate the following:

| Observed situation                                                             | Required user-visible behavior                                                                                                                                                     |
|--------------------------------------------------------------------------------|------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| New desired generation is not yet observed, or provisioning is active          | Show `Progressing` with the attempted revision and pending work; never reuse an earlier generation's healthy result.                                                               |
| Bicep or Terraform provisioning fails, including before output resources exist | Show `Degraded` for the affected Radius deployment and enclosing Argo CD application, with a safe reason and operation context.                                                    |
| Latest deployment succeeds but removed resources are still being cleaned up    | Keep convergence `Progressing`; explain cleanup rather than reporting completion from `ReadyPendingCleanup`.                                                                       |
| Latest generation is successfully provisioned and required cleanup is complete | Report successful provisioning. Label the scope of any `Healthy` assessment; do not imply continuous runtime or external-service health that was not observed.                     |
| A supported dependency observation reports a failure after provisioning        | Show the affected node and an application-visible health problem; do not hide it behind the successful deployment result.                                                          |
| Observations are unavailable, stale, incomplete, or unsupported                | Mark the affected observation as unavailable or stale and show its last-observed time. Do not invent a healthy state; keep the last known result distinct from current confidence. |
| Application or resource access is denied                                       | Explain that the view is restricted without exposing protected names, configuration, diagnostics, or status.                                                                       |

Failures relevant to a Radius deployment must affect its enclosing health assessment even when they occur below a custom resource. Merely displaying a child in the native tree is insufficient. Where continuous monitoring is unsupported, the view must say so and offer an authorized diagnostic path rather than claim to detect later failures.

**Diagnose and recover.** I select a failed node or deployment and see which definition/revision was attempted, which Radius resource and Recipe were involved, the failure category, and a sanitized message. A failure before resource creation remains reachable from the deployment. Kubernetes events and permitted workload logs can remain in their native Argo CD surfaces; Recipe and provider diagnostics use a scoped Radius view or an authorized deep link, not an assertion that Argo CD has those logs natively.

If a Recipe fails because of an invalid input, I follow the source context, fix the input in Git, and inspect the new deployment attempt. If a credential or external service needs operator intervention, the message identifies the required action without exposing the credential. A temporary diagnostic-service outage leaves the deployment view usable, marks unavailable details, and offers refresh; refreshing is not a deployment retry.

I use Argo CD's manifest diff to inspect the desired Kubernetes input changes and history to locate the attempted revision. This is not a full cloud-resource plan: removing a logical resource inside `DeploymentTemplate.spec.template` is an update to that object, not necessarily an Argo CD prune operation. The Radius experience identifies requested logical removals and applies the configured deletion policy; it must not imply a complete provider-level impact analysis.

For recovery, I normally revert the desired configuration, including the image or Recipe reference, in Git. Argo CD [does not permit rollback while automated sync is enabled](https://argo-cd.readthedocs.io/en/stable/user-guide/auto_sync/); its imperative history rollback is not a substitute for restoring the Git source of truth. Neither path promises to recover deleted data. Pending or failed cleanup remains visible until it completes or an operator resolves it.

**Stay within scope and remain usable.** I enter through my authorized Argo CD project/application and select only permitted Radius context. Filtering is a convenience, not an authorization boundary: graph queries, error details, direct URLs, and deployment requests must be checked against the corresponding application and Radius scope. Read-only viewers cannot sync, retry, delete, or modify resources through the extension. Existing Argo CD controls retain their permissions; no new broad permissions are granted simply to enable visualization.

I can search and expand a large graph incrementally instead of loading an unrestricted inventory. The extension provides keyboard navigation, visible focus, textual status labels that do not rely on color, and an accessible list equivalent for graph relationships. Empty, loading, partial, stale, and permission-denied states retain application context and give an appropriate next action. Disabling the optional graph extension leaves core synchronization and configured health reporting available.

## Key investments

### Feature 1: Argo CD source integration using shared Radius deployments

Add Argo CD-specific source handling above the same `DeploymentTemplate` model and controllers used by Flux. Propose a Radius Bicep source integration using Argo CD's config management plugin surface, with definition, parameter, and scope inputs; it generates desired Kubernetes manifests for Argo CD to compare and apply, rather than deploying during generation. A generated-manifest workflow remains available where plugins are not permitted. Both paths require reproducible input revisions, visible generation errors, and shared Radius execution and cleanup semantics, without a Flux dependency or changes to existing Flux configuration.

### Feature 2: Native Argo CD resource visibility

Let users find the Radius deployment and inspect discoverable Kubernetes descendants through Argo CD's existing resource tree and detail panels. Preserve actual ownership and resource identity, avoid competing reconciliation and prune ownership, and identify relationships that require the Radius graph view. Showing only a successful top-level apply does not complete this feature's user outcome.

### Feature 3: Radius convergence and error reporting in Argo CD

Build on existing deployment phrases, observed generations, output records, and failure events to provide Argo CD health behavior and actionable diagnostics. Add the missing feedback needed for failures before resource creation, partial completion, and delayed cleanup; do not equate existing status fields with complete health coverage. Surface relevant downstream failures in the enclosing application and distinguish provisioned state from supported runtime observations. Any shared Radius feedback improvements remain useful to Flux and other consumers without depending on Argo CD.

### Feature 4: Optional Radius dependency graph within Argo CD

Propose a read-only application/resource tab or application view using Argo CD's UI extension surface and Radius's existing modeled and deployed graph capabilities. It presents application relationships alongside observed resource state, including shared dependencies, custom types, and external infrastructure that the Kubernetes tree cannot fully express. Scoped diagnostic links supplement the graph. The extension is optional to install, but native resource visibility alone is not a claim of complete dependency-graph support.

### Feature 5: Git-consistent change review and recovery

Make definition and parameter revisions, desired-input diffs, logical removals, and deployment attempts traceable. Preserve Git-based recovery, operator-configured deletion controls, and clear warnings about cloud-plan, prune, and data-recovery limits. Establish explicit retry semantics rather than adding an ungoverned deployment action to the UI.

### Feature 6: Scoped and accessible operations

Carry authorized application and environment context through selection, graph navigation, status, and diagnostics. Ensure access boundaries and redaction apply beyond the UI, retain usable failure and stale-data states, and support accessible navigation at an agreed representative scale. Configuration and compatibility guidance must make clear which capabilities require a source plugin, health customization, or optional UI extension.
