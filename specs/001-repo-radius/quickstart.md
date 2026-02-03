# Quickstart: Git Workspace Mode

**Feature**: 001-repo-radius | **Date**: 2026-02-02

Get started with Git workspace mode in 5 minutes.

## Prerequisites

- **Git** installed and configured
- **Cloud CLI** for your target platform:
  - AWS: `aws` CLI with credentials configured
  - Azure: `az` CLI logged in
- **Kubernetes**: `kubectl` with kubeconfig configured (if deploying to K8s)
- **Deployment tool**:
  - Terraform CLI (v1.0.0+) for Terraform recipes
  - Bicep CLI for Bicep recipes

## Installation

### macOS (Homebrew)

```bash
brew install radius
```

### Windows (WinGet)

```powershell
winget install radius
```

### Linux (apt)

```bash
# Ubuntu/Debian
apt install radius
```

### Linux (dnf)

```bash
# Fedora/RHEL
dnf install radius
```

## Step 1: Initialize Your Repository

Navigate to your existing Git repository:

```bash
cd my-existing-repo
rad init
```

This initializes the Git workspace in your existing Git repository by creating the `.radius/` directory structure and populating Resource Types from the community registry.

> **Note**: `rad init` requires an existing Git repository. It does not create a new repository.

## Step 2: Configure Your Environment

Create a `.env` file with your environment configuration:

```bash
# Example .env file
AWS_ACCOUNT_ID=123456789012
AWS_REGION=us-east-1
KUBERNETES_CONTEXT=my-cluster
KUBERNETES_NAMESPACE=my-app
RECIPES=.radius/config/recipes/default.yaml
```

For multiple environments, create additional files:

```bash
# .env.staging - staging environment
# .env.production - production environment
```

## Step 3: Create the Configuration Directory

The `.radius/` directory structure was created by `rad init`. The structure looks like this:

```
.radius/
├── config/
│   ├── types/          # Resource Type definitions (populated by rad init)
│   └── recipes/        # Recipe file mappings
├── model/              # Your application model
├── plan/               # Generated deployment artifacts (per app/env)
└── deploy/             # Deployment records (per app/env)
```

## Step 4: Create Your Application Model

Your application model lives in `.radius/model/`. This is created by a separate project.

> **Note**: Application model authoring is outside the scope of this quickstart. See the Application Model documentation.

## Step 5: Generate a Deployment Plan

Once you have an application model (e.g., `myapp.bicep`):

```bash
rad plan .radius/model/myapp.bicep --environment production
```

Review the generated artifacts in `.radius/plan/myapp/production/`:
- `plan.yaml` - Ordered list of deployment steps
- `001-<resource>-<tool>/` - Terraform configs or Bicep templates
- `terraform-plan.txt` or validated Bicep files

## Step 6: Commit Your Plan

```bash
git add .radius/plan/
git commit -m "rad plan: myapp production"
git push
```

## Step 7: Deploy

Deploy from the committed plan:

```bash
rad deploy --application myapp --environment production
```

Or deploy from a specific commit or tag:

```bash
rad deploy $(git rev-parse HEAD) --application myapp --environment production

# Or with a tag:
git tag v1.0.0
git push --tags
rad deploy v1.0.0 --application myapp --environment production
```

> **Note**: If you have uncommitted changes in `.radius/plan/`, you must either commit them first or specify a commit hash to deploy from.

## Step 8: Commit the Deployment Record

After successful deployment:

```bash
git add .radius/deploy/
git commit -m "rad deploy: myapp production @ $(git rev-parse --short HEAD)"
git push
```

---

## GitHub Actions Integration

Git workspace mode is optimized for CI/CD. Here's a minimal workflow:

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
          version: 'latest'
      
      - name: Configure AWS credentials
        uses: aws-actions/configure-aws-credentials@v4
        with:
          role-to-assume: ${{ secrets.AWS_ROLE_ARN }}
          aws-region: us-east-1
      
      - name: Configure EKS kubeconfig
        run: aws eks update-kubeconfig --name my-cluster --region us-east-1
      
      - name: Generate deployment plan
        run: rad plan .radius/model/myapp.bicep --environment production
      
      - name: Commit plan
        run: |
          git config user.name "github-actions"
          git config user.email "github-actions@github.com"
          git add .radius/plan/
          git commit -m "rad plan: myapp production"
          git push
      
      - name: Deploy
        run: rad deploy --application myapp --environment production -y
        # Note: commit is auto-detected from GITHUB_SHA in CI
      
      - name: Commit deployment record
        run: |
          git add .radius/deploy/
          git commit -m "rad deploy: myapp production @ ${{ github.sha }}"
          git push
```

---

## Switching to Control Plane Mode

If you have an existing Radius Control Plane, you can switch between modes:

```bash
# Switch to Control Plane mode
rad workspace switch my-control-plane

# Switch back to Git workspace mode
rad workspace switch git
```

> **Note**: Workspace creation follows the existing Radius pattern. See the Control Plane documentation for details.

---

## Common Commands Reference

| Command | Description |
|---------|-------------|
| `rad init` | Initialize Git workspace in existing Git repository |
| `rad plan <file> -e <env>` | Generate deployment artifacts for application |
| `rad deploy -a <app> -e <env>` | Deploy from current committed plan |
| `rad deploy <commit> -a <app> -e <env>` | Deploy from specific commit or tag |
| `rad diff -a <app> -e <env>` | Detect drift from deployment record |
| `rad delete -a <app> -e <env>` | Delete deployed resources |
| `rad workspace list` | List available workspaces |
| `rad workspace switch git` | Switch to Git workspace mode |
| `rad workspace switch <name>` | Switch to Control Plane workspace |

---

## Exit Codes

For CI/CD workflow control:

| Code | Meaning | Workflow Action |
|------|---------|-----------------|
| 0 | Success | Continue |
| 1 | General error | Fail job |
| 2 | Validation error | Fix configuration |
| 3 | Auth error | Check credentials |
| 4 | Resource conflict | Retry or manual intervention |
| 5 | Deployment failure | Review logs, fix, retry |

---

## Troubleshooting

### "Directory creation failed"

Ensure you have write permissions:

```bash
# Check permissions
ls -la .

# Or create in a different location
rad init ~/projects/my-project
```

### "Cannot connect to Kubernetes cluster"

Verify your kubeconfig and cluster access:

```bash
# Check current context
kubectl config current-context

# Test cluster connectivity
kubectl cluster-info

# Then install Radius
rad install kubernetes
```

### "Radius already installed"

To reinstall Radius on an existing cluster:

```bash
# Reinstall
rad install kubernetes --reinstall

# Or uninstall first
rad uninstall kubernetes
rad install kubernetes
```

### "Missing environment variables"

Ensure cloud credentials are set:

```bash
# AWS
export AWS_ACCESS_KEY_ID=...
export AWS_SECRET_ACCESS_KEY=...

# Azure
az login
# or for service principals:
export AZURE_CLIENT_ID=...
export AZURE_CLIENT_SECRET=...
export AZURE_TENANT_ID=...
```

### "Uncommitted changes in .radius/plan/"

Commit your plan before deploying:

```bash
git add .radius/plan/
git commit -m "rad plan: <environment>"
```

### "Unpinned recipe version"

Pin your recipes to specific versions in your recipe files (`*.recipes.yaml`):

```yaml
# Instead of:
recipeLocation: git::https://github.com/org/repo.git//path

# Use:
recipeLocation: git::https://github.com/org/repo.git//path?ref=v1.0.0
```

---

## Next Steps

- [Configure Multiple Environments](./environments.md)
- [Recipe Development Guide](./recipes.md)
- [Control Plane Migration](./migration.md)
