# Radius Argo CD plugin (prototype)

An Argo CD Config Management Plugin sidecar that compiles Radius Bicep into
`DeploymentTemplate` objects. Argo CD applies them and the Radius controller deploys them.

| File                 | Purpose                                                                 |
|----------------------|-------------------------------------------------------------------------|
| `Dockerfile`         | `rad` plus pre-fetched Bicep and Radius extension, runs as uid 999      |
| `plugin.yaml`        | Plugin `radius-bicep` with parameters `bicepFiles` and `resourceGroup`  |
| `radius-generate.sh` | Compiles each file, stamps revision and sync wave, prints the manifests |
| `prefetch/`          | Dummy Bicep compiled at build time to cache the Radius extension        |
| `argocd-values.yaml` | Argo CD Helm values: Lua health checks and the `radius-bicep` sidecar   |

## One Application per app

The recommended layout keeps the environment out of the app's Application: set
up the environment and Recipe pack with `rad` (or a separate platform
Application), and point the app's Application at the app folder only:

```yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: example-voting-app
  namespace: argocd
spec:
  project: default
  info:
    - {name: Radius group, value: voting-argo}
    - {name: Radius application, value: example-voting-app}
  source:
    repoURL: https://github.com/nithyatsu/ignite2026-argocd
    targetRevision: main
    path: app
    plugin:
      name: radius-bicep
      parameters:
        - {name: bicepFiles, string: app.bicep}
        - {name: resourceGroup, string: voting-argo}
  destination:
    server: https://kubernetes.default.svc
    namespace: voting-argo
```

With a single file in `bicepFiles`, the plugin names the `DeploymentTemplate`
after the Application (`ARGOCD_APP_NAME`), so the tree reads Application
`example-voting-app` → template `example-voting-app`. Renaming an existing
template makes Argo CD delete the old one and create the new one, so Radius
redeploys the app once.

### Several files, ordered by sync waves

To deploy the environment from the same Application, list the Bicep files in
deploy order, for example `env/env.bicep app/app.bicep`. Each template then
keeps its file name so the names stay unique. The plugin compiles each file into one `DeploymentTemplate` and annotates it with
`argocd.argoproj.io/sync-wave` set to its position (0, 1, ...). Argo CD applies wave
`n+1` only after every wave `n` resource is Healthy, and the `DeploymentTemplate`
health check reports Healthy only when Radius reports Ready for the current generation.

Parameters for `x/y.bicep` are read from `x/y.parameters.json` when that file exists.

## Health checks

`argocd-values.yaml` adds two Lua health checks to `argocd-cm`:

| Object / phrase                           | Health      | Message                                                  |
|-------------------------------------------|-------------|----------------------------------------------------------|
| `DeploymentTemplate`, generation not seen | Progressing | Waiting for Radius to pick up this revision              |
| `DeploymentTemplate` Updating             | Progressing | Deploying: Radius is applying this revision              |
| `DeploymentTemplate` ReadyPendingCleanup  | Progressing | Finishing the change: new outputs are ready, cleanup ... |
| `DeploymentTemplate` Ready                | Healthy     | Ready: N outputs tracked (generation G)                  |
| `DeploymentResource` Ready                | Healthy     | Output tracked (provisioned; not runtime health)         |
| `DeploymentResource` Deleting or Deleted  | Progressing | Cleanup in progress: removed from the desired template   |
| Either, Failed                            | Degraded    | Radius error text                                        |

Argo CD reports an object with a `deletionTimestamp` as "Pending deletion" before
it runs these checks. Neither check looks at the running workload, so a crashing
container still shows Healthy. Test a check offline with:

```bash
argocd admin settings resource-overrides health obj.yaml --argocd-cm-path argocd-cm.yaml
```

Optional header rows go in the Application's `spec.info`, for example
`- {name: Radius group, value: voting-argo}`.

```bash
helm upgrade --install argocd argo/argo-cd -n argocd --create-namespace \
  --version 10.10.5 -f argocd-values.yaml
```

## Tile labels

The Radius controller on this branch labels each `DeploymentResource` with
`radapp.io/resource-type` (for example `Radius.Compute.containers`) and
`radapp.io/phrase` (mirrors `status.phrase`). `resource.customLabels` in
`argocd-values.yaml` makes Argo CD show both as chips on the tile. The released
0.61 controller does not set them, so build and load this branch's controller:

```bash
DOCKER_BUILD_ARCH=arm64 DOCKER_REGISTRY=ghcr.io/nithyatsu DOCKER_TAG_VERSION=0.61.1-core2 \
  make docker-build-controller
kind load docker-image ghcr.io/nithyatsu/controller:0.61.1-core2 --name kind-argo
kubectl -n radius-system set image deploy/controller \
  controller=ghcr.io/nithyatsu/controller:0.61.1-core2
```

After changing `resource.customLabels`, hard-refresh the Application; Argo CD
keeps the old tree until then. Environments and recipe packs still show in the
tree only when they are deployed from the same Application; see the next section.

## No tiles for Radius.Core resources

The controller on this branch creates no `DeploymentResource` for output
resources in the `Radius.Core` namespace (applications, environments, Recipe
packs): the Application and the `DeploymentTemplate` already stand for the
application. Without a `DeploymentResource`, these resources are deleted only
when the whole `DeploymentTemplate` is deleted, after every tracked resource is
gone, in the order applications, environments, Recipe packs. Removing one of
them from the Bicep while the template stays leaves it in place.

## Delete API version

The released controller deletes every resource with API version
`2023-10-01-preview`, which `Radius.*` types reject, so deleting a template left
its resources stuck in `Failed`. This branch deletes each resource with the API
version the template deployed it with (for example
`Radius.Compute/containers@2025-08-01-preview`).

## Deploy progress on resource tiles

A Radius deployment applies the whole template in one call, so the
`DeploymentResource` objects do not change while it runs. To show progress, the
controller on this branch adds the annotation
`radapp.io/deploying-generation: "<template generation>"` to every
`DeploymentResource` the template owns when the deploy starts, and removes it
when the deploy succeeds or fails. While it is set, the `radapp.io/phrase` label
reads `Updating` and the health check returns Progressing with "Deploying: Radius
is applying template generation N". Marking every resource is accurate: Radius
re-applies every resource in the template. Resources that are new in this
revision appear only after the deploy finishes, and the Kubernetes objects
Radius creates (Deployments, pods) are still not in the Argo CD tree.

## Build and push

```bash
docker build -t ghcr.io/nithyatsu/radius-cmp:0.61.1-proto.3 .
docker push ghcr.io/nithyatsu/radius-cmp:0.61.1-proto.3
```

Prototype only: no secret handling, no failure messages.
