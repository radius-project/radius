# Feature Specification: Direct Module Support

**Feature Branch**: `001-direct-module-support`  
**Created**: 2026-04-22  
**Updated**: 2026-04-30  
**Status**: Draft  
**Input**: Enable platform engineers to use any standard Bicep or Terraform module as a Radius recipe — without writing a Radius-specific wrapper. Today, using a module as a recipe requires a wrapper that conforms to Radius conventions (a `context` input variable, a structured `result` output). This feature eliminates the wrapper: point `recipeLocation` directly at a standard module, and the system handles input resolution (injecting Radius context like resource name, namespace, etc.) and output resolution (mapping module outputs to resource properties) externally, outside the module. The module doesn't need to know about Radius.

## The Problem

Today, every Bicep or Terraform module used as a Radius recipe must be wrapped in a Radius-specific shim:

- **For both Bicep and Terraform**: The wrapper adds a `context` input variable and a structured `result` output (separating `values`, `secrets`, and `resources`) that conforms to Radius recipe conventions. The platform engineer downloads a community module, writes a wrapper that calls it, re-publishes the wrapper, and references that wrapper as the recipe. The problem is identical regardless of IaC language.

This wrapper tax has real consequences:

1. **Friction**: Every module requires a bespoke wrapper before it can be used. Wrapping a single module takes 15–60 minutes and requires understanding both the module's interface and Radius conventions.
2. **Maintenance burden**: When the upstream module releases a new version, the wrapper must be updated and re-published. Wrapper drift causes silent failures.
3. **Ecosystem lock-out**: Thousands of production-ready modules exist in the Terraform Registry, MCR (for Bicep), and Git repositories. The wrapper requirement means none of them work out of the box.

**Direct module support eliminates the wrapper.** Platform engineers point `recipeLocation` at any standard module. The system resolves inputs (injecting Radius context into the module's native variables) and resolves outputs (mapping the module's native outputs to resource properties) — all externally, without modifying the module.

## User Scenarios & Testing *(mandatory)*

### User Story 1 — Bicep Module Support (Priority: P1)

As a platform engineer, I want to set `recipeLocation` to a standard Bicep module OCI reference (e.g., `br:mcr.microsoft.com/bicep/avm/res/storage/storage-account:0.14.3`) and have Radius deploy it directly — passing my `recipeParameters` as the module's ARM parameters and mapping the module's outputs back to Radius resource properties — without writing a Radius-specific wrapper.

**Today's workflow**: Download the module → write a Radius wrapper that adds the `result` output → publish the wrapper to an OCI registry → reference the wrapper in `recipeLocation`. **With this feature**: set `recipeLocation` to the module's OCI reference directly.

**Why this priority**: Bicep is Radius's native IaC language. The Bicep driver already handles ARM deployments — extending it to handle standard modules (without the Radius `result` output convention) is the smallest incremental step. This immediately unblocks the AVM Bicep catalog (hundreds of production-ready modules).

**Independent Test**: Link a recipe pointing to a public AVM Bicep module from MCR, deploy a resource, verify infrastructure is provisioned, and confirm module outputs are accessible as resource properties.

**Acceptance Scenarios**:

1. **Given** a RecipePack with `recipeKind: 'bicep'` and `recipeLocation` set to a standard Bicep module OCI reference, **When** a resource using this recipe is deployed with `recipeParameters` providing values for the module's parameters, **Then** the module receives those values and provisions infrastructure successfully.
2. **Given** a Bicep module with output values (e.g., `output endpoint string`), **When** the resource is deployed, **Then** the module's outputs are mapped to the resource type's read-only properties via the `outputs` mapping on the RecipePack.
3. **Given** `recipeParameters` containing `{{context.*}}` template expressions (e.g., `name: 'sa-{{context.resource.name}}'`), **When** the resource is deployed, **Then** expressions are resolved to actual Radius context values before being passed to the module as ARM parameters.
4. **Given** a `recipeLocation` pointing to a non-existent Bicep module, **When** deployment is attempted, **Then** the system returns a clear error indicating the module cannot be fetched.
5. **Given** a resource deployed via a direct Bicep module recipe, **When** the resource is deleted, **Then** the underlying ARM deployment and provisioned infrastructure are cleaned up.

---

### User Story 2 — Terraform Module Support (Priority: P1)

As a platform engineer, I want to set `recipeLocation` to a standard Terraform module source — a registry path (e.g., `ballj/postgresql/kubernetes`), a Git URL (e.g., `git::https://github.com/org/module.git`), or an HTTP archive — and have Radius download and execute it directly. The system resolves my `recipeParameters` (including `{{context.*}}` expressions) into the module's input variables, and maps the module's outputs back to Radius resource properties. The module doesn't need a `context` variable, a `result` output, or any knowledge of Radius.

**Today's workflow**: Download the module → write a wrapper that adds the `context` variable and `result` output → publish the wrapper → reference the wrapper. **With this feature**: set `recipeLocation` directly to the module's native source path.

**Why this priority**: This is the core value proposition for Terraform users. Unlocking the Terraform module ecosystem — thousands of community and official modules — without intermediate wrapping steps is essential for adoption.

**Independent Test**: Link a recipe pointing to a public Terraform registry module, deploy a resource, verify the module is downloaded and executed, and confirm outputs are accessible as resource properties.

**Acceptance Scenarios**:

1. **Given** a RecipePack with `recipeKind: 'terraform'` and `recipeLocation` set to a Terraform registry path (e.g., `ballj/postgresql/kubernetes`), **When** a resource is deployed with `recipeParameters` providing values for the module's input variables, **Then** the module receives those values and executes successfully.
2. **Given** a recipe with `recipeLocation` set to a Git URL (e.g., `git::https://github.com/org/module.git`), **When** a resource is deployed, **Then** the system clones the module from Git and executes it.
3. **Given** a recipe with `recipeLocation` pointing to a Git URL with a ref specifier (e.g., `?ref=v2.0.0`) or a subdirectory path (e.g., `//modules/vpc`), **When** deployed, **Then** the system uses the specified ref and/or navigates to the subdirectory.
4. **Given** a Terraform module with output values, **When** the resource is deployed, **Then** the module's outputs are mapped to the resource type's read-only properties via the `outputs` mapping — non-sensitive outputs in the `Values` map, sensitive outputs (marked `sensitive = true`) in the `Secrets` map.
5. **Given** `recipeParameters` containing `{{context.*}}` expressions, **When** the resource is deployed, **Then** expressions are resolved to actual Radius context values before being passed as Terraform input variables.
6. **Given** a module that requires a variable not supplied by any parameter source, **When** deployment is attempted, **Then** Terraform surfaces a clear error indicating which required variable is missing.
7. **Given** a resource deployed via a direct Terraform module recipe, **When** the resource is deleted, **Then** the system runs `terraform destroy` and cleans up all provisioned infrastructure.

---

### User Story 3 — Application-to-Recipe Property Resolution (Priority: P1)

As a platform engineer, I want `recipeParameters` to resolve properties injected from the application layer (via `context.resource.properties.*`) so that application developers can influence infrastructure configuration through resource properties without knowing the underlying module details. For example, an application developer sets a `size` property on a resource, and the recipe resolves it into a concrete infrastructure SKU using expressions like ternary operators.

**Why this priority**: This bridges the application and infrastructure layers — application developers express intent through resource properties, and the recipe translates that intent into concrete module inputs. It builds on the direct module support from P1 stories and works identically for any Bicep or Terraform module.

**Independent Test**: Link a recipe that uses `context.resource.properties.*` expressions in `recipeParameters`, deploy resources with different property values, and verify each deployment passes the correct resolved values to the module.

**Acceptance Scenarios**:

1. **Given** a recipe with `recipeParameters` containing `{{context.resource.properties.*}}` expressions, **When** a resource is deployed with specific property values set by the application developer, **Then** those property values are resolved and passed to the module as input parameters.
2. **Given** a recipe with `recipeParameters` containing a ternary expression (e.g., `{{context.resource.properties.size == "s" ? "B_Standard_B1ms" : "GP_Standard_D2s_v3"}}`), **When** resources are deployed with different property values, **Then** each deployment passes the correct resolved value to the module.
3. **Given** a recipe with `recipeParameters` that combine context property expressions with literal text (e.g., `name: 'pg-{{context.resource.name}}'`), **When** a resource is deployed, **Then** the module receives the fully resolved parameter values.
---

### User Story 4 — Private Module Authentication (Priority: P2)

As a platform engineer, I want to use modules hosted in private registries, private Git repositories, or private OCI registries as recipes, authenticating with credentials configured through the existing secret store.

**Why this priority**: Enterprise teams host modules in private repositories. Without authentication support, direct module support is limited to public modules.

**Independent Test**: Link a recipe pointing to a private Terraform registry module or Git repository, configure credentials via the existing secret mechanism, deploy, and verify the module is fetched successfully.

**Acceptance Scenarios**:

1. **Given** a recipe with `recipeLocation` pointing to a private Terraform registry module, **When** registry credentials are configured via the existing secret store mechanism, **Then** the system authenticates and fetches the module successfully.
2. **Given** a recipe with `recipeLocation` pointing to a private Git repository, **When** Git credentials are configured, **Then** the system clones and executes the module.

---

### User Story 5 — Source Reachability Validation at Link Time (Priority: P2)

As a platform engineer, I want the system to validate that a `recipeLocation` pointing to a direct module source is reachable when I link the recipe to an environment, so I catch typos and inaccessible sources early rather than at deploy time.

**Why this priority**: Early validation prevents wasted time debugging deploy failures caused by simple typos or unreachable sources.

**Independent Test**: Link a recipe with a `recipeLocation` pointing to a non-existent module source and verify a validation error is returned.

**Acceptance Scenarios**:

1. **Given** a recipe with `recipeLocation` pointing to a non-existent registry module, **When** the recipe is linked to an environment, **Then** the system returns a validation error (definitive failures like 404 or authentication denied reject the operation).
2. **Given** a recipe with a valid `recipeLocation` that experiences a transient network failure during validation, **When** the recipe is linked, **Then** the system logs a warning but allows the operation to proceed.

---

### Edge Cases

- What happens when a module has no input variables? The recipe deploys successfully with no parameters required.
- What happens when a module has no outputs? The deployment succeeds with an empty output set.
- What happens when the module source becomes unavailable after initial deployments? Existing resources are unaffected; new deployments fail with a clear error.
- How does the system handle modules that expect specific provider configurations? The existing provider configuration mechanism applies — providers are configured through the environment's recipe configuration.
- What happens when a parameter name does not match any module input variable? The underlying IaC engine surfaces a clear error.
- How does the system handle output resolution? The `outputs` mapping on the RecipePack is the preferred path for all recipes (direct and wrapped). If a module also produces a structured `result` output, it is honored for backward compatibility, but the `outputs` mapping takes precedence when both are present.
- What happens when a `{{context.*}}` expression contains a typo? The unrecognized expression is left as a literal string — deliberate design to avoid masking errors.
- What happens when a `{{context.*}}` expression is malformed (e.g., unclosed `{{`, trailing dot)? Malformed expressions are left as literal strings — the IaC engine will surface errors if the literal doesn't match expected input types.
- What happens when a parameter contains multiple `{{context.*}}` expressions? All expressions are independently resolved.
- What happens when a resource deployed via a direct module recipe is updated (e.g., `recipeParameters` change)? The module is re-executed with the new parameters — ARM redeployment for Bicep, `terraform apply` with updated variables for Terraform. This is idempotent by design and consistent with existing recipe behavior.
- What happens when a module has breaking changes between versions? The platform engineer must update the version in `recipeLocation` deliberately — no automatic version bumping occurs.

## Requirements *(mandatory)*

### Functional Requirements

**Input Resolution (Both Bicep and Terraform)**:

- **FR-001**: System MUST pass `recipeParameters` through to a direct module's native inputs — ARM deployment parameters for Bicep, Terraform input variables for Terraform — without requiring the module to declare any Radius-specific input variables (e.g., no `context` variable needed).
- **FR-002**: System MUST support `{{context.*}}` template expressions in `recipeParameters` values that resolve to Radius runtime context at deployment time. Supported paths include resource metadata (name, id, type, properties), application metadata (name, id), environment metadata (name, id), Kubernetes runtime info (namespace, environmentNamespace), Azure metadata (resource group, subscription), and AWS metadata (region, account). Mixed content (e.g., `prefix-{{context.resource.name}}-suffix`) MUST be resolved while preserving surrounding literal text. Unrecognized paths MUST be left as-is.
- **FR-003**: System MUST support single-level conditional ternary expressions inside `{{...}}` that map context values to concrete configuration values (e.g., `{{context.resource.properties.size == "s" ? "B_Standard_B1ms" : "GP_Standard_D2s_v3"}}`). Nested ternaries are out of scope for V1.
- **FR-004**: System MUST merge `recipeParameters` from the RecipePack and environment-level overrides using shallow merge (top-level keys only). Environment-level parameters take precedence for overlapping keys, replacing the entire value including nested objects.

**Output Resolution (Both Bicep and Terraform)**:

- **FR-005**: System MUST surface module output values as resource properties after successful deployment, without requiring the module to produce a Radius-specific structured `result` output.
- **FR-006**: System MUST support an `outputs` field on the RecipePack that maps module output names to the resource type's read-only properties (e.g., `outputs: { host: 'fqdn', port: 'listen_port' }` where keys are resource property names and values are module output names).
- **FR-007**: For Terraform modules, outputs marked `sensitive = true` MUST be routed to the `Secrets` map instead of the `Values` map, without requiring any module modifications.

**Bicep Module Support**:

- **FR-008**: System MUST accept a Bicep module OCI reference (e.g., `br:mcr.microsoft.com/bicep/avm/res/storage/storage-account:0.14.3`) as the `recipeLocation` value in a RecipePack with `recipeKind: 'bicep'`, without requiring the module to conform to Radius recipe wrapper conventions.
- **FR-009**: System MUST deploy a direct Bicep module through the existing ARM deployment mechanism, passing resolved `recipeParameters` as ARM deployment parameters.
- **FR-010**: System MUST clean up infrastructure provisioned by a direct Bicep module recipe when the deployed resource is deleted.

**Terraform Module Support**:

- **FR-011**: System MUST accept a standard Terraform module source as the `recipeLocation` value in a RecipePack with `recipeKind: 'terraform'`. Supported source formats include: Terraform registry paths (`namespace/name/provider`), Git URLs (`git::https://...` with optional `?ref=` and `//subdir`), HTTP archive URLs, S3 URLs (`s3::...`), and GCS URLs (`gcs::...`). The module MUST NOT be required to include any Radius-specific conventions.
- **FR-012**: System MUST resolve and download the Terraform module at deployment time using standard Terraform module retrieval mechanisms.
- **FR-013**: System MUST execute `terraform destroy` when a resource deployed via a direct Terraform module recipe is deleted.

**Backward Compatibility**:

- **FR-014**: System MUST ensure existing recipe workflows (wrapped recipes with `context` variable and `result` output) continue to function identically — zero behavioral changes to existing deployments.
- **FR-015**: System MUST support the `outputs` mapping for both direct modules and existing wrapped recipes. If a module produces a structured `result` output (with `values`, `secrets`, `resources`), it is still honored for backward compatibility. However, the `outputs` mapping on the RecipePack is the preferred path for output resolution and works uniformly across all recipe types. When both `result` and `outputs` are present, the `outputs` mapping takes precedence.

**Error Handling**:

- **FR-016**: System MUST surface module execution errors (missing variables, provider failures, permission errors) as recipe deployment failures with actionable error messages including the relevant IaC engine error details.
- **FR-017**: System MUST handle modules with no input variables (deploy with no parameters) and modules with no outputs (succeed with empty output set) without errors.

**Authentication**:

- **FR-018**: System MUST support authentication for private module sources (private registries, private Git repositories, private OCI registries) using the existing secret store and credential configuration mechanisms.

**Validation (Best-Effort)**:

- **FR-019**: System SHOULD perform best-effort validation that a `recipeLocation` pointing to a direct module source is reachable at recipe link time, using lightweight probes with a reasonable timeout. Definitive failures (404, authentication denied) SHOULD reject the operation. Transient failures SHOULD be logged as warnings but SHOULD NOT block linking.

### Key Entities

- **RecipePack (`Radius.Core/recipePacks`)**: The primary API resource for this feature. A RecipePack is a collection of recipe configurations keyed by resource type. Each recipe entry has `recipeLocation` that accepts direct module references — Bicep OCI references (`br:...`), Terraform registry paths (`namespace/name/provider`), Git URLs (`git::https://...`), and other Terraform source formats — alongside existing wrapped recipe references. Key fields per recipe entry: `recipeKind` (terraform or bicep), `recipeLocation` (module source, version included in the reference), `recipeParameters` (input values with `{{context.*}}` expression support), and `outputs` (maps module output names to the resource type's read-only properties).
- **Environment**: Configures the deployment context. Recipes are linked to environments (this already works today). Environments can provide environment-level `recipeParameters` that merge with (and override) recipe-level parameters.
- **Recipe Output (from module outputs)**: For direct modules, module output values are mapped to the resource type's read-only properties via the `outputs` mapping. For Terraform, outputs marked `sensitive = true` are routed to the `Secrets` map.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A platform engineer can configure a recipe using an existing Bicep or Terraform module in under 1 minute by setting `recipeLocation` directly — zero wrapping, zero republishing.
- **SC-002**: End-to-end provisioning time for a direct module recipe is within 10% of an equivalent wrapped-recipe deployment — no significant overhead from the direct path.
- **SC-003**: Module output values are mapped to the resource type's read-only properties after deployment via the `outputs` mapping on the RecipePack.
- **SC-004**: Deleting a resource deployed via a direct module recipe fully destroys the underlying infrastructure with zero orphaned resources.
- **SC-005**: Deployment with an inaccessible `recipeLocation` fails within 60 seconds with a clear, actionable error message.
- **SC-006**: Existing wrapped-recipe workflows continue to function with zero behavioral changes — full backward compatibility.
- **SC-007**: Any standard Bicep module from an OCI registry or Terraform module from the public registry/Git (including AVM modules) that does not require Radius-specific conventions can be used directly as a recipe without modification.
- **SC-008**: Platform engineers can use `{{context.*}}` expressions to inject runtime context (resource name, namespace, environment, etc.) into any module's parameters without modifying the module.
- **SC-009**: Linking a recipe with a `recipeLocation` pointing to a non-existent or unreachable module source performs best-effort validation at link time. Definitive failures (404, authentication denied) return a validation error before deployment. Transient failures are logged as warnings but do not block linking.

## Assumptions

- **A-001**: The existing Bicep driver (ARM deployments) and Terraform driver (module execution) are extended to handle direct module references. No new driver or execution engine is introduced.
- **A-002**: Module input variable types are passed through without type transformation. Type checking is delegated to the underlying IaC engine (ARM for Bicep, Terraform CLI for Terraform), which produces clear errors for type mismatches.
- **A-003**: This feature operates alongside the existing recipe workflow. Wrapped recipes continue to work exactly as before.
- **A-004**: Infrastructure state management uses existing mechanisms — ARM deployment tracking for Bicep, Kubernetes secret-backed Terraform state for Terraform.
- **A-005**: Provider configuration uses existing mechanisms — Azure provider context for Bicep, `recipes.Configuration` for Terraform.
- **A-006**: Local filesystem paths as `recipeLocation` are out of scope. The initial scope covers OCI-hosted Bicep modules, Terraform registry modules, Git-hosted modules, and HTTP/S3/GCS archives.
- **A-007**: AVM modules are treated identically to any other direct module — no special handling. The "AVM" designation is purely organizational.
- **A-008**: Recipe linking to environments already works today and is not part of this feature. This feature is solely about what happens when `recipeLocation` points to a standard module instead of a wrapped one.
- **A-009**: Module version is specified as part of `recipeLocation` (e.g., OCI tag for Bicep, `?ref=` for Git, registry version syntax for Terraform). There is no separate `templateVersion` field.
- **A-010**: Direct module deployments use the same observability mechanisms (logging, tracing, metrics) as existing wrapped-recipe deployments. No new observability infrastructure is introduced.
- **A-011**: Sensitive input parameters (e.g., database passwords, API keys) are handled by the existing Radius secret store mechanism. No new secret handling is introduced for direct module support.
- **A-012**: Transient failure retry during module fetch at deploy time is delegated to the underlying IaC engine (Terraform and ARM both have built-in retry for transient failures). No custom Radius-level retry logic is introduced.

## Future Steps

The following are good-to-have capabilities that build on the core feature but are not required for initial delivery:

- **Inspect Module Schema Before Deployment**: Retrieve a module's input variables and outputs before deploying, enabling discoverability and reducing trial-and-error. Engineers can consult module documentation as a workaround today.
