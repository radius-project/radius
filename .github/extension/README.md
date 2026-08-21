# Repo Radius workflow assets

This folder holds the **Repo Radius** workflow templates that are written into a user's repository so that Radius can run on a GitHub Actions runner.

These files are templates: a copy is committed into the target repository under `.github/workflows/` and dispatched there. They are not run from this repository directly.

They live here so the workflow contract has a canonical, reviewed home that any frontend (the Copilot app, the CLI, etc.) can drive. See [radius-project/radius#12118](https://github.com/radius-project/radius/issues/12118) for background.

## Credential-verification workflows

Radius uses these workflows to confirm that a GitHub Environment is wired up correctly before any application is deployed. The environment-setup flow generates the provider-specific file, commits it to the target repo under `.github/workflows/`, and triggers it.

There is one workflow per provider:

- **`verify-azure.yml`** — verifies Azure OIDC login and AKS cluster access.
- **`verify-aws.yml`** — verifies AWS OIDC login and EKS cluster access.

### What they do

Each workflow runs on `ubuntu-24.04`. No long-lived cloud secrets are stored — the runner exchanges its GitHub OIDC token for short-lived cloud credentials.

**`verify-azure.yml`:**

1. **Inspect OIDC token claims (diagnostic breadcrumb).** Decodes the runner's GitHub OIDC token locally (the token is never printed) and logs the claims — `repository`, `repository_owner`, `sub`, `enterprise`, `aud` — to the collapsed step log. It writes nothing to the job summary, so a successful run shows no extra output. This step never fails the job.
2. **Authenticate via OIDC.** Runs `azure/login` with the environment's Azure variables.
3. **Report possible enterprise-claim mismatch (failure-only).** Runs only when Azure Login failed and the OIDC token's `enterprise` claim was empty. That combination is a hint, not a diagnosis — Azure Login can fail for unrelated reasons — so the job summary asks the user to confirm by checking whether the Azure Login error contains `AADSTS7002381` before applying the suggested fix. On a successful run it is skipped and adds nothing.
4. **Verify access.** Runs `az account show` to confirm the login.
5. **Verify AKS access.** Sets up `kubelogin`, fetches cluster credentials with `az aks get-credentials`, converts the kubeconfig, and runs `kubectl cluster-info`.
6. **Verify GHCR package push permission.** Confirms the workflow token can push to the OCI state archive package (`RADIUS_STATE_REGISTRY`) that `rad startup` / `rad shutdown` write to. See [GHCR package push check](#ghcr-package-push-check).
7. **Summary.** Writes the environment, provider, and GHCR check result to the job summary.

**`verify-aws.yml`:**

1. **Authenticate via OIDC.** Runs `aws-actions/configure-aws-credentials` with the environment's AWS variables.
2. **Verify access.** Runs `aws sts get-caller-identity` to confirm the login.
3. **Verify EKS access.** Updates the kubeconfig with `aws eks update-kubeconfig` and runs `kubectl cluster-info`.
4. **Verify GHCR package push permission.** Confirms the workflow token can push to the OCI state archive package (`RADIUS_STATE_REGISTRY`) that `rad startup` / `rad shutdown` write to. See [GHCR package push check](#ghcr-package-push-check).
5. **Summary.** Writes the environment, provider, and GHCR check result to the job summary.

### GHCR package push check

Radius stores its control-plane state as an OCI archive in GHCR (`RADIUS_STATE_REGISTRY`). During a deploy, `rad startup` and `rad shutdown` push to that package, and GHCR rejects the push with a `403 Forbidden` when the workflow token lacks write access to it — a permission that is **not granted by default** for packages not linked to the repo. To surface that early instead of mid-deploy, the check asks GHCR's Docker v2 token endpoint for a `pull,push`-scoped bearer token and uses it to start an empty blob upload session instead of decoding the token. A `202 Accepted` response proves that GHCR authorized a push-required operation without creating a blob, manifest, tag, or package version. The check reports `401 Unauthorized` and `403 Forbidden` as missing package access, reports other failures without claiming permission was denied, skips when `RADIUS_STATE_REGISTRY` is unset or not on `ghcr.io`, and mirrors the deploy workflow's `packages: write` token so it tests the same capability the deploy relies on.

The check lives in the shared [`verify-ghcr-push`](actions/verify-ghcr-push/action.yml) composite action so the two verify templates cannot drift; both workflows reference it from `radius-project/radius` at the `{{RADIUS_REF}}` pinned ref (the same mechanism the run-rad-commands provider workflows use), so the logic is not copied into user repos.

### Trigger and permissions

- **Trigger:** `workflow_dispatch` with a single `environment` input (the GitHub Environment name). The job binds to that environment via `environment: ${{ inputs.environment }}`.
- **Permissions:** `id-token: write` (required for OIDC), `contents: read`, and `packages: write` (so the GHCR package push check tests the same token capability the deploy uses).

### Required environment variables

The workflows read GitHub Actions **variables** (`vars`) — never long-lived secrets. The GHCR package push check also uses the built-in `GITHUB_TOKEN`, which GitHub Actions provides automatically (nothing to configure). Configure these variables on the target GitHub Environment:

| Provider | Variables                                                                                                       |
|----------|-----------------------------------------------------------------------------------------------------------------|
| Common   | `RADIUS_STATE_REGISTRY` (optional; enables the GHCR package push check)                                         |
| Azure    | `AZURE_CLIENT_ID`, `AZURE_TENANT_ID`, `AZURE_SUBSCRIPTION_ID`, `AZURE_RESOURCE_GROUP`, `AZURE_AKS_CLUSTER_NAME` |
| AWS      | `AWS_ROLE_ARN`, `AWS_REGION`, `AWS_EKS_CLUSTER_NAME`                                                            |

### Prerequisites on the cloud side

OIDC trust must already exist before the workflow can authenticate:

- **Azure:** a federated credential on the AAD app whose subject is exactly `repo:<owner>/<repo>:environment:<environment-name>`, audience `api://AzureADTokenExchange`.
- **AWS:** an IAM role trust policy that allows `sts:AssumeRoleWithWebIdentity` from `token.actions.githubusercontent.com` with audience `sts.amazonaws.com` and subject `repo:<owner>/<repo>:environment:<environment-name>`.

## `run-rad-commands.yml` (dispatcher, provider workflows, and shared actions)

The run-rad-commands workflow Radius uses to run one or more `rad` CLI commands on demand against a user's target cluster — deploying by default, but able to run any allowed command (`deploy`, `app graph`, `app delete`, and so on). It stands up an ephemeral [k3d](https://k3d.io) control plane on the runner, restores persisted state, runs the requested commands, then persists state again and tears the control plane down.

To keep the two provider paths from duplicating the ~80% of steps they share, it ships as a unified dispatcher, two thin provider workflows, and shared composite actions:

- **`run-rad-commands.yml`** — the unified **dispatcher** and the only file that is dispatched. It owns the dispatch contract (`workflow_dispatch` inputs and the `Radius - Verify Credentials` auto-trigger). A `detect` job binds the GitHub Environment, reads which provider variable is set (`AZURE_CLIENT_ID` / `AWS_ROLE_ARN`), and calls the matching provider workflow via `workflow_call` with `secrets: inherit`.
- **`run-rad-commands-azure.yml`** — a reusable (`workflow_call`) workflow with only the Azure-specific steps: Azure OIDC login, AKS connection (`az aks get-credentials`), workload-identity credential registration, and the `azure-avm` recipe pack (Azure Verified Modules) downloaded from the immutable `resource-types-contrib` commit recorded in `deploy/manifest/defaults.yaml`. Its Kubernetes recipe sources are rewritten from upstream's floating aliases to the corresponding validated namespace commits before deployment.
- **`run-rad-commands-aws.yml`** — a reusable (`workflow_call`) workflow with only the AWS-specific steps: AWS OIDC login, EKS connection (access entry + static token kubeconfig), IRSA credential registration, and the `aws-terraform` recipe pack. Every Terraform source uses its resource-type namespace's immutable catalog commit; the container image build recipe defaults to the Compute namespace commit.
- **`actions/*`** — composite actions holding the provider-agnostic phases both provider workflows share: [`setup-control-plane`](actions/setup-control-plane/action.yml), [`load-contrib-catalog`](actions/load-contrib-catalog/action.yml), [`restore-state`](actions/restore-state/action.yml), [`apply-custom-recipe-packs`](actions/apply-custom-recipe-packs/action.yml), [`run-rad-commands`](actions/run-rad-commands/action.yml), [`publish-deploy-status`](actions/publish-deploy-status/action.yml), [`delete-resource`](actions/delete-resource/action.yml), and [`teardown`](actions/teardown/action.yml). The provider workflows reference them from `radius-project/radius` at a pinned ref (the `{{RADIUS_REF}}` placeholder the generator fills in), so the shared logic has a single reviewed home and is not copied into user repos. Third-party actions in these workflows are pinned to full commit SHAs (with a `# vX` comment); only the first-party Radius composite actions are referenced by ref.

The deploy flow generates the dispatcher and both provider workflows, commits them to the target repo under `.github/workflows/`, and dispatches `run-rad-commands.yml`.

## `delete-application.yml` / `delete-environment.yml` (delete dispatchers and provider workflows)

Radius deletes a deployed application or an environment with the same ephemeral-control-plane model as the deploy flow. Because deleting recipe-backed resources runs the recipes' delete path (e.g. `terraform destroy`) against the target cluster and cloud, the delete workflows restore the persisted Radius state first, run the delete, then persist the updated state again — so subsequent runs plan against the post-delete state.

- **`delete-application.yml`** — dispatcher to delete one application. `workflow_dispatch` inputs: `environment` (GitHub Environment name) and `application` (application name). A `detect` job binds the environment, reads the provider variable, and calls the matching provider delete workflow with `resource_type: application`.
- **`delete-environment.yml`** — dispatcher to delete one environment. `workflow_dispatch` inputs: `environment` (GitHub Environment name) and optional `environment_name` (the Radius environment name, defaulting to the GitHub Environment name, since the deploy flow names the Radius environment after it). Calls the provider delete workflow with `resource_type: environment`.
- **`delete-azure.yml`** / **`delete-aws.yml`** — reusable (`workflow_call`) workflows with the provider-specific steps (OIDC login, cluster connection, cloud OIDC token projection, and credential registration) shared with the deploy provider workflows. They reuse the `setup-control-plane`, `restore-state`, [`delete-resource`](actions/delete-resource/action.yml), and `teardown` composite actions. Like the deploy provider workflows they log in to GHCR and set the `RADIUS_STATE_*` variables so `rad startup`/`rad shutdown` can open the OCI-backed state archive. Unlike the deploy provider workflows they do **not** create the environment, recipe pack, or the in-pod image-push registry credentials — the environment and its recipes are restored from state, and deleting builds no images.

The `delete-resource` composite action runs `rad app delete <name> --yes --preview` or `rad env delete <name> --yes --preview` (`--preview` selects the Radius.Core surface the deploy flow provisions) and writes a `rad-delete-result` artifact — a JSON document with `outcome`, `exitCode`, `resourceType`, `name`, and the command `output`.

### What it does

The dispatcher routes to the matching provider workflow, which runs on `ubuntu-24.04`. It stands up an ephemeral [k3d](https://k3d.io) cluster to host the Radius control plane on the runner, points that control plane at the user's existing EKS/AKS cluster, and deploys the application there. The control-plane setup, state restore, and run/teardown phases below run from the shared composite actions; the OIDC login, cluster connection, token projection, credential registration, and recipe-pack creation are the provider-specific steps. When a provider's identifying variable is empty, its steps are skipped and resources deploy to the ephemeral control-plane cluster instead of an external target.

1. **Authenticate via OIDC.** Runs `azure/login` (Azure) or `aws-actions/configure-aws-credentials` (AWS) so the runner exchanges its GitHub OIDC token for short-lived cloud credentials. No long-lived cloud secrets are stored.
2. **Build the target-cluster kubeconfig.** Exports `RADIUS_TARGET_KUBECONFIG` to a path under `$HOME/.kube`, then connects to the workload cluster: Azure runs `az aks get-credentials --file`; AWS ensures an EKS access entry and cluster-admin access policy for the IAM role and writes a static, token-based kubeconfig.
3. **Create the ephemeral control plane.** Installs k3d and creates the `radius-cp` cluster, then installs `oras`, the `rad` CLI (edge), and Terraform.
4. **Create the target-kubeconfig secret.** Stores the target kubeconfig as the `target-kubeconfig` secret in `radius-system` (skipped when no target kubeconfig is present).
5. **Install Radius on the control plane.** Runs `rad install kubernetes` with `database.enabled=true` (control-plane PostgreSQL for durable state), `rp.publicEndpointOverride=localhost`, `dynamicrp.buildkit.enabled=true`, and — when a target kubeconfig is present — `global.targetCluster.enabled=true`. The chart mounts the secret into `applications-rp`, `dynamic-rp`, and `bicep-de` and sets `RADIUS_TARGET_KUBECONFIG`, so recipe execution and directly-rendered resources land on the external cluster. The Terraform state backend deliberately stays on the control-plane cluster.
6. **Project cloud OIDC tokens.** Mints a GitHub OIDC token for the provider and patches it into the RP/DE pods at the fixed path each reads for the federated token exchange (AWS IRSA `/var/run/secrets/eks.amazonaws.com/serviceaccount/token`; Azure workload identity `/var/run/secrets/azure/tokens/azure-identity-token`). A GitHub OIDC token expires after 5 minutes, so a single projected assertion cannot cover a longer run. Before Azure `rad` commands or deletes, the shared actions re-mint the assertion, update the Secret, and restart its three consumers so their initial mounts are current. While the operation runs, they update the Secret every two minutes; kubelet propagates those updates to the non-`subPath` volume mounts.
7. **Refresh external target credentials.** AWS re-mints the short-lived EKS token; both providers rewrite the `target-kubeconfig` secret and restart `applications-rp`, `dynamic-rp`, and `bicep-de` so they re-read it.
8. **Configure the workspace.** Runs `rad workspace create kubernetes default` and `rad group create` / `rad group switch default`.
9. **Restore persisted state (`rad startup`).** Restores the control-plane databases and the Terraform recipe-state Secrets saved by the previous run, so `rad deploy` plans against prior state rather than an empty backend. A no-op on the first run.
10. **Register cloud credentials.** Registers the cloud identity with `rad credential register azure wi` / `aws irsa` so Radius holds the identity selector and reads the projected token at runtime.
11. **Create the Radius environment and recipe pack.** `rad deploy`s a `radius-env.bicep` that defines a `Radius.Core/recipePacks` resource and the `Radius.Core/environments` resource that references it. The shared `load-contrib-catalog` action exposes `deploy/manifest/defaults.yaml` from the same pinned Radius ref as the workflow actions. Azure resolves the `azure-avm` pack from that catalog and replaces its floating Kubernetes recipe aliases with catalog-derived sources; AWS generates an inline `aws-terraform` pack whose Git and OCI sources are resolved from the same catalog. No contrib refs are duplicated in generated workflow files. `radius-env.bicep` is written to the app file's directory (e.g. `.radius/`) and deployed from there, so `rad deploy` resolves the repo's own `bicepconfig.json` (which declares the `radius` extension) — bicep resolves the config nearest the `.bicep` file. The `Radius.Compute/containerImages` type ships with the Radius extension, so no separate resource-type registration is needed.
12. **Register custom types and apply custom recipe pack.** When the app's `.radius/` folder carries a `custom-types.yaml` file, the shared `apply-custom-recipe-packs` action registers those resource types with `rad resource-type create --from-file` (skipped when absent). When it carries a `custom-recipe-pack.bicep` file, the action snapshots the recipe-pack IDs before and after `rad deploy`ing that pack to identify the newly-created pack(s), reads the environment's existing `recipePacks` with `rad env show --preview`, and runs `rad env update <env> --recipe-packs <existing ∪ new> --preview` so the environment keeps the default provider pack and gains the custom pack — without pulling in unrelated packs the control plane may know about (skipped when absent). When neither file exists this step is a no-op and the default pack stays in place.
13. **Run the requested rad commands.** Validates each command in `rad_commands` against the allowed-command set, then runs them in order (stopping on the first failure) and writes a combined `rad-commands-result` artifact. Before deploying the app, the shared action compiles its Bicep file once and reads the declared ARM parameters. It passes each extension-generated parameter only when the template declares it: `image` (the workflow input, defaulting to `github.sha`), `registryUsername` (`github.actor`), and `registryPassword` (the built-in `GITHUB_TOKEN`). Caller-configured application parameters from the `RADIUS_DEPLOY_PARAMS` secret remain strict and are passed unchanged. The registry parameters feed the app's `Radius.Security/secrets` resource (`radius-ghcr-registry-creds`), when present, so the containerImages recipe's in-pod BuildKit can push the application image. Secret values are passed via an argv array and never written into the recorded command string.

    Only an application deploy of the configured app file starts live progress polling. While that command runs, the action polls `rad resource list --preview --application <app> --output json` every five seconds and publishes a snapshot only when the canonical resource state changes. Live artifacts are named `radius-deploy-status-<environment>-<app>-live-<run-id>-slot-<0..7>`, retained for one day, and rotated through eight slots. Each payload has an increasing `sequence`, the active `runId`, and run-level state `in_progress`; unknown resource states also normalize to `in_progress`. The action uses a checked-in bundle of the official `@actions/artifact` client because a composite `uses:` step cannot run concurrently with `rad deploy`. Immediately before the composite deploy action, a JavaScript action captures the action-scoped artifact runtime in a private runner-temp file; the deploy process loads it for the background publisher and removes it during EXIT cleanup.

    Live reporting is best-effort. Poll, JSON, deletion, and upload failures warn without changing the deploy result. The poller is stopped and awaited immediately after the application deploy, before any later command runs, and the EXIT cleanup also stops it after an unexpected interruption. Unrelated commands and deploys of other Bicep files do not start the poller.
14. **Publish deployed graph/status artifact.** Whenever the run is not cancelled (`if: !cancelled()`), the shared `publish-deploy-status` action runs `rad app graph --application <app> --preview --include-icons --output json` against the live control plane and publishes `deploy-graph.json` plus sibling status files (`deploy-progress.json` — per-resource status the canvas paints the graph from, `deploy-activity.log` — the rad command result envelope, `deploy-controlplane.log` — control-plane health, `deploy-state.txt` — a flat `key=value` summary of the run, not read by the canvas) as a **workflow artifact** named `radius-deploy-status-<environment>-<app>` (lowercased, with characters outside `[a-z0-9._-]` collapsed to `-`), retained for 30 days. It publishes on failed deploys too, since that is when the Deployed graph is most useful.

    Publishing is best-effort and never changes the deployment result. When the application name cannot be resolved, or `rad app graph` fails, the action warns, publishes nothing, and exits 0. Resource status comes from `rad resource list --preview`, which resolves application ownership through `Radius.Core/applications`; the non-preview command targets the legacy `Applications.Core` identity and is not used by this flow. When preview resource listing fails, the action emits one sanitized warning and still publishes: `deploy-progress.json` carries the run-level state with an empty `resources` array, so the Deployed tab shows the run rather than nothing.

    Radius.Core preview graphs intentionally omit the complete `properties.containers[*].env` map from every `Radius.Compute/containers` node before CLI formatting or artifact publication. This applies to ordinary and secret-derived environment values alike; graph consumers still receive image, port, volume, connection, and other non-environment properties. Existing schema-sensitive and secure-parameter redaction remains in place as an additional layer.

    `deploy-progress.json` is the authority on deploy state and the only status file the canvas reads. Its `state` uses `succeeded`/`failed`/`in_progress`, where `in_progress` also covers "no verdict" — it is what a finished run reports when `rad-commands-result.json` is missing or unreadable, since claiming a failure the deploy may not have had is worse than reporting no outcome. Each entry in `resources[]` carries both the raw `provisioningState` and a normalized `status`; an unrecognized provisioning state normalizes to `in_progress`, never `failed`, so a Radius state this action has not seen cannot paint a node red. `deploy-state.txt` predates `deploy-progress.json` and uses its own older vocabulary (`state=success`, mirroring the rad command outcome), so the two files can describe the same successful run with different words. That is deliberate; `deploy-progress.json` is the one to trust.

    The fixed-name terminal payload uses one sequence greater than the last successful live upload, or sequence 1 when no live upload succeeded. Consumers therefore select the numerically greatest valid sequence without relying on artifact list order or slot number, and the terminal artifact wins naturally after deployment ends.

    Workflow artifacts are the transport because the REST API can read them **while the run is still in progress** (`GET /repos/{owner}/{repo}/actions/runs/{run_id}/artifacts`), which is what lets the canvas show deployment state as it happens; `GET /repos/{owner}/{repo}/actions/artifacts?name=<name>` finds the newest one later without knowing the run. They also require no extra registry, no `packages: write` permission, and no name derivation duplicated between this action and the canvas reader.
15. **Persist state (`rad shutdown`).** Backs the control-plane databases and Terraform recipe-state Secrets up to the state archive — the OCI-backed archive by default (pushed to GHCR, selected by the `RADIUS_STATE_*` variables), or the `radius-state` git orphan branch when `RADIUS_STATE_BACKEND=git`. This runs even when the deploy fails (`if: always()`), so a partially-applied Terraform run is not lost.
16. **Tear down.** Runs `rad app list`, and always deletes the ephemeral `radius-cp` cluster. On failure, Radius and application logs are collected and uploaded as the `radius-logs` artifact (three-day retention).

### Triggers and permissions

Triggers and permissions live on the **dispatcher** (`run-rad-commands.yml`); the provider workflows are `workflow_call`-only and inherit permissions and secrets from it.

- **Triggers:**
  - `workflow_dispatch` with an `environment` input (the GitHub Environment name) plus optional `image` and `rad_commands` inputs. The `detect` job binds that environment via `environment: ${{ inputs.environment }}` to read the provider variables.
  - `workflow_run` after the `Radius - Verify Credentials` workflow completes. The `detect` job runs only when the upstream verify run concluded `success`, so a successful credential check auto-triggers a deploy.
- **Inputs:**

  | Input          | Required | Description                                                                                                                                                                                                                                                                                                                                                                         |
  |----------------|----------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
  | `environment`  | Yes      | The GitHub Environment name, used as the Radius environment.                                                                                                                                                                                                                                                                                                                        |
  | `image`        | No       | Container image for the application. Defaults to the commit SHA (`github.sha`) when unset and is passed only when the application template declares the `image` parameter.                                                                                                                                                                                                          |
  | `rad_commands` | No       | A single `rad` command string, or a JSON array of command strings run in order (the `rad` prefix omitted, e.g. `["deploy .radius/app.bicep --environment dev", "app graph my-app -o json"]`). Each command is validated against the allowed-command set. Falls back to the `RADIUS_RAD_COMMANDS` variable. When empty, the workflow runs its default `rad deploy` of the app bicep. |

- **Outputs:** a combined `rad-commands-result` artifact — a JSON document with a top-level `outcome`/`exitCode` and a `commands` array (one entry per command, in input order, with each command's exit code and output).
- **Permissions:** `id-token: write` (required for OIDC), `contents: write` (so `rad shutdown` can push the `radius-state` branch when the git state backend is selected), and `packages: write` (to push the OCI-backed state archive to GHCR and the application image built by the containerImages recipe).

### Required environment variables

The workflow reads cloud and cluster configuration from GitHub Actions **variables** (`vars`). Configure the relevant provider's set on the target GitHub Environment:

- Common: `KUBERNETES_NAMESPACE` (default `default`), `RADIUS_BUILD_REGISTRY` (default `ghcr.io/<owner>/<repo>`), `RADIUS_RAD_COMMANDS` (optional fallback for `rad_commands`), `RADIUS_GRAPH_REGISTRY` (optional OCI repository for the `rad` CLI's modeled graph archive)
- Azure (`run-rad-commands-azure.yml`): `AZURE_CLIENT_ID`, `AZURE_TENANT_ID`, `AZURE_SUBSCRIPTION_ID`, `AZURE_RESOURCE_GROUP`, `AZURE_AKS_CLUSTER_NAME`
- AWS (`run-rad-commands-aws.yml`): `AWS_ROLE_ARN`, `AWS_REGION`, `AWS_ACCOUNT_ID`, `AWS_EKS_CLUSTER_NAME`, `RADIUS_VPC_ID`, `RADIUS_SUBNET_IDS`

The provider steps run only when the identifying variable (`AZURE_CLIENT_ID` or `AWS_ROLE_ARN`) is non-empty. When it is unset, resources deploy to the ephemeral control-plane cluster instead of an external target.

This workflow also reads GitHub Actions **secrets** for image push and application configuration:

| Secret                 | Purpose                                                                                                                                                  |
|------------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------|
| `GITHUB_TOKEN`         | Built-in. Used with `github.actor` to authenticate the containerImages recipe's image push to GHCR.                                                      |
| `RADIUS_DEPLOY_PARAMS` | Optional. A JSON object of application parameters (`{"password":"…","apiKey":"…"}`) expanded into `--parameters name=value` pairs on the default deploy. |

### State persistence (`rad startup` / `rad shutdown`)

`rad startup` and `rad shutdown` are kind-agnostic CLI commands that restore and back up all durable Radius state (control-plane PostgreSQL + Terraform recipe-state Secrets). These workflows use the OCI-backed state archive by default — the `RADIUS_STATE_*` variables select an OCI repository and the workflow logs in to GHCR before `rad startup`/`rad shutdown` — and fall back to the `radius-state` git orphan branch only when `RADIUS_STATE_BACKEND=git`. They do not manage cluster lifecycle — the workflow owns creating and destroying the ephemeral control plane around them. `rad startup` runs after the install (so `rad deploy` plans against prior state) and `rad shutdown` runs after the commands with `if: always()` (so state survives a failed deploy).

### Prerequisites

- OIDC trust for the environment (federated credential on Azure, IAM role trust policy on AWS). Run the verify workflow first to confirm the environment is wired up correctly — a successful verify run also auto-triggers this workflow.
- The target cluster (`AWS_EKS_CLUSTER_NAME` / `AZURE_AKS_CLUSTER_NAME`) must already exist and be reachable; the assumed identity needs cluster-admin-level access to it.
- The application must define its app bicep file in the target repo.

For the full deploy flow, see the workflow steps described above.
