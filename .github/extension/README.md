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
| AWS | `AWS_ROLE_ARN`, `AWS_REGION` |

The provider's branch runs only when its identifying variable (`AZURE_CLIENT_ID` or `AWS_ROLE_ARN`) is non-empty.

### Prerequisites on the cloud side

OIDC trust must already exist before the workflow can authenticate:

- **Azure:** a federated credential on the AAD app whose subject is exactly `repo:<owner>/<repo>:environment:<environment-name>`, audience `api://AzureADTokenExchange`.
- **AWS:** an IAM role trust policy that allows `sts:AssumeRoleWithWebIdentity` from `token.actions.githubusercontent.com` with audience `sts.amazonaws.com` and subject `repo:<owner>/<repo>:environment:<environment-name>`.

For the full setup flow, troubleshooting, and the variables the discovery step writes back, see the `radius-environment` skill.

## `radius-run-rad-commands.yml`

The run-rad-commands workflow Radius uses to run one or more `rad` CLI commands on demand against a user's target cluster — deploying by default, but able to run any allowed command (`deploy`, `app graph`, `app delete`, and so on). It stands up an ephemeral [k3d](https://k3d.io) control plane on the runner, restores persisted state, runs the requested commands, then persists state again and tears the control plane down. The deploy flow (see the `radius-deploy` skill) generates this file, commits it to the target repo as `.github/workflows/radius-run-rad-commands.yml`, and dispatches it.

### What it does

The workflow runs on `ubuntu-latest`. It stands up an ephemeral [k3d](https://k3d.io) cluster to host the Radius control plane on the runner, points that control plane at the user's existing EKS/AKS cluster, and deploys the application there. As with `radius-verify-credentials.yml`, steps that target a provider are skipped when that provider's variables are empty, so the same file serves both Azure and AWS environments.

1. **Authenticate via OIDC.** Runs `azure/login` and/or `aws-actions/configure-aws-credentials` so the runner exchanges its GitHub OIDC token for short-lived cloud credentials. No long-lived cloud secrets are stored.
2. **Connect to the target cluster.** Fetches a kubeconfig for the user's EKS/AKS cluster (`AWS_EKS_CLUSTER_NAME` / `AZURE_AKS_CLUSTER_NAME`). For EKS it ensures an access entry for the IAM role and writes a static, token-based kubeconfig. The path is exported as `RADIUS_TARGET_KUBECONFIG` for later steps.
3. **Create the ephemeral control plane.** Installs k3d and creates the `radius-cp` cluster, then installs the `rad` CLI (edge) and the Radius control plane via `rad install kubernetes` with `database.enabled=true` so durable state is held in the control-plane PostgreSQL.
4. **Enable the target-cluster seam.** Creates the `target-kubeconfig` secret in `radius-system` and, when a target kubeconfig is present, installs Radius with `--set global.targetCluster.enabled=true`. The chart mounts the secret into `applications-rp`, `dynamic-rp`, and `bicep-de` and sets `RADIUS_TARGET_KUBECONFIG`, so recipe execution and directly-rendered resources land on the external cluster. The Terraform state backend deliberately stays on the control-plane cluster.
5. **Restore persisted state (`rad startup`).** Waits for the control-plane PostgreSQL to be ready, then restores the control-plane databases and the Terraform recipe-state Secrets saved by the previous run, so `rad deploy` plans against prior state rather than an empty backend.
6. **Project cloud OIDC tokens.** Mints GitHub OIDC tokens and mounts them at the fixed paths Radius and the Terraform providers read for the federated token exchange, then registers the cloud identity with `rad credential register`.
7. **Register resource types and recipes.** Clones [resource-types-contrib](https://github.com/radius-project/resource-types-contrib), registers the `Radius.Compute/containerImages` type, builds its local Bicep extension, and creates the environment and a provider-specific Terraform recipe pack.
8. **Run the requested rad commands.** Validates each command in `rad_commands` against the allowed-command set, then runs them in order (stopping on the first failure) and writes a combined `rad-commands-result` artifact. When `rad_commands` is empty it runs the default `rad deploy .radius/app.bicep`, passing the `image` parameter (the `image` input, defaulting to the commit SHA) and, when set, the `password` parameter from the `RADIUS_DB_PASSWORD` secret. It first ensures the `radiusCompute` extension is imported into `.radius/app.bicep`.
9. **Persist state (`rad shutdown`).** Backs the control-plane databases and Terraform recipe-state Secrets up to the `radius-state` git orphan branch. This runs even when the deploy fails (`if: always()`), so a partially-applied Terraform run is not lost.
10. **Tear down.** Deletes the ephemeral `radius-cp` cluster; on failure, Radius and application logs are collected and uploaded as the `radius-logs` artifact (three-day retention).

### Trigger and permissions

- **Trigger:** `workflow_dispatch` with an `environment` input (the GitHub Environment name) plus optional `image` and `rad_commands` inputs. The job binds to that environment via `environment: ${{ inputs.environment }}`.
- **Inputs:**

  | Input | Required | Description |
  |---|---|---|
  | `environment` | Yes | The GitHub Environment name, used as the Radius environment. |
  | `image` | No | Container image for the application, passed to the default deploy as the `image` parameter. Defaults to the commit SHA (`github.sha`) when unset. |
  | `rad_commands` | No | A single `rad` command string, or a JSON array of command strings run in order (the `rad` prefix omitted, e.g. `["deploy .radius/app.bicep --environment dev", "app graph my-app -o json"]`). Each command is validated against the allowed-command set. When empty, the workflow runs its default `rad deploy` of `.radius/app.bicep`. |

- **Outputs:** a combined `rad-commands-result` artifact — a JSON document with a top-level `outcome`/`exitCode` and a `commands` array (one entry per command, in input order, with each command's exit code and output).
- **Permissions:** `id-token: write` (required for OIDC), `contents: write` (so `rad shutdown` can push the `radius-state` branch), and `packages: write` (to push the application image built by the containerImages recipe).

### Required environment variables

The workflow reads cloud and cluster configuration from GitHub Actions **variables** (`vars`). Configure these on the target GitHub Environment:

| Provider | Required | Optional |
|---|---|---|
| Common | — | `KUBERNETES_NAMESPACE` (default `default`), `RADIUS_BUILD_REGISTRY` (default `ghcr.io/<owner>`) |
| Azure | `AZURE_CLIENT_ID`, `AZURE_TENANT_ID`, `AZURE_SUBSCRIPTION_ID`, `AZURE_RESOURCE_GROUP`, `AZURE_AKS_CLUSTER_NAME` | `AZURE_LOCATION` (default `eastus`) |
| AWS | `AWS_ROLE_ARN`, `AWS_REGION`, `AWS_ACCOUNT_ID`, `AWS_EKS_CLUSTER_NAME`, `RADIUS_VPC_ID`, `RADIUS_SUBNET_IDS` | — |

The provider's branch runs only when its identifying variable (`AZURE_CLIENT_ID` or `AWS_ROLE_ARN`) is non-empty. When neither is set, resources deploy to the ephemeral control-plane cluster instead of an external target.

Unlike `radius-verify-credentials.yml`, this workflow also reads a few GitHub Actions **secrets** for image push and application configuration:

| Secret | Purpose |
|---|---|
| `GHCR_PAT` | Optional. Registry token for pushing images built by the containerImages recipe. Falls back to the built-in `GITHUB_TOKEN` when unset. |
| `RADIUS_DB_PASSWORD` | Optional. Passed to a `rad deploy` command as the `password` parameter when the application requires it. |

### State persistence (`rad startup` / `rad shutdown`)

`rad startup` and `rad shutdown` are kind-agnostic CLI commands that restore and back up all durable Radius state (control-plane PostgreSQL + Terraform recipe-state Secrets) to a `radius-state` git orphan branch. They do not manage cluster lifecycle — the workflow owns creating and destroying the ephemeral control plane around them. `rad startup` runs after the install (so `rad deploy` plans against prior state) and `rad shutdown` runs after the deploy with `if: always()` (so state survives a failed deploy).

### Prerequisites

- The same OIDC trust required by `radius-verify-credentials.yml` (federated credential on Azure, IAM role trust policy on AWS). Run that workflow first to confirm the environment is wired up correctly.
- The target cluster (`AWS_EKS_CLUSTER_NAME` / `AZURE_AKS_CLUSTER_NAME`) must already exist and be reachable; the assumed identity needs cluster-admin-level access to it.
- The application must define `.radius/app.bicep` in the target repo.

For the full deploy flow and troubleshooting, see the `radius-deploy` skill.
