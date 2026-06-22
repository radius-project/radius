---
name: radius-environment
description: Create and verify a Radius deploy environment (AWS or Azure) for a GitHub repository. Use when the user asks to set up, configure, verify, or troubleshoot a Radius environment, cloud credentials, or the OIDC trust between GitHub Actions and AWS/Azure.
---

# Radius — Environment Setup

Create a GitHub Environment configured with the cloud credentials (variables + secrets) Radius needs to deploy applications. Supports AWS (OIDC via IAM Role) and Azure (OIDC via Workload Identity).

## When to use this skill

- "Create a new Radius environment named X"
- "Set up Azure credentials for deploys"
- "Configure AWS for my Radius app"
- "Verify my deploy environment works"
- "Why is the verification workflow failing?"
- "Add a new environment 'staging' pointing at my AKS cluster"

## Flow

The canvas drives a 2-step wizard per provider:

### AWS
1. **Form inputs**: env name, IAM Role ARN, AWS region, account ID, EKS cluster name, optional VPC + subnet IDs (required if the app uses `Radius.Data/mySqlDatabases`).
2. **Credential verification**: commits/updates `.github/workflows/radius-verify-credentials.yml` and dispatches it. The workflow logs into AWS via OIDC and runs `aws sts get-caller-identity`. Status is polled and shown live.
3. **Dependency discovery**: same workflow then runs `aws eks list-clusters` and `aws ec2 describe-vpcs/describe-subnets`, writes results to the `radius-discovery` artifact (one-day retention), and the canvas reads it back to write discovered values as env vars.

### Azure
1. **Form inputs**: env name, AAD App (client) ID, tenant ID, subscription ID, resource group, optional AKS cluster name.
2. **Credential verification**: dispatches the same verify workflow which runs `azure/login` via OIDC and `az account show`.
3. **Dependency discovery**: same workflow runs `az group list`, `az aks list`, and `az account list-locations`, writes results to the `radius-discovery` artifact, and the canvas reads it back to write discovered values as env vars.

## How to invoke

When the user asks to create or set up a Radius environment, **open the canvas straight to the environment wizard**:

```
open_canvas({
  canvasId: "radius",
  instanceId: "radius-env-<provider>",
  input: { owner, repo, view: "env", provider: "aws" | "azure" }
})
```

(Use a fresh `instanceId` per provider so the host doesn't refocus a stale graph/hub URL.)

The popup lands directly on the create-environment form for the chosen provider. No navigation needed.

## Required secrets / variables on the GitHub Environment

The verification workflow reads only GitHub Actions **variables** (`vars`), never secrets. OIDC eliminates the need to store long-lived cloud credentials.

**AWS** — required for verification:
- `AWS_IAM_ROLE_ARN` — ARN of the IAM role the runner assumes via OIDC
- `AWS_REGION` — AWS region (e.g. `us-west-2`)

**AWS** — written back by the canvas after discovery:
- `AWS_ACCOUNT_ID`, `RADIUS_K8S_CLUSTER`, `RADIUS_VPC_ID`, `RADIUS_SUBNET_IDS`

**Azure** — required for verification:
- `AZURE_CLIENT_ID` — AAD application (client) ID
- `AZURE_TENANT_ID` — Azure tenant ID
- `AZURE_SUBSCRIPTION_ID` — Azure subscription ID

**Azure** — written back by the canvas after discovery:
- `AZURE_RESOURCE_GROUP`, `RADIUS_K8S_CLUSTER`

The OIDC trust must already exist on the cloud side before the workflow can authenticate (see Prerequisites below).

## Prerequisites on the cloud side

- **Azure:** a federated credential on the AAD app whose subject is exactly `repo:<owner>/<repo>:environment:<environment-name>`, audience `api://AzureADTokenExchange`.
- **AWS:** an IAM role trust policy that allows `sts:AssumeRoleWithWebIdentity` from `token.actions.githubusercontent.com` with audience `sts.amazonaws.com` and subject `repo:<owner>/<repo>:environment:<environment-name>`.

## Common errors and fixes

- **`refusing to allow an OAuth App to create or update workflow .github/workflows/radius-verify-credentials.yml without 'workflow' scope`** — the PAT lacks `workflow` scope. Run `gh auth refresh -s workflow` (the extension auto-prefers a `gh auth token` over `$GITHUB_TOKEN`).
- **"Workflow dispatch accepted, but no new run appeared after 30s"** — usually means GitHub hasn't indexed the just-pushed workflow yet. The extension already retries dispatch with backoff; if it still fails, check the Actions tab in the browser.
- **Azure OIDC fails with `AADSTS70021: No matching federated identity record found`** — the federated credential subject on the AAD app doesn't match. Subject must be exactly `repo:<owner>/<repo>:environment:<env-name>`.
- **AWS OIDC fails with `Not authorized to perform sts:AssumeRoleWithWebIdentity`** — IAM role trust policy missing or wrong audience. Audience should be `sts.amazonaws.com`, condition on `token.actions.githubusercontent.com:sub == repo:<owner>/<repo>:environment:<env-name>`.

## Verifying after creation

After the canvas reports success, the new env appears in the **Envs ▾** dropdown tagged with its provider (AWS/AZURE). The hub's deploy button enables once both an Application and Environment are selected.

## Related files

- `.github/extension/radius-verify-credentials.yml` — the canonical workflow template in this repo; the canvas commits a copy of this file into the target user repo at `.github/workflows/radius-verify-credentials.yml` and dispatches it.
- `.github/extension/README.md` — documents the template directory and the contract between the workflow and the canvas.
