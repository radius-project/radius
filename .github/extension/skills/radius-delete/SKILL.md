---
name: radius-delete
description: Delete a Radius application or environment via the auto-generated GitHub Actions workflow. Use when the user asks to delete, remove, or tear down a deployed Radius application or a Radius environment.
---

# Radius — Delete Application or Environment

Trigger the `Radius - Delete Application` or `Radius - Delete Environment` workflow. Each spins up an ephemeral k3d Radius control plane, connects to the target AKS/EKS cluster, restores persisted state, runs `rad app delete` / `rad env delete`, and persists the updated state again before tearing the control plane down. Deleting an application also deletes that application's resources (running their recipes' delete path against the target cluster and cloud).

## When to use this skill

- "Delete my app"
- "Remove application X from env Y"
- "Tear down the test environment"
- "Delete the environment"

## Prerequisites

Before invoking this skill, all of these must exist:
1. A GitHub Environment configured with cloud credentials → use the `radius-environment` skill if missing.
2. Previously persisted Radius state for that environment (i.e. the app/environment was deployed at least once). Delete restores that state to know what to delete.
3. Authenticated access to dispatch the workflow (e.g. a logged-in `gh` CLI, or a token with `actions: write` on the repo). The token only triggers the run; it is never passed into the workflow.

## How to invoke

Delete an application (`application` is the Radius application name):

```
POST /repos/{owner}/{repo}/actions/workflows/delete-application.yml/dispatches
{ "ref": "main", "inputs": { "environment": "<env-name>", "application": "<app-name>" } }
```

```bash
gh workflow run delete-application.yml -f environment=<env-name> -f application=<app-name>
```

Delete an environment (`environment_name` defaults to the GitHub Environment name, which the deploy flow uses as the Radius environment name):

```
POST /repos/{owner}/{repo}/actions/workflows/delete-environment.yml/dispatches
{ "ref": "main", "inputs": { "environment": "<env-name>", "environment_name": "<optional-radius-env-name>" } }
```

```bash
gh workflow run delete-environment.yml -f environment=<env-name> [-f environment_name=<radius-env-name>]
```

Then follow the run (`gh run watch` or the run URL) until it succeeds, fails, or times out. Each delete workflow is a dispatcher: it detects the environment's provider (from `AZURE_CLIENT_ID` / `AWS_ROLE_ARN`) and calls the matching reusable workflow (`delete-azure.yml` / `delete-aws.yml`), so the actual delete work runs as a called workflow underneath it.

## What the workflow does

1. The dispatcher detects the environment's provider and calls the matching provider delete workflow, which authenticates to that cloud via OIDC.
2. Fetches a kubeconfig for the target cluster, installs `k3d` + the `rad` CLI + Terraform, and installs Radius on the ephemeral control plane wired to the target cluster (same setup as deploy).
3. Projects GitHub OIDC tokens into the pods and registers the cloud identity with `rad credential register`, so recipe deletes can reach the target cluster and cloud.
4. Runs `rad startup` to restore the control-plane databases and Terraform recipe-state Secrets persisted by the previous run — this is what tells the delete which environment, recipe packs, resources, and Terraform state exist. Unlike deploy, it does **not** recreate the environment, recipe pack, or registry credentials.
5. Runs `rad app delete <name> --yes --preview` or `rad env delete <name> --yes --preview` (`--preview` selects the Radius.Core surface) via the `delete-resource` action, which writes a `rad-delete-result` artifact (JSON: `outcome`, `exitCode`, `resourceType`, `name`, `output`).
6. `rad shutdown` (`if: always()`) persists the post-delete control-plane databases and Terraform recipe-state Secrets back to the `radius-state` git orphan branch, so the next operation plans against the updated state. On failure, logs are uploaded as the `radius-logs` artifact; the k3d cluster is always deleted.

## After a successful delete

- Tell the user the delete succeeded and include the workflow run URL.
- Note that deleting an application removed the app's resources; deleting an environment removed the environment and its recipe-pack associations.

## Related files

- `.github/extension/delete-application.yml` and `.github/extension/delete-environment.yml` (this repo) — the delete dispatcher templates; copies are committed into the user repo at `.github/workflows/` and are the files that get dispatched.
- `.github/extension/delete-azure.yml` and `.github/extension/delete-aws.yml` — the provider-specific reusable (`workflow_call`) workflows the dispatchers call; committed alongside the dispatchers.
- `.github/extension/actions/*` — the shared composite actions (`setup-control-plane`, `restore-state`, `delete-resource`, `teardown`) the provider workflows reference from `radius-project/radius`; not copied into the user repo.
- `.github/extension/README.md` — the workflow contract: trigger/inputs, required `vars`, secrets, and prerequisites.
