# Feature Specification: Repo Radius

**Feature Branch**: `001-repo-radius`
**Created**: 2026-01-28
**Updated**: 2026-01-29
**Status**: Draft
**Input**: Modify Radius to support two modes of operation: Control Plane Radius (existing centralized mode) and Repo Radius (new decentralized Git-based mode that runs as a CLI tool without a control plane, stores state in Git, and is optimized for CI/CD workflows, particularly GitHub Actions)

## Overview

Repo Radius is a lightweight, Git-centric mode of Radius designed to run without a centralized control plane. It treats a Git repository as the system of record and is optimized for CI/CD workflows, particularly GitHub Actions. 

This specification focuses **exclusively on Repo Radius** (not Control Plane Radius). Repo Radius takes inspiration from the Terraform vs. Terraform Cloud model: while Control Plane Radius provides centralized orchestration and state management, Repo Radius provides a decentralized, repository-driven workflow suitable for teams who prefer infrastructure-as-code stored in Git with GitOps-style deployment patterns.

## Clarifications

### Session 2026-01-29

- Q: How should Resource Types be populated in `.radius/config/types/` during `rad init`? → A: Clone/fetch specific directory via git sparse-checkout from resource-types-contrib repo
- Q: What should happen if `rad init` cannot reach the resource-types-contrib repository or authentication fails during Resource Types population? → A: Fail initialization with exit code 2 and clear error message instructing user to resolve connectivity/auth and retry
- Q: What level of detail should be captured for each deployed resource's properties in deployment records? → A: Full properties as returned by cloud provider (complete resource snapshot)
- Q: What should happen when a deployment fails partway through resource provisioning? → A: Rollback all successfully deployed resources (attempt to delete them) before exiting. Repo Radius must provide atomic and idempotent deployment semantics - either all resources deploy successfully, or none remain.
- Q: How should Repo Radius handle concurrent deployments to the same environment (e.g., two GitHub Actions workflows triggering simultaneously)? → A: Use deployment tool's native locking (Terraform state locking, ARM deployment locks) - no additional Repo Radius locking layer

### Session 2026-01-30

- Q: How should Kubernetes authentication be configured for deployments? Should separate credential environment variables be introduced, or rely on standard kubeconfig credential chain? → A: Use standard kubeconfig credential chain - no separate K8s credential env vars
- Q: How should Terraform backend configuration be managed? Should Repo Radius initialize/manage the backend, or delegate backend configuration to users? → A: Backend config is user's responsibility, but .env files should support a TF_BACKEND_CONFIG variable that points to a partial backend configuration file (used with terraform init -backend-config=<file>)
- Q: What error message format should Repo Radius use for reporting failures? → A: Single static error format with structured fields (error code, message, affected resource, suggested action)
- Q: How long should deployment records in `.radius/deploy/` be retained? Should there be automatic cleanup of old deployments? → A: Keep all deployment records indefinitely (no automatic cleanup)
- Q: When a recipe's version changes between `rad plan` runs (e.g., updating git ref or OCI tag in recipes.yaml), should the system block, allow silently, or allow with a warning? → A: Allow but warn - Recipe versions are already captured in plan output (main.tf references recipeLocation with version, plan-manifest.yaml), so no additional tracking needed. The warning provides visibility and the audit trail exists in plan artifacts.

## Recommendations for Open Questions

### Q1: What features or behaviors would make Repo Radius particularly well-suited for GitHub Actions?

**Recommendation**: The following features optimize Repo Radius for GitHub Actions:

1. **Exit Codes**: Use semantic exit codes that enable GitHub Actions conditionals:
   - `0` = Success
   - `1` = General error
   - `2` = Validation error (configuration/input problems)
   - `3` = Authentication/authorization error
   - `4` = Resource conflict/state error
   - `5` = Deployment failure

2. **Simple CI Integration**: Commands use exit codes for success/failure. Workflow steps proceed naturally based on exit codes—no parsing required:
   ```yaml
   - name: Generate deployment plan
     run: rad plan --environment production
     # Exit code 0 = success, non-zero = failure
   
   - name: Deploy (only runs if plan succeeded)
     run: rad deploy --environment production -y
   ```

3. **GitHub Actions Artifacts Integration**: Output file paths and metadata in structured format so workflows can easily upload artifacts:
   ```yaml
   - name: Deploy with Radius
     run: rad deploy --commit ${{ github.sha }}
   
   - name: Upload deployment artifacts
     uses: actions/upload-artifact@v4
     with:
       name: radius-deployment
       path: .radius/deploy/
   ```

4. **Silent/Quiet Mode**: Support `--quiet` flag that suppresses progress output but preserves structured JSON/YAML output for cleaner workflow logs

5. **Commit SHA Auto-detection**: When running in GitHub Actions, automatically detect `GITHUB_SHA` environment variable for `rad deploy` so users don't need to manually specify `--commit`

6. **Pre-built GitHub Action**: Provide an official GitHub Action wrapper for easy integration:
   ```yaml
   - uses: radius-project/repo-radius-action@v1
     with:
       command: 'deploy'
       environment: 'production'
   ```

### Q2: What file format should be used to store deployed resource details?

**Recommendation**: **JSON** is the optimal format for the following reasons:

1. **Native Format Preservation**: Deployment details are captured from cloud APIs (Azure ARM, AWS, Kubernetes) which return JSON natively. Using JSON preserves the exact format without conversion.
2. **Programmatic Access**: JSON is easily parsed by scripts and tools for automation and analysis.
3. **Widely Supported**: All programming languages and CI/CD tools have excellent JSON parsing libraries.
4. **Consistency with APIs**: Aligns with the JSON output of `az resource show`, `terraform show -json`, and Kubernetes API responses.

**Structure Example**:
```json
// .radius/deploy/deployment-production-abc1234.json
{
  "deployment": {
    "commit": "abc123def456",
    "timestamp": "2026-01-29T15:30:00Z",
    "environment": "production"
  },
  "resources": [
    {
      "id": "/subscriptions/xyz/resourceGroups/myapp-rg/providers/Microsoft.Storage/storageAccounts/myappsa",
      "type": "Microsoft.Storage/storageAccounts",
      "name": "myappsa",
      "properties": {
        "location": "eastus",
        "sku": {
          "name": "Standard_LRS"
        },
        "kind": "StorageV2",
        "accessTier": "Hot",
        "provisioningState": "Succeeded",
        "creationTime": "2026-01-29T15:28:00Z"
      }
    },
    {
      "id": "arn:aws:s3:::my-app-bucket",
      "type": "AWS::S3::Bucket",
      "name": "my-app-bucket",
      "properties": {
        "region": "us-east-1",
        "versioning": {
          "status": "Enabled"
        },
        "encryption": {
          "type": "AES256"
        },
        "creationDate": "2026-01-29T15:28:00Z"
      }
    }
  ]
}
```

### Q3: Does Radius need a state store for AWS resources to maintain idempotency?

**Recommendation**: **No state store is needed** for the following reasons:

1. **Terraform State Management**: When using Terraform as the deployment tool for AWS, Terraform manages its own state file (`.tfstate`). Repo Radius can rely on Terraform's built-in idempotency mechanisms without introducing a parallel state store.

2. **State Storage Options**: Users can configure Terraform state backends (S3 + DynamoDB, Terraform Cloud) via the `terraformrc` configuration in their `.env` files. Repo Radius doesn't need to manage this—it's part of the existing Terraform ecosystem.

3. **Azure with Bicep**: Azure Resource Manager provides native idempotency, so no additional state store is needed for Azure deployments with Bicep.

**Implementation Guidance**:
- AWS deployments: Terraform only (Terraform manages state)
- Azure deployments: Bicep (ARM provides idempotency) OR Terraform (Terraform manages state)
- Kubernetes deployments: Helm or direct manifest application (Kubernetes provides idempotency)

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Initialize Repository for Repo Radius (Priority: P1)

As a developer, I want to initialize my Git repository for Repo Radius so that I can begin defining my infrastructure and application model in a Git-centric workflow.

**Why this priority**: Initialization is the entry point for all Repo Radius functionality. Without this, no other features can be used.

**Independent Test**: Can be fully tested by running `rad init` in a Git repository and verifying the directory structure is created, Resource Types are populated, and environment configuration is established.

**Acceptance Scenarios**:

1. **Given** a directory exists, **When** I run `rad init`, **Then** the system first verifies it is a Git repository (by checking for `.git/` directory or running `git rev-parse --git-dir`)
2. **Given** I am not in a Git repository, **When** I run `rad init`, **Then** the system displays an error message "Current directory is not a Git repository. Please run 'git init' first." and exits with code 2
3. **Given** a valid Git repository exists, **When** I run `rad init`, **Then** the system creates the directory structure:
   - `.radius/`
   - `.radius/config/` (contains Radius configuration files)
   - `.radius/config/types/` (contains Resource Types)
   - `.radius/model/` (contains application model, created by separate project)
   - `.radius/plan/` (contains deployment artifacts generated by `rad plan`)
   - `.radius/deploy/` (contains resource details captured by `rad deploy`)
4. **Given** I run `rad init` in a valid Git repository, **When** initialization completes, **Then** the system populates `.radius/config/types/` with Resource Types from the Radius resource-types-contrib repository using git sparse-checkout to clone/fetch only the specific types directory (avoiding full repository download)
5. **Given** I run `rad init`, **When** the system cannot reach the resource-types-contrib repository or authentication fails during Resource Types population, **Then** the system displays a clear error message instructing the user to resolve connectivity/authentication issues and retry, and exits with code 2
6. **Given** existing `.env` files exist in the repository, **When** I run `rad init`, **Then** the system searches for all `.env*` files and validates each contains required configuration
7. **Given** a `.env` file exists but lacks required cloud configuration, **When** I run `rad init`, **Then** the system prompts "Existing .env file found but missing cloud platform configuration. Would you like to add configuration?"
8. **Given** no `.env` file exists or existing files are insufficient, **When** I run `rad init`, **Then** the system prompts: "How would you like to deploy containers? 1. Kubernetes 2. Azure Container Instances"
9. **Given** I select option 1 (Kubernetes) for containers, **When** prompted for additional resources, **Then** the system prompts: "Where should other resources (databases, message queues, etc.) be deployed? 1. Kubernetes 2. AWS 3. Azure"
10. **Given** I select option 2 (Azure Container Instances) for containers, **When** configuration continues, **Then** the system assumes Azure for other resources and skips the additional resources prompt
11. **Given** I select Kubernetes for containers and option 1 (Kubernetes) for other resources, **When** prompted for configuration details, **Then** the system collects: Kubernetes context name and namespace only
12. **Given** I select Kubernetes for containers and option 2 (AWS) for other resources, **When** prompted for configuration details, **Then** the system collects: AWS account ID, AWS region, and Kubernetes context name and namespace, and automatically selects Terraform as the deployment tool (Bicep for AWS is out of scope for this release)
13. **Given** I select Kubernetes for containers and option 3 (Azure) for other resources, **When** prompted for configuration details, **Then** the system collects: Azure subscription ID, Azure resource group, and Kubernetes context name and namespace
14. **Given** I select Azure Container Instances for containers, **When** prompted for configuration details, **Then** the system collects: Azure subscription ID and Azure resource group (no Kubernetes configuration needed)
15. **Given** AWS is selected for other resources, **When** configuration collection completes, **Then** the system creates a default `recipes.yaml` in `.radius/config/` with Terraform-based recipe definitions (no deployment tool prompt)
16. **Given** Azure is selected (either via ACI or explicitly for other resources), **When** the system detects available deployment tools, **Then**:
    - If only Terraform CLI is detected: automatically select Terraform (no prompt)
    - If only Bicep CLI is detected: automatically select Bicep (no prompt)
    - If both are detected: prompt "When orchestrating application deployments, what deployment tool should Radius use? (T)erraform, (B)icep"
    - If neither is detected: display error "No deployment tool found. Please install Terraform CLI or Bicep CLI and try again."
17. **Given** I select Terraform for Azure, **When** initialization completes, **Then** the system creates a default `recipes.yaml` in `.radius/config/` with Terraform-based recipe definitions
18. **Given** I select Bicep for Azure, **When** initialization completes, **Then** the system creates a default `recipes.yaml` in `.radius/config/` with Bicep-based recipe definitions
19. **Given** the `.radius/` directory already exists, **When** I run `rad init`, **Then** the system displays a warning "Repo Radius is already initialized. Re-running init may overwrite existing configuration. Continue? (y/N)" and only proceeds if confirmed
20. **Given** `~/.rad/config.yaml` exists with a different current workspace, **When** I run `rad init`, **Then** the system sets `workspaces.current` to `git` and displays a warning:
    ```
    ⚠️  Workspace switched to 'git' for Repo Radius mode.
    
    Previous workspace: my-radius-control-plane
    
    To switch back to Control Plane mode, run:
      rad workspace switch my-radius-control-plane
    ```
21. **Given** `rad init` completes successfully, **When** initialization finishes, **Then** the system displays a summary and next steps:
    ```
    ✅ Repo Radius initialized successfully
    
    📋 Summary:
       • Resource Types populated from radius-project/resource-types-contrib:
         - Radius.Compute/containers
         - Radius.Compute/persistentVolumes
         - Radius.Compute/routes
         - Radius.Security/secrets
         - ... (N total)
       • Environment configured: <cloud platform>
       • Deployment tool: <terraform|bicep>
       • Recipes manifest: .radius/config/recipes.yaml
    
    🚀 Next steps:
       1. Commit the initialized configuration:
          git add .radius/
          git commit -m "Initialize Repo Radius"
       2. Create your application model:
          rad model
       3. Generate a deployment plan:
          rad plan
    
    💡 Run 'rad --help' for more commands and options
    ```

---

### User Story 2 - Generate Deployment Artifacts (Priority: P1)

As a developer, I want to generate ready-to-execute deployment artifacts from my application model so that I can understand and audit the infrastructure changes before deployment.

**Why this priority**: Artifact generation populates the Radius Graph with the concrete deployment instructions (Terraform configurations, Bicep templates, etc.) that will be executed during deployment. This bridges the gap between the abstract application model and actual infrastructure provisioning.

**Independent Test**: Can be fully tested by running `rad plan` with a valid application model and verifying deployment artifacts are generated in `.radius/plan/` and Terraform plan output is captured (if applicable).

**Acceptance Scenarios**:

1. **Given** a valid application model exists in `.radius/model/`, **When** I run `rad plan`, **Then** the system invokes the Radius Deployment Engine to determine the deployment sequence and generates ready-to-execute deployment artifacts in `.radius/plan/`
2. **Given** deployment artifacts are generated, **When** I examine them, **Then** they are derived from the recipe pack specified in the `.env` file (or the environment-specific `.env.<ENVIRONMENT_NAME>` file if specified)
3. **Given** generated deployment artifacts exist, **When** I review their intended use, **Then** the artifacts are not intended to be modified by users but are captured for auditability and to populate the Radius Graph
4. **Given** the Deployment Engine determines the execution sequence, **When** I run `rad plan`, **Then** the system:
   - Creates sequenced subdirectories in `.radius/plan/` named `<sequence>-<resourceName>-<deploymentTool>/` (e.g., `001-network-terraform/`, `002-database-bicep/`)
   - Generates a `plan-manifest.yaml` at `.radius/plan/plan-manifest.yaml` containing the ordered list of deployment steps, resource type mappings, and dependencies between steps
5. **Given** a deployment step uses `recipeKind: terraform`, **When** generating artifacts for that step, **Then** the system:
   - Generates a `terraform.tfvars` file containing all Terraform variable values derived from the application model and environment configuration
   - Generates a `main.tf` file that references the recipe module from `recipeLocation`
   - Executes `terraform init` (with `-backend-config=<file>` if `TF_BACKEND_CONFIG` is specified in the environment) followed by `terraform plan -var-file=terraform.tfvars`
   - Stores the plan output in `terraform-plan.txt`
   - Copies the `terraform.lock.hcl` file (provider lock file with exact versions and checksums)
   - Generates a `terraform-context.log` file capturing execution context for auditability and troubleshooting:
     - Terraform CLI version
     - `TF_CLI_CONFIG_FILE` environment variable value (if set)
     - `TF_BACKEND_CONFIG` environment variable value (if set)
     - Copy of the `provider_installation` block from terraformrc (if present)
     - Backend configuration from the generated `main.tf` and partial backend config file (if used)
   - Generates `deploy.sh` and `deploy.ps1` scripts containing the exact CLI commands that `rad deploy` will execute for this step
6. **Given** a deployment step uses `recipeKind: bicep`, **When** generating artifacts for that step, **Then** the system:
   - Generates a `<resourceName>.bicepparam` file containing all Bicep parameter values derived from the application model and environment configuration
   - Generates a `<resourceName>.bicep` file that references the recipe module from `recipeLocation`
   - Validates the Bicep templates using `bicep build`
   - Generates a `bicep-context.log` file capturing Bicep CLI version and Azure CLI version
   - Generates `deploy.sh` and `deploy.ps1` scripts containing the exact CLI commands that `rad deploy` will execute for this step
7. **Given** a recipe entry has `recipeKind: terraform` and the `recipeLocation` is not pinned to a specific Git commit hash or tag, **When** I run `rad plan`, **Then** the system displays an error "Recipe '<resourceType>' uses an unpinned Terraform module. Use a Git commit hash or tag for reproducible deployments, or run with `--allow-unpinned-recipes` to override." and exits with code 2
8. **Given** a recipe entry has `recipeKind: bicep` and the `recipeLocation` uses the `latest` OCI tag, **When** I run `rad plan`, **Then** the system displays an error "Recipe '<resourceType>' uses the 'latest' OCI tag. Use a specific version tag for reproducible deployments, or run with `--allow-unpinned-recipes to override`." and exits with code 2
9. **Given** recipes use unpinned versions, **When** I run `rad plan --allow-unpinned-recipes`, **Then** the system proceeds with a warning "Warning: Using unpinned recipe versions. Deployments may not be reproducible." but does not fail
10. **Given** a recipe's version has changed since the last `rad plan` run (e.g., updating git ref or OCI tag in recipes.yaml), **When** I run `rad plan`, **Then** the system displays a warning listing each recipe whose version changed: "Warning: Recipe version changed for '<resourceType>': <old-version> → <new-version>. Review plan artifacts to verify intended changes." and continues (recipe versions are captured in plan output: main.tf references recipeLocation with version, plan-manifest.yaml provides audit trail)
11. **Given** no application model exists in `.radius/model/`, **When** I run `rad plan`, **Then** the system displays an error "No application model found in .radius/model/. Please create an application model first." and exits with code 2
12. **Given** the recipe pack specified in `.env` does not exist, **When** I run `rad plan`, **Then** the system displays an error "Recipe pack not found at <path>. Please check your .env configuration." and exits with code 2
13. **Given** I want to plan for a specific environment, **When** I run `rad plan --environment staging`, **Then** the system uses the `.env.staging` configuration file
14. **Given** `rad plan` completes successfully, **When** the plan is generated, **Then** the system displays a summary and next steps:
    ```
    ✅ Plan generated successfully
    
    📋 Summary:
       • 3 deployment steps generated
       • Resources: network (terraform), database (bicep), app (terraform)
       • Artifacts saved to: .radius/plan/
    
    🚀 Next steps:
       1. Review the generated plan in .radius/plan/
       2. Commit the plan to Git:
          git add .radius/plan/
          git commit -m "rad plan: <environment>"
       3. Deploy with:
          rad deploy --commit <commit-hash>
    ```

---

### User Story 3 - Deploy from Git Commit (Priority: P1)

As a developer, I want to deploy infrastructure only from a specific Git commit hash or tag so that I have an auditable, reproducible deployment process, and I am prevented from accidentally deploying uncommitted changes.

**Why this priority**: Deployment is the ultimate goal of the workflow. Requiring Git commits ensures auditability, reproducibility, and prevents accidental deployment of uncommitted or untested changes.

**Independent Test**: Can be fully tested by committing changes, running `rad deploy --commit <hash>` or `rad deploy --tag <tag>`, and verifying resources are deployed and details captured in `.radius/deploy/`.

**Acceptance Scenarios**:

1. **Given** I run `rad deploy`, **When** the command starts, **Then** the system validates that required environment variables are set for the target platform
2. **Given** I am deploying to AWS, **When** environment validation runs, **Then** the system checks for AWS_ACCOUNT_ID and AWS_REGION and fails with exit code 3 if missing
3. **Given** I am deploying to Azure, **When** environment validation runs, **Then** the system checks for AZURE_CLIENT_ID, AZURE_CLIENT_SECRET, and AZURE_TENANT_ID and fails with exit code 3 if missing
4. **Given** the environment includes Kubernetes configuration (context and namespace), **When** environment validation runs, **Then** the system validates:
   - A kubeconfig file exists at `$KUBECONFIG` or `~/.kube/config`
   - The specified Kubernetes context exists in the kubeconfig
   - The system can connect to the cluster (basic connectivity check)
   - If validation fails, displays error with instructions and exits with code 3:
     ```
     ❌ Kubernetes configuration error
     
     Issue: <kubeconfig not found | context not found | cannot connect to cluster>
     
     To configure Kubernetes:
       # Ensure kubeconfig exists:
       export KUBECONFIG=/path/to/kubeconfig
       
       # Or configure a cluster:
       az aks get-credentials --resource-group <rg> --name <cluster>
       aws eks update-kubeconfig --name <cluster>
       
       # Verify context:
       kubectl config get-contexts
       kubectl config use-context <context-name>
     ```
5. **Given** environment variables are missing, **When** validation fails, **Then** the system displays a clear error with login instructions:
   ```
   ❌ Missing required environment variables for <platform>
   
   Missing: <list of missing vars>
   
   To authenticate:
   
   AWS:
     aws configure
     # Or set environment variables:
     export AWS_ACCESS_KEY_ID=<your-access-key>
     export AWS_SECRET_ACCESS_KEY=<your-secret-key>
     export AWS_REGION=<your-region>
   
   Azure:
     az login
     # For service principal (CI/CD):
     export AZURE_CLIENT_ID=<your-client-id>
     export AZURE_CLIENT_SECRET=<your-client-secret>
     export AZURE_TENANT_ID=<your-tenant-id>
     export AZURE_SUBSCRIPTION_ID=<your-subscription-id>
   ```
   and exits with code 3
6. **Given** I run `rad deploy` without `--commit` or `--tag` and the `GITHUB_SHA` environment variable is not set, **When** the command starts, **Then** the system displays an error: "The --commit or --tag flag is required. Specify the Git commit hash or tag to deploy from." and exits with code 2
7. **Given** only a single `.env` file exists (no `.env.<name>` files), **When** I run `rad deploy --commit abc1234`, **Then** the system auto-selects the default environment
8. **Given** multiple environment files exist (`.env`, `.env.staging`, `.env.production`), **When** I run `rad deploy --commit abc1234` without `--environment`, **Then** the system displays an error:
   ```
   ❌ Multiple environments detected. Please specify which environment to deploy to:
   
   Available environments:
     • default     (.env)
     • staging     (.env.staging)
     • production  (.env.production)
   
   Usage: rad deploy --commit abc1234 --environment <name>
   ```
   and exits with code 2
9. **Given** I run `rad deploy --commit abc1234 --environment production`, **When** the commit and environment are valid, **Then** the system displays a confirmation prompt:
   ```
   📍 You are about to deploy from:
      Commit:  abc1234
      Message: "Add redis cache to app"
      Author:  user@example.com
      Date:    2026-01-30 10:30:00
   
   🎯 Target environment: production (.env.production)
      AWS Account ID:        123456789012
      AWS Region:            us-east-1
      Azure Subscription ID: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
      Azure Resource Group:  my-resource-group
      Kubernetes Context:    my-cluster
      Kubernetes Namespace:  my-namespace
   
   📦 Deployment steps: 3 resources
      001-network-terraform
      002-database-bicep
      003-app-terraform
   
   Continue? [y/N]
   ```
   and proceeds only if the user confirms with 'y' (only the relevant platform fields are shown based on the environment configuration)
10. **Given** I want to skip the confirmation prompt, **When** I run `rad deploy --commit abc1234 --environment production -y`, **Then** the system proceeds without prompting for confirmation
11. **Given** deployment artifacts exist in `.radius/plan/` and are committed to Git, **When** I confirm deployment, **Then** the system checks out that commit (or verifies current HEAD matches), executes the deployment using artifacts from that commit, and orchestrates the application deployment
12. **Given** I run `rad deploy --tag v1.2.3`, **When** the tag exists, **Then** the system deploys from that tagged commit (with the same confirmation prompt unless `-y` is specified)
13. **Given** deployment is in progress, **When** resources are being deployed, **Then** the system displays real-time progress with animated spinner:
    ```
    🚀 Deploying to production from commit abc1234...
    
    Step 1/3: 001-network-terraform
       ... vpc                    creating
       ✓ subnet-a                 created (12s)
       ✓ subnet-b                 created (11s)
       ✓ security-group           created (8s)
    
    Step 2/3: 002-database-bicep
       ... sql-server             creating
       ✓ sql-database             created (45s)
    
    Step 3/3: 003-app-terraform
       ... container-app          creating
       ... container-registry     creating
    
    Elapsed: 1m 23s
    ```
14. **Given** I have uncommitted changes in `.radius/plan/`, **When** I run `rad deploy`, **Then** the system detects uncommitted changes via `git status --porcelain` and displays error: "Cannot deploy with uncommitted changes in .radius/plan/. Please commit your changes first." and exits with code 2
15. **Given** I am running in GitHub Actions (GITHUB_ACTIONS=true), **When** I run `rad deploy` without specifying `--commit`, **Then** the system automatically uses the `GITHUB_SHA` environment variable as the commit reference and skips confirmation (non-interactive)
16. **Given** deployment is executed successfully, **When** all resources are deployed, **Then** the system captures structured details about the deployed resources
17. **Given** resource deployment completes, **When** capturing resource details, **Then** the system records the full resource state by querying the appropriate API for each resource type:
    - **Azure resources**: captured via `az resource show` or ARM API
    - **Kubernetes resources**: captured via Kubernetes API server (regardless of whether deployed via Bicep Kubernetes extension or Terraform)
    - **AWS resources**: captured from Terraform state output (`terraform show -json`)
    - The deployment record includes: the Environment used, cloud platform resource IDs for each deployed resource, and the full set of properties for each deployed resource
18. **Given** resource details are captured, **When** writing to storage, **Then** the system stores deployment details as JSON files in `.radius/deploy/deployment-<environment>-<commit-short>.json` (JSON preserves native format from each platform's API)
19. **Given** deployment completes successfully, **When** the deployment record is saved, **Then** the system displays a summary and next steps:
    ```
    ✅ Deployment completed successfully
    
    📋 Summary:
       Commit:      abc1234
       Environment: production
       Duration:    2m 34s
       
       Resources deployed:
         ✓ 001-network-terraform    (3 resources)
         ✓ 002-database-bicep       (2 resources)
         ✓ 003-app-terraform        (4 resources)
    
    📁 Deployment record saved to:
       .radius/deploy/deployment-production-abc1234.json
    
    🚀 Next steps:
       1. Commit the deployment record:
          git add .radius/deploy/
          git commit -m "rad deploy: production @ abc1234"
    ```
20. **Given** a deployment fails during resource provisioning, **When** the failure occurs, **Then** the system implements atomic deployment semantics by attempting to rollback (delete) all successfully deployed resources in reverse dependency order
21. **Given** rollback operations are executed, **When** rollback completes, **Then** the system captures rollback results (successful deletions and any deletion failures), saves a deployment record with failure status including both deployment and rollback details, and exits with code 5
22. **Given** I want quiet mode for cleaner logs, **When** I run `rad deploy --quiet`, **Then** the system suppresses progress output but still provides final status and error messages

---

### User Story 4 - Configure Multiple Environments (Priority: P2)

As a developer, I want to configure multiple deployment environments (dev, staging, production) so that I can deploy the same application model to different cloud accounts, regions, Kubernetes clusters, or with different deployment settings.

**Why this priority**: Multi-environment support enables the dev/staging/production workflow that enterprise teams require.

**Independent Test**: Can be fully tested by creating multiple `.env` files (`.env`, `.env.staging`, `.env.production`) and verifying `rad plan` and `rad deploy` respect the selected environment when using `--environment` flag.

**Acceptance Scenarios**:

1. **Given** I need a default environment, **When** I create environment configuration, **Then** I store it in `.env` in the repository root
2. **Given** I need additional named environments, **When** I create environment configuration, **Then** I store it in `.env.<ENVIRONMENT_NAME>` (e.g., `.env.production`, `.env.staging`, `.env.dev`)
3. **Given** I need to deploy to AWS, **When** I create an environment configuration file, **Then** I can specify AWS account ID and AWS region without including any credentials
4. **Given** I need to deploy to Azure, **When** I create an environment configuration file, **Then** I can specify Azure subscription ID and resource group without including credentials
5. **Given** I need to deploy to Kubernetes, **When** I create an environment configuration file, **Then** I can specify the Kubernetes context name and namespace
6. **Given** I have environment-specific recipe packs, **When** I create an environment configuration file, **Then** I can specify one or more recipe pack file paths via `RECIPE_PACKS` (comma-separated, e.g., `RECIPE_PACKS=.radius/config/recipes-core.yaml,.radius/config/recipes-prod.yaml`)
7. **Given** I need custom Terraform settings, **When** I create an environment configuration file, **Then** I can specify Terraform CLI configuration path via `TF_CLI_CONFIG_FILE` (referencing a `terraformrc` file)
8. **Given** I need custom Terraform backend configuration, **When** I create an environment configuration file, **Then** I can specify a partial backend configuration file path via `TF_BACKEND_CONFIG` (e.g., `TF_BACKEND_CONFIG=.radius/config/backend-production.hcl`), which will be used with `terraform init -backend-config=<file>`
9. **Given** I have multiple environments configured, **When** I run `rad plan --environment production`, **Then** the system uses configuration from `.env.production` instead of the default `.env`
10. **Given** I have multiple environments configured, **When** I run `rad deploy --environment staging`, **Then** the system uses configuration from `.env.staging`
11. **Given** I specify an environment that doesn't exist, **When** I run `rad plan --environment nonexistent`, **Then** the system displays error: "Environment 'nonexistent' not found. Available environments: default, staging, production" and exits with code 2
12. **Given** environment files must not contain credentials, **When** I accidentally include credentials in a `.env` file, **Then** `rad init` or other commands should warn: "Warning: .env files should not contain credentials. Use environment variables instead."

---

### User Story 5 - Install via Package Manager (Priority: P2)

As a developer, I want to install Repo Radius using my operating system's native package manager so that installation is simple and follows platform conventions.

**Why this priority**: Easy installation reduces friction for adoption and enables consistent tooling across development teams.

**Independent Test**: Can be fully tested by installing via the appropriate package manager and verifying `rad` commands are available.

**Acceptance Scenarios**:

1. **Given** I am on Windows, **When** I run `winget install radius`, **Then** the Repo Radius CLI is installed and available in my PATH
2. **Given** I am on macOS, **When** I run `brew install radius`, **Then** the Repo Radius CLI is installed and available in my PATH
3. **Given** I am on Linux (Debian/Ubuntu), **When** I run `apt install radius`, **Then** the Repo Radius CLI is installed
4. **Given** I am on Linux (Fedora/RHEL), **When** I run `dnf install radius`, **Then** the Repo Radius CLI is installed
5. **Given** Radius is installed via any package manager, **When** installation completes, **Then** the system creates `~/.rad/config.yaml` with default content:
   ```yaml
   workspaces:
     current: git
   ```
6. **Given** `~/.rad/config.yaml` already exists, **When** Radius is installed or upgraded, **Then** the existing config file is preserved (not overwritten)

---

### User Story 6 - Workspace Management (Priority: P2)

As a developer, I want to switch between Repo Radius (Git-centric) and Control Plane Radius modes using workspace commands so that I can choose the appropriate deployment mode for my needs.

**Why this priority**: Enables smooth transition between Git-centric and Control Plane modes without configuration friction.

#### Acceptance Scenarios

1. **Given** Radius is freshly installed, **When** I run `rad workspace list`, **Then** the system displays:
   ```
   WORKSPACE   TYPE        STATUS
   git         built-in    active
   ```
2. **Given** I want to connect to a Control Plane, **When** I run `rad workspace create my-radius-control-plane --context my-k8s-context --group default --environment production`, **Then** the system creates a workspace entry in `~/.rad/config.yaml`:
   ```yaml
   workspaces:
     current: git
     items:
       my-radius-control-plane:
         connection:
           context: my-k8s-context
           kind: kubernetes
         group: default
         environment: production
   ```
3. **Given** I have a Control Plane workspace configured, **When** I run `rad workspace list`, **Then** the system displays:
   ```
   WORKSPACE                  TYPE        STATUS
   git                        built-in    active
   my-radius-control-plane    kubernetes  
   ```
4. **Given** I am on the `git` workspace, **When** I run `rad workspace switch my-radius-control-plane`, **Then** the system switches to the Control Plane workspace and displays "Switched to workspace 'my-radius-control-plane'"
5. **Given** I am on the `my-radius-control-plane` workspace, **When** I run `rad workspace switch git`, **Then** the system switches back to Repo Radius mode and displays "Switched to workspace 'git'"
6. **Given** I am on the `git` workspace, **When** I run a Control Plane-only command like `rad resource list`, **Then** the system displays an error "The 'rad resource list' command is not available in Git workspace. Switch to a Control Plane workspace with 'rad workspace switch <name>'." and exits with code 2
7. **Given** I am on the `my-radius-control-plane` workspace, **When** I run `rad deploy`, **Then** the deployment is executed via the Control Plane using the default environment configured in the workspace
8. **Given** I am on the `git` workspace, **When** I run `rad deploy`, **Then** the deployment is executed locally using Repo Radius

---

### User Story 7 - Run in GitHub Actions (Priority: P2)

As a CI/CD engineer, I want to run Repo Radius in a GitHub Actions workflow so that I can automate infrastructure deployment as part of my CI/CD pipeline.

**Why this priority**: CI/CD integration is explicitly called out as the primary optimization target for Repo Radius.

**Independent Test**: Can be fully tested by creating a GitHub Actions workflow that runs `rad plan` and `rad deploy` and verifying successful execution.

**Acceptance Scenarios**:

1. **Given** Radius is running in any environment, **When** the system checks for GitHub Actions, **Then** it detects GitHub Actions by checking for `GITHUB_ACTIONS=true` environment variable and uses `GITHUB_SHA` for automatic commit reference
2. **Given** a GitHub Actions workflow, **When** I invoke `rad plan`, **Then** the command completes successfully in non-interactive mode without prompting for user input
3. **Given** a GitHub Actions workflow, **When** I invoke `rad deploy` without `--commit`, **Then** the system automatically uses `GITHUB_SHA` as the commit reference
4. **Given** a GitHub Actions workflow, **When** I invoke `rad deploy`, **Then** the system skips the confirmation prompt (non-interactive) and proceeds directly to deployment
5. **Given** deployment artifacts were generated in a previous workflow step, **When** I run `rad deploy` in a subsequent step, **Then** the deployment uses the committed artifacts from the `GITHUB_SHA` commit
6. **Given** a deployment fails in GitHub Actions, **When** the workflow exits, **Then** the exit code (2, 3, 4, or 5) is propagated to the workflow step, enabling conditional job handling
7. **Given** I want to use Repo Radius in a reusable workflow, **When** I create a GitHub Actions workflow, **Then** I can use a pattern like:
   ```yaml
   name: Deploy with Radius
   
   on:
     push:
       branches: [main]
   
   jobs:
     deploy:
       runs-on: ubuntu-latest
       steps:
         - uses: actions/checkout@v4
         
          - name: Install Radius CLI
            uses: radius-project/setup-rad@v1
            with:
              version: 'latest'  # or pin to specific version
         
         - name: Configure AWS credentials
           uses: aws-actions/configure-aws-credentials@v4
           with:
             role-to-assume: ${{ secrets.AWS_ROLE_ARN }}
             aws-region: us-east-1
         
         - name: Configure EKS kubeconfig
           run: aws eks update-kubeconfig --name my-cluster --region us-east-1
         
         - name: Generate deployment plan
           run: rad plan --environment production
         
         - name: Commit plan
           run: |
             git config user.name "github-actions"
             git config user.email "github-actions@github.com"
             git add .radius/plan/
             git commit -m "rad plan: production"
             git push
         
         - name: Deploy
           run: rad deploy --environment production -y
         
         - name: Commit deployment record
           run: |
             git add .radius/deploy/
             git commit -m "rad deploy: production @ ${{ github.sha }}"
             git push
   ```

---

### User Story 8 - Deploy via Control Plane Radius (Priority: P2)

As a developer using Repo Radius, I want to deploy my application through an existing Control Plane Radius installation so that I can leverage centralized orchestration, observability, and team collaboration features while keeping my application model in Git.

**Why this priority**: Enables teams with existing Control Plane infrastructure to use Repo Radius for Git-based application modeling while benefiting from centralized deployment capabilities.

#### Acceptance Scenarios

1. **Given** I have an application model in my Git repository and a Control Plane workspace configured, **When** I run `rad workspace switch my-radius-control-plane` followed by `rad deploy`, **Then** the system deploys via the Control Plane using the environment configured in the workspace
2. **Given** I am on the `my-radius-control-plane` workspace, **When** I run `rad deploy`, **Then** the system:
   - Reads the application model from `.radius/model/`
   - Uses the Resource Types already configured on the Control Plane
   - Uses the Recipe Packs already registered on the Control Plane
   - Submits the deployment request to the Control Plane API
   - Streams deployment progress from the Control Plane
3. **Given** the application model references a resource type not configured on the Control Plane, **When** I run `rad deploy`, **Then** the system displays an error "Resource type 'Radius.Compute/containers' is not configured on the Control Plane. Please contact your platform administrator." and exits with code 2
4. **Given** the Control Plane deployment completes, **When** I view the results, **Then** the system displays:
   ```
   ✅ Deployment completed via Control Plane
   
   📋 Summary:
      • Commit: abc1234
      • Workspace: my-radius-control-plane
      • Group: default
      • Environment: production
      • Duration: 3m 42s
      • Resources deployed: 5
   
   📁 Deployment record saved to: .radius/deploy/deployment-production-abc1234.json
   ```
5. **Given** the Control Plane is unreachable, **When** I run `rad deploy` on a Control Plane workspace, **Then** the system displays an error "Cannot connect to Control Plane. Please verify your kubeconfig context 'my-k8s-context' is valid and the cluster is accessible." and exits with code 3
6. **Given** I am not authenticated to the Kubernetes cluster, **When** I run `rad deploy` on a Control Plane workspace, **Then** the system displays an error with instructions for authenticating to the cluster

---

### User Story 9 - Migrate Configuration to Control Plane Radius (Priority: P3)

As a platform engineer, I want to migrate my team's Repo Radius configuration to a new Control Plane Radius installation so that I can fully transition to centralized management while preserving existing configuration and deployment history.

**Why this priority**: This is a growth path for users who outgrow the Git-centric model and want to fully adopt centralized control plane features. Not required for initial adoption.

#### Acceptance Scenarios

1. **Given** I have an existing Repo Radius configuration in my Git repository and a Control Plane workspace configured, **When** I run `rad migrate --workspace my-radius-control-plane`, **Then** the system validates that the Control Plane is accessible via the workspace's kubeconfig context
2. **Given** I run `rad migrate --workspace my-radius-control-plane`, **When** the migration starts, **Then** the system displays a detailed preview of what will be migrated to the Control Plane:
   ```
   🔍 Migration Preview
   
   Target Workspace: my-radius-control-plane
   Target Context: my-k8s-context
   Target Group: default
   
   📦 Resource Types (4):
      • Radius.Compute/containers
      • Radius.Compute/persistentVolumes
      • Radius.Compute/routes
      • Radius.Security/secrets
   
   🌍 Environments (3):
      • default (from .env)
        - AWS: 123456789012 / us-east-1
        - Kubernetes: arn:aws:eks:us-east-1:123456789012:cluster/prod
      • staging (from .env.staging)
        - Azure: xxxxxxxx-xxxx-xxxx-xxxx / my-staging-rg
        - Kubernetes: my-aks-staging-cluster
      • production (from .env.production)
        - AWS: 123456789012 / us-west-2
        - Kubernetes: arn:aws:eks:us-west-2:123456789012:cluster/prod
   
   📋 Recipe Packs (2):
      • .radius/config/recipes.yaml (5 recipes)
      • .radius/config/recipes-aws.yaml (3 recipes)
   
   ⚠️  Note: Deployment history and plan artifacts will not be migrated.
       Control Plane Radius does not currently support these features.
       (See Future Enhancements FE-009, FE-010)
   
   Proceed with migration? (y/N)
   ```
3. **Given** the migration preview is displayed, **When** I confirm the migration, **Then** the system creates corresponding resources in the Control Plane:
   - Resource Types from `.radius/config/types/`
   - Recipe Packs from `.radius/config/`
   - Environments for each `.env.<ENVIRONMENT_NAME>` file
4. **Given** the migration completes successfully, **When** I view the results, **Then** the system displays:
   ```
   ✅ Migration to Control Plane Radius completed
   
   📋 Migrated Resources:
      • Resource Types: 4
      • Recipe Packs: 2
      • Environments: 3 (default, staging, production)
   
   💡 Next steps:
      1. Verify configuration: rad resource list --workspace my-radius-control-plane
      2. Switch to Control Plane: rad workspace switch my-radius-control-plane
      3. Deploy via Control Plane: rad deploy
      4. Optionally remove .radius/config/ from Git (keep .radius/model/)
   ```
5. **Given** I want to preview the migration without making changes, **When** I run `rad migrate --workspace my-radius-control-plane --dry-run`, **Then** the system displays what would be migrated without creating any resources in the Control Plane
6. **Given** I have deployment records in `.radius/deploy/`, **When** the migration completes, **Then** the system displays a note that deployment history is not migrated (Control Plane Radius does not currently support deployment history)
7. **Given** the Control Plane already has conflicting resources (e.g., environment with same name), **When** I run `rad migrate`, **Then** the system displays a warning and prompts for resolution strategy (skip, overwrite, or rename)

---

### Edge Cases

- **What happens when `rad init` is run in a non-Git directory?**
  - System MUST detect absence of Git repository via `git rev-parse --git-dir` check
  - System MUST display error: "Current directory is not a Git repository. Please run 'git init' first."
  - System MUST exit with code 2

- **What happens when `rad init` cannot reach the resource-types-contrib repository or authentication fails during Resource Types population?**
  - System MUST detect connectivity failure or authentication error when attempting git sparse-checkout
  - System MUST display clear error message instructing user to resolve connectivity/authentication issues (e.g., "Failed to fetch Resource Types from repository. Please check network connectivity and Git authentication, then retry 'rad init'.")
  - System MUST exit with code 2
  - System SHOULD provide troubleshooting hints (e.g., check proxy settings, verify Git credentials, confirm repository URL is accessible)

- **What happens when `rad deploy` is run with uncommitted changes in `.radius/plan/`?**
  - System MUST detect uncommitted changes using `git status --porcelain .radius/plan/`
  - System MUST display error: "Cannot deploy with uncommitted changes in .radius/plan/. Please commit your changes first."
  - System MUST exit with code 2

- **What happens when the specified Environment does not exist?**
  - System MUST search for `.env.<ENVIRONMENT_NAME>` file
  - If not found, system MUST list available environments by scanning for `.env*` files
  - System MUST display error: "Environment '<name>' not found. Available environments: default, staging, production"
  - System MUST exit with code 2

- **What happens when the recipe pack specified in `.env` does not exist?**
  - System MUST validate all recipe pack paths from `.env` (via `RECIPE_PACKS` variable)
  - If any file doesn't exist, system MUST display error: "Recipe pack not found at <path>. Please check your .env configuration."
  - System MUST exit with code 2

- **What happens when required environment variables (AWS_ACCOUNT_ID, AZURE_CLIENT_ID, etc.) are missing at deployment time?**
  - System MUST validate all required variables before executing deployment
  - System MUST display error: "Missing required environment variables: <list>. Please set these before deploying."
  - System MUST NOT expose or log any credential values in error messages
  - System MUST exit with code 3

- **What happens when `rad init` finds an existing `.env` file without required cloud configuration?**
  - System MUST parse the `.env` file and check for cloud platform variables (AWS_ACCOUNT_ID/AWS_REGION, AZURE_SUBSCRIPTION_ID/AZURE_RESOURCE_GROUP, or KUBERNETES_CONTEXT/KUBERNETES_NAMESPACE)
  - If insufficient, system MUST prompt: "Existing .env file found but missing cloud platform configuration. Would you like to add configuration? (y/N)"
  - If user confirms, system MUST prompt for cloud platform selection and append configuration to existing `.env`

- **What happens when `.radius/` directory already exists during `rad init`?**
  - System MUST detect existing `.radius/` directory
  - System MUST display warning: "Repo Radius is already initialized. Re-running init may overwrite existing configuration. Continue? (y/N)"
  - System MUST only proceed if user confirms with 'y'
  - If user declines, system MUST exit gracefully with code 0

- **What happens when deployment fails partway through resource provisioning?**
  - System MUST implement **atomic deployment semantics**: either all resources deploy successfully, or none remain
  - System MUST attempt to rollback (delete) all successfully deployed resources before the failure point
  - System MUST execute rollback operations in reverse dependency order to avoid orphaned resources
  - System MUST capture the rollback attempt results, including which resources were successfully deleted and which failed to delete
  - System MUST save a deployment record as JSON with failure status, including both the original deployment error and rollback results
  - System MUST provide clear error output indicating: (1) which resource failed during deployment and why, (2) which resources were rolled back, (3) any resources that failed to rollback
  - System MUST exit with code 5
  - System MUST be **idempotent**: running the same deployment again after a failed deployment must produce the same result (either success or the same failure)

- **What happens when running `rad plan` or `rad deploy` without having run `rad init` first?**
  - System MUST check for existence of `.radius/` directory
  - If not found, system MUST display error: "Repo Radius not initialized. Please run 'rad init' first."
  - System MUST exit with code 2

- **What happens when multiple environments specify different deployment tools (Terraform vs Bicep)?**
  - System MUST respect the `RECIPE_PACKS` specified in each `.env.<ENVIRONMENT_NAME>` file
  - Different environments MAY use different recipe packs that specify different deployment tools
  - System MUST validate that the required deployment tool (terraform or bicep binaries) is available in PATH

- **What happens when multiple concurrent deployments target the same environment (e.g., two GitHub Actions workflows triggering simultaneously)?**
  - System MUST rely on deployment tool's native locking mechanisms (Terraform state locking via backend configuration, Azure Resource Manager deployment locks) to prevent conflicting modifications
  - Repo Radius does NOT implement its own locking layer
  - Users are responsible for configuring state locking in their Terraform backend (e.g., DynamoDB for S3 backend) or Azure ARM deployment locks if concurrent deployment protection is required
  - If deployment tool lock acquisition fails, the error MUST propagate to the user with exit code 4 (resource conflict/state error)
  - System documentation SHOULD provide guidance on configuring state locking for supported deployment tools


## Requirements *(mandatory)*

### Functional Requirements

#### Execution Model

- **FR-001**: System MUST run as an executable on Windows, Linux, and macOS
- **FR-002**: System MUST be installable via WinGet (Windows), Homebrew (macOS), apt (Debian/Ubuntu), and dnf (Fedora/RHEL)
- **FR-003**: System MUST be optimized for non-interactive execution in GitHub Actions
- **FR-004**: System MUST expose a command surface similar to the existing `rad` CLI
- **FR-005**: System MUST return semantic exit codes:
  - `0` = Success
  - `1` = General error
  - `2` = Validation error (configuration/input problems)
  - `3` = Authentication/authorization error
  - `4` = Resource conflict/state error
  - `5` = Deployment failure
- **FR-006**: System MUST support `--quiet` flag that suppresses progress output for cleaner CI/CD logs
- **FR-007**: System SHOULD provide an official GitHub Action (`radius-project/setup-rad`) for easy CLI installation in workflows

#### `rad init` Command

- **FR-010**: System MUST verify the current working directory is a Git repository before initialization by checking for `.git/` directory or running `git rev-parse --git-dir`
- **FR-011**: System MUST create the directory structure: `.radius/`, `.radius/config/`, `.radius/config/types/`, `.radius/model/`, `.radius/plan/`, `.radius/deploy/`
- **FR-012**: System MUST populate `.radius/config/types/` with Resource Types from the Radius resource-types-contrib repository using git sparse-checkout to clone/fetch only the specific types directory
- **FR-012a**: System MUST fail initialization with exit code 2 and display a clear error message instructing the user to resolve connectivity/authentication issues and retry if the resource-types-contrib repository cannot be reached or authentication fails during Resource Types population
- **FR-013**: System MUST search the repository for existing `.env` and `.env.*` files and validate they contain cloud platform configuration
- **FR-014**: System MUST prompt the user with a two-step configuration flow if no `.env` file exists or existing files are insufficient:
  - First prompt: "How would you like to deploy containers? 1. Kubernetes 2. Azure Container Instances"
  - Second prompt (if Kubernetes selected): "Where should other resources (databases, message queues, etc.) be deployed? 1. Kubernetes 2. AWS 3. Azure"
  - If Azure Container Instances is selected, Azure is assumed for other resources (skip second prompt)
- **FR-015**: System MUST collect Kubernetes context name and namespace when Kubernetes is selected for containers
- **FR-015a**: System MUST collect Azure subscription ID and resource group when Azure Container Instances is selected (no Kubernetes config needed)
- **FR-016**: System MUST automatically select Terraform as the deployment tool when AWS is selected for other resources (Bicep for AWS is out of scope for this release)
- **FR-016a**: System MUST detect available deployment tools when Azure is selected and:
  - If only Terraform CLI is detected: automatically select Terraform (no prompt)
  - If only Bicep CLI is detected: automatically select Bicep (no prompt)
  - If both are detected: prompt "When orchestrating application deployments, what deployment tool should Radius use? (T)erraform, (B)icep"
  - If neither is detected: display error "No deployment tool found. Please install Terraform CLI or Bicep CLI and try again." and exit with code 2
- **FR-017**: System MUST create a default `recipes.yaml` in `.radius/config/` based on the selected deployment tool
- **FR-018**: System MUST display a warning and request confirmation if `.radius/` directory already exists: "Repo Radius is already initialized. Re-running init may overwrite existing configuration. Continue? (y/N)"
- **FR-019**: System MUST NOT store credentials in `.env` files and SHOULD warn users if credential-like values are detected

#### `rad plan` Command

- **FR-020**: System MUST generate ready-to-execute deployment artifacts (Terraform configurations or Bicep templates)
- **FR-021**: System MUST derive deployment artifacts from the recipe pack specified in the `.env` file (or `.env.<ENVIRONMENT_NAME>` if `--environment` flag is used)
- **FR-022**: System MUST support Terraform (plan/apply) as a deployment tool
- **FR-023**: System MUST support Bicep as a deployment tool
- **FR-024**: Generated artifacts are captured for auditability and Radius Graph construction (not intended for user modification)
- **FR-025**: Generated scripts MUST be stored in `.radius/plan/`
- **FR-026**: When the deployment tool is Terraform, system MUST execute `terraform plan` and store the output in `.radius/plan/terraform-plan.txt`
- **FR-027**: System MUST support `--environment <name>` flag to specify which environment configuration to use
- **FR-028**: System MUST validate that the application model exists in `.radius/model/` before planning and exit with code 2 if missing
- **FR-029**: System MUST validate that the recipe pack specified in `.env` exists and exit with code 2 if not found

#### `rad deploy` Command

- **FR-040**: System MUST validate that required environment variables are set for the target platform before deployment:
  - AWS: AWS_ACCOUNT_ID, AWS_REGION
  - Azure: AZURE_CLIENT_ID, AZURE_CLIENT_SECRET, AZURE_TENANT_ID
- **FR-041**: System MUST exit with code 3 and display clear error messages listing missing environment variables if validation fails
- **FR-042**: System MUST orchestrate application deployment by executing deployment only from a Git commit hash or tag
- **FR-043**: System MUST NOT deploy directly from uncommitted local files
- **FR-044**: System MUST detect uncommitted changes in `.radius/plan/` using `git status --porcelain` and refuse deployment with exit code 2
- **FR-045**: System MUST support `--commit <hash>` flag to specify which commit to deploy from
- **FR-046**: System MUST support `--tag <tag>` flag to specify which tag to deploy from
- **FR-047**: System MUST automatically detect and use `GITHUB_SHA` environment variable when running in GitHub Actions if no `--commit` flag is provided
- **FR-048**: System MUST capture structured details about deployed resources after successful deployment
- **FR-049**: System MUST record the Environment used for each deployment
- **FR-050**: System MUST record cloud platform resource IDs for each deployed resource
- **FR-051**: System MUST record the full set of properties for each deployed resource as returned by the cloud platform
- **FR-052**: System MUST store deployment details as JSON files in `.radius/deploy/deployment-<environment>-<commit-short>.json`
- **FR-053**: System MUST support `--environment <name>` flag to specify which environment configuration to use
- **FR-054**: System MUST support `--quiet` flag to suppress progress output while preserving final status and error messages
- **FR-055**: System MUST implement **atomic deployment semantics**: if resource provisioning fails partway through deployment, system MUST attempt to rollback (delete) all successfully deployed resources before exiting, execute rollback in reverse dependency order, capture rollback results, and exit with code 5
- **FR-056**: System MUST be **idempotent**: running the same deployment multiple times must produce the same result, and re-running a failed deployment must either succeed completely or fail with the same error
- **FR-057**: System MUST provide detailed error output for deployment failures using the structured error format defined in NFR-021, including: error code, human-readable message, affected resource name/ID, suggested remediation action, rollback status (which resources were successfully rolled back, any resources that failed to rollback)

#### Configuration Model (Input Files)

- **FR-060**: All configuration MUST be stored in the Git repository
- **FR-061**: Resource Types MUST be stored as YAML files in `.radius/config/types/` (same format as Radius Resource Types today without modification)
- **FR-062**: Default Environment configuration MUST be stored in `.env` file in the repository root
- **FR-063**: Named Environment configurations MUST be stored as `.env.<ENVIRONMENT_NAME>` files (e.g., `.env.production`, `.env.staging`, `.env.dev`)
- **FR-064**: Environment files MUST support AWS account ID and region configuration via `AWS_ACCOUNT_ID` and `AWS_REGION` variables
- **FR-065**: Environment files MUST support Azure subscription ID and resource group configuration via `AZURE_SUBSCRIPTION_ID` and `AZURE_RESOURCE_GROUP` variables
- **FR-066**: Environment files MUST support Kubernetes context name and namespace configuration via `KUBERNETES_CONTEXT` and `KUBERNETES_NAMESPACE` variables
- **FR-067**: Environment files MUST include `RECIPE_PACKS` variable (required, comma-separated list of recipe pack paths, e.g., `RECIPE_PACKS=.radius/config/recipes.yaml`)
- **FR-068**: Environment files MUST support Terraform CLI configuration via `TF_CLI_CONFIG_FILE` variable (referencing a `terraformrc` file path)
- **FR-068a**: Environment files MUST support Terraform backend configuration via `TF_BACKEND_CONFIG` variable (referencing a partial backend configuration file path, used with `terraform init -backend-config=<file>`)
- **FR-069**: Environment files MUST NOT contain credentials (secrets MUST be provided via environment variables at runtime)
- **FR-070**: Recipes MUST be stored as YAML files in the `.radius/config/` directory (default: `recipes.yaml`)
- **FR-071**: System SHOULD warn users if credential-like patterns (e.g., values starting with "ey", containing "password", "secret", "key") are detected in `.env` files
- **FR-072**: System MUST validate that `RECIPE_PACKS` is present in `.env` file and exit with code 2 if missing

### Non-Functional Requirements

#### Reliability & Deployment Semantics

- **NFR-001**: System MUST provide **atomic deployment semantics**: deployments either complete fully or leave no resources behind
- **NFR-002**: System MUST be **idempotent**: executing the same deployment multiple times produces consistent results (subsequent runs after initial success are no-ops; subsequent runs after failure either succeed or fail with the same error)
- **NFR-003**: System MUST implement automatic rollback on deployment failure: when any resource fails to provision, all previously provisioned resources in that deployment MUST be deleted in reverse dependency order
- **NFR-004**: System MUST tolerate rollback failures gracefully: if some resources cannot be deleted during rollback, system MUST capture which resources remain and report them clearly to the user
- **NFR-005**: System MUST ensure state consistency: deployment records in `.radius/deploy/` MUST accurately reflect the actual state of cloud resources after both successful deployments and failed deployments with rollback

#### Performance

- **NFR-010**: `rad init` MUST complete in under 2 minutes for typical repository initialization (including Resource Types population via sparse checkout)
- **NFR-011**: `rad plan` MUST complete in under 2 minutes for application models with 5-10 resources
- **NFR-012**: System MUST support parallel resource provisioning where dependencies allow (via deployment tool capabilities)

#### Observability

- **NFR-020**: System MUST emit progress indicators for long-running operations (resource provisioning, rollback) unless `--quiet` flag is specified
- **NFR-021**: System MUST use a single static error format with structured fields for all failure messages: error code (semantic code matching exit code context), message (human-readable description), affected resource (name/ID of the failed resource), and suggested action (remediation guidance). This format ensures consistency across all error scenarios and enables programmatic parsing in CI/CD workflows

### Explicit Non-Goals

- **NG-001**: Repo Radius does NOT have a concept of Radius Resource Groups
- **NG-002**: Repo Radius does NOT have a formal Environment object beyond the simple `.env.<ENVIRONMENT_NAME>` files
- **NG-003**: Repo Radius does NOT have a Terraform Settings object because it relies upon the user's existing Terraform configuration in their execution environment
- **NG-004**: Repo Radius does NOT have Credentials or Bicep Settings objects because it uses the existing authentication to an OCI registry in the user's execution environment
- **NG-005**: Repo Radius does NOT implement its own deployment locking mechanism; it relies on deployment tools' native locking (Terraform state locking, Azure ARM deployment locks)


### Key Entities

- **Environment**: A deployment target configuration specifying cloud provider details, recipe packs reference, and deployment tool settings. Stored as `.env` (default) or `.env.<ENVIRONMENT_NAME>` files. Configuration includes:
  - AWS: `AWS_ACCOUNT_ID`, `AWS_REGION`
  - Azure: `AZURE_SUBSCRIPTION_ID`, `AZURE_RESOURCE_GROUP`
  - Kubernetes: `KUBERNETES_CONTEXT`, `KUBERNETES_NAMESPACE`
  - Required: `RECIPE_PACKS` (comma-separated list of recipe YAML paths)
  - Optional: `TF_CLI_CONFIG_FILE` (path to terraformrc)
  - Optional: `TF_BACKEND_CONFIG` (path to partial backend configuration file, used with `terraform init -backend-config=<file>`)
  - MUST NOT contain credentials (provided via runtime environment variables)
  - No formal Environment object exists—configuration is file-based only

  **Example: AWS + EKS (`.env.production`)**
  ```
  # AWS Configuration
  AWS_ACCOUNT_ID=123456789012
  AWS_REGION=us-east-1
  
  # Kubernetes (EKS) Configuration
  KUBERNETES_CONTEXT=arn:aws:eks:us-east-1:123456789012:cluster/my-production-cluster
  KUBERNETES_NAMESPACE=my-app
  
  # Required: Recipe packs (comma-separated)
  RECIPE_PACKS=.radius/config/recipes.yaml
  
  # Optional: Terraform backend configuration
  TF_BACKEND_CONFIG=.radius/config/backend-production.hcl
  ```

  **Example: Azure + AKS (`.env.staging`)**
  ```
  # Azure Configuration
  AZURE_SUBSCRIPTION_ID=xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
  AZURE_RESOURCE_GROUP=my-staging-rg
  
  # Kubernetes (AKS) Configuration
  KUBERNETES_CONTEXT=my-aks-staging-cluster
  KUBERNETES_NAMESPACE=my-app-staging
  
  # Required: Recipe packs (comma-separated, multiple allowed)
  RECIPE_PACKS=.radius/config/recipes-core.yaml,.radius/config/recipes-azure.yaml
  ```

  **Note (GitHub Actions):** In CI/CD, the kubeconfig is typically generated at runtime by cloud CLI commands (`aws eks update-kubeconfig`, `az aks get-credentials`) before `rad deploy` runs. The `.env` file only stores the context name.

- **Workspace**: A named configuration that determines how `rad` commands execute. Stored in `~/.rad/config.yaml`. Two types:
  - `git` (built-in): Repo Radius mode - executes deployments locally using Git repository as source of truth
  - Control Plane workspace: Connects to a Radius Control Plane via kubeconfig for centralized orchestration. Environments fan out from the Control Plane (selected via `--environment` flag).

  **Example: `~/.rad/config.yaml`**
  ```yaml
  workspaces:
    current: git
    items:
      my-radius-control-plane:
        connection:
          context: my-k8s-context
          kind: kubernetes
        group: default
        environment: production
  ```
  
  **Workspace commands:**
  - `rad workspace list` - Show all workspaces
  - `rad workspace create <name>` - Create a Control Plane workspace
  - `rad workspace switch <name>` - Switch active workspace
  - `rad workspace switch git` - Switch back to Repo Radius mode
  
  **Backwards Compatibility:**
  The `rad` CLI maintains backwards compatibility with existing `config.yaml` files:
  - `default` and `current` are treated as equivalent (prefer `current` for new configs)
  - `scope` and `group` are treated as equivalent (prefer `group` for new configs)
  - Full resource paths (e.g., `/planes/radius/local/resourceGroups/default`) and short names (e.g., `default`) are both accepted for `group` and `environment`
  
- **Recipes**: A YAML file (default: `recipes.yaml`) containing an array of recipe entries that map abstract resource types to deployment artifacts. Each entry contains:
  - `resourceType`: The abstract resource type (e.g., `Applications.Core/containers`)
  - `recipeKind`: The deployment tool to use (`terraform`, `bicep`)
  - `recipeLocation`: The location of the recipe template (OCI registry reference, file path, or URL)
  
  Stored in `.radius/config/`. Referenced via `RECIPE_PACKS` variable in `.env` files (required).

  **Example: `.radius/config/recipes.yaml`**
  ```yaml
  recipes:
    - resourceType: Radius.Compute/containers
      recipeKind: terraform
      recipeLocation: git::https://github.com/radius-project/resource-types-contrib.git//containers?ref=v1.0.0
    
    - resourceType: Radius.Compute/persistentVolumes
      recipeKind: terraform
      recipeLocation: git::https://github.com/radius-project/resource-types-contrib.git//persistent-volumes?ref=v1.0.0
    
    - resourceType: Radius.Security/secrets
      recipeKind: bicep
      recipeLocation: br:radiusacr.azurecr.io/recipes/secrets:1.0.0
  ```

- **Application Model**: The user-defined model describing the application and its resource dependencies. Stored in `.radius/model/`. Produced by a separate project (out of scope for this specification). Consumed by `rad plan` to generate deployment artifacts.

- **Deployment Artifact**: A ready-to-execute configuration or template generated by `rad plan` that deploys infrastructure using a supported deployment tool (Terraform configurations, Bicep templates, etc.). Stored in `.radius/plan/`. Not intended for user modification—captured for auditability and Radius Graph construction.

- **Deployment Record**: Structured details captured after `rad deploy` completes. Stored as JSON files in `.radius/deploy/deployment-<environment>-<commit-short>.json`. Contains:
  - Deployment metadata: commit hash, timestamp, environment name
  - Deployed resources: cloud resource IDs, resource types, **complete resource snapshots with full properties as returned by cloud provider** (e.g., all Azure ARM properties, all AWS resource attributes, all Kubernetes object fields)
  - Deployment status: success, partial failure, or failure
  - JSON format preserves native API response format for programmatic access
  - **Retention**: All deployment records are retained indefinitely with no automatic cleanup (users manage retention manually if needed)

- **Resource Type**: A definition of an abstract resource type stored as YAML (same format as existing Radius Resource Types). Stored in `.radius/config/types/`. Initialized from the Radius resource-types-contrib repository during `rad init`. Examples: containers, databases, message queues, storage.

- **GitHub Actions Integration**: When running in GitHub Actions, Repo Radius automatically detects the `GITHUB_ACTIONS=true` environment variable and uses `GITHUB_SHA` for deployment commit reference. CI/CD integration uses exit codes for flow control—no output parsing required.

### Assumptions

- The application model in `.radius/model/` is produced by a separate project and is available at `rad plan` time
- Cloud provider credentials are provided via environment variables at runtime and are not managed or stored by Repo Radius:
  - AWS: `AWS_ACCOUNT_ID`, `AWS_REGION` (plus standard AWS SDK environment variables for authentication)
  - Azure: `AZURE_CLIENT_ID`, `AZURE_CLIENT_SECRET`, `AZURE_TENANT_ID`, `AZURE_SUBSCRIPTION_ID`
  - Kubernetes: Uses standard kubeconfig credential chain (no separate credential environment variables; authentication follows kubectl behavior using kubeconfig file at `$KUBECONFIG` or `~/.kube/config` with embedded credentials, tokens, or exec plugins)
- Users have Git installed and understand basic Git operations (commit, tag, checkout)
- The Radius resource-types-contrib repository is accessible during `rad init` (via HTTPS or SSH)
- Users have their own Terraform configuration in their execution environment if using Terraform as deployment tool:
  - Terraform binary is available in PATH
  - Terraform backend configuration is managed by user (e.g., S3 + DynamoDB for state locking)
  - Terraform version is compatible (>=1.0.0)
  - Users are responsible for configuring Terraform state locking (Repo Radius does NOT provide its own locking mechanism)
- Users have existing OCI registry authentication configured if using Bicep (Repo Radius does not manage Bicep/OCI credentials):
  - Bicep binary is available in PATH if using Bicep as deployment tool
  - Azure CLI is authenticated for Azure deployments
  - Azure Resource Manager provides native deployment locks; users are responsible for configuring if concurrent deployment protection is required (Repo Radius does NOT provide its own locking mechanism)
- Generated deployment artifacts are expected to become part of the Radius Graph (separate project handles graph construction)
- **AWS Idempotency**: AWS deployments use Terraform exclusively; Terraform manages its own state, so no additional Radius state store is required for AWS idempotency
- **Azure Idempotency**: Bicep is supported for Azure deployments; Azure Resource Manager provides native idempotency, so no additional state store is required
- **Kubernetes Idempotency**: Kubernetes provides native idempotency for declarative manifests
- GitHub Actions workflows provide `GITHUB_SHA` environment variable automatically
- Deployment scripts execute in the same environment where `rad deploy` is invoked (local machine or CI/CD runner)
- Users are responsible for securing `.env` files (e.g., not committing cloud account IDs to public repositories if sensitive)
- The repository is the single source of truth—no external databases or control planes are involved

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Users can initialize a repository for Repo Radius in under 2 minutes (including interactive prompts for cloud platform and deployment tool selection)
- **SC-002**: Users can generate deployment artifacts with `rad plan` in under 2 minutes for a typical application model with 5-10 resources
- **SC-003**: 90% of GitHub Actions workflows using Repo Radius complete without manual intervention (using `--quiet`, `-y`, and automatic `GITHUB_SHA` detection)
- **SC-004**: Users can deploy infrastructure with `rad deploy` and have full resource details automatically captured in JSON format
- **SC-005**: Users can install Repo Radius via their native package manager (WinGet, Homebrew, apt, dnf) without additional manual configuration steps
- **SC-006**: Users can configure and deploy to multiple environments (dev, staging, production) using separate `.env` files without code changes
- **SC-007**: Deployment failures provide actionable error messages with semantic exit codes that enable GitHub Actions workflow error handling
- **SC-008**: All Repo Radius configuration and state is stored in Git with zero external dependencies (no control plane, databases, or external state stores)

## Future Enhancements

The following features are out of scope for the initial implementation but are planned for future releases:

### Adaptive IaC Discovery (FE-001, FE-002)

- **FE-001**: Adapt to existing IaC by examining the Git repository for existing infrastructure code:
  - Terraform: `*.tf` files
  - Bicep: `*.bicep` files
  - Helm: `chart.yaml` files
  - Kustomize: `kustomization.yaml` files
  - CloudFormation: `template.yaml`, `*.template.json` files
  - When detected, offer to integrate existing scripts into Radius recipes instead of generating new ones

- **FE-002**: Adapt to existing GitOps configurations (ArgoCD, Flux):
  - Detect existing GitOps manifests and offer integration
  - Generate Radius application model from existing GitOps resources
  - Support GitOps-style continuous deployment patterns

### Additional Deployment Engines (FE-003 - FE-006)

- **FE-003**: Support Helm as a deployment tool for Kubernetes resources
- **FE-004**: Support CloudFormation as a deployment tool for AWS resources
- **FE-005**: Support Crossplane as a deployment tool for multi-cloud resources
- **FE-006**: Support Ansible as a deployment tool for infrastructure provisioning

### GitHub Actions Official Action (FE-007)

- **FE-007**: Provide official `radius-project/repo-radius-action` GitHub Action:
  ```yaml
  - uses: radius-project/repo-radius-action@v1
    with:
      command: 'deploy'
      environment: 'production'
      output-format: 'json'
  ```
  - Automatically handles installation of Repo Radius CLI
  - Provides structured output as GitHub Actions outputs
  - Supports artifact upload integration

### Multi-Repository Support (FE-008)

- **FE-008**: Support application models spanning multiple Git repositories:
  - Reference remote recipes from other repositories
  - Coordinate deployments across multiple repos
  - Dependency resolution between repos

### Control Plane Radius Enhancements (FE-009, FE-010)

The following features would need to be added to Control Plane Radius to enable full feature parity with Repo Radius during migration:

- **FE-009**: Add deployment history support to Control Plane Radius:
  - Store deployment records with full resource snapshots
  - Track deployment metadata (commit hash, timestamp, environment, duration)
  - Provide deployment history API and dashboard view
  - Enable migration of Repo Radius deployment history during `rad migrate`

- **FE-010**: Add plan artifact storage to Control Plane Radius:
  - Store generated deployment artifacts (Terraform configs, Bicep templates)
  - Provide plan preview and approval workflow
  - Enable auditability of what was deployed and when
  - Support plan comparison between deployments
