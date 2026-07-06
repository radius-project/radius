# Repo Radius workflow assets

This folder holds the **Repo Radius** workflow templates that are written into a user's repository so that Radius can run on a GitHub Actions runner.

These files are templates: a copy is committed into the target repository under `.github/workflows/` and dispatched there. They are not run from this repository directly.

They live here so the workflow contract has a canonical, reviewed home that any frontend (the Copilot app, the CLI, etc.) can drive. See [radius-project/radius#12118](https://github.com/radius-project/radius/issues/12118) for background.

## Credential-verification workflows

Radius uses these workflows to confirm that a GitHub Environment is wired up correctly before any application is deployed. The environment-setup flow (see the `radius-environment` skill) generates the provider-specific file, commits it to the target repo under `.github/workflows/`, and triggers it.

There is one workflow per provider:

- **`verify-azure.yml`** — verifies Azure OIDC login and AKS cluster access.
- **`verify-aws.yml`** — verifies AWS OIDC login and EKS cluster access.

### What they do

Each workflow runs on `ubuntu-latest`. No long-lived cloud secrets are stored — the runner exchanges its GitHub OIDC token for short-lived cloud credentials.

**`verify-azure.yml`:**

1. **Authenticate via OIDC.** Runs `azure/login` with the environment's Azure variables.
2. **Verify access.** Runs `az account show` to confirm the login.
3. **Verify AKS access.** Sets up `kubelogin`, fetches cluster credentials with `az aks get-credentials`, converts the kubeconfig, and runs `kubectl cluster-info`.
4. **Summary.** Writes the environment and provider to the job summary.

**`verify-aws.yml`:**

1. **Authenticate via OIDC.** Runs `aws-actions/configure-aws-credentials` with the environment's AWS variables.
2. **Verify access.** Runs `aws sts get-caller-identity` to confirm the login.
3. **Verify EKS access.** Updates the kubeconfig with `aws eks update-kubeconfig` and runs `kubectl cluster-info`.
4. **Summary.** Writes the environment and provider to the job summary.

### Trigger and permissions

- **Trigger:** `workflow_dispatch` with a single `environment` input (the GitHub Environment name). The job binds to that environment via `environment: ${{ inputs.environment }}`.
- **Permissions:** `id-token: write` (required for OIDC) and `contents: read`.

### Required environment variables

The workflows read only GitHub Actions **variables** (`vars`), never secrets. Configure these on the target GitHub Environment:

| Provider | Variables |
|---|---|
| Azure | `AZURE_CLIENT_ID`, `AZURE_TENANT_ID`, `AZURE_SUBSCRIPTION_ID`, `AZURE_RESOURCE_GROUP`, `AZURE_AKS_CLUSTER_NAME` |
| AWS | `AWS_ROLE_ARN`, `AWS_REGION`, `AWS_EKS_CLUSTER_NAME` |

### Prerequisites on the cloud side

OIDC trust must already exist before the workflow can authenticate:

- **Azure:** a federated credential on the AAD app whose subject is exactly `repo:<owner>/<repo>:environment:<environment-name>`, audience `api://AzureADTokenExchange`.
- **AWS:** an IAM role trust policy that allows `sts:AssumeRoleWithWebIdentity` from `token.actions.githubusercontent.com` with audience `sts.amazonaws.com` and subject `repo:<owner>/<repo>:environment:<environment-name>`.

For the full setup flow and troubleshooting, see the `radius-environment` skill.
