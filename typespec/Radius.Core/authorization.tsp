import "@typespec/http";
import "@typespec/rest";
import "@typespec/openapi";
import "@azure-tools/typespec-azure-resource-manager";
import "@azure-tools/typespec-azure-core";

using TypeSpec.Http;
using TypeSpec.Rest;
using Azure.Core;
using Azure.ResourceManager;
using OpenAPI;

namespace Radius.Core;

@doc("A stable principal identity. Equality uses type, issuer, and subject, never displayName. Group membership is resolved by UCP, not supplied by an access-check caller.")
model Principal {
  @doc("The kind of principal.")
  @visibility(Lifecycle.Read, Lifecycle.Create)
  type: PrincipalType;

  @doc("The trusted identity system, including its trust domain. Kubernetes issuers use kubernetes:<cluster-id>; controller namespace principals use radius-controller.")
  @minLength(1)
  @visibility(Lifecycle.Read, Lifecycle.Create)
  issuer: string;

  @doc("The identity within the issuer. Kubernetes service accounts use system:serviceaccount:<namespace>:<name>.")
  @minLength(1)
  @visibility(Lifecycle.Read, Lifecycle.Create)
  subject: string;

  @doc("An optional mutable label that is never used to match principals.")
  displayName?: string;
}

@doc("User-facing principal kinds, distinct from internal component identities.")
enum PrincipalType {
  user: "user",
  group: "group",
  workload: "workload",
}

@doc("An installation, plane, resource group, or supported resource ID. Inheritance follows the ID hierarchy, never resource references. Supported resources are Radius.Core environments, recipePacks, applications and Applications.Core environments and applications. UCP validates the scope's topology and supported type using ARM's case-insensitive ID rules; the string constraint only validates path syntax.")
@pattern("^(/|/[^/\\s*]+(/[^/\\s*]+)*)$")
scalar AuthorizationScope extends string;

@doc("The installation, a plane, or a resource group. Role definitions cannot be created beneath individual resources.")
scalar RoleDefinitionScope extends AuthorizationScope;

@doc("A scope ID as a URI path prefix, without the leading slash. Resource properties and IDs always retain their leading slash. Installation operations have separate paths.")
@pattern("^[^/\\s*]+(/[^/\\s*]+)*$")
scalar AuthorizationScopePath extends string;

@doc("An explicit catalog permission such as Radius.Core/environments/use/action. Custom permissions cannot contain wildcards. Catalog membership is validated on writes.")
@pattern("^[^*/\\s]+(/[^*/\\s]+){2,}$")
scalar PermissionName extends string;

@doc("An authorization resource name. Assignment names are deterministic Bicep guid(scope, roleDefinitionId, type, issuer, subject) values; the API also accepts stable caller-provided names.")
@minLength(1)
@maxLength(128)
@pattern("^[A-Za-z0-9][A-Za-z0-9_.-]*$")
scalar AuthorizationResourceName extends string;

@doc("The decimal generation identifying one applied policy change.")
@pattern("^[1-9][0-9]*$")
scalar PolicyGenerationName extends string;

@doc("The non-installation scope of a role definition.")
model RoleDefinitionScopeParameter {
  @path
  @extension("x-ms-skip-url-encoding", true)
  @doc("The plane or resource-group ID without a leading or trailing slash, for example planes/radius/local/resourceGroups/team-a. Use the installation operations for scope /.")
  scope: AuthorizationScopePath;
}

@doc("The non-installation scope of an assignment or access check.")
model AuthorizationScopeParameter {
  @path
  @extension("x-ms-skip-url-encoding", true)
  @doc("The plane, resource-group, or supported-resource ID without a leading or trailing slash, for example planes/radius/local/resourceGroups/platform/providers/Radius.Core/environments/staging. Use the installation operations for scope /.")
  scope: AuthorizationScopePath;
}

@doc("A role definition. Built-ins live at /providers/Radius.Core/roleDefinitions/<name> and cannot be changed or deleted. Custom roles can live at installation, plane, or resource-group scope.")
model RoleDefinitionResource is ProxyResource<RoleDefinitionProperties, false> {
  @key("roleDefinitionName")
  @path
  @segment("roleDefinitions")
  @doc("The role definition name.")
  name: AuthorizationResourceName;
}

@doc("The role definition properties.")
model RoleDefinitionProperties {
  @doc("A human-readable role name.")
  displayName?: string;

  @doc("A description of the intended access.")
  description?: string;

  @doc("Server-owned distinction between immutable release-defined built-ins and custom roles.")
  @visibility(Lifecycle.Read)
  roleType: RoleType;

  @doc("Explicit catalog permissions. Unknown permissions fail with UnknownPermission; custom wildcards and deny rules are not supported.")
  permissions: PermissionName[];

  @doc("Scopes where the role may be assigned, including descendants. Defaults to the role definition's scope. Writers must be allowed to create roles at every listed scope.")
  assignableScopes?: AuthorizationScope[];
}

@doc("Who defines and maintains a role.")
enum RoleType {
  BuiltIn: "BuiltIn",
  Custom: "Custom",
}

@doc("An ARM-style extension assignment whose ID ends with /providers/Radius.Core/roleAssignments/<name>. Supported resource scopes include a single environment. A repeated PUT with the same role, principal identity, and scope is a no-op; changing that tuple fails with RoleAssignmentImmutable.")
model RoleAssignmentResource is ExtensionResource<
  RoleAssignmentProperties,
  false
> {
  @key("roleAssignmentName")
  @path
  @segment("roleAssignments")
  @doc("A stable assignment name.")
  name: AuthorizationResourceName;
}

@doc("Role assignment properties. Only the description and principal display label may change; assignment ownership is checked independently.")
model RoleAssignmentProperties {
  @doc("The immutable full role definition ID. The assignment scope must be covered by the role's assignableScopes.")
  @visibility(Lifecycle.Read, Lifecycle.Create)
  roleDefinitionId: string;

  @doc("The principal. Its type, issuer, and subject cannot change.")
  principal: Principal;

  @doc("An optional description of why access was granted.")
  description?: string;

  @doc("Server-owned canonical scope derived from the assignment ID, including / for installation assignments.")
  @visibility(Lifecycle.Read)
  scope: AuthorizationScope;

  @doc("Server-owned provenance. Only the same GitOps source may modify its assignments unless an authorized deletion explicitly takes ownership. Bootstrap assignments are managed through installation settings.")
  @visibility(Lifecycle.Read)
  managedBy: AssignmentManagement;

  @doc("Server-owned identity of the creator, formatted as type:issuer:subject for display, not parsed to establish identity.")
  @visibility(Lifecycle.Read)
  createdBy: string;
}

@doc("Verified assignment provenance supplied by UCP, never trusted from a request body.")
model AssignmentManagement {
  @doc("How the assignment was created.")
  kind: AssignmentManagementKind;

  @doc("The verified Git source URL for GitOps assignments.")
  source?: string;

  @doc("The Git revision.")
  revision?: string;

  @doc("The template path within the source.")
  path?: string;

  @doc("The verified submitting principal, formatted for display.")
  principal?: string;
}

@doc("The owner of an assignment.")
enum AssignmentManagementKind {
  Imperative: "Imperative",
  Deployment: "Deployment",
  GitOps: "GitOps",
  Bootstrap: "Bootstrap",
}

@doc("Installation-only user-facing authorization settings. Exactly one resource exists, named default. These settings do not control internal component authorization.")
@tenantResource
@singleton("default")
model AuthorizationSettingsResource is ProxyResource<
  AuthorizationSettingsProperties,
  false
> {
  @key("authorizationSettingsName")
  @path
  @segment("authorizationSettings")
  @doc("The singleton name.")
  name: "default";
}

@doc("User-facing authorization configuration. Mode writes remain subject to bootstrap, history, last-administrator, and internal-protocol checks even when user checks do not block requests.")
model AuthorizationSettingsProperties {
  @doc("Disabled performs no user authorization checks; DryRun evaluates and records would-be denials without blocking; Enforce blocks denied operations. This contract does not enable any mode.")
  mode: UserAuthorizationMode;

  @doc("Server-owned generation of the applied policy. It increases once per applied access change.")
  @visibility(Lifecycle.Read)
  generation: int64;
}

@doc("User RBAC modes, deliberately separate from internal service security modes.")
enum UserAuthorizationMode {
  Disabled: "Disabled",
  DryRun: "DryRun",
  Enforce: "Enforce",
}

@doc("A catalog entry. The catalog is generated from API definitions, UCP routes, and registered resource types; this contract does not implement catalog generation.")
model Permission {
  @doc("The exact permission identifier.")
  name: PermissionName;

  @doc("The display name.")
  displayName: string;

  @doc("The operation's meaning.")
  description: string;
}

@doc("An immutable installation-scoped access change. One record exists per applied policy generation and is never deleted. Access decisions are not access-change records.")
@tenantResource
model AccessChangeResource is ProxyResource<AccessChangeProperties, false> {
  @key("accessChangeName")
  @path
  @segment("accessChanges")
  @doc("The decimal policy generation, used as the record key.")
  name: PolicyGenerationName;
}

@doc("The committed access change, written atomically with the applied generation by the future policy store.")
model AccessChangeProperties {
  @doc("The generation identifying this change.")
  @minValue(1)
  generation: int64;

  @doc("When the change was applied.")
  time: utcDateTime;

  @doc("The request identifier.")
  requestId: string;

  @doc("The correlation identifier.")
  correlationId?: string;

  @doc("The authenticated actor.")
  principal: Principal;

  @doc("The user-facing mode at the time of the change.")
  mode: UserAuthorizationMode;

  @doc("The changed policy and its provenance. No request bodies, secrets, or credentials are included.")
  policyChange: PolicyChange;

  @doc("Whether the change grants administrative access, includes the actor, or overrides an enablement safety check.")
  highImpact: boolean;

  @doc("Whether the actor used temporary recovery access.")
  recovery: boolean;
}

@doc("The applied policy operation.")
model PolicyChange {
  @doc("The operation that changed policy.")
  operation: PolicyChangeOperation;

  @doc("The changed resource ID.")
  target: string;

  @doc("The policy before the change, absent for creation.")
  before?: PolicySnapshot;

  @doc("The policy after the change, absent for deletion.")
  after?: PolicySnapshot;

  @doc("The verified source of the change.")
  managedBy: AssignmentManagement;
}

@doc("An access configuration snapshot. Exactly the property matching the target resource type is present.")
model PolicySnapshot {
  @doc("A role definition snapshot.")
  roleDefinition?: RoleDefinitionProperties;

  @doc("An assignment snapshot.")
  roleAssignment?: RoleAssignmentProperties;

  @doc("A settings snapshot.")
  authorizationSettings?: AuthorizationSettingsProperties;
}

@doc("An applied change operation.")
enum PolicyChangeOperation {
  Create: "Create",
  Update: "Update",
  Delete: "Delete",
}

@doc("A paginated collection. nextLink is an opaque continuation URL, not a count of filtered or inaccessible items.")
model AuthorizationList<T> {
  @doc("The visible items.")
  @pageItems
  value: T[];

  @doc("The next page, if present.")
  @nextLink
  nextLink?: string;
}

@doc("The trusted identity UCP sees for the current authenticated caller.")
model WhoAmIResult {
  @doc("The authenticated user or workload.")
  principal: Principal;

  @doc("Verified group principals. Anonymous groups are never accepted.")
  groups: Principal[];

  @doc("The trusted issuer.")
  issuer: string;

  @doc("The current user-facing mode.")
  mode: UserAuthorizationMode;

  @doc("Whether the caller has Radius Administrator access.")
  isRadiusAdministrator: boolean;
}

@doc("A permission and concrete target to evaluate. A reference never implies access to its target.")
model AccessCheck {
  @doc("The permission being evaluated.")
  permission: PermissionName;

  @doc("The concrete target resource or scope ID.")
  target: string;
}

@doc("Access-check input. Omitting principal checks the caller. Checking another principal requires Radius.Core/checkAccess/action. Group membership is resolved from trusted identity context, not caller-provided groups.")
model AccessCheckRequest {
  @doc("The principal to check, or the authenticated caller when omitted.")
  principal?: Principal;

  @doc("The checks to evaluate.")
  @minItems(1)
  checks: AccessCheck[];
}

@doc("The policy evaluation, independent of whether the current mode blocks requests.")
model AccessCheckResult extends AccessCheck {
  @doc("Whether the evaluated policy grants the permission.")
  allowed: boolean;
}

@doc("The results under one consistent policy generation.")
model CheckAccessResponse {
  @doc("The evaluated principal.")
  principal: Principal;

  @doc("The policy generation used.")
  generation: int64;

  @doc("The current mode. A denied evaluation in DryRun does not block the actual request.")
  mode: UserAuthorizationMode;

  @doc("One result for each requested check, in request order.")
  checks: AccessCheckResult[];
}

@doc("A grant visible to the caller. Assignment identifiers and details must be omitted unless the caller can read assignments at the grant's scope.")
model AccessGrant {
  @doc("The assignment that grants access.")
  assignmentId: string;

  @doc("The granting role definition.")
  roleDefinitionId: string;

  @doc("The assignment's scope.")
  scope: AuthorizationScope;
}

@doc("An explained evaluation, redacted according to the caller's assignment-read permissions.")
model ExplainedAccessCheck extends AccessCheckResult {
  @doc("Readable assignments and roles that granted the permission. Absence can mean that grant details are not readable.")
  grants?: AccessGrant[];

  @doc("Roles that could grant a denied permission; suggestions are not themselves grants.")
  suggestedRoleDefinitionIds?: string[];
}

@doc("Access-check explanations under one consistent policy generation.")
model ExplainAccessResponse {
  @doc("The evaluated principal.")
  principal: Principal;

  @doc("The evaluated generation.")
  generation: int64;

  @doc("The user-facing mode.")
  mode: UserAuthorizationMode;

  @doc("Explanations in request order.")
  checks: ExplainedAccessCheck[];
}

@doc("A direct or inherited assignment used in effective access.")
model EffectiveAssignment {
  @doc("A readable assignment for the principal or one of its verified groups.")
  assignment: RoleAssignmentResource;

  @doc("True if the assignment is inherited from an ancestor scope.")
  inherited: boolean;
}

@doc("The combined access of a principal at a scope. Requires roleAssignments/read. For a group, only the group's own assignments are included; group members cannot be enumerated.")
model EffectiveAccessResponse {
  @doc("The evaluated principal.")
  principal: Principal;

  @doc("The scope being inspected.")
  scope: AuthorizationScope;

  @doc("The policy generation used.")
  generation: int64;

  @doc("The contributing direct and inherited assignments.")
  assignments: EffectiveAssignment[];

  @doc("The union of explicit permissions at this scope.")
  permissions: PermissionName[];
}

@doc("Shared API-version parameter.")
model AuthorizationParameters {
  ...ApiVersionParameter;
}

@doc("Assignment deletion options.")
model AssignmentDeleteOptions {
  @doc("Explicitly override GitOps ownership after authorization and safety checks. The override is recorded in access history. Does not bypass bootstrap ownership or authorization.")
  @query
  takeOwnership?: boolean;
}

@doc("Assignment list options.")
model AssignmentListOptions {
  @doc("Use atScopeAndBelow() to include assignments below the requested scope. Inherited assignments are obtained by separately checking each ancestor.")
  @query("$filter")
  filter?: "atScopeAndBelow()";
}

@doc("A principal query encoded as a JSON object with type, issuer, and subject. Omit to inspect the caller. No group memberships may be asserted in this query.")
model PrincipalQuery {
  @query
  principal?: string;
}

@doc("Optional access-history filters. Principal is a JSON-encoded type, issuer, and subject tuple; scope includes changes at or below that scope.")
model AccessChangeListOptions {
  @doc("Filter by the authenticated actor that made the change.")
  @query
  principal?: string;

  @doc("Filter by the changed policy's scope.")
  @query
  scope?: AuthorizationScope;

  @doc("Include changes applied at or after this time.")
  @query
  since?: utcDateTime;
}

@doc("UCP-served installation role definitions. Custom roles can be created here; BuiltInRoleImmutable protects built-ins and RoleDefinitionInUse protects assigned roles from deletion.")
@route("/providers/Radius.Core/roleDefinitions")
@armResourceOperations(#{ allowStaticRoutes: true })
interface InstallationRoleDefinitions {
  @get
  @route("/{roleDefinitionName}")
  @armResourceRead(RoleDefinitionResource)
  get(
    ...AuthorizationParameters,
    @path roleDefinitionName: AuthorizationResourceName,
  ): ArmResponse<RoleDefinitionResource> | ErrorResponse;

  @get
  @list
  @extension("x-ms-pageable", #{ nextLinkName: "nextLink" })
  @armResourceList(RoleDefinitionResource)
  list(...AuthorizationParameters):
    | ArmResponse<AuthorizationList<RoleDefinitionResource>>
    | ErrorResponse;

  @put
  @route("/{roleDefinitionName}")
  @armResourceCreateOrUpdate(RoleDefinitionResource)
  createOrUpdate(
    ...AuthorizationParameters,
    @path roleDefinitionName: AuthorizationResourceName,
    @bodyRoot resource: RoleDefinitionResource,
  ):
    | ArmResourceUpdatedResponse<RoleDefinitionResource>
    | ArmResourceCreatedSyncResponse<RoleDefinitionResource>
    | ErrorResponse;

  @delete
  @route("/{roleDefinitionName}")
  @armResourceDelete(RoleDefinitionResource)
  delete(
    ...AuthorizationParameters,
    @path roleDefinitionName: AuthorizationResourceName,
  ): ArmDeletedResponse | ArmDeletedNoContentResponse | ErrorResponse;
}

@doc("UCP-served role definitions on a plane or resource group.")
@route("/{+scope}/providers/Radius.Core/roleDefinitions")
@armResourceOperations(#{ allowStaticRoutes: true })
interface ScopedRoleDefinitions {
  @get
  @route("/{roleDefinitionName}")
  @armResourceRead(RoleDefinitionResource)
  get(
    ...AuthorizationParameters,
    ...RoleDefinitionScopeParameter,
    @path roleDefinitionName: AuthorizationResourceName,
  ): ArmResponse<RoleDefinitionResource> | ErrorResponse;

  @get
  @list
  @extension("x-ms-pageable", #{ nextLinkName: "nextLink" })
  @armResourceList(RoleDefinitionResource)
  list(...AuthorizationParameters, ...RoleDefinitionScopeParameter):
    | ArmResponse<AuthorizationList<RoleDefinitionResource>>
    | ErrorResponse;

  @put
  @route("/{roleDefinitionName}")
  @armResourceCreateOrUpdate(RoleDefinitionResource)
  createOrUpdate(
    ...AuthorizationParameters,
    ...RoleDefinitionScopeParameter,
    @path roleDefinitionName: AuthorizationResourceName,
    @bodyRoot resource: RoleDefinitionResource,
  ):
    | ArmResourceUpdatedResponse<RoleDefinitionResource>
    | ArmResourceCreatedSyncResponse<RoleDefinitionResource>
    | ErrorResponse;

  @delete
  @route("/{roleDefinitionName}")
  @armResourceDelete(RoleDefinitionResource)
  delete(
    ...AuthorizationParameters,
    ...RoleDefinitionScopeParameter,
    @path roleDefinitionName: AuthorizationResourceName,
  ): ArmDeletedResponse | ArmDeletedNoContentResponse | ErrorResponse;
}

@doc("UCP-served installation assignments. Installation grants are limited to admin-type roles, reader, and auditor.")
@route("/providers/Radius.Core/roleAssignments")
@armResourceOperations(#{ allowStaticRoutes: true })
interface InstallationRoleAssignments {
  @get
  @route("/{roleAssignmentName}")
  @armResourceRead(RoleAssignmentResource)
  get(
    ...AuthorizationParameters,
    @path roleAssignmentName: AuthorizationResourceName,
  ): ArmResponse<RoleAssignmentResource> | ErrorResponse;

  @get
  @list
  @extension("x-ms-pageable", #{ nextLinkName: "nextLink" })
  @armResourceList(RoleAssignmentResource)
  list(...AuthorizationParameters, ...AssignmentListOptions):
    | ArmResponse<AuthorizationList<RoleAssignmentResource>>
    | ErrorResponse;

  @put
  @route("/{roleAssignmentName}")
  @armResourceCreateOrUpdate(RoleAssignmentResource)
  createOrUpdate(
    ...AuthorizationParameters,
    @path roleAssignmentName: AuthorizationResourceName,
    @bodyRoot resource: RoleAssignmentResource,
  ):
    | ArmResourceUpdatedResponse<RoleAssignmentResource>
    | ArmResourceCreatedSyncResponse<RoleAssignmentResource>
    | ErrorResponse;

  @delete
  @route("/{roleAssignmentName}")
  @armResourceDelete(RoleAssignmentResource)
  delete(
    ...AuthorizationParameters,
    @path roleAssignmentName: AuthorizationResourceName,
    ...AssignmentDeleteOptions,
  ): ArmDeletedResponse | ArmDeletedNoContentResponse | ErrorResponse;
}

@doc("UCP-served extension assignments on a plane, group, or supported resource. Resource-scoped IDs preserve the target's entire ID before appending /providers/Radius.Core/roleAssignments/<name>.")
@route("/{+scope}/providers/Radius.Core/roleAssignments")
@armResourceOperations(#{ allowStaticRoutes: true })
interface ScopedRoleAssignments {
  @get
  @route("/{roleAssignmentName}")
  @armResourceRead(RoleAssignmentResource)
  get(
    ...AuthorizationParameters,
    ...AuthorizationScopeParameter,
    @path roleAssignmentName: AuthorizationResourceName,
  ): ArmResponse<RoleAssignmentResource> | ErrorResponse;

  @get
  @list
  @extension("x-ms-pageable", #{ nextLinkName: "nextLink" })
  @armResourceList(RoleAssignmentResource)
  list(
    ...AuthorizationParameters,
    ...AuthorizationScopeParameter,
    ...AssignmentListOptions,
  ): ArmResponse<AuthorizationList<RoleAssignmentResource>> | ErrorResponse;

  @put
  @route("/{roleAssignmentName}")
  @armResourceCreateOrUpdate(RoleAssignmentResource)
  createOrUpdate(
    ...AuthorizationParameters,
    ...AuthorizationScopeParameter,
    @path roleAssignmentName: AuthorizationResourceName,
    @bodyRoot resource: RoleAssignmentResource,
  ):
    | ArmResourceUpdatedResponse<RoleAssignmentResource>
    | ArmResourceCreatedSyncResponse<RoleAssignmentResource>
    | ErrorResponse;

  @delete
  @route("/{roleAssignmentName}")
  @armResourceDelete(RoleAssignmentResource)
  delete(
    ...AuthorizationParameters,
    ...AuthorizationScopeParameter,
    @path roleAssignmentName: AuthorizationResourceName,
    ...AssignmentDeleteOptions,
  ): ArmDeletedResponse | ArmDeletedNoContentResponse | ErrorResponse;
}

@doc("UCP-served singleton settings. No delete, patch, or list operation is defined.")
@route("/providers/Radius.Core/authorizationSettings/default")
@armResourceOperations(#{ allowStaticRoutes: true })
interface AuthorizationSettings {
  @get
  @armResourceRead(AuthorizationSettingsResource)
  get(...AuthorizationParameters):
    | ArmResponse<AuthorizationSettingsResource>
    | ErrorResponse;

  @put
  @armResourceCreateOrUpdate(AuthorizationSettingsResource)
  createOrUpdate(
    ...AuthorizationParameters,
    @bodyRoot resource: AuthorizationSettingsResource,
  ): ArmResourceUpdatedResponse<AuthorizationSettingsResource> | ErrorResponse;
}

@doc("The read-only permission catalog.")
@route("/providers/Radius.Core/permissions")
interface Permissions {
  @get
  @list
  @extension("x-ms-pageable", #{ nextLinkName: "nextLink" })
  list(...AuthorizationParameters):
    | ArmResponse<AuthorizationList<Permission>>
    | ErrorResponse;
}

@doc("Read-only, append-only access history. There are deliberately no public write or delete operations.")
@route("/providers/Radius.Core/accessChanges")
@armResourceOperations(#{ allowStaticRoutes: true })
interface AccessChanges {
  @get
  @route("/{accessChangeName}")
  @armResourceRead(AccessChangeResource)
  get(
    ...AuthorizationParameters,
    @path accessChangeName: PolicyGenerationName,
  ): ArmResponse<AccessChangeResource> | ErrorResponse;

  @get
  @list
  @extension("x-ms-pageable", #{ nextLinkName: "nextLink" })
  @armResourceList(AccessChangeResource)
  list(...AuthorizationParameters, ...AccessChangeListOptions):
    | ArmResponse<AuthorizationList<AccessChangeResource>>
    | ErrorResponse;
}

@doc("Access inspection for the installation. Signed-in users can inspect their own identity and evaluate their own access; inspecting another identity and reading assignments require the documented permissions.")
@route("/providers/Radius.Core")
interface InstallationAccess {
  @get
  @route("/whoAmI")
  whoAmI(...AuthorizationParameters): ArmResponse<WhoAmIResult> | ErrorResponse;

  @post
  @route("/checkAccess")
  checkAccess(...AuthorizationParameters, @body request: AccessCheckRequest):
    | ArmResponse<CheckAccessResponse>
    | ErrorResponse;

  @post
  @route("/explainAccess")
  explainAccess(...AuthorizationParameters, @body request: AccessCheckRequest):
    | ArmResponse<ExplainAccessResponse>
    | ErrorResponse;

  @get
  @route("/effectiveAccess")
  effectiveAccess(...AuthorizationParameters, ...PrincipalQuery):
    | ArmResponse<EffectiveAccessResponse>
    | ErrorResponse;
}

@doc("Access inspection at plane, group, and supported-resource scopes. A denial in these results describes policy, not whether the current mode blocks the actual operation.")
@route("/{+scope}/providers/Radius.Core")
interface ScopedAccess {
  @post
  @route("/checkAccess")
  checkAccess(
    ...AuthorizationParameters,
    ...AuthorizationScopeParameter,
    @body request: AccessCheckRequest,
  ): ArmResponse<CheckAccessResponse> | ErrorResponse;

  @post
  @route("/explainAccess")
  explainAccess(
    ...AuthorizationParameters,
    ...AuthorizationScopeParameter,
    @body request: AccessCheckRequest,
  ): ArmResponse<ExplainAccessResponse> | ErrorResponse;

  @get
  @route("/effectiveAccess")
  effectiveAccess(
    ...AuthorizationParameters,
    ...AuthorizationScopeParameter,
    ...PrincipalQuery,
  ): ArmResponse<EffectiveAccessResponse> | ErrorResponse;
}
