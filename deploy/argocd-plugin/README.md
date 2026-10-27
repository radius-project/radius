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

## One Application per app, ordered by sync waves

An app's environment and Recipe pack must exist before the app. List the Bicep
files in deploy order in `bicepFiles`, for example `env/env.bicep app/app.bicep`.
The plugin compiles each file into one `DeploymentTemplate` and annotates it with
`argocd.argoproj.io/sync-wave` set to its position (0, 1, ...). Argo CD applies wave
`n+1` only after every wave `n` resource is Healthy, and the `DeploymentTemplate`
health check reports Healthy only when Radius reports Ready for the current generation.

Parameters for `x/y.bicep` are read from `x/y.parameters.json` when that file exists.

## Health checks

`argocd-values.yaml` adds two Lua health checks to `argocd-cm`:

| Object               | Healthy when                                       | Notes                                                                                   |
|----------------------|----------------------------------------------------|-----------------------------------------------------------------------------------------|
| `DeploymentTemplate` | `phrase` is Ready and `observedGeneration` current | Drives Application health and sync waves                                                |
| `DeploymentResource` | `phrase` is Ready                                  | Ready means the Radius resource exists; `observedGeneration` is only set while deleting |

Failed maps to Degraded; anything else is Progressing. Neither check looks at the
running workload, so a crashing container still shows Healthy.

```bash
helm upgrade --install argocd argo/argo-cd -n argocd --create-namespace \
  --version 10.10.5 -f argocd-values.yaml
```

## Build and push

```bash
docker build -t ghcr.io/nithyatsu/radius-cmp:0.61.1-proto.2 .
docker push ghcr.io/nithyatsu/radius-cmp:0.61.1-proto.2
```

Prototype only: no secret handling, no failure messages.
