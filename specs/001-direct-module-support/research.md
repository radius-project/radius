# Research: Direct Terraform and AVM Module Support via Recipe Packs

## R-001: Source Format Detection Strategy

**Decision**: Use pattern-matching heuristics on the `recipeLocation` string to classify source type before attempting resolution.

**Rationale**: The existing codebase already uses `hashicorp/go-getter` which handles the actual download. However, we need pre-resolution classification to determine: (a) whether to attempt direct module resolution vs. OCI/wrapped path, (b) which validation to apply at creation time, and (c) whether version pinning applies. Pattern-matching is simple, deterministic, and avoids network calls during classification.

**Classification Rules**:
| Pattern | Source Type | Example |
|---------|-------------|---------|
| `<ns>/<name>/<provider>` (3-segment, no scheme) | Terraform Registry | `hashicorp/consul/aws` |
| `git::` prefix | Git repository | `git::https://github.com/org/module.git` |
| `http://` or `https://` (no `git::` prefix) | HTTP archive | `https://example.com/module.tar.gz` |
| `s3::` prefix | S3 bucket | `s3::bucket/path` |
| `gcs::` prefix | GCS bucket | `gcs::bucket/path` |
| Contains `oci://` or matches existing OCI patterns | OCI/Wrapped (fallback) | `ghcr.io/org/recipe:v1` |

**Alternatives Considered**:
1. **Always try direct first, catch errors** — Rejected because it would add latency for existing OCI recipes (network timeout before fallback).
2. **Require explicit `terraform::` prefix** — Rejected per FR-012 (no new schema fields/flags) and the principle of using standard Terraform source syntax.
3. **Parse with go-getter's Detect** — Considered but go-getter's detect is designed for runtime, not pre-validation classification. It doesn't distinguish "valid but unreachable" from "not a valid source format."

---

## R-002: Direct Module Resolution vs. OCI/Wrapped Fallback Strategy

**Decision**: Implement a two-phase resolution in the config loader's `LoadRecipe` path:
1. Classify `recipeLocation` using source format detector
2. If classified as a known Terraform source (registry, git, http, s3, gcs) → mark as direct module
3. If classification is ambiguous or unknown → attempt direct resolution first, then fall back to existing OCI path

**Rationale**: Per FR-015, the system must try direct resolution first and fall back. However, for unambiguous sources (3-segment registry paths, `git::` prefixed), we can skip the fallback path entirely, improving performance. The ambiguous case (e.g., a plain HTTPS URL that could be either an OCI manifest or a terraform archive) needs the try-then-fallback approach.

**Alternatives Considered**:
1. **Purely sequential try/fallback for all paths** — Rejected because it adds unnecessary latency for clearly-Terraform sources.
2. **Add a flag to RecipeDefinition** — Rejected per FR-012 (no new fields).

---

## R-003: Output Mapping for Direct Modules (FR-015 Precedence)

**Decision**: For direct module recipes, apply a three-tier precedence for output handling:
1. If `outputs` mapping is configured → only mapped outputs flow through (sensitive outputs go to Secrets)
2. Elif module has a `result` output → use wrapped recipe convention (backward compat)
3. Else (direct module, no mapping) → pass all outputs through (sensitive → Secrets, non-sensitive → Values)

**Rationale**: Per FR-015, the `outputs` mapping gives platform engineers explicit control over which module outputs map to which resource properties. When no mapping is configured, pass-through gives visibility into all module outputs. The `result` output is honored for backward compatibility with modules that follow the wrapped recipe convention but don't have an `outputs` mapping configured.

**Implementation Approach**:
```go
// In prepareRecipeResponse, detect mode via precedence:
switch {
case hasOutputsMapping:
    // Only mapped outputs flow through
    values, secrets = outputmapping.Apply(rawOutputs, definition.Outputs)
case hasResultOutput:
    // Wrapped recipe convention
    recipeResponse.PrepareRecipeResponse(resultValue)
case isDirectModule:
    // Pass through all outputs (sensitive → Secrets)
    values, secrets = outputmapping.Apply(rawOutputs, nil)
}
```

**Alternatives Considered**:
1. **Auto-detect Resources/Secrets from output types** — Rejected; sensitivity flag is the only signal used.
2. **`result` takes precedence over `outputs`** — Rejected because `outputs` mapping is a platform engineer's explicit intent and should win.

---

## R-004: RecipePack Creation-Time Validation

**Decision**: Add a validation step in `CreateOrUpdateRecipePack` controller that, for Terraform recipe definitions with recognized direct module source formats, attempts to resolve the module source to verify reachability.

**Rationale**: Per FR-014, the system must validate reachability at creation time. This catches typos, inaccessible registries, and bad URLs early rather than at deploy time. The validation uses a lightweight check (e.g., registry API probe for registry modules, HEAD request for HTTP, `git ls-remote` for Git) rather than a full module download.

**Implementation Approach**:
- Extract validation into a `ValidateModuleSource(ctx, recipeLocation, templateVersion, secrets)` function in `pkg/recipes/source/`
- For registry modules: HTTP GET to `https://registry.terraform.io/v1/modules/{ns}/{name}/{provider}/{version}` (or configured registry)
- For Git sources: `git ls-remote` to verify the ref exists
- For HTTP sources: HTTP HEAD to verify URL responds with 2xx
- Validation is best-effort with a 30-second timeout; transient failures are logged as warnings but don't block creation

**Alternatives Considered**:
1. **Full module download at creation time** — Rejected because it's expensive and slow; downloads can be large.
2. **No validation at creation time** — Rejected per FR-014.
3. **Async validation (create succeeds, mark status)** — Rejected for simplicity; synchronous validation with timeout is sufficient.

---

## R-005: Version Pinning for Registry Modules

**Decision**: Use the existing `TemplateVersion` field on `RecipeDefinition`/`EnvironmentDefinition`. When the source is a Terraform registry module, pass `TemplateVersion` as the `version` constraint in the generated `main.tf.json` module block (this already happens in `config.New()`).

**Rationale**: The existing code in `pkg/recipes/terraform/config/config.go` already handles this:
```go
if moduleVersion != "" {
    moduleConfig["version"] = moduleVersion
}
```
No new code needed for version pinning itself — only for validating that the specified version exists (as part of creation-time validation in R-004).

**Alternatives Considered**:
1. **Embed version in recipeLocation** (e.g., `hashicorp/consul/aws@1.2.0`) — Rejected because it diverges from Terraform conventions and the existing `TemplateVersion` field already exists.

---

## R-006: Detecting Direct Module Mode at Execution Time

**Decision**: Add a field or method to distinguish direct-module mode from wrapped-recipe mode during execution. Use the source resolver's classification result stored as a lightweight signal (e.g., a boolean `IsDirectModule` on `EnvironmentDefinition` or detected at runtime from `TemplatePath`).

**Rationale**: The driver needs to know whether to use flat output mapping (direct) or `result`-based output mapping (wrapped). Since FR-012 prohibits new data model fields for persistence, the detection happens at runtime by re-classifying `TemplatePath` in the driver layer.

**Implementation**:
```go
// In pkg/recipes/source/resolver.go
func IsDirectModuleSource(recipeLocation string) bool {
    sourceType := Classify(recipeLocation)
    return sourceType == SourceTypeTerraformRegistry ||
           sourceType == SourceTypeGit ||
           sourceType == SourceTypeHTTP ||
           sourceType == SourceTypeS3 ||
           sourceType == SourceTypeGCS
}
```

This avoids persisting new fields while giving the driver a clear signal.

**Alternatives Considered**:
1. **Check for `result` output after module inspection** — Could work but adds complexity and breaks the "no changes to wrapped behavior" guarantee if a direct module happens to have a `result` output.
2. **Add a field to the persisted model** — Rejected per FR-012.

---

## R-007: Terraform Config Generation for Direct Modules

**Decision**: The existing `config.New()` function already generates valid Terraform configuration for any module source. For direct modules, the key differences are:
1. **Outputs**: Generate output blocks for ALL module outputs (not just `result`)
2. **Recipe Context**: Skip adding `recipe_context` variable (direct modules don't know about it)
3. **Parameters**: Pass through as-is (existing behavior works)

**Rationale**: The existing config generation is already source-agnostic. The main change is in `AddOutputs()` — for direct modules, we need to generate output blocks that forward all module outputs:
```hcl
output "<name>" {
  value     = module.<recipe_name>.<name>
  sensitive = <if marked sensitive>
}
```

This requires knowing the module's output names before generating config, which is already available from `inspectModule()`.

**Implementation**: After `downloadAndInspect()`, pass the inspection result to a new `AddAllOutputs(moduleName, moduleOutputs)` method that generates forwarding outputs for each module output.

**Alternatives Considered**:
1. **Use a wildcard output** — Not supported by Terraform.
2. **Don't generate output blocks, read from state directly** — Rejected because `terraform output` only returns values for declared outputs in the root module.

---

## R-008: GetRecipeMetadata for Direct Modules

**Decision**: The existing `GetRecipeMetadata` path already works for direct modules because it uses `downloadAndInspect()` which fetches the module and reads its variables. The returned `Parameters` map already contains all module input variables with their metadata.

**Rationale**: No changes needed to the metadata retrieval flow. The `inspectModule()` function already parses all variables regardless of source type. This satisfies FR-010 and User Story 5.

**Alternatives Considered**: None — existing code handles this correctly.

---

## R-009: Error Handling for Direct Module Failures

**Decision**: Surface Terraform errors directly through existing error paths. Use existing `RecipeError` types with appropriate error codes:
- `RecipeDownloadFailed` for source resolution failures
- `RecipeDeploymentFailed` for terraform apply errors (missing variables, provider failures)
- `RecipeDeletionFailed` for terraform destroy errors

**Rationale**: Per FR-009, errors must be actionable with relevant Terraform error details. The existing error wrapping in the terraform executor and driver already includes the full Terraform error message. No new error types needed.

**Alternatives Considered**:
1. **Parse and categorize Terraform errors** — Rejected per YAGNI; raw Terraform errors are already actionable for platform engineers.

---

## R-010: Sensitive Output Handling

**Decision**: For direct modules, outputs marked `sensitive = true` (TF) or `SecureString`/`SecureObject` type (Bicep ARM) are routed to `RecipeOutput.Secrets` instead of `RecipeOutput.Values`. This applies in both pass-through mode (no outputs mapping) and mapped mode (outputs mapping configured).

**Rationale**: The existing split between Values and Secrets on RecipeOutput provides the security mechanism. The `outputmapping.Apply()` utility in `pkg/recipes/outputmapping/` handles this routing for both TF and Bicep drivers — sensitive outputs go to Secrets, non-sensitive to Values.

**Implementation** (via shared `outputmapping` package):
```go
// outputmapping.Apply routes based on OutputValue.Sensitive flag
for propertyName, outputName := range mapping {
    if output, ok := rawOutputs[outputName]; ok {
        if output.Sensitive {
            secrets[propertyName] = output.Value
        } else {
            values[propertyName] = output.Value
        }
    }
}
```

---

## R-011: Context Injection Strategy

**Decision**: Use explicit `{{context.*}}` template expressions in recipe parameters, resolved by `paramresolver.ResolveParameters()`, instead of automatic variable name-matching.

**Rationale**: Direct Terraform modules have arbitrarily named input variables. An automatic name-matching approach (e.g., injecting the Kubernetes namespace if a module has a variable named `namespace`) is semantically ambiguous — a `namespace` variable might refer to a Terraform provider namespace, a DNS namespace, or something else entirely. Automatic injection based on variable names would silently produce incorrect values with no way for the platform engineer to detect or override the behavior.

Template expressions give platform engineers explicit control over which context values flow into which module variables. The mapping is visible in the RecipePack definition, auditable, and unambiguous. This also means modules with non-standard variable names (e.g., `k8s_ns` instead of `namespace`) work correctly — the platform engineer simply writes `{{context.runtime.kubernetes.namespace}}` as the parameter value for `k8s_ns`.

**Supported Expression Paths**:
| Expression | Description |
|------------|-------------|
| `context.resource.name` | Deployed resource name |
| `context.resource.id` | Deployed resource ID |
| `context.resource.type` | Deployed resource type |
| `context.application.name` | Application name |
| `context.application.id` | Application ID |
| `context.environment.name` | Environment name |
| `context.environment.id` | Environment ID |
| `context.runtime.kubernetes.namespace` | Target Kubernetes namespace |
| `context.runtime.kubernetes.environmentNamespace` | Environment Kubernetes namespace |
| `context.azure.resourceGroup.name` | Azure resource group name |
| `context.azure.resourceGroup.id` | Azure resource group ID |
| `context.azure.subscription.subscriptionId` | Azure subscription ID |
| `context.azure.subscription.id` | Azure subscription ID (alias) |
| `context.aws.region` | AWS region |
| `context.aws.account` | AWS account ID |

**Implementation Approach**:
- `ResolveParameters()` in `pkg/recipes/paramresolver/resolver.go` scans parameter string values for `{{...}}` patterns using regex
- `buildContextLookup()` creates a flat map from the `recipecontext.Context` struct (e.g., `"context.resource.name" → "my-resource"`)
- `resolveStringValue()` uses `regexp.ReplaceAllStringFunc` to replace each `{{context.*}}` match with the looked-up value
- Unrecognized expressions (no match in the lookup map) are left as-is — no silent failure, no empty substitution
- Called from both TF and Bicep drivers in the direct module path (when no `context` variable/parameter exists)
- Works alongside existing `AddRecipeContext` for wrapped recipes (which still use the single `context` variable)

**Alternatives Considered**:
1. **Automatic name-matching (`AddDirectModuleContext`)** — Rejected due to semantic ambiguity of variable names. A variable named `namespace` could mean many things. Silent injection of wrong values is worse than requiring explicit mapping.
2. **New schema field for context mapping** — Rejected per FR-012 (no new data model fields). Template expressions in existing `Parameters` achieve the same result.
3. **Helm-style `{{ .Values.* }}` syntax** — Rejected because it conflicts with Terraform's own interpolation syntax (`${...}`) and Go template syntax. The `{{context.*}}` prefix is unambiguous and clearly indicates Radius context injection.
