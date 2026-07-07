# Setting up a Radius deploy environment

## Purpose

This document is the prose walkthrough for configuring a **GitHub Environment** so Radius can deploy applications from that repository to AWS or Azure. It describes the same end-to-end workflow the [`radius-environment`](../../.github/skills/radius-environment/SKILL.md) skill automates, so any contributor (or any agent) can follow it without the skill wrapper.

A Radius deploy environment is a GitHub Actions Environment whose **variables** (`vars`) carry the cloud coordinates Radius needs (region, cluster name, account/subscription identifiers, etc.) and whose **OIDC trust** allows the runner to obtain short-lived cloud credentials. No long-lived cloud secrets are stored on GitHub.

## Prerequisites

- A GitHub repository where you can create/edit Environments and push to `.github/workflows/`.
- For **AWS**: an EKS cluster, an IAM role configured for GitHub OIDC, and (if your app uses `Radius.Data/mySqlDatabases`) a VPC plus subnet IDs.
- For **Azure**: an AAD application configured for federated credentials, an Azure subscription, a resource group, and an AKS cluster.
- A GitHub PAT with `workflow` scope (so the verification workflow file can be created or updated) — run `gh auth refresh -s workflow` if needed.

## Steps

### 1. Create the GitHub Environment

In the target repository, go to **Settings → Environments → New environment** and pick a name (for example `dev` or `staging`). The environment name is part of the OIDC subject the cloud side will trust.

### 2. Set the required environment variables

The credential-verification workflow reads only Actions **variables**, never secrets.

**AWS** — read by `verify-aws.yml`:

- `AWS_ROLE_ARN` — ARN of the IAM role the runner assumes via OIDC.
- `AWS_REGION` — AWS region (e.g. `us-west-2`).
- `AWS_EKS_CLUSTER_NAME` — name of the EKS cluster the workflow verifies access to.

**AWS** — also set for deploys (not read by verification):

- `AWS_ACCOUNT_ID`, `RADIUS_VPC_ID`, `RADIUS_SUBNET_IDS`.

**Azure** — read by `verify-azure.yml`:

- `AZURE_CLIENT_ID` — AAD application (client) ID.
- `AZURE_TENANT_ID` — Azure tenant ID.
- `AZURE_SUBSCRIPTION_ID` — Azure subscription ID.
- `AZURE_RESOURCE_GROUP` — resource group holding the AKS cluster.
- `AZURE_AKS_CLUSTER_NAME` — name of the AKS cluster the workflow verifies access to.

### 3. Configure the cloud-side OIDC trust

- **Azure:** add a federated credential to the AAD app whose subject is exactly `repo:<owner>/<repo>:environment:<environment-name>`, audience `api://AzureADTokenExchange`.
- **AWS:** add a trust policy to the IAM role that allows `sts:AssumeRoleWithWebIdentity` from `token.actions.githubusercontent.com` with audience `sts.amazonaws.com` and a subject of `repo:<owner>/<repo>:environment:<environment-name>`.

### 4. Add the credential-verification workflow

Copy the provider's template from `.github/extension/` into the target repository at `.github/workflows/verify-<provider>.yml` — [`verify-aws.yml`](../../.github/extension/verify-aws.yml) for AWS or [`verify-azure.yml`](../../.github/extension/verify-azure.yml) for Azure (both are named `Radius - Verify Credentials`). See [`.github/extension/README.md`](../../.github/extension/README.md) for the contract between the workflow and any frontend that drives it.

> **Note:** the template's `environment` input has a `default: '{{ENV}}'` placeholder that the canvas/skill substitutes automatically. When copying the file by hand, replace `{{ENV}}` with your environment name (or delete the `default:` line).

### 5. Dispatch the workflow

Trigger the workflow with `workflow_dispatch`, passing the environment name as the `environment` input. The workflow:

1. Authenticates via OIDC (Azure: `azure/login`; AWS: `aws-actions/configure-aws-credentials`).
2. Verifies cloud access (Azure: `az account show`; AWS: `aws sts get-caller-identity`).
3. Verifies cluster access (Azure: `az aks get-credentials` + `kubelogin convert-kubeconfig` + `kubectl cluster-info`; AWS: `aws eks update-kubeconfig` + `kubectl cluster-info`).

Once the run is green, the environment is ready for Radius to deploy into.

## Verification

- The verification workflow run finishes with a green check on every step.
- The credential step prints the active subscription (Azure) or the caller ARN (AWS).
- The cluster step prints `kubectl cluster-info` output for the configured AKS/EKS cluster.
- The environment shows all required variables populated for its provider.

## Troubleshooting

- **`refusing to allow an OAuth App to create or update workflow .github/workflows/verify-<provider>.yml without 'workflow' scope`** — the PAT lacks the `workflow` scope. Run `gh auth refresh -s workflow`.
- **"Workflow dispatch accepted, but no new run appeared after 30s"** — GitHub usually hasn't finished indexing a just-pushed workflow. Retry the dispatch with backoff or check the Actions tab in the browser.
- **Azure OIDC fails with `AADSTS70021: No matching federated identity record found`** — the federated credential subject on the AAD app does not match. It must be exactly `repo:<owner>/<repo>:environment:<environment-name>`.
- **AWS OIDC fails with `Not authorized to perform sts:AssumeRoleWithWebIdentity`** — the IAM role trust policy is missing or uses the wrong audience. Audience must be `sts.amazonaws.com`, with the subject condition `token.actions.githubusercontent.com:sub == repo:<owner>/<repo>:environment:<environment-name>`.

## Related

- [`.github/skills/radius-environment/SKILL.md`](../../.github/skills/radius-environment/SKILL.md) — the agent-invocable wrapper over this workflow.
- [`.github/extension/README.md`](../../.github/extension/README.md) — the canonical workflow template and its contract with frontends.
