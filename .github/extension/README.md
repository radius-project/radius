# Repo Radius workflow assets

This folder holds the **Repo Radius** workflow templates that are written into a user's repository so that Radius can run on a GitHub Actions runner.

These files are templates: a copy is committed into the target repository under `.github/workflows/` and dispatched there. They are not run from this repository directly.

They live here so the workflow contract has a canonical, reviewed home that any frontend (the Copilot app, the CLI, etc.) can drive. See [radius-project/radius#12118](https://github.com/radius-project/radius/issues/12118) for background, and the [Repo Radius feature specification](https://github.com/radius-project/radius/pull/12078) for the end-to-end design.

## `radius-verify-credentials.yml`

The credential-verification workflow Radius uses to confirm that a GitHub Environment is wired up correctly before any application is deployed. The environment-setup flow generates this file, commits it to the target repo as `.github/workflows/radius-verify-credentials.yml`, and triggers it once per provider.

### What it does

The workflow runs on `ubuntu-latest` and branches by provider based on which variables are present on the selected GitHub Environment. Steps that target a provider are skipped when that provider's variables are empty, so the same file serves both Azure-only and AWS-only environments.

1. **Authenticate via OIDC.** For Azure it runs `azure/login`; for AWS it runs `aws-actions/configure-aws-credentials`. No long-lived cloud secrets are stored — the runner exchanges its GitHub OIDC token for short-lived cloud credentials.
2. **Verify access.** Azure runs `az account show`; AWS runs `aws sts get-caller-identity`. The AWS account ID in the caller ARN is masked in the logs.
3. **Discover resources.** Azure lists resource groups, AKS clusters, and physical locations; AWS lists EKS clusters, VPCs, and subnets. The results are written to `/tmp/radius-discovery.json` and uploaded as the `radius-discovery` artifact (one-day retention) for the caller to read back into the environment's variables.

### Trigger and permissions

- **Trigger:** `workflow_dispatch` with a single `environment` input (the GitHub Environment name, default `dev`). The job binds to that environment via `environment: ${{ inputs.environment }}`.
- **Permissions:** `id-token: write` (required for OIDC) and `contents: read`.

## `radius-deploy.yml`

The deploy workflow that runs Radius on demand: it stands up an ephemeral [k3d](https://k3d.io) control plane on the runner, restores persisted state, runs the caller's `rad` commands against the user's target cluster, then persists state again and tears the control plane down. The frontend generates this file, commits it to the target repo as `.github/workflows/radius-deploy.yml`, and dispatches it.

### What it does

1. **Authenticate via OIDC** (Azure `azure/login` or AWS `aws-actions/configure-aws-credentials`).
2. **Build a target-cluster kubeconfig** on the runner (`az aks get-credentials` / `aws eks get-token`) for the workload cluster named by `RADIUS_K8S_CLUSTER`.
3. **Create an ephemeral k3d control-plane cluster** and install the `rad` CLI.
4. **Create the `target-kubeconfig` secret** and **install Radius** with `database.enabled=true` (for state persistence) and, when a target cluster is configured, `global.targetCluster.enabled=true` — the chart seam that mounts the kubeconfig and sets `RADIUS_TARGET_KUBECONFIG` on `applications-rp`, `dynamic-rp`, and `bicep-de`.
5. **`rad startup`** restores the control-plane PostgreSQL databases and Terraform recipe state saved by the previous run.
6. **Create the Radius environment**, set its cloud **scope** with `rad env update`, and configure the provider's credentials (see [the credential contract](#cloud-credentials-provider-native) below).
7. **Run the caller's `rad` commands** (the `radius_commands` input), uploading each command's output as the `radius-output` artifact.
8. **`rad shutdown`** backs the control-plane and Terraform state up to the `radius-state` git orphan branch (runs even on failure).
9. **Tear down** the k3d cluster; collect logs on failure.

### Trigger, inputs, and permissions

- **Trigger:** `workflow_dispatch`.
- **Inputs:**

  | Input             | Required | Description                                                                                                                                                                                                 |
  |-------------------|----------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
  | `environment`     | Yes      | The GitHub Environment name, used as the Radius environment.                                                                                                                                                |
  | `radius_commands` | Yes      | A single `rad` CLI command string, or a JSON-encoded array of command strings run in order. The `rad` prefix is omitted from each command (e.g. `deploy app.bicep` or `["deploy app.bicep", "app graph"]`). |

- **Permissions:** `id-token: write` (OIDC), `contents: write` (so `rad shutdown` can push the `radius-state` branch), and `packages: write` (so container-image recipes can push to GHCR).

## The integration contract (owned by Radius)

These workflows depend on a stable seam that Radius owns and the multi-cluster v1 work ([#12106](https://github.com/radius-project/radius/pull/12106)) honors:

### Target cluster (`RADIUS_TARGET_KUBECONFIG`)

The workflow builds a kubeconfig for the external workload cluster on the runner and stores it as the `target-kubeconfig` secret in `radius-system`. Installing the chart with `--set global.targetCluster.enabled=true` mounts that secret into `applications-rp`, `dynamic-rp`, and `bicep-de` and sets `RADIUS_TARGET_KUBECONFIG` to the mounted path. Radius then directs recipe execution and directly-rendered output resources at that cluster; the Terraform kubernetes provider follows the same kubeconfig through the cluster access resolver. The secret's lifecycle (creation, EKS-token refresh, RBAC) is the workflow's responsibility, not the chart's.

### Cloud credentials (provider-native)

Repo Radius does not store long-lived cloud secrets; each provider uses its native OIDC model:

- **AWS** follows Terraform's model: the workflow does **not** run `rad credential register`. The OIDC session credentials obtained on the runner are injected into the `applications-rp`, `dynamic-rp`, and `bicep-de` pods as environment variables, and the AWS SDKs/Terraform AWS provider in those pods consume them.
- **Azure** uses **Workload Identity**. The chart is installed with `global.azureWorkloadIdentity.enabled=true` (which labels the RP/DE pods), the client/tenant IDs are registered with `rad credential register azure wi` (the Terraform azurerm provider fetches them from UCP), and the workflow projects the GitHub Actions OIDC JWT into the pods as the federated token file (`/var/run/secrets/azure/tokens/azure-identity-token`) that the Azure SDKs exchange with Entra for a short-lived AAD token.

In both cases `rad env update` records only the deployment **scope** (AWS region/account, Azure subscription/resource group), never credentials.

> **Azure token lifetime.** The GitHub Actions OIDC JWT is short-lived. The Azure SDKs exchange it once for an ~1-hour AAD token, so a normal run is unaffected, but a run whose Azure work outlives that window may fail. Refreshing the federated token mid-run is a planned fast follow and is intentionally out of scope.

### State persistence (`rad startup` / `rad shutdown`)

`rad startup` and `rad shutdown` are kind-agnostic CLI commands that back up and restore all durable Radius state (control-plane PostgreSQL + Terraform recipe-state Secrets) to a `radius-state` git orphan branch. They do not manage cluster lifecycle — the workflow owns creating and destroying the ephemeral control plane around them.

### Required GitHub Environment variables

The workflows read only Actions **variables** (`vars`), never secrets, for cloud configuration:

| Provider | Variables                                                                                                      |
|----------|----------------------------------------------------------------------------------------------------------------|
| Azure    | `AZURE_CLIENT_ID`, `AZURE_TENANT_ID`, `AZURE_SUBSCRIPTION_ID`, `AZURE_RESOURCE_GROUP`, `RADIUS_K8S_CLUSTER`    |
| AWS      | `AWS_IAM_ROLE_ARN`, `AWS_REGION`, `AWS_ACCOUNT_ID`, `RADIUS_K8S_CLUSTER`, `RADIUS_VPC_ID`, `RADIUS_SUBNET_IDS` |
| Common   | `RADIUS_K8S_NAMESPACE` (target namespace; defaults to `default`)                                               |

A provider's branch runs only when its identifying variable (`AZURE_CLIENT_ID` or `AWS_IAM_ROLE_ARN`) is non-empty. Configuring both on one environment is rejected.

### Cloud-side OIDC prerequisites

OIDC trust must already exist before either workflow can authenticate (this provisioning is **out of scope** for these templates):

- **Azure:** a federated credential on the Entra ID app whose subject is exactly `repo:<owner>/<repo>:environment:<environment-name>`, audience `api://AzureADTokenExchange`.
- **AWS:** an IAM role trust policy that allows `sts:AssumeRoleWithWebIdentity` from `token.actions.githubusercontent.com` with audience `sts.amazonaws.com` and subject `repo:<owner>/<repo>:environment:<environment-name>`.

For the full setup flow, see [docs/contributing/contributing-deploy-environments.md](../../docs/contributing/contributing-deploy-environments.md).
