---
name: radius-deploy
description: Deploy a Radius application to a configured environment via the auto-generated GitHub Actions workflow. Use when the user asks to deploy, redeploy, trigger a deployment, or troubleshoot a failed Radius deploy.
---

# Radius — Deploy Application

Trigger the `Radius - Run rad Commands` workflow which spins up an ephemeral k3d Radius control plane, connects to the target AKS/EKS cluster, registers the right recipes for the env's provider, restores persisted state, runs the requested `rad` commands (deploying by default), and persists state again before tearing the control plane down.

## When to use this skill

- "Deploy my app"
- "Redeploy to the test environment"
- "Trigger a deploy"
- "Why did my deploy fail?"
- "Deploy app X to env Y"

## Prerequisites

Before invoking this skill, all of these must exist:
1. A GitHub Environment configured with cloud credentials → use the `radius-environment` skill if missing.
2. A `.radius/app.bicep` file → use the `radius-app-bicep` skill if missing.
3. Authenticated access to dispatch the workflow (e.g. a logged-in `gh` CLI, or a token with `actions: write` on the repo). The token only triggers the run; it is never passed into the workflow.

## How to invoke

Trigger the workflow via the GitHub API (or `gh`). Omit `rad_commands` to run the default `rad deploy` of `.radius/app.bicep`:

```
POST /repos/{owner}/{repo}/actions/workflows/run-rad-commands.yml/dispatches
{ "ref": "main", "inputs": { "environment": "<env-name>", "image": "<optional-image>" } }
```

```bash
gh workflow run run-rad-commands.yml -f environment=<env-name> [-f image=<optional-image>]
```

Then follow the run (`gh run watch` or the run URL) until it succeeds, fails, or times out. `run-rad-commands.yml` is a dispatcher: it detects the environment's provider and calls the matching reusable workflow (`run-rad-commands-azure.yml` / `run-rad-commands-aws.yml`), so the actual deploy work runs as a called workflow underneath it.

## What the workflow does

1. The dispatcher detects the environment's provider (from `AZURE_CLIENT_ID` / `AWS_ROLE_ARN`) and calls the matching provider workflow, which authenticates to that cloud via OIDC.
2. Fetches a kubeconfig for the target cluster into `RADIUS_TARGET_KUBECONFIG` (EKS via `aws eks describe-cluster` + a static bearer-token kubeconfig; AKS via `az aks get-credentials`).
3. Installs `k3d`, creates the ephemeral `radius-cp` cluster, and installs the `rad` CLI (edge) and Terraform.
4. When a target kubeconfig exists, creates the `target-kubeconfig` secret and installs Radius with `--set global.targetCluster.enabled=true` (plus `--set database.enabled=true` for state backup/restore and `--set dynamicrp.buildkit.enabled=true` for in-pod image builds). The chart mounts the secret into `applications-rp`, `dynamic-rp`, and `bicep-de` and sets `RADIUS_TARGET_KUBECONFIG` so recipes and directly-rendered resources target the external cluster. Without a target kubeconfig, resources deploy to the k3d control plane. The Terraform state backend stays on the control plane.
5. Projects GitHub OIDC tokens into the pods and registers the cloud identity with `rad credential register` (`aws irsa` / `azure wi`).
6. Refreshes the (short-lived EKS) target token, updates the `target-kubeconfig` secret, and restarts the recipe-executing pods so they re-read it.
7. Creates the CLI workspace/group, then runs `rad startup` to restore the control-plane databases and Terraform recipe-state Secrets saved by the previous run (a no-op on the first run), clones `radius-project/resource-types-contrib@main` (or the ref in `RESOURCE_TYPES_CONTRIB_REF`), registers the `Radius.Compute/containerImages` resource type, and builds its local Bicep extension.
8. Deploys a `Radius.Core/environments` resource and recipe pack. The pack bundles the Kubernetes recipes (`containers`, `containerImages`, `persistentVolumes`, `routes`, `postgreSqlDatabases`, `secrets`) plus a provider-gated `mySqlDatabases` recipe (AWS RDS vs Azure Flexible Server).
9. Creates registry credentials for image builds, then runs `rad deploy` on `.radius/app.bicep` (passing the `image` parameter, and any application parameters from the `RADIUS_DEPLOY_PARAMS` secret when set). Afterwards `rad shutdown` (`if: always()`) backs the control-plane databases and Terraform recipe-state Secrets up to the `radius-state` git orphan branch. On failure, logs are uploaded as the `radius-logs` artifact; the k3d cluster is always deleted.

## Common failure modes

- **`RecipeDeploymentFailed` with `the resource with id '/planes/aws/aws/providers/System.AWS/credentials/default' was not found`**
  → The `mySqlDatabases` recipe was registered for the wrong provider. The fix lives in the `Create Radius environment and recipe pack` step: an AWS env uses `recipes/aws/terraform`, an Azure env uses `recipes/azure/terraform`. If you see this error, the committed workflow is stale — re-commit the updated workflow and re-trigger the deploy.

- **`RecipeDownloadFailed` with `subdir not found`**
  → The recipe path doesn't exist on the configured `RESOURCE_TYPES_CONTRIB_REF` branch. Check the actual layout in `radius-project/resource-types-contrib` for that branch. `mySqlDatabases` specifically has **no** `recipes/kubernetes/terraform` directory — only aws, azure, and a kubernetes/bicep variant.

- **Workflow runs but pod never reaches Ready**
  → Look at the `Install Radius on control plane` and `Refresh external deployment target credentials` steps in the run logs. Usually a target cluster kubeconfig issue (expired EKS token, AKS network restriction).

## After a successful deploy

- Tell the user the deploy succeeded and include the workflow run URL.

## Related files

- `.github/extension/run-rad-commands.yml` (this repo) — the unified dispatcher template; a copy is committed into the user repo at `.github/workflows/run-rad-commands.yml` and is the file that gets dispatched.
- `.github/extension/run-rad-commands-azure.yml` and `.github/extension/run-rad-commands-aws.yml` — the provider-specific reusable (`workflow_call`) workflows the dispatcher calls; committed alongside the dispatcher.
- `.github/extension/actions/*` — the shared composite actions (`setup-control-plane`, `restore-state`, `register-resource-types`, `run-rad-commands`, `teardown`) the provider workflows reference from `radius-project/radius`; not copied into the user repo.
- `.github/extension/README.md` — the workflow contract: trigger/inputs, required `vars`, secrets, and prerequisites.
