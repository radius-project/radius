# Argo CD plugin prototype — setup log

Goal: prove the design in
[2026-10-argocd-functional-spec.md](https://github.com/radius-project/radius/pull/13323):
an Argo CD Config Management Plugin (CMP) sidecar compiles Bicep into a
`DeploymentTemplate`, Argo CD applies it, the existing Radius controller
deploys it, and a Lua health check reports Radius provisioning health.

## Plan

| Phase | What                                                              | Status  |
|-------|-------------------------------------------------------------------|---------|
| 0     | Cluster and tools                                                 | done    |
| 1     | Install Radius                                                    | done    |
| 2     | Install Argo CD and its CLI                                       | done    |
| 3     | Sample app repository                                             | pending |
| 4     | Build the Radius CMP sidecar image                                | pending |
| 5     | Register the plugin and the `DeploymentTemplate` Lua health check | pending |
| 6     | Create the Argo CD Application and sync                           | pending |
| 7     | Exercise scenarios: change, failure, removal, refresh vs retry    | pending |

## Decisions

| # | Decision                         | Choice                                                                                                                  | Why                                                                                                                                                                                                                                           |
|---|----------------------------------|-------------------------------------------------------------------------------------------------------------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| 1 | Radius build                     | Released 0.61.1 first; source build later                                                                               | Released already has the `DeploymentTemplate` controller, which is enough for sync, health and the resource tree. Failure messages, revision and Radius-created objects need the status additions in the spec, so a source build comes later. |
| 2 | Argo CD install                  | Helm chart `argo/argo-cd` 10.10.5 (Argo CD v3.5.5) with a values file                                                   | The sidecar, plugin config and Lua health check all go into one reproducible values file, as a platform team would do it.                                                                                                                     |
| 3 | Sample app                       | Kubernetes-only copy of `/Users/nithya/ignite2026` in a new public repo `nithyatsu/ignite2026-argocd` (not yet created) | Real Radius.Core app (vote, result, Postgres) and an existing `environments/kubernetes.bicep`. Public, so Argo CD needs no credentials.                                                                                                       |
| 4 | `postgresPassword` (`@secure()`) | Argo CD plugin parameter on the Application, not in Git                                                                 | Keeps it out of Git. It still leaks in plaintext into `DeploymentTemplate.spec.parameters`, the Argo CD desired manifest and diff, and the repo-server cache. Record where; add the gap to the spec.                                          |

## Phase 0 — Cluster and tools

Cluster, created by Nithya:

```bash
kind create cluster --name kind-argo
kubectl config use-context kind-kind-argo
kubectl get nodes   # kind-argo-control-plane Ready, v1.35.0
```

Local tools found:

| Tool     | Version                 |
|----------|-------------------------|
| `rad`    | 0.61.1 (Bicep 0.46.1)   |
| `helm`   | v4.1.4                  |
| `docker` | 29.4.0                  |
| `kind`   | v0.31.0                 |
| `argocd` | not installed (Phase 2) |

Cluster starts empty: no Radius, no Argo CD.

## Phase 1 — Install Radius

```bash
rad install kubernetes --kubecontext kind-kind-argo
```

Installs Radius 0.61.1 into `radius-system`, plus Contour. Verify:

```bash
kubectl -n radius-system get pods        # applications-rp, bicep-de, controller, dynamic-rp, ucp, dashboard Running
kubectl get crd | grep radapp            # deploymenttemplates.radapp.io, deploymentresources.radapp.io
rad version                              # Control plane: Installed 0.61.1
```

No `rad init`, resource group or environment is needed up front. The
`DeploymentTemplate` controller creates the resource group named in the
template's `providerConfig` if it is missing
(`createResourceGroupIfNotExists` in `pkg/controller/reconciler/deploymenttemplate_reconciler.go`).
The environment will be declared in the app's Bicep.

What the released controller exposes in `DeploymentTemplate.status`:
`phrase` (`Updating`, `Ready`, `ReadyPendingCleanup`, `Failed`, `Deleting`,
`Deleted`), `observedGeneration`, `statusHash`, `outputResources`,
`operation`. No failure message and no revision.

## Phase 2 — Install Argo CD and its CLI

Prototype files live in this folder (`argocd-values.yaml`).
Values file `argocd-values.yaml` (plain install for now):

```yaml
configs:
  params:
    server.insecure: true   # local kind cluster; we port-forward over HTTP
```

Install:

```bash
helm repo add argo https://argoproj.github.io/argo-helm
helm repo update argo
helm upgrade --install argocd argo/argo-cd --version 10.10.5 \
  -n argocd --create-namespace -f argocd-values.yaml \
  --kube-context kind-kind-argo --wait
brew install argocd        # CLI v3.5.6
kubectl -n argocd get pods # all Running
```

### UI access

Port 8080 is already used on this Mac, so use 8090. A plain
`kubectl port-forward` exits on the first reset connection, so run it in a
restart loop:

```bash
while true; do
  kubectl --context kind-kind-argo -n argocd port-forward svc/argocd-server 8090:80
  sleep 1
done
```

Open <http://localhost:8090>. User `admin`; password:

```bash
kubectl -n argocd get secret argocd-initial-admin-secret \
  -o jsonpath="{.data.password}" | base64 -d
```

### CLI access

`argocd login 127.0.0.1:8090 --grpc-web` failed with
`error dial proxy ... connection refused` even though the UI worked. Use the
CLI's built-in port-forward instead:

```bash
export ARGOCD_OPTS="--port-forward --port-forward-namespace argocd --kube-context kind-kind-argo"
argocd login --username admin --password <password> --plaintext
argocd version   # argocd-server: v3.5.5
```

## Phase 3 — Sample app repository (in progress)

Source: `/Users/nithya/ignite2026` (public `nithyatsu/ignite2026`). Copy into a
new public repo `nithyatsu/ignite2026-argocd` with only the Kubernetes parts:

| Keep                            | Change                                                                                                                                                                                                                                                                                            |
|---------------------------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `app.bicep`                     | Remove `platformOptions` (ACI and CCE only; the containers recipe ignores it). Default `imageTag` to `047c81d1f93c53cad6074db9ad525bf37a541178-arm64` and `redisImage` to `ghcr.io/nithyatsu/redis@sha256:7e180df929e2aa87c71fd61838e7775ebab7e7e7a08e438045fbeaa123960320` (kind node is arm64). |
| `bicepconfig.json`              | None (`br:biceptypes.azurecr.io/radius:0.61`).                                                                                                                                                                                                                                                    |
| `environments/kubernetes.bicep` | None. Recipe pack pins kube-recipes `0.61.0`; env namespace `voting-k8s`.                                                                                                                                                                                                                         |
| `README.md`                     | Rewrite for Argo CD.                                                                                                                                                                                                                                                                              |

Drop: `cce/`, `environments/aci.bicep`, `.github/workflows/cce-policy.yaml`,
`.env.local`, Azure steps.

Facts the setup must handle:

- Namespace `voting-k8s` must exist before the environment is deployed.
- The app's `environment` parameter is the environment's resource ID,
  `/planes/radius/local/resourceGroups/<group>/providers/Radius.Core/environments/kubernetes`.
- The spec's plugin compiles one Bicep file per Application, so the
  environment and the app are separate Applications.
- The released controller does not retry a failed `DeploymentTemplate` until
  its spec changes, so the app must not sync before the environment is
  Healthy.

## Spec gaps found so far

1. **`@secure()` parameters.** Neither our spec, Will's feature spec nor the
   Flux design says how secure parameters reach Radius. In the plugin path the
   value lands in plaintext in the generated `DeploymentTemplate`
   (`spec.parameters`), Argo CD's desired manifest, diff view and repo-server
   cache. In the CI path it lands in Git. Likely fix: a `DeploymentTemplate`
   parameter can reference a Kubernetes Secret that the Radius controller
   reads at deploy time.
2. **An application without parameters is always OutOfSync** (seen
   2026-10-22, learning step A6). Logged in the design note, Open Question 10.
   - **Setup:** Radius 0.61.1, Argo CD 3.5.5 (chart 10.10.5), kind
     `kind-argo`. Argo CD Application `playground` reads
     `https://github.com/nithyatsu/argocd-playground.git`, branch `main`, path
     `plain`, namespace `playground`, automated sync with prune.
   - **Steps:**

     ```bash
     cd ~/argocd-playground
     echo "// empty" > empty.bicep
     rad bicep generate-kubernetes-manifest empty.bicep \
       --destination-file plain/fake-dt.yaml --group playground
     rm empty.bicep
     git add plain && git commit -m "add DeploymentTemplate" && git push
     argocd app get playground --refresh
     ```

   - **Generated file:** `metadata.name: empty.bicep`, `spec.parameters: {}`,
     `spec.providerConfig` scoped to `/planes/radius/local/resourceGroups/playground`,
     and `spec.template` pretty-printed ARM JSON with an empty `resources` list
     and `_generator.version: 0.46.1.21595`.
   - **What happened:** the sync succeeded at commit `c4ae964`. Radius
     deployed the template (`kubectl -n playground get deploymenttemplates`
     shows `empty.bicep  Ready`), but Argo CD shows the `DeploymentTemplate`
     as **OutOfSync**. `argocd app diff playground` shows only:

     ```text
     ===== radapp.io/DeploymentTemplate playground/empty.bicep ======
     53a54
     >   parameters: {}
     ```

   - **Cause:** the live object has no `spec.parameters`, and
     `metadata.generation` is 2, so its spec was rewritten after Argo CD
     applied it. The Radius controller calls `r.Client.Update` to add its
     finalizer `radapp.io/deployment-template-finalizer`
     (`deploymenttemplate_reconciler.go`, about line 326). `Parameters` is
     `map[string]string` tagged `json:"parameters,omitempty"`
     (`pkg/controller/api/radapp.io/v1alpha3/deploymenttemplate_types.go`,
     line 30), so the update serializes the empty map away.
   - **Why re-sync does not help:** Argo CD re-applies `parameters: {}`, and
     the next controller update removes it again.
   - **Possible fixes:** the generator or plugin leaves out empty
     `parameters`; the controller uses a patch that keeps unknown fields;
     or the Argo CD Application ignores that field with `ignoreDifferences`.
   - **Workaround in the playground:** delete the `parameters: {}` line from
     `plain/fake-dt.yaml` and push.
3. **`DeploymentTemplate` name comes from the Bicep file name** (seen
   2026-10-22, A6). The generator set `metadata.name: empty.bicep`. With the
   plugin, every app compiled from `app.bicep` gets the name `app.bicep`, so
   two Applications in one namespace would overwrite each other's template.
   Logged in the design note as an open decision, not finalized, because we
   recommend one Radius application per namespace.
4. **Confirmed by the same generated file** (A6). The ARM JSON is
   pretty-printed across lines, so Argo CD diffs will be line by line. It
   carries `_generator.version` and `templateHash`, so upgrading Bicep in the
   sidecar changes every generated template (design note, Open Question 10).

## Resume here (paused 2026-10-21)

Open question for Nithya: how to order the environment and the app.

- **A (recommended):** two Applications created in order, environment first,
  then the app once the environment is Healthy. Test the wrong order later as
  a scenario (it shows the no-retry gap).
- **B:** parent app-of-apps with sync waves (environment wave 0, app wave 1).
- **C:** one Application; extend the plugin to compile both files with sync
  waves.
- **D:** the `DeploymentTemplate` reconciler retries until the environment
  exists. No ordering needed in Argo CD; the most robust option, but it is a
  Radius change.

Why ordering is needed at all (learned 2026-10-22):

- Inside one Bicep file, Radius already orders resources. A symbolic
  reference such as `recipePacks: [ recipepack.id ]` becomes a `dependsOn` in
  the ARM template, and the deployment engine follows it. So a recipe pack and
  the environment that uses it, in the same `env.bicep`, need nothing extra.
- Between files, nothing links them. Each file becomes its own
  `DeploymentTemplate`, and `app.bicep` gets the environment as a plain string
  (`param environment string`). Argo CD applies both together; if the app
  deploys first it fails and is not retried.
- A recipe pack in its own file (for example a shared platform pack) is a
  third `DeploymentTemplate` and needs the same treatment: pack, then
  environment, then app.
- Sync waves order objects only inside one Application, and wait only because
  the Lua health check reports Healthy on `Ready`. They cannot order two
  separate Applications (option A); in option B the child Applications are
  objects of the parent, so waves on them work.

Next steps after that answer:

1. Create `nithyatsu/ignite2026-argocd` with `gh repo create --public` and
   push the Kubernetes-only copy.
2. Phase 4: build the sidecar image (`rad` 0.61.1 and its Bicep binary
   baked in). Released `generate-kubernetes-manifest` only writes to a file, so
   the plugin writes to a temp file and prints it.
   Check that the sidecar can pull `br:biceptypes.azurecr.io/radius:0.61`.
   Load the image into kind with `kind load docker-image --name kind-argo`.
3. Phase 5: `helm upgrade` Argo CD with the sidecar, plugin config and Lua
   health check for `radapp.io/DeploymentTemplate`.
4. Phase 6: create the Applications and sync.

State left running: a detached port-forward loop on port 8090 for the Argo CD
UI. To restart it in a new session, run the loop from Phase 2 again.
