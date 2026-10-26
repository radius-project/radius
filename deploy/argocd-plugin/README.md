# Radius Argo CD plugin (prototype)

An Argo CD Config Management Plugin sidecar that compiles Radius Bicep into a
`DeploymentTemplate`. Argo CD applies it and the Radius controller deploys it.

| File                 | Purpose                                                                          |
|----------------------|----------------------------------------------------------------------------------|
| `Dockerfile`         | `rad` plus pre-fetched Bicep and Radius extension, runs as uid 999               |
| `plugin.yaml`        | Plugin `radius-bicep` with parameters `bicepFile`, `resourceGroup`, `paramsFile` |
| `radius-generate.sh` | Compiles, stamps `radapp.io/source-revision`, prints the manifest                |
| `prefetch/`          | Dummy Bicep compiled at build time to cache the Radius extension                 |

Build and push:

```bash
docker build -t ghcr.io/nithyatsu/radius-cmp:0.61.1-proto .
docker push ghcr.io/nithyatsu/radius-cmp:0.61.1-proto
```

Prototype only: no secret handling, no failure messages.
