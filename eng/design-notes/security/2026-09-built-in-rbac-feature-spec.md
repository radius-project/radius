# Topic: Built-in role-based access control (RBAC) for Radius

- **Author**: Will Tsai (@willtsai)

## Topic Summary

Radius needs a built-in authorization model that lets organizations control who can view, create, change, deploy, and administer Radius resources. The model must support separation of duties between platform teams, application teams, operators, auditors, and automation without requiring every user to receive broad Kubernetes access to the Radius API.

This specification defines the product behavior for role-based access control (RBAC) across the Radius API, `rad` CLI, dashboard and Backstage plugin, and Radius automation such as GitHub Actions and the Radius Copilot integration. It covers Radius resources including resource groups, applications, application resources, environments, Recipe Packs and their Recipes, resource type registrations, configuration resources, credentials, authorization resources, and resource-specific actions.

The specification enables a product and architecture decision on the first enterprise RBAC release tracked by [radius-project/radius#13030](https://github.com/radius-project/radius/issues/13030) and [radius-project/roadmap#27](https://github.com/radius-project/roadmap/issues/27). Direct customer research, compliance requirements, and production usage baselines are not yet available. Statements about demand beyond those roadmap items are hypotheses that require validation.

### Top level goals

- Enforce least-privilege access consistently for every supported Radius API operation and every client surface.
- Let administrators assign understandable built-in roles to users, groups, and workload identities at useful Radius scopes.
- Let advanced administrators create custom roles from a stable catalog of Radius permissions.
- Separate the ability to define and manage platform capabilities from the ability to build and deploy applications.
- Make authorization decisions explainable and auditable without exposing secrets or unnecessary resource data.
- Preserve a low-friction local development experience while giving shared and production installations secure defaults.
- Provide a safe migration path that does not silently lock out existing installations or leave enforcement gaps.

### Non-goals (out of scope)

- Implementing an identity provider, user directory, group directory, login flow, token issuer, or password store. Radius consumes identities established by a trusted authentication layer.
- Replacing Kubernetes RBAC for access to Kubernetes objects or replacing Azure, AWS, GitHub, registry, or other external authorization systems.
- Defining attribute-based access control, Rego policy management, admission policy, or governance rules about allowed resource configuration. These are distinct from RBAC and [radius-project/roadmap#55](https://github.com/radius-project/roadmap/issues/55) tracks policy management.
- Property-level or field-level permissions within a Radius resource.
- Granting access to raw secret values. Existing secret handling and redaction requirements continue to apply regardless of role.
- Automatically deriving Radius access from Azure, AWS, Kubernetes namespace, GitHub repository, or Backstage catalog permissions.
- Building a complete graphical role administration experience in the first release. All graphical clients must respect and explain authorization, but initial policy administration may be API, CLI, and declarative configuration only.
- Solving tenant isolation or billing. This specification uses the current Radius plane and resource group hierarchy and does not introduce a new tenant concept.
- Authorizing direct access to internal Radius components as an alternative to the public control-plane API.

## User profile and challenges

### User persona(s)

**Primary user: Radius platform administrator**

The platform administrator operates a shared Radius installation for multiple teams. They establish access, delegate platform responsibilities, protect production environments and credentials, and need to prove that users and automation have no more access than their job requires.

**Secondary user: application developer or operator**

The application user creates, deploys, reads, troubleshoots, and operates assigned applications. They need predictable access to their team's resource groups and approved environments without gaining the ability to alter platform-wide Recipes, credentials, resource types, or access policy.

**Secondary user: platform capability owner**

The capability owner manages a subset of platform resources, such as environments, Recipe Packs, or resource type registrations. They need to maintain those resources without becoming a Radius or Kubernetes administrator.

**Secondary user: auditor or security operator**

The auditor needs read-only visibility into selected resources, role definitions, assignments, and authorization events. They must be able to determine who could perform or attempted an action without receiving write access or secret values.

**Secondary user: automation identity**

CI/CD workflows, GitHub Actions, service accounts, agents, and other automation need stable, revocable access that can be narrower than a human administrator's access and is attributable to the workload identity.

### Challenge(s) faced by the user

- Radius currently relies on access to the Kubernetes API server for inbound access. It does not expose a Radius-specific role model for different teams and resources.
- Broad access prevents platform administrators from separating environment, Recipe Pack, resource type, credential, application, and access-management duties.
- Application deployment crosses resource boundaries: a deployment writes application resources in one resource group while using an environment and its platform configuration, which may live in another resource group.
- Radius has multiple clients and automation paths. Client-only checks would be inconsistent and bypassable.
- Resource types are extensible. A role model tied only to today's built-in types will drift as new types and actions are registered.
- Administrators need enough context to resolve denied requests, while unauthorized users must not receive sensitive resource or policy details.
- Existing installations assume today's access behavior. Enabling enforcement without bootstrap and migration controls could lock out operators.

### Positive user outcome

As a Radius platform administrator, I can grant each person, team, or workload only the Radius capabilities and scopes they need, verify the effective access before rollout, review authorization activity, and revoke access predictably. Application teams can deploy and operate their applications in approved environments without being able to change the platform capabilities or credentials behind those environments.

## Key scenarios

### Scenario 1: Bootstrap local, shared, and ephemeral installations safely

A user installing Radius on a local development cluster becomes the initial Radius administrator without completing an additional authorization setup flow. A platform administrator installing Radius on a shared cluster explicitly bootstraps one or more administrator principals, while all other principals start with no Radius permissions.

An automation workflow that creates an isolated, ephemeral control plane uses an explicit automation bootstrap profile. The workflow identity is administrator only within that control-plane instance, and any restored authorization state is integrity-checked and preflighted before it becomes authoritative.

An existing installation can preview the effect of enforcement, correct missing assignments, and roll back before making RBAC authoritative. Radius must detect and prevent a transition or state restore that would leave no recoverable administrator.

### Scenario 2: Delegate an application team to an approved environment

A platform administrator grants a development group permission to create and manage applications and application resources in the group's resource group. The administrator separately grants that group permission to deploy to a specific non-production environment.

The group can deploy to that environment but cannot modify the environment, its Recipe Packs, settings, credentials, or resource type registrations. An attempt to deploy to production is denied before Radius starts changing application or cloud resources.

### Scenario 3: Separate platform capability ownership

An environment administrator manages selected environments. A Recipe Pack administrator maintains approved Recipe Packs, and a resource type administrator manages selected resource type registrations. These users do not automatically gain access to applications or authorization administration.

When an environment administrator attaches a Recipe Pack or settings resource to an environment, Radius verifies both permission to update the environment and permission to use the referenced platform resource. Application developers who later deploy to that environment do not need direct read or use permission on its Recipe Packs or engine settings.

Cloud credentials are not environment resources today. They are plane-scoped Radius resources used internally when an environment targets the corresponding provider. Deploy permission does not grant direct access to credential metadata or credential administration, but it does authorize Radius to use the configured plane credential for that environment's deployment. Cloud-side least privilege and the relationship between a shared plane credential and environment isolation must be explicit during setup.

### Scenario 4: Authorize CI/CD, GitOps controllers, and agent workflows

A team assigns a workload identity permission to deploy an application from CI/CD to one environment and to read the resulting application status. The workflow cannot manage roles, change platform resources, deploy other applications, or use another environment.

GitOps controllers and Flux reconciliation use a named, scoped controller identity. Radius attributes the action to that workload identity and records the source repository and revision as context; it does not treat the Git author as an authenticated Radius principal. Git write access can trigger only the Radius actions and scopes granted to the controller identity.

Repo Radius workflows that create a fresh control plane on a runner use the automation bootstrap profile from Scenario 1. Authorization failures are returned as terminal access failures rather than retried or represented as an empty successful result. The decision is attributed to the workload identity in audit events.

### Scenario 5: Investigate, explain, and revoke access

An administrator can inspect the built-in and custom roles, assignments, and effective permissions for a principal at a scope. A user can preflight whether they may perform an action. When an action is denied, the user receives the denied action and target scope plus a safe remediation path.

After an assignment is removed, new requests and retries are denied within a defined propagation objective. An operation already accepted by Radius follows the documented in-flight operation policy and remains attributable to the initiating principal.

### Scenario 6: Add a custom resource type without accidental privilege expansion

A platform team registers a new resource type and its actions. Existing custom roles do not gain permission to use or administer the new type unless an administrator explicitly updates them or previously chose a clearly marked future-inclusive permission pattern.

Administrators can create a custom role for the new type, assign it at an allowed scope, and use the same effective-access and audit tools as for built-in types.

## Key dependencies and risks

### Dependencies

- **Trusted principal contract** - Radius needs a verified principal identifier, principal type, and group or workload claims from the authentication boundary. The current Kubernetes API aggregation path authenticates clients, but the exact identity contract available to UCP must be validated before RBAC can enforce user and group assignments.
- **Complete operation catalog** - Every public API operation, including resource-specific actions such as application graph retrieval, logs, deployment, and credential operations, must map to a stable permission.
- **Central enforcement boundary** - Authorization must run at a server-side boundary that covers UCP routing, built-in resource providers, dynamic resource providers, deployment flows, asynchronous operations, and future resource types. Direct internal component access must not bypass the decision.
- **Authenticated internal communication** - UCP-to-resource-provider communication remains a separate trust-boundary concern tracked by [radius-project/radius#8083](https://github.com/radius-project/radius/issues/8083). RBAC cannot provide end-to-end enforcement if an untrusted caller can bypass UCP and invoke an internal provider as a trusted service.
- **Delegated operation identity** - Controllers, the deployment engine, reconcilers, and other internal workers must preserve a trustworthy initiating authorization context or use an operation-scoped internal identity that cannot expand the permissions already approved for the operation.
- **Authorization state storage and propagation** - Role definitions and assignments need durable storage, concurrency behavior, and bounded propagation across replicas.
- **State archive integrity and restore semantics** - Repo Radius archives the control-plane state used by ephemeral runs. Authorization definitions and assignments carried in that state need integrity protection, compatibility validation, bootstrap rules, and audit behavior that prevent a tampered or stale archive from granting administrator access.
- **Client capability discovery** - CLI, dashboard, Backstage, and agent experiences need an authoritative way to check effective capabilities. This improves UX but never replaces server-side enforcement.
- **Audit event destination** - Radius needs a supported way to emit, retain, and export authorization decisions and policy changes that fits its operational model.
- **Plane-scoped credential model** - Azure and AWS credentials are managed beneath their respective UCP planes rather than an environment. The authorization design must define administration and internal use across Radius, Azure, and AWS plane boundaries.

### Architecture review input

The feature is **feasible with conditions** based on current repository evidence. UCP already parses a resource ID, HTTP operation, correlation data, and several principal-related headers into an ARM request context, and Radius resources already use a hierarchical plane/resource-group/resource ID model. The conditions are: establish a non-spoofable principal contract, place enforcement before every externally reachable mutation and read, secure or isolate internal provider endpoints, define authorization behavior for cross-scope deployments and lists, and prove that dynamic resource types cannot omit enforcement.

This verdict is a feasibility input, not an implementation design. The detailed architecture must compare centralized and distributed enforcement options, define storage and cache consistency, model internal service identities, and produce a bypass analysis before implementation begins.

### Risks

- **Authorization bypass** - A direct resource-provider route, background worker, legacy API version, or newly registered resource type could omit enforcement. Mitigation requires a complete operation inventory, default-deny behavior for unmapped operations, and end-to-end negative tests.
- **Privilege escalation through references** - A user who can update an environment could attach a privileged Recipe Pack or settings resource they cannot otherwise use, or change inline provider and identity settings. Cross-resource references require explicit authorization, and sensitive inline settings require an appropriately granular environment permission.
- **Delegated authority becomes transitive** - Deploying to an environment necessarily causes Radius to use platform-owned Recipes, settings, and a plane-scoped cloud credential. The product must make this delegation explicit, prevent the caller from reading or reusing those dependencies directly, and explain that Radius RBAC does not reduce the cloud permissions held by the shared credential.
- **Lockout during bootstrap or migration** - A missing or invalid administrator assignment could make the installation unmanageable. Radius needs preflight, recoverability, and a documented break-glass path that is not a permanent authorization bypass.
- **Stale permissions** - Group membership, assignment changes, or cached decisions may take effect too slowly. Define and test a propagation objective and expose stale-state diagnostics.
- **Stale identity-provider group claims** - Some authentication paths refresh group claims only when a user reauthenticates. Radius must document claim freshness, distinguish it from assignment propagation, and avoid promising immediate revocation it cannot enforce.
- **Information disclosure** - Lists, graphs, error messages, effective-access views, and audit records can reveal resources or policy relationships. Responses must disclose only what the caller is allowed to know.
- **Client inconsistency** - A dashboard may hide an action that the API allows, or show an empty list when the API returned forbidden. Clients must use shared permission identifiers and preserve authorization errors.
- **Privilege drift as Radius evolves** - Wildcards or mutable built-in roles can silently grant access to new actions or resource types. Role evolution needs explicit compatibility rules and release visibility.
- **Performance and availability** - Authorization checks on every operation and filtered list can increase latency or create a control-plane dependency. The design needs measurable overhead and failure behavior that never fails open.
- **Confusion with external permissions** - A Radius authorization success does not guarantee the backing cloud, Kubernetes cluster, registry, or GitHub identity is authorized. Error messages must distinguish Radius denial from external provider denial.
- **Authorization state import** - Restoring an ephemeral control-plane archive can import obsolete or malicious role assignments. A restore must not silently replace trusted bootstrap policy or activate unverified access.

## Key assumptions to test and questions to answer

| Assumption or question | Current confidence | Validation plan | Owner |
| --- | --- | --- | --- |
| Platform teams need Radius-specific separation of duties beyond Kubernetes API access. | Medium. Supported by open enterprise and roadmap issues, but no direct customer evidence is attached. | Interview at least five shared-cluster or enterprise evaluators; ask about current access boundaries, not preferred RBAC features. Stop or rescope if teams consistently operate one trusted Radius admin identity and delegate only through CI/CD. | Product |
| Resource group and individual resource scopes cover the first release's delegation needs. | Low. Application membership is a relationship rather than an ID hierarchy. | Test proposed assignments against three real organization models: team-per-group, shared platform group, and shared application group. Identify cases that require application-aggregate or label-based scope. | Product and architecture |
| Users understand separate application-management and environment-deployment grants. | Medium. This follows common platform separation patterns and the prior Radius proposal, but is unvalidated. | Prototype role assignment and denied deployment flows with platform engineers and developers. | Product design |
| The Kubernetes aggregation path provides stable user, group, and workload identity data that UCP can trust. | Low until verified in supported cluster distributions. | Build a read-only principal diagnostic across local Kubernetes, AKS, and EKS authentication paths and document exact forwarded claims. | Architecture and security |
| Allow-only roles with implicit deny meet initial requirements. | Medium. They reduce conflict and explainability risk, while Argo CD demonstrates the complexity of explicit deny precedence. | Validate against target compliance scenarios and identify any requirement that cannot be represented without explicit deny. | Security and product |
| Environment-level deploy permission can safely delegate transitive Recipe, settings, and plane-credential use. | Medium. It matches the environment's product role, but the complete data and privilege flow needs threat modeling. | Threat-model a deployment that uses cross-group Recipe Packs and settings, inline provider identities, plane-scoped credentials, and cloud targets. | Security and architecture |
| Authorization policy can be evaluated with acceptable overhead on list and deployment operations. | Unknown. | Benchmark representative list, graph, and multi-resource deployment operations with policy evaluation and cache invalidation enabled. | Engineering |
| Existing installations can migrate without a prolonged compatibility mode. | Unknown. | Run migration rehearsals against representative single-user, shared-cluster, dashboard, and CI/CD installations. | Engineering and docs |
| Authorization state can be restored safely in Repo Radius ephemeral runs. | Low. Current archives contain the durable control-plane state, so role definitions and assignments are expected to move with it. | Prototype signed or otherwise integrity-verified restore, incompatible-state rejection, bootstrap reconciliation, and lockout recovery. | Security, Repo Radius, and architecture |
| A scoped controller identity is sufficient attribution for GitOps. | Medium. Git commits provide source context but not a trustworthy live Radius user identity. | Validate controller assignments and audit output against GitOps team workflows; identify any requirement for on-behalf-of user attribution. | Product, controller, and security |

## Current state

### Verified Radius behavior

- Radius inbound authentication and coarse authorization are delegated to the Kubernetes API server. The `rad` CLI uses the user's kubeconfig, and Radius does not issue user accounts, passwords, or tokens. See [Credentials in Radius](../../../docs/architecture/credentials.md#summary).
- Current threat models state that Radius has no granular authorization controls and relies on Kubernetes to authorize access to the aggregated API. Applications RP relies on UCP for future granular authorization. See the [UCP](./2024-11-ucp-component-threat-model.md#trust-model-of-ucp-clients), [Applications RP](./2024-08-applications-rp-component-threat-model.md), and [controller](./2024-08-controller-component-threat-model.md) threat models.
- UCP contains separate Radius, Azure, and AWS planes. Resource groups are first-class resources beneath the Radius plane. Applications, environments, Recipe Packs, configuration resources, and dynamic application resources are addressed beneath a Radius resource group and provider namespace. Resource type registrations are plane-scoped, and Azure and AWS credentials are resources beneath their corresponding provider planes.
- Applications reference an environment by full resource ID. Resources join an application by setting their `application` property, so application membership is not an ID parent-child relationship. See [`typespec/Radius.Core/applications.tsp`](../../../typespec/Radius.Core/applications.tsp).
- Recipe Packs are now first-class `Radius.Core/recipePacks` resources that can be shared across environments and resource groups. Environments reference Recipe Packs and optional Terraform and Bicep settings by resource ID. See [`typespec/Radius.Core/recipePacks.tsp`](../../../typespec/Radius.Core/recipePacks.tsp) and [`typespec/Radius.Core/environments.tsp`](../../../typespec/Radius.Core/environments.tsp).
- Azure and AWS credentials are registered separately from environments and stored as plane-scoped UCP resources plus Kubernetes Secrets. An environment selects provider targets and may contain inline identity settings, but it does not reference a credential resource. See [Credentials in Radius](../../../docs/architecture/credentials.md) and [`typespec/Radius.Core/environments.tsp`](../../../typespec/Radius.Core/environments.tsp).
- The `resource-types-contrib` repository publishes resource type definitions, Recipes, and Recipe Packs. Its artifacts are platform capabilities that Radius imports and uses; the repository does not provide Radius control-plane authorization.
- The dashboard is a Backstage application and is being productized as a distributable plugin. Its current design explicitly defers entity-level Radius authorization and requires forbidden responses to remain distinct from empty results. The backend reaches UCP through the Backstage Kubernetes integration. See [Radius Dashboard as a distributable Backstage plugin](https://github.com/radius-project/dashboard/blob/main/docs/design/2026-09-radius-backstage-plugin.md).
- The Radius Copilot and Canvas integration uses Repo Radius GitHub Actions workflows for operations. This introduces human and workload access paths that must receive the same server-side decisions as direct CLI and API callers. See [Radius Copilot app integration exception scenarios](https://github.com/radius-project/ai-extensions/blob/main/docs/design/2026-07-radius-copilot-app-exception-scenarios.md).
- Repo Radius deploy workflows create an ephemeral control plane on the runner, register platform resources, and restore and save durable control-plane state through an OCI archive. Authorization state stored in the same data store would therefore participate in restore and backup unless deliberately separated. See [Repo Radius deployment workflow](../environments/2026-06-repo-radius-deploy-workflow.md).
- The Kubernetes controller and Flux reconciliation call UCP using configured control-plane connectivity rather than an end user's live authenticated request. Their workload identity and source revision are therefore separate audit concepts.
- The application graph can expose resource topology and provider links, subject to existing sensitive-data redaction. Graph retrieval therefore requires an explicit read permission and cannot be treated as harmless metadata.

### Existing planning evidence

- [radius-project/radius#13030](https://github.com/radius-project/radius/issues/13030) asks for a role model and permission matrix, server and CLI enforcement, audit logs, secure defaults, and migration guidance as part of the enterprise-grade Radius epic.
- [radius-project/roadmap#27](https://github.com/radius-project/roadmap/issues/27) describes authorization controls for resource groups and built-in roles.
- [radius-project/roadmap#55](https://github.com/radius-project/roadmap/issues/55) separates Rego resource policy from RBAC.
- The [2024 authorization feature specification](https://github.com/radius-project/design-notes/blob/main/features/2024-11-authz-feature-spec.md) is useful evidence for persona separation, role definitions, assignments, and distinct application/deployment permissions. It is not the baseline for this specification because it assumes an unfinished tenant model, individual Recipes stored as environment properties, older resource namespaces, and unrelated CLI/resource-group changes.

### Comparative product evidence

- [Argo CD](https://argo-cd.readthedocs.io/en/stable/operator-manual/rbac/) maps users and SSO groups to roles and expresses permissions as resource, action, object, and allow/deny effect. It demonstrates the value of application-specific actions and the risks of glob matching, default roles, explicit deny precedence, and inheritance compatibility.
- [Grafana](https://grafana.com/docs/grafana/latest/administration/roles-and-permissions/access-control/) separates fixed and custom roles and models each permission as an action plus scope. It demonstrates assignable team and service-account roles, a broad permission catalog, and the privilege-drift risk when default roles gain new permissions.
- [Backstage](https://backstage.io/docs/permissions/overview/) separates centralized policy decisions from enforcement by the backend that owns a resource and supports conditional decisions. It demonstrates why UI checks are advisory, why each product surface must declare its actions, and why list filtering may require resource-aware enforcement.

### Evidence limitations

No customer interviews, support-ticket analysis, Radius authorization usage data, compliance controls, or measured latency budgets were provided. The first release should not claim a specific compliance certification or market demand until that evidence exists.

## Details of user problem

I operate Radius for more than one team. Today, if I let someone use the Radius API, I cannot express that they may manage only their applications, deploy only to approved environments, or administer only the Recipe Packs or resource types they own. I either grant broad access through the Kubernetes API boundary or create external workflow controls that are inconsistent with direct API access.

I need to separate platform management, application delivery, operations, auditing, and automation. I also need to understand the effective result of assignments, recover from mistakes, and prove that denied actions stay denied across the CLI, dashboard, APIs, and automated workflows. Without those controls, I cannot safely offer Radius as a shared production platform.

## Desired user experience outcome

I can assign a built-in or custom Radius role to a user, group, or workload identity at the Radius plane, a resource group, or an individual resource. The user can immediately see and perform only the allowed actions. Cross-scope operations require all relevant permissions, denied operations fail before causing side effects, and every decision can be traced without exposing secrets. I can preview and roll out enforcement safely, and I can recover administrative access if configuration is wrong.

### Detailed user experience

1. During installation or upgrade, I choose a local, shared, or automation installation profile.
   - A local profile binds the installing principal as Radius Administrator and requires no additional setup before the first deployment.
   - A shared profile requires explicit administrator principals and starts all other principals with no Radius access.
   - An automation profile binds an explicit workflow identity inside an isolated ephemeral control plane and defines how restored authorization state is validated and reconciled with bootstrap access.
   - An upgrade initially preserves behavior until I complete a migration preflight and explicitly enable enforcement.
2. I list built-in roles and inspect their descriptions and permissions. Built-in role definitions are versioned and cannot be edited or deleted.
3. I assign roles to a trusted principal identifier and scope. Radius validates the principal shape, role, scope, and my authority to grant that role.
4. Before enabling enforcement or making a sensitive assignment, I preview the effective access and whether at least one recoverable administrator remains.
5. Developers list only the resource groups and resources visible to them. A filtered list is distinguishable from a complete list when that distinction matters for administration and troubleshooting.
6. A developer deploys an application. Radius preflights permission to write the target application resources and to deploy to the selected environment before starting the deployment.
7. Radius uses the environment's attached Recipe Packs, settings, inline provider configuration, and the applicable plane-scoped cloud credential as platform-owned dependencies. The developer cannot inspect or directly use those dependencies unless separately authorized.
8. If access is denied, the API returns a stable authorization error. The CLI and graphical clients show the denied action, the target, the identity used, and a safe next step such as selecting another environment or contacting an administrator. They do not retry the request or show an empty-success state.
9. A client can ask whether the current principal may perform one or more actions so it can render accurate controls and perform preflight checks. The final action is still authorized by the server.
10. I inspect effective access and audit events to learn which assignment and role allowed or denied an operation. Sensitive resource payloads, tokens, group claims not needed for the decision, and secret values are not recorded.
11. I remove an assignment. New actions and retries reflect the change within the documented propagation objective. The behavior of already accepted asynchronous operations is documented and visible in the audit trail.
12. If policy administration would remove the last recoverable administrator or enable enforcement without an administrator, Radius rejects the change and explains the recovery requirement.
13. Before a multi-resource deployment begins, Radius evaluates the caller's complete requested Radius mutation set and environment use. If authorization changes after acceptance or a request cannot be fully preflighted, Radius follows a documented in-flight policy and reports any partial state plus a safe recovery action.
14. When a controller reconciles a Git revision, the audit event identifies the controller workload identity and source revision. The controller can act only within its assigned scopes.
15. When an automation run restores Radius state, it verifies the archive before applying authorization state, shows any bootstrap-policy conflict, and refuses an unsafe or incompatible restore.

## Key investments

### Feature 1: Radius permission and scope model

Define a stable permission catalog for Radius resources and actions. Permissions pair an action with a resource target and are evaluated at supported scope anchors: the Radius installation, an individual UCP plane, a Radius resource group, or an individual resource. Assignments inherit only along the documented hierarchy. Resource-group creation is authorized at the Radius plane or installation scope, and resource creation is authorized at the resource's parent scope.

The first release uses additive allow grants with implicit deny. Explicit deny is out of scope unless customer or compliance validation finds a blocking use case. Custom roles do not gain new permissions when Radius adds an action or resource type unless the administrator explicitly chose a future-inclusive permission pattern.

### Feature 2: Built-in and custom roles

Provide immutable, documented built-in roles for common separation-of-duties scenarios. The final permission matrix must include at least:

| Built-in role | Intended capability |
| --- | --- |
| Radius Administrator | Manage all Radius resources and authorization for the assigned installation or plane scope. |
| Access Administrator | Manage role definitions and assignments at an installation or plane scope without automatically receiving application or platform-resource access. |
| Platform Administrator | Manage Radius resource groups, environments, provider configuration, and platform settings without automatically administering access. Separate plane assignments govern Azure and AWS credential administration. |
| Application Developer | Create, read, update, delete, and operate applications and application resources in the assigned scope, but not select arbitrary environments. |
| Environment Deployer | Deploy applications to and read the minimum safe metadata for assigned environments without changing environment configuration. |
| Recipe Pack Administrator | Create and manage Recipe Packs and their Recipe definitions in the assigned scope. |
| Resource Type Administrator | Register and manage resource types and related schema metadata in the assigned scope. |
| Reader | Read authorized resource metadata and application graphs without mutation or secret access. |
| Auditor | Read authorization configuration and audit events without resource mutation or secret access. |

Administrators can compose custom roles from supported permissions when built-in roles are too broad. The product must show which permissions are grantable at which scopes and reject invalid combinations.

### Feature 3: Role assignments and effective-access inspection

Support assignments for users, groups, and workload identities using stable identifiers from the trusted authentication boundary. An assignment binds one role to one principal at one scope. Multiple assignments are additive.

Administrators and principals with appropriate permission can list assignments, inspect a principal's effective roles and permissions, preview a policy change, and ask whether a principal may perform a specified action. Users can inspect their own effective access without gaining access to other principals' group membership or unrelated policy. The effective-access view identifies the plane and scope hierarchy so similarly named resources on different planes cannot be confused.

### Feature 4: Cross-scope and transitive-use authorization

Define authorization for operations that read or mutate more than one resource:

- Creating or updating application resources requires write permission in the target resource group.
- Deploying requires permission to deploy to the selected environment in addition to application write permission.
- Updating an environment to reference a Recipe Pack, Bicep settings, Terraform settings, or future referenced platform resource requires permission to update the environment and use the referenced resource.
- Changing sensitive inline environment provider or identity settings requires a documented environment-administration permission.
- Deploy permission delegates Radius's internal use of the environment's configured dependencies and applicable plane-scoped cloud credential. It does not grant the caller direct read, update, or administration permission on those dependencies or credentials.
- Reading an application graph, logs, deployment status, secrets metadata, or output-resource links uses distinct documented permissions where disclosure or operational impact differs from ordinary resource reads.
- Multi-resource deployments perform authorization preflight for the complete requested Radius mutation set before resource creation begins. Downstream deployment-engine requests retain the approved operation context and cannot acquire broader service permissions.

The detailed permission matrix must cover legacy and preview API versions and all registered resource-specific actions, including existing `listsecrets` actions that return raw secret values; it must explicitly define whether each is internal-only or protected by a separate permission.

### Feature 5: Consistent enforcement and client behavior

Enforce authorization on the server for direct API, CLI, dashboard, Backstage, controller, GitHub Actions, agent, and other clients. Unknown or unmapped public operations fail closed. Controllers and reconcilers act as explicitly assigned workload identities, with source revision recorded as context rather than treated as user identity.

Clients use capability discovery to improve UX, but they preserve server authorization errors and never convert forbidden responses into empty resources, retries, cluster switching, or generic provider errors. Graphical clients hide or disable unavailable actions when they have a current capability decision and remain correct when that decision becomes stale.

### Feature 6: Auditability and safe administration

Emit structured events for authorization decisions and changes to roles, assignments, enforcement mode, and break-glass use. Events identify the principal, principal type, action, target scope, result, matched role or assignment identifier, correlation identifier, and reason category without recording sensitive payloads.

Role administration protects against self-escalation beyond delegated authority, circular or invalid definitions, deletion of referenced custom roles, and removal of the last recoverable administrator. A time-bounded, auditable break-glass procedure exists for cluster operators and is not part of normal access.

### Feature 7: Migration and compatibility

Provide an inventory and shadow-evaluation mode that reports what enforcement would deny without changing request outcomes. Administrators can generate initial assignments from observed principals only as a reviewable proposal, never as an automatic permanent grant.

Existing installations explicitly transition from compatibility mode to enforcement after validation. New shared installations start with enforcement enabled after bootstrap. Local installations retain a one-user setup path.

Ephemeral automation installations use a distinct bootstrap profile. If role definitions and assignments are included in archived control-plane state, restore verifies archive integrity, validates policy compatibility, prevents silent replacement of trusted bootstrap access, and audits the imported authorization revision. All modes and deprecation plans are visible and documented.

## Acceptance criteria

### Model and administration

1. A Radius administrator can list immutable built-in roles and create, update, and delete custom roles from documented permissions.
2. Radius rejects a custom role containing an unknown permission, an action invalid for the selected resource kind, or a scope pattern unsupported by that permission.
3. A permitted administrator can assign a role to a user, group, or workload identity at installation, plane, resource-group, or supported individual-resource scope.
4. Assignments at a broader scope apply only to documented descendants; assignments do not cross to sibling planes, resource groups, or resources.
5. Radius rejects a policy change that would leave enforcement enabled without a recoverable administrator.
6. A custom role does not automatically receive a newly introduced permission unless it contains an explicitly future-inclusive pattern whose effect was shown when authored.

### End-to-end enforcement

1. For every public API operation and API version, tests demonstrate an allowed request and a denied request at the authoritative server boundary. An operation without a permission mapping is denied.
2. The same principal, action, and target receive the same decision whether invoked through the direct API, `rad`, dashboard or Backstage plugin, supported controller path, GitHub Actions, or Radius agent integration.
3. A user with Application Developer access to one resource group can manage applications and resources there and cannot read or mutate a sibling resource group.
4. A user with deploy access to one environment can deploy an authorized application to it and cannot deploy to another environment.
5. Deploying to an environment does not grant the caller direct read or mutation access to the environment's Recipe Packs or settings, the applicable plane-scoped credential, or inline provider identity settings.
6. Updating an environment to attach a cross-scope Recipe Pack or settings resource is denied unless the caller can both update the environment and use the referenced resource.
7. A deployment with insufficient permission for any requested Radius mutation or the selected environment is denied during whole-request preflight before Radius begins a resource or cloud mutation.
8. A principal with resource read access but without graph or operational-data access cannot retrieve the protected application graph, logs, or equivalent action output.
9. Dynamic resource types and newly registered actions are denied until represented in the permission catalog and granted by a role.
10. A controller or Flux reconciliation request is evaluated as its configured workload identity, cannot exceed that identity's assignments, and records the source repository and revision without claiming the Git author as the authenticated Radius principal.
11. Downstream deployment-engine and worker requests cannot substitute an unrestricted service identity for the authorization context approved at deployment acceptance.
12. If authorization changes after a deployment is accepted, Radius applies the documented in-flight operation policy and reports any partial state and recovery action.

### Lists, errors, and clients

 1. List and search results contain only resources the caller may discover. Pagination does not leak hidden resource names, counts, or continuation behavior beyond the approved disclosure contract.
 2. A denied direct request returns a stable machine-readable authorization error and no protected resource payload.
 3. The CLI and graphical clients show the identity, denied action, target, and a safe recovery step without exposing unrelated assignments or secrets.
 4. Dashboard, Backstage, and agent clients preserve forbidden and partial-result states rather than presenting them as empty success or retrying them as transient failures.
 5. A client can check one or more capabilities for the current principal, and a stale positive capability check never bypasses authorization on the subsequent server request.

### Revocation, audit, and migration

 1. Removing an assignment prevents new requests within the defined propagation objective. The test suite verifies behavior across multiple control-plane replicas and documents when refreshed authentication claims are also required.
 2. Authorization decisions and policy mutations emit structured audit events with correlation identifiers and no secret values or protected request payloads.
 3. An administrator can trace an allow or deny decision to the effective role and assignment without receiving unauthorized group or resource data.
 4. An existing installation can run shadow evaluation, identify would-be denials, validate administrator recovery, enable enforcement, and roll back according to the supported migration procedure.
 5. New local and shared installations complete bootstrap without an unprotected window: the local installer is administrator, while a shared installation denies non-bootstrap principals by default.
 6. An ephemeral automation installation uses an explicit workflow bootstrap identity whose authority is limited to that isolated control-plane instance.
 7. A restored control-plane archive is integrity-verified and policy-preflighted before imported roles or assignments become authoritative. An unsafe, incompatible, or lockout-inducing restore is rejected and audited.
 8. Break-glass access is time-bounded, explicitly invoked, separately authorized at the cluster boundary, and always audited.

## Success measures

### Outcome measures

| Measure | Baseline | Initial target | Measurement plan and owner |
| --- | --- | --- | --- |
| Shared-installation pilots that enforce RBAC without a standing broad Kubernetes grant for application teams | Unknown | TODO after pilot recruitment | Product measures pilot configurations and interviews administrators after one release cycle. |
| Administrators who can complete the application-team delegation scenario without maintainer assistance | Unknown | TODO after usability baseline | Product design runs task-based validation for assignment, preflight, denial diagnosis, and revocation. |
| Unauthorized operations blocked in end-to-end conformance tests | No Radius-specific coverage | 100% of cataloged public operations and supported API versions | Engineering owns an operation-to-test traceability report. |
| Authorization decisions attributable through audit events | No Radius-specific decision events | 100% of enforced requests carry principal, action, target, result, and correlation identifier | Security and engineering validate sampled events and automated schema checks. |
| Existing-installation migrations completed without unrecoverable lockout | No migration path | 100% of qualification environments recover through the documented procedure | Release engineering runs migration rehearsals before default-on rollout. |

### Guardrail measures

| Guardrail | Baseline | Target or stop condition | Owner |
| --- | --- | --- | --- |
| Authorization bypasses | Granular RBAC absent | Zero known bypasses; any confirmed bypass stops rollout and triggers security response. | Security |
| Incorrect allow decisions after revocation | Unknown | Zero in conformance tests after the propagation objective; target duration is TODO after architecture benchmarking. | Engineering |
| Added request latency | Unknown | TODO based on representative API, list, graph, and deployment benchmarks; rollout stops if the agreed budget is exceeded. | Architecture and performance |
| False empty-success experiences after forbidden responses | Current clients vary | Zero in supported CLI, dashboard, Backstage, and agent authorization test cases. | Client owners |
| Administrator lockouts | Not measured | Zero unrecoverable lockouts in migration and upgrade qualification. | Release engineering |
| Sensitive data in audit or denial output | Existing redaction requirements apply | Zero secret values or protected payloads in automated fixtures and security review. | Security |

## Rollout and learning plan

1. **Problem and identity validation**
   - Conduct customer discovery with shared-cluster and enterprise evaluators.
   - Verify principal and group propagation on supported Kubernetes distributions.
   - Complete a threat model and operation inventory.
   - Stop or rescope if customers do not need Radius-specific delegation or if supported authentication paths cannot provide a trustworthy stable principal.
2. **Permission catalog and shadow evaluation**
   - Publish the draft permission matrix and built-in roles for review.
   - Add decision logging and shadow evaluation without denying requests.
   - Compare observed operations with the catalog and investigate every unmapped path.
   - Do not proceed if any externally reachable operation bypasses evaluation.
3. **Preview enforcement**
   - Enable RBAC for test installations and selected pilots with explicit bootstrap and rollback.
   - Qualify direct API, CLI, dashboard, Backstage, GitHub Actions, agent, dynamic-resource, graph, and asynchronous-operation paths.
   - Gather denial quality, role-fit, list behavior, latency, and propagation feedback.
4. **New shared installations**
   - Enable enforcement by default after bootstrap for new shared installations.
   - Keep local development bootstrap frictionless.
   - Existing installations remain in explicit compatibility mode until their administrator completes preflight.
5. **Existing-installation migration and general availability**
   - Publish migration guidance, role-diff tooling, audit integration guidance, and a compatibility-mode deprecation policy.
   - Revisit built-in roles and scope granularity using pilot evidence before declaring general availability.

Feedback comes from structured pilot interviews, support and issue analysis, authorization telemetry that contains no resource payloads, and conformance results. Rollout stops for a confirmed bypass, unrecoverable administrator lockout, unbounded stale authorization, sensitive audit disclosure, or latency beyond the approved guardrail.

## Open decisions

| Decision | Why it matters | Required evidence | Owner |
| --- | --- | --- | --- |
| Final built-in role permission matrix | Names alone do not prove separation of duties or prevent escalation. | Scenario walkthroughs, threat model, and customer role mapping. | Product and security |
| Supported principal and group identifiers | Email-like names can change or collide; stable IDs and issuer context affect portability and UX. | Authentication contract across local Kubernetes, AKS, EKS, dashboard, and automation. | Architecture and security |
| Individual-resource scope and application aggregate behavior | Application resources are siblings linked by a property, not children of the application resource ID. | Prototype authorization for create, deploy, graph, logs, and delete across realistic application layouts. | Product and architecture |
| List filtering and existence-disclosure contract | Filtering improves usability but affects pagination, counts, errors, and security. | Security review and API performance prototype. | API and security |
| Permission naming and wildcard semantics | Extensibility needs patterns, but patterns can grant future privileges accidentally. | Resource-type lifecycle analysis and administrator usability testing. | Architecture and product |
| In-flight operation behavior after revocation | Cancelling may leave partial infrastructure; continuing may outlive access. | Threat model and recovery analysis for deploy, delete, and asynchronous operations. | Product, security, and architecture |
| Authorization state storage and propagation objective | Revocation correctness and control-plane availability depend on it. | Architecture options, failure testing, and benchmarks. | Architecture |
| Plane-scoped credentials and environment isolation | One Azure or AWS plane credential may serve multiple environments, so Radius RBAC and cloud least privilege must compose predictably. | Threat model and deployment tests across non-production and production environments that share or isolate cloud credentials. | Product, security, and architecture |
| Controller and GitOps principal model | Git authorship is not a trusted live Radius identity, while a broad controller identity can become a confused deputy. | Controller flow prototype, scoped-assignment tests, and audit review. | Controller, product, and security |
| Ephemeral control-plane bootstrap and authorization-state restore | Repo Radius creates control planes per run and restores durable state, which can import privileges or lock out the workflow. | Archive integrity design, restore preflight prototype, and recovery rehearsal. | Repo Radius, security, and architecture |
| Audit retention, export, and administrator access | Enterprise evidence requirements differ and can create privacy obligations. | Customer compliance discovery and operational design. | Product, security, and operations |
| Compatibility-mode duration and break-glass mechanism | Too short risks lockout; too long preserves broad access. | Migration rehearsals and security review. | Release engineering and security |
| Policy administration UI scope | CLI and declarative management may be sufficient initially, but dashboard users need discoverability. | Product-design validation with platform administrators. | Product design |
