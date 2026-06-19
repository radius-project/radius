# Repo Radius workflow assets

This folder holds the **Repo Radius** workflow templates that are written into a user's repository so that Radius can run on a GitHub Actions runner.

These files are templates: a copy is committed into the target repository under `.github/workflows/` and dispatched there. They are not run from this repository directly.

They live here so the workflow contract has a canonical, reviewed home that any frontend (the Copilot app, the CLI, etc.) can drive. See [radius-project/radius#12118](https://github.com/radius-project/radius/issues/12118) for background.

## `radius-verify-credentials.yml`

The credential-verification workflow Radius uses to confirm that a GitHub Environment is wired up correctly before any application is deployed. The environment-setup flow (see the `radius-environment` skill) generates this file, commits it to the target repo as `.github/workflows/radius-verify-credentials.yml`, and triggers it once per provider.

### What it does

The workflow runs on `ubuntu-latest` and branches by provider based on which variables are present on the selected GitHub Environment. Steps that target a provider are skipped when that provider's variables are empty, so the same file serves both Azure-only and AWS-only environments.

1. **Authenticate via OIDC.** For Azure it runs `azure/login`; for AWS it runs `aws-actions/configure-aws-credentials`. No long-lived cloud secrets are stored — the runner exchanges its GitHub OIDC token for short-lived cloud credentials.
2. **Verify access.** Azure runs `az account show`; AWS runs `aws sts get-caller-identity`. The AWS account ID in the caller ARN is masked in the logs.
3. **Discover resources.** Azure lists resource groups, AKS clusters, and physical locations; AWS lists EKS clusters, VPCs, and subnets. The results are written to `/tmp/radius-discovery.json` and uploaded as the `radius-discovery` artifact (one-day retention) for the caller to read back into the environment's variables.

### Trigger and permissions

- **Trigger:** `workflow_dispatch` with a single `environment` input (the GitHub Environment name, default `dev`). The job binds to that environment via `environment: ${{ inputs.environment }}`.
- **Permissions:** `id-token: write` (required for OIDC) and `contents: read`.

### Required environment variables

The workflow reads only GitHub Actions **variables** (`vars`), never secrets. Configure these on the target GitHub Environment:

| Provider | Variables |
|---|---|
| Azure | `AZURE_CLIENT_ID`, `AZURE_TENANT_ID`, `AZURE_SUBSCRIPTION_ID` |
| AWS | `AWS_IAM_ROLE_ARN`, `AWS_REGION` |

The provider's branch runs only when its identifying variable (`AZURE_CLIENT_ID` or `AWS_IAM_ROLE_ARN`) is non-empty.

### Prerequisites on the cloud side

OIDC trust must already exist before the workflow can authenticate:

- **Azure:** a federated credential on the AAD app whose subject is exactly `repo:<owner>/<repo>:environment:<environment-name>`, audience `api://AzureADTokenExchange`.
- **AWS:** an IAM role trust policy that allows `sts:AssumeRoleWithWebIdentity` from `token.actions.githubusercontent.com` with audience `sts.amazonaws.com` and subject `repo:<owner>/<repo>:environment:<environment-name>`.

For the full setup flow, troubleshooting, and the variables the discovery step writes back, see the `radius-environment` skill.
