# CLI Contracts: Git Workspace Commands

**Feature**: 001-repo-radius | **Date**: 2026-02-02

This document defines the CLI command contracts for Git workspace mode.

## Command Summary

| Command | Description | Workspace |
|---------|-------------|-----------|
| `rad init` | Initialize Git workspace | Git |
| `rad plan` | Generate deployment artifacts | Git |
| `rad deploy` | Deploy infrastructure | Both (behavior varies) |
| `rad diff` | Detect drift from deployment | Git |
| `rad delete` | Delete deployed resources | Git |
| `rad workspace` | Manage workspaces | Both |

## Exit Codes

All commands use semantic exit codes as defined in FR-005:

| Code | Name | Description |
|------|------|-------------|
| 0 | Success | Command completed successfully |
| 1 | GeneralError | Unexpected error |
| 2 | ValidationError | Configuration or input problems |
| 3 | AuthError | Authentication/authorization failure |
| 4 | ResourceConflict | State conflict (e.g., lock acquisition failed) |
| 5 | DeploymentFailure | Deployment failed (resources may need manual cleanup) |

---

## Command: `rad init`

Initialize a Git repository as a Radius workspace.

> **Breaking Change**: This command **completely replaces** the existing `rad init` functionality. The previous `rad init` installed a Radius control plane on Kubernetes. This new implementation initializes a Git repository for Git workspace mode. To install a Radius control plane, use `rad install kubernetes` instead.

### Usage

```
rad init [flags]
```

### Flags

| Flag | Type | Required | Default | Description |
|------|------|----------|---------|-------------|
| `--help` | bool | No | false | Show help |

### Behavior

1. Verify current directory is a Git repository (check for `.git/` or run `git rev-parse --git-dir`)
2. If `.radius/` already exists, prompt for confirmation before overwriting
3. Create directory structure: `.radius/config/`, `.radius/config/types/`, `.radius/model/`, `.radius/plan/`, `.radius/deploy/`
4. Populate `.radius/config/types/` with Resource Types from `radius-project/resource-types-contrib` via git sparse-checkout
5. Detect existing `.env` files and validate or prompt for cloud platform configuration
6. Prompt for container deployment target (Kubernetes or Azure Container Instances)
7. Prompt for other resources deployment target (Kubernetes, AWS, or Azure)
8. Detect available deployment tools (Terraform CLI, Bicep CLI) and select or prompt
9. Create default `recipes.yaml` in `.radius/config/`
10. Set workspace to `git` in `~/.rad/config.yaml`
11. Display success message with next steps

### Exit Codes

| Code | Condition |
|------|-----------|
| 0 | Initialization successful |
| 2 | Not a Git repository |
| 2 | Failed to fetch Resource Types from repository |
| 2 | No deployment tool found |

### Output Examples

**Success**:
```
✅ Git workspace initialized successfully

📋 Summary:
   • Resource Types populated from radius-project/resource-types-contrib:
     - Radius.Compute/containers
     - Radius.Compute/persistentVolumes
     - Radius.Security/secrets
     - ... (N total)
   • Environment configured: AWS + Kubernetes
   • Deployment tool: terraform
   • Recipes manifest: .radius/config/recipes.yaml

🚀 Next steps:
   1. Commit the initialized configuration:
      git add .radius/
      git commit -m "Initialize Radius Git workspace"
   2. Create your application model in .radius/model/
   3. Generate a deployment plan:
      rad plan .radius/model/myapp.bicep -e production

💡 Run 'rad --help' for more commands and options
```

**Error - Not a Git repository**:
```
❌ Current directory is not a Git repository.

Please run 'git init' first, then retry 'rad init'.
```

**Error - Failed to fetch Resource Types**:
```
❌ Failed to fetch Resource Types from repository.

Please check network connectivity and Git authentication, then retry 'rad init'.

Troubleshooting:
  • Verify you can access https://github.com/radius-project/resource-types-contrib
  • Check proxy settings if behind a corporate firewall
  • Ensure Git credentials are configured
```

**Warning - Already initialized**:
```
⚠️  Git workspace is already initialized.

Re-running init may overwrite existing configuration. Continue? (y/N)
```

---

## Command: `rad plan`

Generate deployment artifacts from an application model Bicep file.

#### Usage

```
rad plan <app.bicep> [--environment <name>] [flags]
```

#### Arguments

| Argument | Type | Required | Default | Description |
|----------|------|----------|---------|-------------|
| `<app.bicep>` | string | **Yes** | | Path to the application model Bicep file (e.g., `.radius/model/frontend.bicep`) |

#### Flags

| Flag | Type | Required | Default | Description |
|------|------|----------|---------|-------------|
| `--environment, -e` | string | Conditional | | Environment name (required if multiple environments exist) |
| `--allow-unpinned-recipes` | bool | No | false | Allow unpinned recipe versions |
| `--quiet, -q` | bool | No | false | Suppress progress output |
| `--help` | bool | No | false | Show help |

#### Auto-Selection

- **Environment**: If only `.env` exists (no `.env.<name>` files), the default environment is auto-selected
- If multiple environment files exist (`.env`, `.env.staging`, `.env.production`), `--environment` is required

#### Behavior

1. Validate Git workspace is initialized (`.radius/` exists)
2. Parse the Bicep file to extract the `Application` resource name
3. Load environment configuration from `.env` or `.env.<environment>`
4. Validate recipe files exist
5. Validate recipe versions are pinned (unless `--allow-unpinned-recipes`)
6. Generate deployment artifacts for each resource:
   - For Terraform: `main.tf`, `terraform.tfvars`, run `terraform init && terraform plan`, save output to `terraform-plan.txt`
   - For Bicep: `.bicep`, `.bicepparam`, run `bicep build`, run `az deployment group what-if` or equivalent, save output to `bicep-whatif.txt`
7. Save artifacts to `.radius/plan/<application>/<environment>/`
8. Create `plan.yaml` with metadata
9. Display summary and next steps

#### Exit Codes

| Code | Condition |
|------|-----------|
| 0 | Plan generated successfully |
| 2 | Git workspace not initialized |
| 2 | Multiple environments exist and `--environment` not specified |
| 2 | Bicep file not found or invalid |
| 2 | No Application resource found in Bicep file |
| 2 | Recipe file not found |
| 2 | Unpinned recipe version (without flag) |

#### Output Examples

**Error - Multiple environments exist**:
```
❌ Multiple environments found. Please specify --environment:

Available environments:
   • default     (.env)
   • staging     (.env.staging)
   • production  (.env.production)

Usage: rad plan .radius/model/frontend.bicep --environment <name>
```

**Success**:
```
✅ Plan generated successfully

📋 Summary:
   • Application: frontend
   • Environment: production
   • Deployment steps: 3
   • Artifacts saved to: .radius/plan/frontend/production/

🚀 Next steps:
   1. Review the generated plan:
      ls -la .radius/plan/frontend/production/

   2. Commit the plan to Git:
      git add .radius/plan/frontend/production/
      git commit -m "rad plan: frontend for production"

   3. Deploy from the commit:
      rad deploy --application frontend --environment production
```

**Error - Unpinned recipe**:
```
❌ Recipe 'Radius.Compute/containers' uses an unpinned Terraform module

The recipe location does not specify a Git commit hash or tag:
  git::https://github.com/radius-project/resource-types-contrib.git//containers

Use a pinned version for reproducible deployments:
  git::https://github.com/radius-project/resource-types-contrib.git//containers?ref=v1.0.0

Or run with --allow-unpinned-recipes to override (not recommended for production).
```

**Error - No Application resource**:
```
❌ No Application resource found in .radius/model/frontend.bicep

The Bicep file must contain an Application resource to identify the application name.

Example:
   resource frontend 'Applications.Core/applications@2023-10-01-preview' = {
     name: 'frontend'
     ...
   }
```


## Command: `rad diff`

Compare infrastructure state across the lifecycle—between commits and optionally against live cloud resources.

### Usage

```
rad diff [<commit>[...<commit>]] --application <name> --environment <name> [flags]
```

Inspired by `git diff`, the behavior depends on the arguments:

| Usage | Description |
|-------|-------------|
| `rad diff` | Show uncommitted changes to `.radius/` directories |
| `rad diff <commit>` | Compare `<commit>` to current working tree |
| `rad diff <commit>...<commit>` | Compare two commits |
| `rad diff <commit>...live` | Compare commit to current live cloud state |

### Arguments

| Argument | Type | Required | Description |
|----------|------|----------|-------------|
| `<commit>` | string | No | Git commit hash, tag, branch, or `HEAD~N` |
| `<commit>...<commit>` | string | No | Two commits separated by `...` (git-style range) |
| `<commit>...live` | string | No | Compare commit to live cloud state |

### Commit State Types

Each commit may contain different artifacts depending on where it falls in the lifecycle:

| Commit State | Contains | Created By |
|--------------|----------|------------|
| **Model commit** | `.radius/model/` only | Editing model files followed by `git commit` |
| **Plan commit** | `.radius/model/` + `.radius/plan/` | `rad plan` followed by `git commit` |
| **Deployment Record commit** | `.radius/model/` + `.radius/plan/` + `.radius/deploy/` | `rad deploy` followed by `git commit` |

### Flags

| Flag | Type | Required | Default | Description |
|------|------|----------|---------|-------------|
| `--application, -a` | string | Conditional | | Application name (required if multiple applications exist) |
| `--environment, -e` | string | Conditional | | Environment name (required if multiple environments exist, unless `--all-environments`) |
| `--all-environments` | bool | No | false | Check all deployed environments |
| `--plan-only` | bool | No | false | Only compare plan artifacts (ignore deployment records) |
| `--output, -o` | string | No | text | Output format: `text` or `json` |
| `--quiet, -q` | bool | No | false | Suppress progress output, only show summary |
| `--help` | bool | No | false | Show help |

### Auto-Selection

- **Application**: If only one application exists in `.radius/plan/`, it is auto-selected
- **Environment**: If only `.env` exists (no `.env.<name>` files), the default environment is auto-selected
- If multiple applications or environments exist, the respective flag is required

### Comparison Types

| Source | Target | Comparison Type |
|--------|--------|-----------------|
| Model only | Model only | Shows structural changes to the application model |
| Model only | Plan | Shows how the model gets translated to a deployment plan |
| Plan only | Plan only | Shows changes to generated IaC files (Terraform, Bicep) |
| Plan only | Deployment Record | Shows differences between what was planned and what was deployed |
| Deployment Record | Plan only | Shows what a new plan would change from the last deployment |
| Deployment Record | Deployment Record | Shows changes between two deployment states |
| Deployment Record | Live | Shows drift between deployed state and current cloud resources |

### Examples

```bash
# Show uncommitted changes to .radius/ directories
rad diff -a myapp -e production

# Compare HEAD to a previous commit
rad diff HEAD~1 -a myapp -e production

# Compare two commits (git-style range syntax)
rad diff abc123...def456 -a myapp -e production

# Compare a deployment commit against live cloud (drift detection)
rad diff abc123...live -a myapp -e production

# Compare current HEAD against live cloud
rad diff HEAD...live -a myapp -e production
```

### Behavior

**When no commits specified (`rad diff`):**

1. Validate Git workspace is initialized
2. Show uncommitted changes to `.radius/model/`, `.radius/plan/`, and `.radius/deploy/` directories
3. Similar to `git diff` showing unstaged changes

**When one commit specified (`rad diff <commit>`):**

1. Validate Git workspace is initialized
2. Compare specified commit to current working tree
3. Similar to `git diff <commit>`

**When two commits specified (`rad diff <commit1>...<commit2>`):**

1. Validate Git workspace is initialized
2. Checkout artifacts from both commits
3. Determine comparison type based on what artifacts exist:
   - Both have plan only: compare IaC files (terraform configs, bicep templates)
   - Both have plan + deployment record: compare deployment records
   - One plan, one deploy: compare planned resources vs deployed resources
4. Display diff in unified format

**When using `live` keyword (`rad diff <commit>...live`):**

1. Validate Git workspace is initialized
2. Load artifacts from specified commit (or find latest deploy commit)
3. Query current cloud resource state:
   - Azure: Azure Resource Manager API
   - AWS: Terraform state or AWS APIs
   - Kubernetes: Kubernetes API
4. Compare commit state with live cloud
5. Exit with appropriate code

### Exit Codes

| Code | Condition |
|------|-----------|
| 0 | No differences found |
| 1 | Differences detected |
| 2 | Git workspace not initialized |
| 2 | Specified commit not found |
| 2 | No plan or deployment found at specified commit |
| 3 | Missing authentication environment variables |

### Output Examples

**Plan vs Plan (two commits):**
```
📊 Comparing commits: abc1234 → def5678

Application: frontend
Environment: production

.radius/plan/frontend/production/001-network-terraform/main.tf
  @@ -15,7 +15,7 @@
  -  instance_type = "t3.small"
  +  instance_type = "t3.medium"

.radius/plan/frontend/production/plan.yaml
  @@ -8,6 +8,9 @@
  +- name: 004-cache
  +  tool: terraform
  +  dependsOn: [001-network]

Summary: 2 files changed, 4 insertions, 1 deletion
```

**Deploy commit vs live cloud (`rad diff abc1234...live`):**
```
⚠️  Drift detected in 2 resources

Comparing: commit abc1234 (deploy) → live cloud

Application: frontend
Environment: production
Deployed at: 2026-02-01 14:30:00

Resource: myappsa (Microsoft.Storage/storageAccounts)
   - accessTier: Hot → Cool (modified)
   - tags.environment: production → (removed)

Resource: my-app-bucket (AWS::S3::Bucket)
   + versioning.mfaDelete: Enabled (added)

Resources with no drift: 7

To reconcile:
   rad plan .radius/model/frontend.bicep -e production
   git add .radius/plan/ && git commit -m "Reconcile drift"
   rad deploy -a frontend -e production
```

**Plan commit vs live cloud (`rad diff abc1234...live` where abc1234 is a plan-only commit):**
```
📋 Deployment preview: commit abc1234 (plan) → live cloud

Application: frontend
Environment: production

The following changes would occur if this plan is deployed:

  + Create: aws_instance.web_server
      instance_type: t3.medium
      ami: ami-12345678
      
  ~ Modify: azurerm_storage_account.myappsa
      - access_tier: Cool → Hot
      
  - Delete: kubernetes_deployment.legacy_app
      (resource exists in cloud but not in plan)

Summary: 1 to create, 1 to modify, 1 to delete
```

**No differences:**
```
✅ No differences detected

Comparing: commit abc1234 (deploy) → live cloud

Application: frontend
Environment: production
Resources checked: 9

Cloud resources match deployment record.
```

**Warning - Uncommitted deployment records:**
```
⚠️  Warning: Uncommitted deployment records found

The following deployment records are not committed:
   .radius/deploy/frontend/production/deployment-def5678.json

Commit these for full diff history:
   git add .radius/deploy/
   git commit -m "Deployment record: frontend production"

Proceeding with diff using committed records only...
```

**Error - No deployment found:**
```
❌ No deployment record found for frontend/production

To compare against live cloud, first deploy:
   rad deploy -a frontend -e production

To compare two plan commits:
   rad diff <commit1>...<commit2> -a frontend -e production --plan-only
```

**JSON Output** (`--output json`):
```json
{
  "application": "frontend",
  "environment": "production",
  "deploymentCommit": "abc1234",
  "driftDetected": true,
  "resources": [
    {
      "id": "/subscriptions/.../myappsa",
      "type": "Microsoft.Storage/storageAccounts",
      "name": "myappsa",
      "drift": [
        {"property": "accessTier", "expected": "Hot", "actual": "Cool", "change": "modified"},
        {"property": "tags.environment", "expected": "production", "actual": null, "change": "removed"}
      ]
    }
  ],
  "summary": {
    "totalResources": 9,
    "resourcesWithDrift": 2,
    "resourcesWithoutDrift": 7
  }
}
```

**Error - No deployment found**:
```
❌ No deployment record found for frontend/production

Run 'rad deploy' first to create a deployment record.

Available deployments in .radius/deploy/:
   • frontend/staging
   • backend/production
```

---

## Command: `rad delete`

Delete deployed resources for an application environment.

#### Usage

```
rad delete --application <name> [--environment <name>] [flags]
```

#### Flags

| Flag | Type | Required | Default | Description |
|------|------|----------|---------|-------------|
| `--application, -a` | string | **Yes** | | Application name |
| `--environment, -e` | string | Conditional | | Environment name (required if multiple environments exist) |
| `--yes, -y` | bool | No | false | Skip confirmation prompt |
| `--quiet, -q` | bool | No | false | Suppress progress output |
| `--help` | bool | No | false | Show help |

#### Environment Auto-Selection

- If only `.env` exists (no `.env.<name>` files), the default environment is auto-selected
- If multiple environment files exist (`.env`, `.env.staging`, `.env.production`), `--environment` is required

#### Behavior

1. Validate Git workspace is initialized (`.radius/` exists)
2. Load the most recent deployment record from `.radius/deploy/<application>/<environment>/`
3. Display confirmation prompt with list of resources to be deleted (unless `-y`)
4. Execute deletion in reverse dependency order:
   - For Terraform: invoke `terraform destroy` programmatically via `terraform-exec`
   - For Bicep (Azure): invoke Azure CLI to delete resource group or individual resources
   - For Bicep (Kubernetes): invoke Kubernetes API to delete resources
5. Write deletion record to `.radius/deploy/<application>/<environment>/`
6. Display summary

#### Exit Codes

| Code | Condition |
|------|-----------|
| 0 | Deletion successful |
| 2 | Git workspace not initialized |
| 2 | No deployment record found for application/environment |
| 2 | Multiple environments exist and `--environment` not specified |
| 3 | Missing authentication environment variables |
| 5 | Deletion failed (some resources may remain) |

#### Output Examples

**Confirmation Prompt**:
```
⚠️  You are about to delete all resources for:
   Application: frontend
   Environment: production

Resources to be deleted:
   • 003-app-terraform        (4 resources)
   • 002-database-bicep       (2 resources)
   • 001-network-terraform    (3 resources)

This action cannot be undone. Continue? [y/N]
```

**Success**:
```
✅ Resources deleted successfully

📋 Summary:
   Application: frontend
   Environment: production
   Duration:    1m 15s
   
   Resources deleted:
     ✓ 003-app-terraform        (4 resources)
     ✓ 002-database-bicep       (2 resources)
     ✓ 001-network-terraform    (3 resources)

📁 Deletion record saved to:
   .radius/deploy/frontend/production/deletion-abc1234.json
```

**Error - No deployment found**:
```
❌ No deployment record found for frontend/production

Available deployments in .radius/deploy/:
   • frontend/staging
   • backend/production

Usage: rad delete --application <name> --environment <name>
```

**Error - Multiple environments exist**:
```
❌ Multiple environments found. Please specify --environment:

Available environments:
   • default (.env)
   • staging (.env.staging)
   • production (.env.production)

Usage: rad delete --application frontend --environment <name>
```

---

## Command: `rad deploy`

Deploy infrastructure. Behavior depends on the current workspace type and arguments provided.

### Usage

```
rad deploy [<source>] [flags]
```

Where `<source>` specifies what to deploy from:
- **Nothing** (Git workspace): Deploy from current HEAD's committed plan
- **Commit hash or tag** (Git workspace): Deploy from plan at that specific commit
- **Bicep file path** (Control Plane workspace): Deploy Bicep template via Control Plane

### Flags

| Flag | Type | Required | Default | Description |
|------|------|----------|---------|-------------|
| `--application, -a` | string | Conditional | "" | Application name (required if multiple applications exist) |
| `--environment, -e` | string | Conditional | "" | Environment name (required if multiple environments exist) |
| `--commit` | bool | No | false | Auto-commit deployment record after successful deployment |
| `--message, -m` | string | No | "" | Custom commit message (requires `--commit`) |
| `--yes, -y` | bool | No | false | Skip confirmation prompt |
| `--quiet, -q` | bool | No | false | Suppress progress output |
| `--group, -g` | string | Conditional | "" | Resource group (Control Plane only) |
| `--help` | bool | No | false | Show help |

### Auto-Selection (Git Workspace)

- **Application**: If only one application exists in `.radius/plan/`, it is auto-selected
- **Environment**: If only `.env` exists (no `.env.<name>` files), the default environment is auto-selected
- If multiple applications or environments exist, the respective flag is required

### Behavior by Workspace Type

#### Git Workspace - Deploy from Current Plan

```bash
rad deploy --application frontend --environment production
```

1. Validate Git workspace is initialized (`.radius/` exists)
2. Check for uncommitted changes in `.radius/plan/`
   - If uncommitted changes exist → **error** (must commit plan first)
3. Detect available application-environment plans in `.radius/plan/<app>/<env>/`
4. If multiple plans exist and `--application` or `--environment` not specified → error with list
5. If single plan exists → auto-select that application and environment
6. Validate required environment variables for cloud platform
7. Display confirmation prompt (unless `-y` or CI environment)
8. Execute deployment steps from the committed plan
9. Capture resource details and write deployment record to `.radius/deploy/`
10. **Auto-stage** deployment record (`git add .radius/deploy/`)
11. If `--commit` flag provided:
    - Commit with default message: `"Deploy <app> to <env> (<commit>)"`
    - Or use custom message from `--message` flag
12. Display summary with next steps

#### Git Workspace - Deploy from Specific Commit

```bash
rad deploy abc1234 --application frontend --environment production
rad deploy v1.2.0 --application frontend --environment production
```

1. Validate Git workspace is initialized
2. **Skip uncommitted changes check** (deploying from specific commit)
3. Checkout plan artifacts from specified commit/tag
4. Continue with steps 4-12 above

> **Note**: In GitHub Actions, if `GITHUB_SHA` is set and no target is provided, the system automatically uses `GITHUB_SHA` as the commit reference.

#### Git Workspace - Bicep File Provided (Error)

```bash
rad deploy myapp.bicep  # ❌ Error
```

If a `.bicep` file is provided in a Git workspace:
```
❌ Bicep files cannot be deployed directly from a Git workspace.

To deploy a Bicep file:
  1. Switch to a Control Plane workspace:
     rad workspace switch <control-plane-name>
  
  2. Then deploy:
     rad deploy myapp.bicep

To deploy from a Git workspace, use rad plan first:
  rad plan myapp.bicep --environment <env>
  git add .radius/plan/ && git commit -m "Plan"
  rad deploy --application <app> --environment <env>
```

#### Control Plane Workspace - Deploy Bicep File

```bash
rad deploy myapp.bicep --group my-resource-group --environment production
```

Deploys the Bicep template via the Radius Control Plane (existing `rad deploy` behavior, unchanged).

### Exit Codes

| Code | Condition |
|------|-----------|
| 0 | Deployment successful |
| 2 | Git workspace not initialized |
| 2 | Uncommitted changes in `.radius/plan/` (when deploying current plan) |
| 2 | Bicep file provided in Git workspace |
| 2 | Multiple plans found, `--application` or `--environment` not specified |
| 2 | Application/environment not found in `.radius/plan/` |
| 3 | Missing authentication environment variables |
| 4 | Resource conflict / state lock acquisition failed |
| 5 | Deployment failed |

### Output Examples

**Error - Uncommitted changes**:
```
❌ Cannot deploy with uncommitted changes in .radius/plan/

Modified files:
  .radius/plan/frontend/production/001-network-terraform/main.tf
  .radius/plan/frontend/production/plan.yaml

To deploy the current plan:
  git add .radius/plan/
  git commit -m "Update plan"
  rad deploy --application frontend --environment production

To deploy from a specific commit (ignoring local changes):
  rad deploy <commit-hash> --application frontend --environment production
```

**Error - Bicep file in Git workspace**:
```
❌ Bicep files cannot be deployed directly from a Git workspace.

To deploy a Bicep file:
  1. Switch to a Control Plane workspace:
     rad workspace switch <control-plane-name>
  
  2. Then deploy:
     rad deploy myapp.bicep

To deploy from a Git workspace, use rad plan first:
  rad plan myapp.bicep --environment <env>
  git add .radius/plan/ && git commit -m "Plan"
  rad deploy --application <app> --environment <env>
```

**Error - Multiple plans found**:
```
❌ Multiple plans found. Please specify which application and environment to deploy.

Available plans in .radius/plan/:
   • frontend/production
   • frontend/staging
   • backend/production

Usage: rad deploy --application <name> --environment <name>
```

**Success (Git workspace - default, no --commit)**:
```
✅ Deployment completed successfully

📋 Summary:
   • Application: frontend
   • Environment: production
   • Commit: abc1234
   • Duration: 2m 34s
   • Resources deployed: 9

📁 Deployment record staged:
   .radius/deploy/frontend/production/deployment-abc1234.json

💡 To complete the deployment record:
   git commit -m "Deploy frontend to production (abc1234)"
```

**Success (Git workspace - with --commit)**:
```
✅ Deployment completed successfully

📋 Summary:
   • Application: frontend
   • Environment: production
   • Commit: abc1234
   • Duration: 2m 34s
   • Resources deployed: 9

📁 Deployment record committed:
   .radius/deploy/frontend/production/deployment-abc1234.json
   
   Commit: def5678
   Message: "Deploy frontend to production (abc1234)"
```

**Success (Git workspace - with --commit and custom message)**:
```bash
rad deploy -a frontend -e production --commit -m "Release v1.2.3 to production"
```
```
✅ Deployment completed successfully

📋 Summary:
   • Application: frontend
   • Environment: production
   • Commit: abc1234
   • Duration: 2m 34s
   • Resources deployed: 9

📁 Deployment record committed:
   .radius/deploy/frontend/production/deployment-abc1234.json
   
   Commit: def5678
   Message: "Release v1.2.3 to production"
```

---

## Command: `rad workspace`

Manage workspaces (switching between Git workspace and Control Plane workspace).

### Subcommands

#### `rad workspace list`

List all configured workspaces.

##### Usage

```
rad workspace list [flags]
```

##### Flags

| Flag | Type | Required | Default | Description |
|------|------|----------|---------|-------------|
| `--help` | bool | No | false | Show help |

##### Output

```
WORKSPACE                  TYPE        STATUS
git                        built-in    active
my-radius-control-plane    kubernetes  
```

---

#### `rad workspace switch`

Switch to a different workspace.

##### Usage

```
rad workspace switch <name> [flags]
```

##### Arguments

| Argument | Type | Required | Default | Description |
|----------|------|----------|---------|-------------|
| `<name>` | string | **Yes** | | Name of the workspace to switch to (`git` for Git workspace, or a Control Plane workspace name) |

##### Flags

| Flag | Type | Required | Default | Description |
|------|------|----------|---------|-------------|
| `--help` | bool | No | false | Show help |

##### Output Examples

**Switch to Git workspace**:
```
rad workspace switch git
```
```
Switched to workspace 'git' (Git workspace)
```

**Switch to Control Plane workspace**:
```
rad workspace switch my-radius-control-plane
```
```
Switched to workspace 'my-radius-control-plane' (Control Plane)
```

---

#### `rad workspace create`

Create a new Control Plane workspace.

> **Note**: No changes from current `rad workspace create` behavior. See existing documentation for full details.

---

## Error Message Format

All errors follow a structured format (NFR-021):

```
❌ <Brief error summary>

<Detailed explanation>

<Suggested remediation action>
```

Example:
```
❌ Environment 'staging' not found

Available environments:
  • default     (.env)
  • production  (.env.production)

Usage: rad deploy abc1234 --environment <name>
```
