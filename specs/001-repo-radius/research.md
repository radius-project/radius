# Research: Git Workspace Mode

**Feature**: 001-repo-radius | **Date**: 2026-02-02

This document resolves all NEEDS CLARIFICATION items from the Technical Context and documents research findings for key technical decisions.

---

## Terminology

The Radius resource hierarchy uses these terms:

```
Resource Type → Application Resource → Cloud Resource → Cloud Provider API
```

| Term | Definition | Example |
|------|------------|---------|
| **Resource Type** | Schema definition (what CAN exist) | `Applications.Datastores/redisCaches` |
| **Application Resource** | Resource declared in the Bicep application model | `resource redis 'Applications.Datastores/redisCaches@...'` in `.radius/model/app.bicep` |
| **Cloud Resource** | Actual deployed infrastructure managed by Terraform/Bicep | AWS ElastiCache, Azure Cache for Redis |
| **Cloud Provider API** | Underlying provider API | AWS ElastiCache API, Azure Redis API |

**Key insight**: Application Resources are defined in the **Bicep application model** (`.radius/model/app.bicep`). Radius resolves each Application Resource's Resource Type to a Recipe, then orchestrates deployment via Terraform or Bicep. The resulting Cloud Resources are managed by those tools' state files.

---

## Research Tasks

### 1. Git Operations Library Selection

**Question**: Should we use go-git/go-git (pure Go) or shell out to git CLI for Git operations?

**Research Findings**:

| Option | Pros | Cons |
|--------|------|------|
| **go-git/go-git** | Pure Go, no external dependency, testable | Limited sparse-checkout support, larger binary size |
| **Git CLI (exec)** | Full feature support, sparse-checkout works, users already have it | External dependency, harder to test |

**Decision**: Use **Git CLI via os/exec**

**Rationale**: 
- Sparse-checkout (for fetching Resource Types from resource-types-contrib) is better supported via CLI
- Users running `rad` already need Git installed (FR-001 assumes Git knowledge)
- Existing Radius codebase uses shell commands in similar scenarios
- Testable via interface abstraction (mock the executor)

**Alternatives Rejected**:
- go-git: Sparse-checkout support is experimental/limited; would require workarounds

---

### 2. Terraform Execution Library

**Question**: How should we invoke Terraform CLI operations (init, plan, apply)?

**Research Findings**:

| Option | Pros | Cons |
|--------|------|------|
| **hashicorp/terraform-exec** | Official HashiCorp library, typed API, handles JSON output | New dependency |
| **Shell exec** | Simple, no new dependency | Manual JSON parsing, error handling |

**Decision**: Use **hashicorp/terraform-exec**

**Rationale**:
- Official library from HashiCorp specifically designed for programmatic Terraform execution
- Handles `terraform plan -json` output parsing
- Provides structured error handling
- Well-maintained and used by Terraform Cloud

**Alternatives Rejected**:
- Shell exec: Would require manual JSON parsing of plan output, more error-prone

#### Environment Variable Handling

**Important Finding**: terraform-exec **inherits** the user's environment (`os.Environ()`) but **strips out** certain "managed" variables:
- `TF_LOG`, `TF_LOG_PATH`, `TF_LOG_CORE`, `TF_LOG_PROVIDER` (logging)
- `TF_INPUT`, `TF_IN_AUTOMATION`
- `TF_APPEND_USER_AGENT`, `TF_REATTACH_PROVIDERS`, `TF_DISABLE_PLUGIN_TLS`, `TF_SKIP_PROVIDER_VERIFY`

**What IS inherited automatically**:
- AWS credentials: `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, `AWS_PROFILE`, `AWS_REGION`
- Azure credentials: `ARM_CLIENT_ID`, `ARM_TENANT_ID`, `ARM_CLIENT_SECRET`, `ARM_SUBSCRIPTION_ID`
- Provider mirrors: `TF_CLI_CONFIG_FILE`, `.terraformrc` settings
- Terraform variables: `TF_VAR_*`

**What requires explicit forwarding**:
- `TF_LOG` - Must be forwarded via `tf.SetLog(os.Getenv("TF_LOG"))`
- `TF_LOG_PATH` - Must be forwarded via `tf.SetLogPath(os.Getenv("TF_LOG_PATH"))`

**Implementation Pattern**:
```go
tf, _ := tfexec.NewTerraform(workingDir, execPath)

// Forward user's TF_LOG if set, otherwise default based on --verbose flag
if tfLog := os.Getenv("TF_LOG"); tfLog != "" {
    tf.SetLog(tfLog)
} else if verbose {
    tf.SetLog("DEBUG")
}

// Forward user's TF_LOG_PATH if set, otherwise use our log directory
if tfLogPath := os.Getenv("TF_LOG_PATH"); tfLogPath != "" {
    tf.SetLogPath(tfLogPath)
} else {
    tf.SetLogPath(filepath.Join(logsDir, "terraform.log"))
}

// Stream stdout/stderr for real-time feedback
tf.SetStdout(os.Stdout)
tf.SetStderr(os.Stderr)
```

This ensures:
1. User credentials (AWS, Azure, etc.) are automatically inherited
2. User's existing `TF_LOG=DEBUG` preference is honored when set
3. `--verbose` flag enables debug logging when user hasn't specified
4. Full traceability with debug logs for troubleshooting

---

### 3. .env File Parsing

**Question**: How should we parse `.env` and `.env.<ENVIRONMENT>` files?

**Research Findings**:

| Option | Pros | Cons |
|--------|------|------|
| **joho/godotenv** | Popular, well-tested, handles edge cases | New dependency |
| **Custom parser** | No dependency | More code to maintain, edge cases |
| **spf13/viper** | Already a dependency | Requires config file type hints, more complex |

**Decision**: Use **joho/godotenv**

**Rationale**:
- Battle-tested library specifically for .env file parsing
- Handles quoting, multi-line values, comments correctly
- Small dependency footprint
- Used widely in Go ecosystem

**Alternatives Rejected**:
- Custom parser: .env parsing has subtle edge cases (quoting, escaping, comments)
- spf13/viper: Can do it but requires more setup, not its primary use case

---

### 4. Workspace Detection and Switching

**Question**: How should the "git" workspace type integrate with existing workspace management?

**Research Findings**:

Existing workspace code in `pkg/cli/workspaces/`:
- `types.go` defines `Workspace` struct with `Connection` field
- `connection.go` defines connection types (currently `KubernetesConnection`)
- `config.go` manages `~/.rad/config.yaml` file

**Decision**: Add **GitConnection** type as a new connection kind

**Rationale**:
- Follows existing pattern (KubernetesConnection, etc.)
- Workspace switching logic already exists
- "git" workspace is built-in (always available, doesn't need explicit creation)
- Clear separation between workspace types

**Implementation Notes**:
```go
type GitConnection struct {
    Kind string // "git"
}

// Built-in workspace, doesn't require connection details
func (c *GitConnection) IsBuiltIn() bool { return true }
```

---

### 5. Bicep Execution and Configuration

**Question**: How should Radius invoke Bicep CLI without requiring users to create `bicepconfig.json`?

**Problem**: Users currently must:
1. Create `bicepconfig.json` with extension registry configuration
2. Add `extension radius` to every `.bicep` file

This creates friction and pollutes the repository.

**Research Findings**:

| Option | Pros | Cons |
|--------|------|------|
| **Auto-generate bicepconfig.json** | Simple to implement | Pollutes repo, file management issues |
| **In-memory config injection** | Zero file pollution, seamless UX | Requires understanding Bicep CLI internals |
| **Temp file approach** | Works with existing Bicep CLI | Still creates files, cleanup needed |

**Decision**: Use **in-memory bicepconfig injection via temp directory**

**Rationale**:
- Bicep CLI uses nearest `bicepconfig.json` in directory hierarchy
- Radius generates a temp directory with `bicepconfig.json` containing:
  - Extension registry config (Radius extension location)
  - Alias mappings (e.g., `radius`)
  - Required experimental flags
- Bicep is invoked from this temp directory (or with file copied there)
- Temp directory is cleaned up after execution
- **No files are created in user's repository**

**Implementation Pattern**:
```go
// pkg/cli/git/bicep/executor.go

func (e *BicepExecutor) Build(bicepFile string, outputDir string) error {
    // Create temp directory for bicepconfig.json
    tempDir, err := os.MkdirTemp("", "radius-bicep-*")
    if err != nil {
        return err
    }
    defer os.RemoveAll(tempDir)

    // Generate bicepconfig.json in temp directory
    bicepConfig := BicepConfig{
        Extensions: map[string]string{
            "radius": "br:ghcr.io/radius-project/bicep-extensions/radius:0.1.0",
        },
        ExperimentalFeaturesEnabled: map[string]bool{
            // Add any required experimental features
        },
    }
    
    configPath := filepath.Join(tempDir, "bicepconfig.json")
    if err := writeBicepConfig(configPath, bicepConfig); err != nil {
        return err
    }

    // Copy user's bicep file to temp directory so it picks up the config
    tempBicepFile := filepath.Join(tempDir, filepath.Base(bicepFile))
    if err := copyFile(bicepFile, tempBicepFile); err != nil {
        return err
    }

    // Run bicep build from temp directory
    cmd := exec.Command("bicep", "build", tempBicepFile, "--outdir", outputDir)
    cmd.Dir = tempDir
    return cmd.Run()
}
```

**User Experience**:
- Users write `app.bicep` with `extension radius` directive
- Users run `rad plan` or `rad deploy`
- **No `bicepconfig.json` required in their repository**
- VS Code users who want IntelliSense can optionally create `bicepconfig.json` at repo root

**✅ Resolved: Per-Resource-Type Bicep Extensions**

**Decision**: Single `radius` Bicep extension with built-in types; custom types validated at `rad plan` only.

**Details**:
- Single `radius` extension published to `ghcr.io/radius-project/bicep-extensions/radius`
- Extension contains all built-in Resource Types from `resource-types-contrib`
- Custom/user-defined types won't have Bicep IntelliSense (accepted limitation)
- Radius CLI validates all types at `rad plan` time using `.radius/config/types/` YAML schemas

**Trade-off Accepted**: Most users use standard types; custom types are validated at plan time; changing this requires Bicep CLI changes outside our control.

**Future Enhancement**: See spec.md FE-011 for planned IntelliSense support for custom Resource Types.

---

### 6. Progress Display for CI vs Interactive

**Question**: How should progress display adapt between CI (GITHUB_ACTIONS=true) and interactive terminal?

**Research Findings**:

Existing code uses Bubble Tea for interactive prompts. The spec requires:
- Interactive: Animated spinner, real-time progress
- CI (`--quiet` or `GITHUB_ACTIONS=true`): Simple line-by-line output

**Decision**: Use **conditional rendering** based on environment detection

**Rationale**:
- Detect CI via `os.Getenv("CI")` or `os.Getenv("GITHUB_ACTIONS")`
- Detect `--quiet` flag
- If CI or quiet: use simple `output.LogInfo()` calls
- If interactive: use existing Bubble Tea progress models

**Implementation Notes**:
```go
func (r *Runner) isCI() bool {
    return os.Getenv("CI") != "" || os.Getenv("GITHUB_ACTIONS") != ""
}
```

---

### 7. Atomic Deployment and Rollback Strategy

**Question**: How should rollback be implemented when deployment fails partway?

**Research Findings**:

**Important clarification**: Neither Terraform nor Bicep provide true atomic semantics:
- **Terraform**: If `terraform apply` fails at resource 5 of 10, resources 1-4 **remain deployed**. No auto-rollback.
- **Bicep/ARM**: Incremental mode only adds/updates. Complete mode is destructive. Limited rollback.

**What these tools DO provide**:
- **Idempotency**: Re-running converges to desired state
- **State tracking**: Know exactly what exists
- **Plan phase**: Catch issues before apply
- **Destroy command**: Clean teardown of everything in state

**Decision**: Radius operates at **Application Resource level**, not Cloud Resource level

**Terminology**:
```
Resource Type → Application Resource → Cloud Resource → Cloud Provider API
     ↑                  ↑                    ↑                 ↑
   Schema        Resource in Bicep    Actual deployed    AWS/Azure
  (defines)     application model    infrastructure     actual API
```

**Rationale**:
- Radius is an **orchestrator**, not a resource manager
- Terraform/Bicep own the Cloud Resource lifecycle and state
- Radius tracks which **Application Resources** (from the Bicep model) were deployed
- On failure: User fixes issue and re-runs (`rad deploy` is idempotent)
- `rad app delete` calls underlying tool's destroy for each Application Resource in reverse dependency order
- **No custom resource-level rollback** - trust Terraform/Bicep's native capabilities

**Implementation Notes**:
```go
// pkg/cli/git/deploy/orchestrator.go

// DeploymentState tracks Application Resource deployment (not Cloud Resources)
// Application Resources are parsed from the Bicep model (.radius/model/app.bicep)
type DeploymentState struct {
    ApplicationResources []ApplicationResourceStatus
}

type ApplicationResourceStatus struct {
    Name         string       // Resource name from Bicep model (e.g., "redis", "container")
    ResourceType string       // e.g., "Applications.Datastores/redisCaches"
    Status       DeployStatus // pending, deploying, success, failed
    Error        error        // If failed
}

// On failure, Radius does NOT auto-rollback.
// User fixes the issue, re-runs `rad deploy`, and Terraform/Bicep
// handle idempotent convergence to desired state.
//
// For explicit teardown, user runs `rad app delete` which calls
// terraform destroy / az deployment delete in reverse dependency order.
```

**What Radius tracks vs. what Terraform/Bicep tracks**:

| Radius tracks | Terraform/Bicep tracks |
|---------------|------------------------|
| "redis" Application Resource → success | AWS ElastiCache cluster in state |
| "container" Application Resource → success | K8s Deployment + Service |
| "frontend" Application Resource → failed | Partial ARM deployment |

---

### 8. Exit Code Propagation

**Question**: How should semantic exit codes (0-5) be propagated through Cobra?

**Research Findings**:

Current code in `cmd/rad/main.go`:
```go
func main() {
    err := cmd.Execute()
    if err != nil {
        os.Exit(1)
    }
}
```

**Decision**: Extend with **typed errors** that carry exit codes

**Rationale**:
- Create error types that embed exit codes
- `clierrors` package can be extended with exit code support
- Main function extracts exit code from error

**Implementation Notes**:
```go
// pkg/cli/clierrors/exitcode.go
type ExitCodeError struct {
    Code    int
    Message string
    Err     error
}

const (
    ExitSuccess           = 0
    ExitGeneralError      = 1
    ExitValidationError   = 2
    ExitAuthError         = 3
    ExitResourceConflict  = 4
    ExitDeploymentFailure = 5
)

// cmd/rad/main.go
func main() {
    err := cmd.Execute()
    if err != nil {
        if exitErr, ok := err.(*clierrors.ExitCodeError); ok {
            os.Exit(exitErr.Code)
        }
        os.Exit(1)
    }
}
```

---

### 9. Resource Types Sparse Checkout

**Question**: How should Resource Types be fetched from resource-types-contrib during `rad init`?

**Research Findings**:

Git sparse-checkout allows fetching only specific directories:
```bash
git clone --filter=blob:none --sparse <repo>
git sparse-checkout set <directory>
```

**Decision**: Use **Git CLI sparse-checkout** commands

**Rationale**:
- Minimizes download size (only fetch types directory)
- Works with both HTTPS and SSH authentication
- User's existing Git credentials are used
- Graceful failure with clear error messages

**Implementation Notes**:
```bash
# Implementation pseudocode
cd .radius/config/types
git init
git remote add origin https://github.com/radius-project/resource-types-contrib.git
git config core.sparseCheckout true
echo "types/" >> .git/info/sparse-checkout
git pull origin main --depth=1
rm -rf .git  # Remove .git, keep only types files
```

---

## Dependencies Summary

### New Dependencies Required

| Dependency | Purpose | Version |
|------------|---------|---------|
| `hashicorp/terraform-exec` | Terraform CLI execution | latest |
| `joho/godotenv` | .env file parsing | v1.5.x |

### Existing Dependencies Used

| Dependency | Purpose |
|------------|---------|
| `spf13/cobra` | CLI framework |
| `spf13/viper` | Config file management (config.yaml) |
| `charmbracelet/bubbletea` | Interactive prompts |
| `os/exec` | Git CLI execution |

---

## Open Questions Resolved

| Question | Resolution |
|----------|------------|
| Git library (go-git vs CLI)? | Git CLI via os/exec |
| Terraform execution? | hashicorp/terraform-exec |
| .env parsing? | joho/godotenv |
| Workspace type for Git mode? | New GitConnection type |
| CI vs interactive progress? | Environment detection + conditional rendering |
| Rollback implementation? | No auto-rollback; rely on idempotent re-runs |
| Exit code propagation? | Extended clierrors with ExitCodeError type |
| Resource Types fetching? | Git sparse-checkout via CLI |
| Per-Resource-Type Bicep Extensions? | Single `radius` extension with built-in types; custom types validated at `rad plan` only (no IntelliSense). See FE-011 for future enhancement. |

---

## Open Questions Requiring Further Design

### ⚠️ Control Plane Components Reuse in Git Workspace Mode

**Status**: Needs architectural decision

**Context**: Radius has several control plane components that handle different aspects of deployment and resource management. A key architectural question is which of these can/should be reused in Git workspace mode vs. re-implemented for local execution.

**Existing Control Plane Components**:

| Component | Location | Purpose | Git Workspace Relevance |
|-----------|----------|---------|----------------------|
| **Recipe Engine** | `pkg/recipes/engine/` | Orchestrates recipe execution, handles prevState for cleanup | ⭐ HIGH - Core to deployment |
| **Terraform Driver** | `pkg/recipes/driver/terraform/` | Executes Terraform recipes via terraform-exec | ⭐ HIGH - Already uses terraform-exec! |
| **Bicep Driver** | `pkg/recipes/driver/bicep/` | Executes Bicep recipes | ⭐ HIGH - Core to deployment |
| **Deployment Engine** | `cmd/deployment-engine/` | Long-running deployment orchestration | ❓ UNKNOWN - May have server dependencies |
| **Applications RP** | `pkg/corerp/` | Resource provider for Applications.Core | ⚠️ MEDIUM - API handlers, processors |
| **Dynamic RP** | `pkg/dynamicrp/` | Handles UDT (user-defined types) | ⚠️ MEDIUM - If UDT support needed |
| **UCP** | `pkg/ucp/` | Universal Control Plane API layer | ❓ LOW - Server-side API routing |
| **Controller** | `pkg/controller/` | Kubernetes reconciliation | ❌ LOW - K8s-specific |
| **Resource Renderers** | `pkg/corerp/renderers/` | Converts resources to deployable artifacts | ⭐ HIGH - Core to plan generation |
| **Resource Processors** | `pkg/corerp/processors/` | Handles resource lifecycle | ⚠️ MEDIUM - May have dependencies |

**Key Findings from Code Review**:

1. **Terraform Driver already uses terraform-exec** (`pkg/recipes/driver/terraform/terraform.go`)
   - Uses `hashicorp/terraform-json` for plan parsing
   - Has `TerraformExecutor` interface that could potentially be reused
   - Creates execution directories, handles secrets, manages state

2. **Recipe Engine interface** (`pkg/recipes/engine/types.go`)
   ```go
   type Engine interface {
       Execute(ctx context.Context, opts ExecuteOptions) (*recipes.RecipeOutput, error)
       Delete(ctx context.Context, opts DeleteOptions) error
       GetRecipeMetadata(ctx context.Context, opts GetRecipeMetadataOptions) (map[string]any, error)
   }
   ```
   - Clean interface that could be reused or adapted for local execution

3. **Driver interface** (`pkg/recipes/driver/types.go`)
   - Abstracts Terraform/Bicep execution
   - Handles secrets loading via `DriverWithSecrets` interface

**Architectural Options**:

| Option | Description | Pros | Cons |
|--------|-------------|------|------|
| **A. Reuse Existing Drivers** | Use `pkg/recipes/driver/terraform` and `bicep` directly | Code reuse, tested | May have UCP/K8s dependencies |
| **B. Extract Core Library** | Create `pkg/recipes/local/` that shares common logic | Best of both worlds | Refactoring effort |
| **C. New Implementation** | Fresh implementation using terraform-exec directly | No legacy constraints | Duplicate logic, divergence risk |

**Questions to Answer**:

1. What dependencies do the existing drivers have?
   - UCP connection (`sdk.Connection`)
   - Secret provider (`secretprovider.SecretProvider`)
   - Kubernetes client (`kubernetesclientprovider.KubernetesClientProvider`)
   
2. Can these dependencies be satisfied locally?
   - UCP connection: Likely needs abstraction for local mode
   - Secrets: Could use local file/env var based secret provider
   - K8s client: Only needed if deploying to K8s

3. What interface boundaries exist?
   - `recipes.Engine` - high level orchestration
   - `driver.Driver` - Terraform/Bicep execution
   - `terraform.TerraformExecutor` - low-level TF execution

**Recommendation**: 
1. Investigate if `pkg/recipes/driver/terraform` can run with mock/local implementations of its dependencies
2. Consider option B (extract core library) if dependencies are too heavy
3. Ensure interface compatibility so that future control plane mode can delegate to same underlying logic

**Code Reuse Strategy** (Required for Implementation):

The implementation MUST follow a code reuse strategy that enables shared logic between Git workspace mode (local/git mode) and Control Plane Radius (server mode). This is critical for:
- Avoiding behavior divergence between modes
- Reducing maintenance burden
- Ensuring recipes work consistently

**Proposed Architecture**:
```
┌─────────────────────────────────────────────────────────────────┐
│                        pkg/recipes/                             │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │                    Core Interfaces                        │  │
│  │  • Engine (orchestration)                                 │  │
│  │  • Driver (Terraform/Bicep execution)                     │  │
│  │  • RecipeResolver (template lookup)                       │  │
│  └──────────────────────────────────────────────────────────┘  │
│                              │                                  │
│              ┌───────────────┼───────────────┐                  │
│              ▼               ▼               ▼                  │
│  ┌────────────────┐ ┌────────────────┐ ┌────────────────┐      │
│  │ Terraform      │ │ Bicep          │ │ (Future        │      │
│  │ Driver         │ │ Driver         │ │  Drivers)      │      │
│  └────────────────┘ └────────────────┘ └────────────────┘      │
└─────────────────────────────────────────────────────────────────┘
                              │
         ┌────────────────────┴────────────────────┐
         ▼                                         ▼
┌─────────────────────┐                 ┌─────────────────────┐
│   Git Workspace       │                 │  Control Plane      │
│   (pkg/cli/...)     │                 │  (pkg/corerp/...)   │
│                     │                 │                     │
│ • Local execution   │                 │ • Server execution  │
│ • File-based config │                 │ • UCP/K8s config    │
│ • Local secrets     │                 │ • Secret stores     │
│ • CLI output        │                 │ • Async processing  │
└─────────────────────┘                 └─────────────────────┘
```

**Implementation Principles**:
1. **Shared interfaces** - Both modes use same `Engine` and `Driver` interfaces
2. **Dependency injection** - Drivers accept interfaces for secrets, config, output
3. **Mode-specific adapters** - Each mode provides its own implementations of dependencies
4. **No duplication of recipe logic** - Recipe parsing, execution, output handling in one place

---

### ⚠️ Deployment Atomicity and Resource Lifecycle

**Status**: Needs more thought before implementation

**Context**: Neither Terraform nor Bicep provide true atomic deployment semantics. When failures occur, users need clear information about what succeeded, what failed, and what state the system is in.

**Concerns**:

1. **Failure Reporting UX**
   - When `terraform apply` fails at resource 5 of 10, resources 1-4 remain deployed
   - Radius needs to provide clear, actionable information to users:
     - Which Application Resources succeeded?
     - Which failed and why?
     - What is the current state of Cloud Resources?
     - What should the user do next?
   - Consider: Should Radius query Terraform state / ARM to show current deployment status?

2. **Bicep Incremental vs Complete Mode**
   - **Incremental mode** (default): Only adds/updates resources, doesn't delete missing ones
   - **Complete mode**: Deletes resources not in template (dangerous, can delete unrelated resources)
   - Current decision: Use incremental mode for safety
   - Open question: How does Radius handle resources that were in the model but are now removed?

3. **Detecting Removed Application Resources**
   - Scenario: User removes a `redis` resource from their Bicep model
   - With incremental mode: The old Redis Cloud Resources remain orphaned
   - Ideal UX: Radius detects the removal and offers to delete the orphaned resources
   - Danger: Accidental deletions if user temporarily comments out a resource
   - **Needs design**: 
     - How do we diff "what was deployed before" vs "what's in the model now"?
     - Should `rad plan` show resources that will be orphaned?
     - Should `rad deploy` prompt before deleting orphaned resources?
     - Should there be a separate `rad app cleanup` or `rad app prune` command?
     - What confirmation UX prevents accidental deletions?

4. **State Tracking**
   - Terraform maintains `.tfstate` files
   - Bicep/ARM has deployment history
   - Should Radius maintain its own state of "what we deployed" separate from tool state?
   - Trade-off: Additional complexity vs. better orphan detection

**Recommendation**: For MVP, implement conservative behavior:
- Use incremental mode (no automatic deletions)
- Surface clear error information on failures
- Document that orphaned resources must be manually cleaned up
- Add proper orphan detection and deletion UX in a subsequent iteration
