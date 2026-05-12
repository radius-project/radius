# Data Model: Direct Terraform and AVM Module Support via Recipe Packs

## Overview

This feature extends the existing recipe pack data model with two new fields on `RecipeDefinition` — `recipeParameters` and `outputs` — and broadens the `recipeLocation` field to accept standard Terraform module sources (registry, Git, HTTP, S3, GCS) alongside existing wrapped recipe references.

## Extended Entities

### RecipeDefinition (extended schema)

**Location**: `pkg/corerp/datamodel/recipepack.go`

```go
type RecipeDefinition struct {
    RecipeKind       string            `json:"recipeKind"`                 // "terraform" or "bicep"
    RecipeLocation   string            `json:"recipeLocation"`             // Template source path/URL
    RecipeParameters map[string]any    `json:"recipeParameters,omitempty"` // Input parameters with {{context.*}} support
    PlainHTTP        bool              `json:"plainHTTP,omitempty"`        // Allow insecure connections
    Outputs          map[string]string `json:"outputs,omitempty"`          // Maps resource property → module output name
}
```

**New Fields**:
- `RecipeParameters`: Input parameters passed through to Terraform module variables. Values may contain `{{context.*}}` template expressions resolved at deployment time.
- `Outputs`: Maps resource property names to module output names (e.g., `{"host": "hostname"}` means resource property `host` gets its value from module output `hostname`). When empty/nil, all module outputs pass through with original names.

**`RecipeLocation` accepts**:
- Terraform Registry: `hashicorp/consul/aws`, `Azure/avm-res-storage-storageaccount/azurerm`, `ballj/postgresql/kubernetes`
- Git URLs: `git::https://github.com/org/terraform-aws-vpc.git`
- Git with ref: `git::https://github.com/org/module.git?ref=v2.0.0`
- Git with subdirectory: `git::https://github.com/org/repo.git//modules/vpc`
- HTTP archives: `https://example.com/modules/vpc.tar.gz`
- S3: `s3::https://bucket.s3.amazonaws.com/module.zip`
- GCS: `gcs::https://bucket.storage.googleapis.com/module.zip`
- Existing OCI/wrapped recipes: `ghcr.io/org/recipe:v1` (unchanged behavior)

**Template Expressions in RecipeParameters**: Values can contain `{{context.*}}` expressions resolved at deployment time. For example, `{{context.runtime.kubernetes.namespace}}` resolves to the target Kubernetes namespace. Mixed content is supported (e.g., `prefix-{{context.resource.name}}-suffix`). Unrecognized expressions are left as-is.

**Validation Rules** (at creation time):
- Source must be reachable (lightweight probe, 30s timeout) — definitive failures reject, transient warnings allowed
- Source format must be classifiable by the resolver
- If unclassifiable, accepted without validation (fallback to existing behavior)

### EnvironmentDefinition (internal, extended)

**Location**: `pkg/recipes/types.go`

```go
type EnvironmentDefinition struct {
    Name            string            // Recipe name
    Driver          string            // "terraform" or "bicep"
    ResourceType    string            // Portable resource type
    Parameters      map[string]any    // Default recipe parameters
    TemplatePath    string            // Module source URL/path (expanded behavior)
    TemplateVersion string            // Module version (used for registry pinning)
    PlainHTTP       bool              // Allow insecure connections
    Outputs         map[string]string // Maps resource property → module output name
}
```

**New Field**: `Outputs` — populated from RecipeDefinition.Outputs via the config loader.

### RecipeOutput (extended with DirectModule flag)

**Location**: `pkg/recipes/types.go`

```go
type RecipeOutput struct {
    Resources    []string       // Deployed resource IDs (from TF state)
    Secrets      map[string]any // Sensitive output values
    Values       map[string]any // Non-sensitive output values
    Status       *rpv1.RecipeStatus
    DirectModule bool           // True when outputs come from a direct module (skip schema filter)
}
```

**Behavioral Change for Direct Modules**:
- `Values`: Populated with ALL non-sensitive Terraform module outputs. If `Outputs` mapping exists, values are renamed (module output name → resource property name). If no mapping, original names pass through.
- `Secrets`: Populated with ALL sensitive Terraform module outputs (same rename logic).
- `DirectModule`: Set to `true` by the TF driver for direct modules. When true, the DynamicProcessor skips schema filtering and dumps all outputs to resource.Properties.
- **`result` output priority**: The system checks for a `result` output FIRST for all sources. If `result` exists and no `outputs` mapping is configured, the module is treated as a wrapped recipe. This prevents misclassifying wrapped recipes hosted on registries.

**Behavioral Change for Wrapped Recipes** (unchanged):
- Existing logic: looks for `result` output, parses into Resources/Secrets/Values

## New Internal Types (not persisted)

### SourceType Enum

**Location**: `pkg/recipes/source/types.go`

```go
type SourceType int

const (
    SourceTypeUnknown            SourceType = iota // Unclassified — use fallback
    SourceTypeTerraformRegistry                    // e.g., "hashicorp/consul/aws"
    SourceTypeGit                                  // e.g., "git::https://..."
    SourceTypeHTTP                                 // e.g., "https://example.com/module.tar.gz"
    SourceTypeS3                                   // e.g., "s3::bucket/key"
    SourceTypeGCS                                  // e.g., "gcs::bucket/key"
    SourceTypeOCI                                  // Existing OCI/wrapped recipe path
)
```

### ResolvedSource

**Location**: `pkg/recipes/source/types.go`

```go
type ResolvedSource struct {
    Type           SourceType // Classified source type
    OriginalPath   string     // Original recipeLocation value
    IsDirectModule bool       // True if this is a direct TF module (not wrapped)
}
```

## State Transitions

### Recipe Deployment Lifecycle (with direct module)

```
┌─────────────────┐
│  RecipePack     │ ← recipeLocation validated at creation
│  Created        │
└────────┬────────┘
         │ Deploy resource using recipe
         ▼
┌─────────────────┐
│  Source          │ ← Classify recipeLocation
│  Classification │
└────────┬────────┘
         │ Direct module detected
         ▼
┌─────────────────┐
│  Module          │ ← terraform get (fresh download, no cache)
│  Download        │
└────────┬────────┘
         │ Success
         ▼
┌─────────────────┐
│  Module          │ ← Extract variables, outputs, providers
│  Inspection     │
└────────┬────────┘
         │ Generate config with all-output forwarding
         ▼
┌─────────────────┐
│  Expression      │ ← Resolve {{context.*}} in recipeParameters
│  Resolution     │   via ResolveParameterExpressions()
└────────┬────────┘
         │ Parameters with resolved context values
         ▼
┌─────────────────┐
│  Terraform       │ ← init + apply
│  Execution      │
└────────┬────────┘
         │ Success
         ▼
┌─────────────────┐
│  Output          │ ← Apply outputs mapping (rename/filter)
│  Mapping        │   or pass-through all outputs
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│  Resource        │ ← Outputs accessible via Radius API
│  Deployed       │   (bypasses schema filter for direct modules)
└─────────────────┘
```

## Relationships

```
RecipePack (1) ──contains──▶ (N) RecipeDefinition
     │                              │
     │                    recipeLocation + recipeParameters + outputs
     │                              │
     │                    ┌─────────┴──────────┐
     │                    │                    │
     │              Direct Module         Wrapped/OCI Recipe
     │              (new behavior)        (existing behavior)
     │                    │                    │
     │              ┌─────┴─────┐              │
     │              │           │              │
     │         Registry      Git/HTTP          │
     │              │           │              │
     │              └─────┬─────┘              │
     │                    │                    │
     ▼                    ▼                    ▼
Environment ──uses──▶ TerraformDriver ◀──uses── Environment
  (recipePacks,           │
   recipeParameters)┌─────┴─────┐
                    │           │
              Direct Mode   Wrapped Mode
              (flat output  (result output
               + outputs     parsing)
               mapping)
```

## Validation Rules

| Field | Rule | When Applied |
|-------|------|--------------|
| `RecipeLocation` | Must be non-empty string | Always (existing) |
| `RecipeLocation` | If classifiable as direct module, source must be reachable | RecipePack create/update |
| `RecipeLocation` | Format must match one of: registry, git, http, s3, gcs, or OCI | Soft validation (unknown = fallback) |
| `RecipeKind` | Must be "terraform" for direct module sources | RecipePack create/update |
| `RecipeParameters` | Keys should match module input variable names | At terraform apply time (Terraform validates) |
| `RecipeParameters` | Values may contain `{{context.*}}` template expressions | Resolved at deploy time by ResolveParameterExpressions() |
| `Outputs` | Keys are resource property names, values are module output names | Applied at output mapping time in TF driver |
| `Outputs` | No duplicate target property names allowed | RecipePack create/update (validation pending) |
| `Outputs` | Target property names must not collide with reserved properties (application, environment, status, connections) | RecipePack create/update (validation pending) |
