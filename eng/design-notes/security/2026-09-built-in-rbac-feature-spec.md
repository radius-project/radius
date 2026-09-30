# Topic: Built-in role-based access control (RBAC) for Radius

- **Author**: Will Tsai (@willtsai)

## Topic Summary

Radius needs a built-in authorization model that lets organizations control who can view, create, change, deploy, and administer Radius resources. The model must support separation of duties between platform teams, application teams, operators, auditors, and automation without requiring every user to receive broad Kubernetes access to the Radius API.

This specification defines the product behavior for role-based access control (RBAC) across the Radius API, `rad` CLI, dashboard and Backstage plugin, and Radius automation such as GitHub Actions and the Radius Copilot integration. RBAC administration is available through the API and through imperative and declarative `rad` CLI workflows; graphical administration is out of scope. It covers Radius resources including resource groups, applications, application resources, environments, Recipe Packs and their Recipes, resource type registrations, configuration resources, credentials, authorization resources, and resource-specific actions.

The specification enables product, design, security, and architecture stakeholders to decide the scope and user experience of the first enterprise RBAC release tracked by [radius-project/radius#13030](https://github.com/radius-project/radius/issues/13030) and [radius-project/roadmap#27](https://github.com/radius-project/roadmap/issues/27). Direct customer research, compliance requirements, and production usage baselines are not yet available. Statements about demand beyond those roadmap items are hypotheses that require validation.

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
- Defining attribute-based access control, resource policy management, admission policy, or governance rules about allowed resource configuration. These are distinct from RBAC and [radius-project/roadmap#55](https://github.com/radius-project/roadmap/issues/55) tracks policy management.
- Administrator-defined property-level or field-level permissions within a Radius resource. Radius may still provide product-defined safe views and redact sensitive fields.
- Granting access to raw secret values. Existing secret handling and redaction requirements continue to apply regardless of role.
- Automatically deriving Radius access from Azure, AWS, Kubernetes namespace, GitHub repository, or Backstage catalog permissions.
- Managing roles, assignments, or enforcement settings through the Radius dashboard, Backstage plugin, or another graphical interface. Graphical clients must honor RBAC and present safe authorization results, but RBAC administration is limited to the API, imperative `rad auth` commands, and declarative Bicep deployments through `rad deploy`.
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

Local, shared, and automation installations establish initial administrator access appropriate to their use case without an unprotected window. Existing installations can preview enforcement and correct access gaps before enabling it, and Radius prevents changes that would leave no recoverable administrator.

### Scenario 2: Delegate an application team to an approved environment

A platform administrator grants a development team permission to manage its applications and deploy them to an approved non-production environment. The team cannot change the environment or its platform-managed capabilities, and an attempt to deploy elsewhere is denied before Radius begins making changes.

### Scenario 3: Separate platform capability ownership

Environment, Recipe Pack, and resource type owners can manage only the platform capabilities delegated to them without gaining application or access-administration privileges. Application teams can deploy through an approved environment without receiving direct access to its Recipes, settings, credentials, or provider configuration.

### Scenario 4: Authorize CI/CD, GitOps controllers, and agent workflows

Teams give CI/CD, GitOps controllers, agents, and other automation narrowly scoped workload identities for specific applications and environments. Automated actions remain attributable to the workload identity, and denials appear as access failures rather than retries or successful empty results.

### Scenario 5: Investigate, explain, and revoke access

Administrators and users can understand effective access, preview whether an action is allowed, and receive a safe remediation path when it is denied. Revocation affects new work within a documented period, while the product explains and attributes work that was already in progress.

### Scenario 6: Add a custom resource type without accidental privilege expansion

When a platform team adds a resource type or action, existing roles do not silently gain access. Administrators can deliberately extend custom roles and use the same assignment, explanation, and audit experience as for built-in resources.

## Key dependencies and risks

- **Dependency: Identity integration** - Radius must receive stable user, group, and workload identities from supported authentication providers so administrators can assign access predictably. **Owner:** Architecture and security.
- **Dependency: Consistent product coverage** - The API, CLI, dashboard, Backstage plugin, GitOps controllers, and agent experiences must recognize the same roles and permissions. **Owner:** Product and client owners.
- **Dependency: Safe adoption path** - Installation, migration, automation, and recovery flows must establish administrator access without preserving a permanent bypass. **Owner:** Product and release engineering.
- **Dependency: Audit integration** - Organizations need a supported way to retain and export access decisions and policy changes. **Owner:** Product and security.
- **Risk: Roles or scopes do not match customer organizations** - If the built-in roles are too broad or the available scopes do not reflect team ownership, administrators will continue to rely on external workflow controls. Validate the model with representative platform teams before finalizing it.
- **Risk: Delegated deployment is misunderstood** - A user allowed to deploy to an environment indirectly uses its platform-managed Recipes, settings, and cloud access. Explain this delegation without implying that the user can inspect or reuse those dependencies.
- **Risk: Inconsistent access experiences** - Different results across the API, CLI, dashboard, Backstage, GitOps, and agents would make permissions difficult to trust and troubleshoot. Treat cross-client consistency as an acceptance and rollout criterion.
- **Risk: Administrative lockout or delayed revocation** - Unsafe bootstrap, migration, recovery, or revocation behavior could leave an installation inaccessible or a former user with access longer than administrators expect. Require preview, recovery, and revocation qualification before default rollout.
- **Risk: Sensitive information disclosure** - Denials, filtered lists, graphs, effective-access views, and audit records must not reveal resources or policy relationships the caller cannot otherwise see. Validate these experiences through security and product-design review.
- **Risk: Confusion with external permissions** - Radius authorization does not guarantee access to the backing cloud, Kubernetes cluster, registry, or GitHub resources. Errors and guidance must identify which system denied an operation.

## Key assumptions to test and questions to answer

| Assumption or question | Current confidence | Validation plan | Owner |
| --- | --- | --- | --- |
| Supported authentication paths provide stable user, group, and workload identity attributes suitable for assignments. | Low until validated across supported installation contexts. | Document the available identity attributes for local Kubernetes, managed Kubernetes, dashboard, and automation paths. Rescope group assignments if stable group identity is unavailable. | Architecture and security |
| Allow-only roles with implicit deny are sufficient for the first release. | Medium. This is simpler to understand, but some organizations may expect explicit deny rules. | Validate the model against customer governance scenarios and identify requirements that cannot be represented. | Product and security |
| Existing installations can adopt enforcement without unacceptable disruption or lockout risk. | Unknown. | Test migration and recovery guidance with single-user, shared-platform, and automated Radius installations. | Product and release engineering |
| Effective-access views, denial messages, and audit records provide enough information for administrators to troubleshoot access safely. | Medium. Comparable products provide these capabilities, but the right Radius detail level is unvalidated. | Test common access investigations with administrators and auditors, including cases involving environment deployment and external-provider denials. | Product design and security |

## Current state

**Verified current behavior.** Radius relies on the Kubernetes authentication boundary and does not provide Radius-specific roles or resource-level authorization today. Its resources span installation, plane, resource-group, and individual-resource scopes, while application deployment uses environments and platform capabilities that may be owned by another team. The API, `rad` CLI, dashboard and Backstage plugin, controllers, GitHub Actions, and agent experiences expose multiple access paths that need consistent behavior. Relevant references include [Credentials in Radius](../../../docs/architecture/credentials.md#summary), the [UCP threat model](./2024-11-ucp-component-threat-model.md#trust-model-of-ucp-clients), the [dashboard design](https://github.com/radius-project/dashboard/blob/main/docs/design/2026-09-radius-backstage-plugin.md), the [Radius Copilot integration design](https://github.com/radius-project/ai-extensions/blob/main/docs/design/2026-07-radius-copilot-app-exception-scenarios.md), and the [Repo Radius deployment workflow](../environments/2026-06-repo-radius-deploy-workflow.md).

**Existing planning evidence.** [radius-project/radius#13030](https://github.com/radius-project/radius/issues/13030) and [radius-project/roadmap#27](https://github.com/radius-project/roadmap/issues/27) call for Radius authorization controls, built-in roles, auditability, secure defaults, and migration guidance. [radius-project/roadmap#55](https://github.com/radius-project/roadmap/issues/55) treats resource policy as separate from RBAC. The [2024 authorization feature specification](https://github.com/radius-project/design-notes/blob/main/features/2024-11-authz-feature-spec.md) provides prior thinking but predates the current Radius resource model.

**Comparative evidence.** [Argo CD](https://argo-cd.readthedocs.io/en/stable/operator-manual/rbac/) demonstrates application-focused roles and the usability tradeoffs of allow, deny, inheritance, and pattern matching. [Grafana](https://grafana.com/docs/grafana/latest/administration/roles-and-permissions/access-control/) demonstrates fixed and custom roles, action-and-scope permissions, and assignments to teams and service accounts. [Backstage](https://backstage.io/docs/permissions/overview/) demonstrates consistent permission decisions across user interfaces and the services that own protected resources.

## Details of user problem

I operate Radius for more than one team. Today, if I let someone use the Radius API, I cannot express that they may manage only their applications, deploy only to approved environments, or administer only the Recipe Packs or resource types they own. I either grant broad access through the Kubernetes API boundary or create external workflow controls that are inconsistent with direct API access.

I need to separate platform management, application delivery, operations, auditing, and automation. I also need to understand the effective result of assignments, recover from mistakes, and prove that denied actions stay denied across the CLI, dashboard, APIs, and automated workflows. Without those controls, I cannot safely offer Radius as a shared production platform.

## Desired user experience outcome

I can assign a built-in or custom Radius role to a user, group, or workload identity at the Radius plane, a resource group, or an individual resource. The user can immediately see and perform only the allowed actions. Cross-scope operations require all relevant permissions, denied operations fail before causing side effects, and every decision can be traced without exposing secrets. I can preview and roll out enforcement safely, and I can recover administrative access if configuration is wrong.

### Detailed user experience

**As a Radius platform administrator,** I can establish initial administrator access appropriate for local development, a shared installation, or automation. I can understand the built-in roles, assign built-in or custom roles at appropriate Radius scopes, and preview the effect before enabling enforcement or making a sensitive change. Radius protects me from removing all recoverable administrator access. When I revoke access, new actions reflect the change within a documented period, and I can understand what happens to work already in progress.

**As an application developer or operator,** I see only the resources and actions available to me. I can manage my team's applications and deploy them to approved environments without gaining access to the platform-managed Recipes, settings, credentials, or resource types behind those environments. When an action is denied, every supported client consistently explains which identity, action, and target were denied and gives me a safe next step rather than showing an empty or unrelated error.

**As a platform capability owner,** I can manage the environments, Recipe Packs, resource types, or other platform capabilities delegated to me without becoming a Radius or Kubernetes administrator. Radius requires appropriate access when I connect capabilities across scopes and makes clear when I need help from another owner or administrator.

**As an auditor or security operator,** I can inspect effective access and authorization activity to understand who could perform or attempted an action and which role or assignment affected the decision. I can investigate access without receiving write permissions, secret values, or unrelated identity and resource information.

**As an automation identity owner,** I can give CI/CD, controllers, agents, and other automation only the access needed for a specific Radius instance, application, and environment. Automated actions remain attributable to the workload identity, and authorization failures appear as clear access failures rather than retries or successful empty results.

### Identity and sign-in experience

Users do not create a separate Radius account or password. They authenticate through the trusted identity system configured for their Radius installation and continue to use the sign-in experience their organization already provides. The first release uses identities established through the Kubernetes access path; support for other trusted identity providers may be added when Radius supports clients that connect through a different authentication boundary.

Administrators assign roles to stable user, group, and workload identities supplied by that trusted system. Radius shows a recognizable name when available together with the identity type and source, so administrators can distinguish similarly named identities and avoid granting access to the wrong principal. Group-based access follows changes in the source identity system within a documented period rather than requiring administrators to duplicate group membership in Radius.

Every supported client preserves the identity of the person or workload performing the action. Users can see which identity Radius recognizes and receive a clear authentication error when no supported identity is available. Authorization denials separately explain that the recognized identity lacks Radius access, while failures from Kubernetes, a cloud provider, a registry, or another external system remain distinguishable.

Automation uses a dedicated workload identity rather than a shared human account. Actions performed through the dashboard, Backstage, agents, or other intermediaries remain attributable to the initiating user or workload even though graphical RBAC administration is out of scope.

### Proposed CLI experience

The API is the common management surface for RBAC. The `rad` CLI provides a single `rad auth` command group for imperative administration, while declarative role definitions and assignments use Radius resources in Bicep and the existing `rad deploy` workflow. Users do not have to learn a separate policy file format or apply engine, and no graphical RBAC management experience is included. The exact permission names, flags, and resource identifiers remain subject to technical design and usability validation.

| Command family | User purpose |
| --- | --- |
| `rad auth whoami` | Show the stable identity, identity type, issuer, and group information Radius is using for the current request. |
| `rad auth status` | Summarize whether authorization is in preview or enforcement mode, whether the current identity is an administrator, and any adoption or recovery warnings. |
| `rad auth permission` | List and explain the Radius permissions available when creating a custom role, including the resource types and scopes to which each permission applies. |
| `rad auth role` | List and inspect built-in and custom roles, and create, update, or delete custom roles. Built-in roles are immutable. |
| `rad auth assignment` | List and inspect direct or inherited role assignments, grant a role to a user, group, or workload identity at an explicit scope, and revoke an assignment. |
| `rad auth access` | List effective access, check whether an identity can perform an action at a scope, and explain which role or assignment allowed or denied a decision without exposing hidden data. |
| `rad auth enforcement` | Inspect enforcement status, preview would-be denials, enable enforcement after reviewing access impact, and perform any supported migration-period rollback. |

A platform administrator can grant access imperatively for bootstrap, investigation, or an immediate operational need:

```console
rad auth assignment create \
  --principal group:<stable-group-id> \
  --role "Application Developer" \
  --scope <resource-group-id>
```

Before creating the assignment, the CLI resolves and displays the principal, role, exact scope, inheritance effect, and management origin. Broad administrator grants, self-assignment, enforcement changes, and destructive operations require confirmation. `--yes` can skip an interactive prompt for automation, but cannot bypass authorization, validation, or protection against removing the final recoverable administrator.

An application developer can check access before starting an operation and investigate a denial:

```console
rad auth access check \
  --action <permission> \
  --scope <resource-id>

rad auth access explain \
  --action <permission> \
  --scope <resource-id>
```

An administrator can inspect effective access and distinguish assignments made directly at a scope from those inherited from a broader scope:

```console
rad auth access list --principal group:<stable-group-id> --scope <resource-id>
rad auth assignment list --scope <resource-id> --include-inherited
```

For repeatable management, a platform team defines roles and assignments as Radius resources and previews the access impact before deployment:

```console
rad deploy platform-access.bicep --scope <resource-id> --what-if
rad deploy platform-access.bicep --scope <resource-id>
```

Declaratively managed authorization resources identify their management origin. Conflicting imperative changes fail with guidance to update the source of truth or, if supported, perform an explicit and audited ownership transfer. Initial administrator access remains part of installation or RBAC enablement rather than an ordinary assignment command, and exceptional recovery remains a separate, audited security-boundary workflow.

Across these commands, reads support table and JSON output, writes show the resolved scope, ambiguous names fail rather than selecting a resource silently, and incomplete results are labeled rather than presented as complete. Authorization failures distinguish Radius access decisions from failures in Kubernetes, cloud providers, registries, or source-control systems.

### Proposed Bicep experience

Radius exposes custom roles and role assignments as `Radius.Core/roleDefinitions` and `Radius.Core/roleAssignments` resources. Platform teams manage them through Bicep and the existing `rad deploy` workflow rather than a separate policy format or apply command.

> **Note**: the Bicep schemas below are proposals which may change during technical design and implementation.

```bicep
extension radius

param identityIssuer string
param developerGroupSubject string

resource applicationOperator 'Radius.Core/roleDefinitions@<api-version>' = {
  name: 'application-operator'
  properties: {
    displayName: 'Application Operator'
    description: 'Operate applications without changing platform configuration.'
    permissions: [
      'Radius.Core/applications/read'
      'Radius.Core/applications/write'
      'Radius.Core/applications/deploy/action'
      'Radius.Core/environments/use/action'
    ]
  }
}

resource developerAccess 'Radius.Core/roleAssignments@<api-version>' = {
  name: guid(applicationOperator.id, identityIssuer, developerGroupSubject)
  properties: {
    roleDefinitionId: applicationOperator.id
    principal: {
      type: 'group'
      issuer: identityIssuer
      subject: developerGroupSubject
    }
  }
}
```

The assignment applies at the Bicep resource's deployment scope; an assignment targeted to an individual Radius resource uses that resource as its Bicep scope. Built-in roles are immutable `Radius.Core/roleDefinitions` resources that templates reference as `existing`. Role assignments use stable identity identifiers rather than display names or email addresses.

Before deployment, `rad deploy --what-if` shows the roles and assignments that will be added, changed, or revoked and warns about lockout or privilege-escalation risk. Redeploying reconciles the authorization resources previously managed by that declaration, so removing an assignment from Bicep revokes it instead of leaving access behind. Declaratively managed resources identify their management origin, and conflicting imperative changes fail with guidance to update or explicitly take ownership from the declarative source.

## Key investments

### Feature 1: Radius permission and scope model

Define a stable permission catalog for Radius resources and actions. The scope hierarchy is an installation, its configured planes, the resource groups within each plane, and supported individual resources. Assignments inherit downward within that hierarchy but never sideways into a sibling plane or unrelated resource group. A plane-scoped assignment therefore does not grant access to resources or credentials in another plane, and referencing a resource in another scope does not transfer access to it.

Installation-level assignments are reserved for roles explicitly intended to span the installation. Administration of Azure or AWS credentials requires access in the corresponding provider plane unless an installation-wide role explicitly includes that capability. The exact resource identifiers and supported individual-resource scopes remain subject to technical design and validation.

The first release uses additive allow grants with implicit deny. Explicit deny remains out of scope unless customer or compliance validation finds a blocking use case. Custom roles do not silently gain access when Radius adds actions or resource types.

### Feature 2: Built-in and custom roles

Provide immutable, documented built-in roles for common separation-of-duties scenarios:

| Built-in role | Intended capability |
| --- | --- |
| Radius Administrator | Manage all Radius resources and authorization for the assigned installation or plane scope. |
| Access Administrator | Manage role definitions and assignments at an installation or plane scope without automatically receiving application or platform-resource access. |
| Platform Administrator | Manage Radius resource groups, environments, provider configuration, and platform settings without automatically administering access. Separate plane assignments govern Azure and AWS credential administration. |
| Application Developer | Create, read, update, delete, and operate applications and application resources in the assigned scope, but not select arbitrary environments. |
| Environment Deployer | Deploy applications to assigned environments without changing environment configuration or receiving access to secret values. |
| Recipe Pack Administrator | Create and manage Recipe Packs and their Recipe definitions in the assigned scope. |
| Resource Type Administrator | Register and manage resource types and related schema metadata in the assigned scope. |
| Reader | Read authorized resource metadata and application graphs without mutation or secret access. |
| Auditor | Read authorization configuration and audit events without resource mutation or secret access. |

### Feature 3: Role assignments and effective-access inspection

Support assignments for users, groups, and workload identities. An assignment binds one role to one principal at one scope, and multiple assignments are additive.

Administrators can list assignments, inspect effective access, preview policy changes, and check whether a principal may perform an action. Users can inspect their own access without learning about unrelated principals or policy.

### Feature 4: Cross-scope and transitive-use authorization

Define authorization for operations that read or mutate more than one resource:

- Application management and environment deployment are separate permissions.
- Referencing a platform capability requires appropriate access to both the resource being changed and the referenced capability.
- Deploying to an environment allows Radius to use its platform-managed dependencies but does not grant the caller direct access to them.
- Sensitive Radius-provided views and operations, such as graphs and secret-related actions, may require permissions beyond ordinary resource read access.
- Radius checks the required access before beginning a multi-resource operation and provides a clear recovery path if authorization changes while work is in progress.

### Feature 5: Consistent enforcement and client behavior

Enforce authorization consistently for the API, CLI, dashboard, Backstage, controllers, GitHub Actions, agents, and other supported clients. New operations and resource types do not become accessible until their permissions are deliberately defined and granted. Automation acts through explicit workload identities.

Users authenticate through the trusted identity system configured for the installation rather than through a separate Radius account. Every client preserves the initiating user or workload identity, and Radius clearly identifies the principal it recognizes. Clients can use capability checks to improve the experience, but the server remains authoritative. Clients preserve authentication and authorization errors instead of presenting them as empty results, retries, or unrelated failures.

Radius RBAC governs operations performed through the Radius API; it does not replace Kubernetes RBAC for clients that access Kubernetes objects directly. For example, `rad run` currently streams pod logs through the user's Kubernetes access, so log access remains governed by Kubernetes and may differ from the user's Radius permissions. The CLI makes this boundary clear. A future Radius log permission would govern logs only when they are provided through an authorized Radius endpoint.

### Feature 6: Auditability and safe administration

Record authorization decisions and administrative changes with enough information to identify who acted, what they attempted, the target, the result, and the policy that affected the decision without recording sensitive data.

Role administration prevents unauthorized escalation and loss of all administrator access. A time-bounded, auditable recovery procedure exists for exceptional lockout situations and is not part of normal access.

### Feature 7: Migration and compatibility

Provide an inventory and preview mode that shows what enforcement would deny without changing request outcomes. Any suggested initial assignments require administrator review before becoming active.

Existing installations explicitly enable enforcement after validation. New shared installations start with enforcement enabled after bootstrap, while local installations retain a simple one-user setup.

Automation installations use a dedicated bootstrap path, and restored access policy cannot silently replace trusted bootstrap access or create an unrecoverable lockout. All installation modes and compatibility timelines are documented.
