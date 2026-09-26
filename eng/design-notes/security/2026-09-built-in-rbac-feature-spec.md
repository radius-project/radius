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

An automation workflow that creates an isolated, ephemeral control plane uses an explicit automation identity whose access is limited to that instance. Restored access policy cannot silently override trusted bootstrap access or leave the workflow locked out.

An existing installation can preview the effect of enforcement, correct missing assignments, and roll back before making RBAC authoritative. Radius prevents a transition that would leave no recoverable administrator.

### Scenario 2: Delegate an application team to an approved environment

A platform administrator grants a development group permission to create and manage applications and application resources in the group's resource group. The administrator separately grants that group permission to deploy to a specific non-production environment.

The group can deploy to that environment but cannot modify the environment, its Recipe Packs, settings, credentials, or resource type registrations. An attempt to deploy to production is denied before Radius starts changing application or cloud resources.

### Scenario 3: Separate platform capability ownership

An environment administrator manages selected environments. A Recipe Pack administrator maintains approved Recipe Packs, and a resource type administrator manages selected resource type registrations. These users do not automatically gain access to applications or authorization administration.

When an environment administrator attaches a Recipe Pack or settings resource to an environment, Radius verifies both permission to update the environment and permission to use the referenced platform resource. Application developers who later deploy to that environment do not need direct read or use permission on its Recipe Packs or engine settings.

Deploy permission authorizes Radius to use the platform-managed cloud access configured for the selected environment. It does not grant the developer access to credential metadata or administration. Setup guidance makes clear that Radius RBAC and cloud-provider permissions are separate controls.

### Scenario 4: Authorize CI/CD, GitOps controllers, and agent workflows

A team assigns a workload identity permission to deploy an application from CI/CD to one environment and to read the resulting application status. The workflow cannot manage roles, change platform resources, deploy other applications, or use another environment.

GitOps controllers and other automation use named, scoped workload identities. Radius attributes actions to the workload identity and records useful source context without treating a code author as the authenticated Radius user.

Authorization failures are presented as access failures rather than retried or represented as successful empty results, and the responsible workload identity appears in the audit trail.

### Scenario 5: Investigate, explain, and revoke access

An administrator can inspect the built-in and custom roles, assignments, and effective permissions for a principal at a scope. A user can preflight whether they may perform an action. When an action is denied, the user receives the denied action and target scope plus a safe remediation path.

After an assignment is removed, new requests and retries are denied within a documented period. The product clearly explains what happens to work that was already in progress and keeps it attributable to the initiating principal.

### Scenario 6: Add a custom resource type without accidental privilege expansion

A platform team registers a new resource type and its actions. Existing custom roles do not gain permission to use or administer the new type unless an administrator explicitly updates them or previously chose a clearly marked future-inclusive permission pattern.

Administrators can create a custom role for the new type, assign it at an allowed scope, and use the same effective-access and audit tools as for built-in types.

## Key dependencies and risks

### Dependencies

- **Identity integration** - Radius must receive stable user, group, and workload identities from supported authentication providers so administrators can assign access predictably.
- **Consistent product coverage** - The API, CLI, dashboard, Backstage plugin, GitOps controllers, and agent experiences must recognize the same roles and permissions.
- **Safe adoption path** - Installation, migration, automation, and recovery flows must establish administrator access without preserving a permanent bypass.
- **Audit integration** - Organizations need a supported way to retain and export access decisions and policy changes.

### Risks

- **Roles or scopes do not match customer organizations** - If the built-in roles are too broad or the available scopes do not reflect team ownership, administrators will continue to rely on external workflow controls.
- **Delegated deployment is misunderstood** - A user allowed to deploy to an environment indirectly uses its platform-managed Recipes, settings, and cloud access. The product must explain this without implying that the user can inspect or reuse those dependencies.
- **Inconsistent access experiences** - Different results across the API, CLI, dashboard, Backstage, GitOps, and agents would make permissions difficult to trust and troubleshoot.
- **Administrative lockout or delayed revocation** - Unsafe bootstrap, migration, recovery, or revocation behavior could leave an installation inaccessible or a former user with access longer than administrators expect.
- **Sensitive information disclosure** - Denials, filtered lists, graphs, effective-access views, and audit records must not reveal resources or policy relationships the caller cannot otherwise see.
- **Confusion with external permissions** - Radius authorization does not guarantee access to the backing cloud, Kubernetes cluster, registry, or GitHub resources, so users need clear errors that identify which system denied an operation.

## Key assumptions to test and questions to answer

| Assumption or question                                                                                                                  | Current confidence                                                                                        | Validation plan                                                                                                                                                                                                                                     | Owner                           |
|-----------------------------------------------------------------------------------------------------------------------------------------|-----------------------------------------------------------------------------------------------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|---------------------------------|
| Platform teams need Radius-specific separation of duties beyond Kubernetes API access.                                                  | Medium. Supported by open enterprise and roadmap issues, but no direct customer evidence is attached.     | Interview at least five shared-cluster or enterprise evaluators; ask about current access boundaries, not preferred RBAC features. Stop or rescope if teams consistently operate one trusted Radius admin identity and delegate only through CI/CD. | Product                         |
| The proposed built-in roles and scopes match how organizations divide platform and application ownership.                               | Low. Radius has no customer-validated role model today.                                                   | Map the proposed model to representative team structures and identify responsibilities that require custom roles or additional scopes.                                                                                                              | Product                         |
| Users understand the distinction between managing an application, deploying to an environment, and administering platform capabilities. | Medium. The separation follows common platform patterns but is unvalidated in Radius.                     | Prototype role assignment, access explanation, and denied-operation flows with platform administrators and developers.                                                                                                                              | Product design                  |
| Allow-only roles with implicit deny are sufficient for the first release.                                                               | Medium. This is simpler to understand, but some organizations may expect explicit deny rules.             | Validate the model against customer governance scenarios and identify requirements that cannot be represented.                                                                                                                                      | Product and security            |
| Existing installations can adopt enforcement without unacceptable disruption or lockout risk.                                           | Unknown.                                                                                                  | Test migration and recovery guidance with single-user, shared-platform, and automated Radius installations.                                                                                                                                         | Product and release engineering |
| Effective-access views, denial messages, and audit records provide enough information for administrators to troubleshoot access safely. | Medium. Comparable products provide these capabilities, but the right Radius detail level is unvalidated. | Test common access investigations with administrators and auditors, including cases involving environment deployment and external-provider denials.                                                                                                 | Product design and security     |

## Current state

### Verified Radius behavior

- Radius relies on the Kubernetes authentication boundary and does not provide Radius-specific roles or resource-level authorization today. See [Credentials in Radius](../../../docs/architecture/credentials.md#summary) and the [UCP threat model](./2024-11-ucp-component-threat-model.md#trust-model-of-ucp-clients).
- Radius resources span installation, plane, resource-group, and individual-resource scopes. Applications also use environments and other platform capabilities that may be owned by a different team, so authorization cannot be based only on simple ownership.
- Environments, Recipe Packs, resource types, provider configuration, and credentials are distinct platform capabilities that require different administrative responsibilities.
- The API, `rad` CLI, dashboard and Backstage plugin, controllers, GitHub Actions, and agent experiences all need consistent authorization behavior. See the [dashboard design](https://github.com/radius-project/dashboard/blob/main/docs/design/2026-09-radius-backstage-plugin.md) and [Radius Copilot integration design](https://github.com/radius-project/ai-extensions/blob/main/docs/design/2026-07-radius-copilot-app-exception-scenarios.md).
- Repo Radius and other automation can create short-lived control planes and restore prior state, so bootstrap and recovery behavior must account for both human and workload access. See the [Repo Radius deployment workflow](../environments/2026-06-repo-radius-deploy-workflow.md).

### Existing planning evidence

- [radius-project/radius#13030](https://github.com/radius-project/radius/issues/13030) asks for a role model and permission matrix, server and CLI enforcement, audit logs, secure defaults, and migration guidance as part of the enterprise-grade Radius epic.
- [radius-project/roadmap#27](https://github.com/radius-project/roadmap/issues/27) describes authorization controls for resource groups and built-in roles.
- [radius-project/roadmap#55](https://github.com/radius-project/roadmap/issues/55) separates Rego resource policy from RBAC.
- The [2024 authorization feature specification](https://github.com/radius-project/design-notes/blob/main/features/2024-11-authz-feature-spec.md) provides useful prior thinking about personas, roles, assignments, and deployment permissions, but it predates the current Radius resource model.

### Comparative product evidence

- [Argo CD](https://argo-cd.readthedocs.io/en/stable/operator-manual/rbac/) demonstrates application-focused roles and the usability tradeoffs of allow, deny, inheritance, and pattern matching.
- [Grafana](https://grafana.com/docs/grafana/latest/administration/roles-and-permissions/access-control/) demonstrates fixed and custom roles, action-and-scope permissions, and assignments to teams and service accounts.
- [Backstage](https://backstage.io/docs/permissions/overview/) demonstrates consistent permission decisions across user interfaces and the services that own protected resources.

### Evidence limitations

No customer interviews, support-ticket analysis, Radius authorization usage data, compliance controls, or measured latency budgets were provided. The first release should not claim a specific compliance certification or market demand until that evidence exists.

## Details of user problem

I operate Radius for more than one team. Today, if I let someone use the Radius API, I cannot express that they may manage only their applications, deploy only to approved environments, or administer only the Recipe Packs or resource types they own. I either grant broad access through the Kubernetes API boundary or create external workflow controls that are inconsistent with direct API access.

I need to separate platform management, application delivery, operations, auditing, and automation. I also need to understand the effective result of assignments, recover from mistakes, and prove that denied actions stay denied across the CLI, dashboard, APIs, and automated workflows. Without those controls, I cannot safely offer Radius as a shared production platform.

## Desired user experience outcome

I can assign a built-in or custom Radius role to a user, group, or workload identity at the Radius plane, a resource group, or an individual resource. The user can immediately see and perform only the allowed actions. Cross-scope operations require all relevant permissions, denied operations fail before causing side effects, and every decision can be traced without exposing secrets. I can preview and roll out enforcement safely, and I can recover administrative access if configuration is wrong.

### Detailed user experience

1. During installation or upgrade, I choose a local, shared, or automation installation profile.
   - Local installations remain easy to start.
   - Shared installations require explicit administrators and deny access to everyone else by default.
   - Automation receives access only within its intended Radius instance.
   - Existing installations can preview and validate enforcement before enabling it.
2. I list built-in roles and inspect their descriptions and permissions. Built-in role definitions are versioned and cannot be edited or deleted.
3. I assign a role to a user, group, or workload identity at an appropriate Radius scope.
4. Before enabling enforcement or changing sensitive access, I preview the result and confirm that administrator recovery remains possible.
5. Users see only the resources and actions available to them, and clients clearly distinguish restricted results from genuinely empty results.
6. A developer can deploy an authorized application only to an approved environment. Radius may use the environment's platform-managed dependencies without exposing or granting direct access to them.
7. A denied action produces a consistent explanation of the identity, action, target, and safe next step across the API, CLI, and graphical clients.
8. I inspect effective access and audit events to understand why access was allowed or denied without exposing secrets or unrelated policy.
9. When I revoke access, new actions reflect the change within a documented period, and the product explains the treatment of work already in progress.
10. Automation and controllers act only within their assigned scopes and remain attributable to their workload identities.

## Key investments

### Feature 1: Radius permission and scope model

Define a stable permission catalog for Radius resources and actions. Administrators can grant permissions at useful Radius scopes, including the installation, a plane, a resource group, or a supported individual resource. The product clearly explains how broader assignments apply to contained resources and where they do not.

The first release uses additive allow grants with implicit deny. Explicit deny remains out of scope unless customer or compliance validation finds a blocking use case. Custom roles do not silently gain access when Radius adds actions or resource types.

### Feature 2: Built-in and custom roles

Provide immutable, documented built-in roles for common separation-of-duties scenarios. The final permission matrix must include at least:

| Built-in role               | Intended capability                                                                                                                                                                                               |
|-----------------------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| Radius Administrator        | Manage all Radius resources and authorization for the assigned installation or plane scope.                                                                                                                       |
| Access Administrator        | Manage role definitions and assignments at an installation or plane scope without automatically receiving application or platform-resource access.                                                                |
| Platform Administrator      | Manage Radius resource groups, environments, provider configuration, and platform settings without automatically administering access. Separate plane assignments govern Azure and AWS credential administration. |
| Application Developer       | Create, read, update, delete, and operate applications and application resources in the assigned scope, but not select arbitrary environments.                                                                    |
| Environment Deployer        | Deploy applications to and read the minimum safe metadata for assigned environments without changing environment configuration.                                                                                   |
| Recipe Pack Administrator   | Create and manage Recipe Packs and their Recipe definitions in the assigned scope.                                                                                                                                |
| Resource Type Administrator | Register and manage resource types and related schema metadata in the assigned scope.                                                                                                                             |
| Reader                      | Read authorized resource metadata and application graphs without mutation or secret access.                                                                                                                       |
| Auditor                     | Read authorization configuration and audit events without resource mutation or secret access.                                                                                                                     |

Administrators can compose custom roles from supported permissions when built-in roles are too broad. The product must show which permissions are grantable at which scopes and reject invalid combinations.

### Feature 3: Role assignments and effective-access inspection

Support assignments for users, groups, and workload identities. An assignment binds one role to one principal at one scope, and multiple assignments are additive.

Administrators can list assignments, inspect effective access, preview policy changes, and check whether a principal may perform an action. Users can inspect their own access without learning about unrelated principals or policy.

### Feature 4: Cross-scope and transitive-use authorization

Define authorization for operations that read or mutate more than one resource:

- Application management and environment deployment are separate permissions.
- Referencing a platform capability requires appropriate access to both the resource being changed and the referenced capability.
- Deploying to an environment allows Radius to use its platform-managed dependencies but does not grant the caller direct access to them.
- Sensitive views and operations, such as graphs, logs, and secret-related actions, may require permissions beyond ordinary resource read access.
- Radius checks the required access before beginning a multi-resource operation and provides a clear recovery path if authorization changes while work is in progress.

### Feature 5: Consistent enforcement and client behavior

Enforce authorization consistently for the API, CLI, dashboard, Backstage, controllers, GitHub Actions, agents, and other supported clients. New operations and resource types do not become accessible until their permissions are deliberately defined and granted. Automation acts through explicit workload identities.

Clients can use capability checks to improve the experience, but the server remains authoritative. Clients preserve authorization errors instead of presenting them as empty results, retries, or unrelated failures.

### Feature 6: Auditability and safe administration

Record authorization decisions and administrative changes with enough information to identify who acted, what they attempted, the target, the result, and the policy that affected the decision without recording sensitive data.

Role administration prevents unauthorized escalation and loss of all administrator access. A time-bounded, auditable recovery procedure exists for exceptional lockout situations and is not part of normal access.

### Feature 7: Migration and compatibility

Provide an inventory and preview mode that shows what enforcement would deny without changing request outcomes. Any suggested initial assignments require administrator review before becoming active.

Existing installations explicitly enable enforcement after validation. New shared installations start with enforcement enabled after bootstrap, while local installations retain a simple one-user setup.

Automation installations use a dedicated bootstrap path, and restored access policy cannot silently replace trusted bootstrap access or create an unrecoverable lockout. All installation modes and compatibility timelines are documented.

## Acceptance criteria

### Model and administration

1. A Radius administrator can list immutable built-in roles and create, update, and delete custom roles from documented permissions.
2. Radius rejects unsupported permission and scope combinations with an actionable explanation.
3. A permitted administrator can assign a role to a user, group, or workload identity at installation, plane, resource-group, or supported individual-resource scope.
4. Broader assignments apply only to documented contained scopes and do not cross unrelated planes, resource groups, or resources.
5. Radius rejects a policy change that would leave enforcement enabled without a recoverable administrator.
6. Custom roles do not silently receive newly introduced permissions.

### End-to-end enforcement

1. The same identity, action, and target receive the same authorization result across every supported Radius client and automation path.
2. An application team can manage resources in its assigned scope and cannot discover or change resources in an unrelated scope.
3. A user can deploy an authorized application only to environments where they have deploy access.
4. Deploying to an environment does not grant direct access to its platform-managed Recipes, settings, credentials, or provider identity.
5. A user cannot attach or change a referenced platform capability without the required access to both the resource being changed and the referenced capability.
6. Radius denies an unauthorized multi-resource operation before beginning changes.
7. Resource read access does not automatically grant access to protected graphs, logs, secret-related actions, or other sensitive operational data.
8. New resource types and actions remain inaccessible until an administrator deliberately grants their permissions.
9. Automation cannot exceed its assigned access, and its actions remain attributable to its workload identity.
10. Radius documents and reports what happens when access changes while an operation is already in progress.

### Lists, errors, and clients

1. Lists and searches reveal only resources the caller is allowed to discover and clearly communicate when results are access-filtered where appropriate.
2. A denied request returns a consistent authorization error and no protected resource data.
3. The CLI and graphical clients show the identity, denied action, target, and a safe next step without exposing unrelated assignments or secrets.
4. Supported clients preserve forbidden and partial-result states instead of presenting them as empty success or retrying them as transient failures.
5. Clients can check current capabilities to improve the experience, but a capability result never replaces authorization of the requested action.

### Revocation, audit, and migration

1. Removing an assignment prevents new requests within the documented revocation period.
2. Authorization decisions and access-policy changes produce audit records without secret values or protected request data.
3. An administrator can trace an allow or deny decision to the effective role and assignment without receiving unauthorized identity or resource information.
4. An existing installation can preview enforcement, resolve access gaps, enable enforcement, and recover according to the supported migration procedure.
5. Local, shared, and automation installations establish initial access appropriate to their use case without leaving an unprotected window.
6. Restoring previous Radius state cannot silently replace trusted bootstrap access or create an unrecoverable lockout.
7. Exceptional recovery access is time-bounded, explicitly invoked, and always audited.

## Success measures

### Outcome measures

| Measure                                                                                                      | Baseline                    | Initial target                      | Measurement plan and owner                                                                             |
|--------------------------------------------------------------------------------------------------------------|-----------------------------|-------------------------------------|--------------------------------------------------------------------------------------------------------|
| Shared-installation pilots that enforce RBAC without broad Kubernetes access for application teams           | Unknown                     | TODO after pilot recruitment        | Product reviews pilot configuration and administrator feedback after one release cycle.                |
| Administrators who complete the application-team delegation scenario without maintainer assistance           | Unknown                     | TODO after usability baseline       | Product design validates assignment, access explanation, denial diagnosis, and revocation tasks.       |
| Administrators who can correctly explain and resolve a denied action                                         | Unknown                     | TODO after usability baseline       | Product design tests representative denials across applications, environments, and platform resources. |
| Existing installations that migrate without unrecoverable lockout or an extended compatibility-mode fallback | No supported migration path | 100% of qualification installations | Release engineering validates the documented migration and recovery experience before default rollout. |

### Guardrail measures

| Guardrail                                                 | Baseline                              | Target or stop condition                                                                                           | Owner               |
|-----------------------------------------------------------|---------------------------------------|--------------------------------------------------------------------------------------------------------------------|---------------------|
| Unauthorized access                                       | Granular RBAC absent                  | Zero known authorization bypasses; any confirmed bypass stops rollout.                                             | Security            |
| Incorrect access after revocation                         | Unknown                               | Zero known cases beyond the documented revocation period.                                                          | Engineering         |
| False empty-success experiences after forbidden responses | Current clients vary                  | Zero known cases in supported clients.                                                                             | Client owners       |
| Administrator lockouts                                    | Not measured                          | Zero unrecoverable lockouts during installation, migration, or recovery.                                           | Release engineering |
| Sensitive data in audit or denial output                  | Existing redaction requirements apply | Zero known disclosures of secret values or protected payloads.                                                     | Security            |
| User-visible performance regression                       | Unknown                               | No material degradation to common read, deployment, or access-administration workflows; target set before preview. | Product             |

## Rollout and learning plan

1. **Problem and role-model validation**
   - Validate the need for Radius-specific delegation with shared-platform and enterprise evaluators.
   - Test the proposed roles, scopes, and separation of application and platform responsibilities.
   - Stop or rescope if customers do not need Radius-specific delegation or the model does not fit their operating structures.
2. **Access preview**
   - Publish the draft permission matrix and built-in roles for review.
   - Let administrators preview how proposed enforcement affects current users and automation.
   - Refine role coverage, explanations, and recovery guidance before enforcement.
3. **Preview enforcement**
   - Enable RBAC for selected pilots with explicit bootstrap, recovery, and rollback.
   - Gather feedback on delegation, denial quality, troubleshooting, revocation, and client consistency.
4. **New shared installations**
   - Enable enforcement by default after bootstrap for new shared installations.
   - Keep local development bootstrap frictionless.
   - Keep existing installations in compatibility mode until an administrator completes access preview.
5. **Existing-installation migration and general availability**
   - Publish migration, recovery, and audit-integration guidance with a compatibility-mode deprecation policy.
   - Revisit built-in roles and scope granularity using pilot evidence before declaring general availability.

Feedback comes from pilot interviews, usability studies, support and issue analysis, and privacy-preserving product telemetry. Rollout stops for unauthorized access, unrecoverable administrator lockout, unacceptable revocation delay, sensitive information disclosure, or material user-facing performance regression.

## Open decisions

| Decision                                                     | Why it matters                                                                                                    | Required evidence                                                  | Owner                               |
|--------------------------------------------------------------|-------------------------------------------------------------------------------------------------------------------|--------------------------------------------------------------------|-------------------------------------|
| Final built-in role permission matrix                        | The roles must match real separation-of-duties needs without encouraging broad grants.                            | Customer role mapping, scenario walkthroughs, and security review. | Product and security                |
| Supported identity types and administrator-facing names      | Administrators need stable identities they can recognize and manage across human and automation use cases.        | Authentication constraints and administrator usability testing.    | Product, architecture, and security |
| Scope and inheritance model                                  | The model must fit team ownership while remaining understandable for applications and related resources.          | Customer organization models and access-management prototypes.     | Product and architecture            |
| List filtering and resource-disclosure experience            | Users need useful results without learning about resources they cannot access.                                    | User testing and security review.                                  | Product design and security         |
| Explicit deny and future-inclusive permissions               | These features may meet advanced governance needs but can make access difficult to predict.                       | Customer governance scenarios and usability testing.               | Product and security                |
| Treatment of work already in progress after revocation       | Users need predictable security and recovery behavior when access changes during a long-running operation.        | Customer expectations, threat modeling, and recovery scenarios.    | Product and security                |
| Automation bootstrap, attribution, and recovery experience   | CI/CD and GitOps need useful least-privilege access without becoming hidden administrators or getting locked out. | Automation workflow validation and administrator interviews.       | Product and security                |
| Audit retention, export, and access                          | Organizations have different evidence, privacy, and operational requirements.                                     | Customer compliance discovery.                                     | Product and security                |
| Compatibility-mode duration and exceptional recovery process | Existing installations need enough time to migrate without leaving broad access enabled indefinitely.             | Pilot migration and recovery feedback.                             | Product and release engineering     |
| Policy administration UI scope                               | CLI and declarative management may be sufficient initially, but administrators still need discoverability.        | Product-design validation with platform administrators.            | Product design                      |
