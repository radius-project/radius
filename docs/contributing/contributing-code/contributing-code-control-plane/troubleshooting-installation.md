# Troubleshooting a Radius installation

This page explains some troubleshooting steps that can help you diagnose installation failures or crashes using a custom build of Radius.

## Verify all Radius pods are up and running

```bash
kubectl get pods -n radius-system
```

The installation includes UCP, Applications RP, Dynamic RP, Controller, Deployment Engine, and the PostgreSQL database. The exact pod names contain generated suffixes. Every pod should become `Running` or complete successfully; use `kubectl describe pod <pod-name> -n radius-system` for a pod that remains pending or repeatedly restarts.

## Examine logs

Read logs by deployment name instead of copying an ephemeral pod name:

```bash
kubectl logs deployment/ucp -n radius-system
kubectl logs deployment/applications-rp -n radius-system
kubectl logs deployment/dynamic-rp -n radius-system
kubectl logs deployment/controller -n radius-system
kubectl logs deployment/bicep-de -n radius-system
```

For a specific pod or container, use:

```bash
kubectl logs <pod-name> -n <namespace>
```

Logs with `severity` set to `error` or `warn` could point to why the pods are not in Running state.

Every `rad` CLI error includes a `traceId` that also appears in the control-plane logs. Search all Radius pod logs for that value:

```bash
kubectl logs -n radius-system -l app.kubernetes.io/part-of=radius --all-containers=true --prefix | grep <traceId>
```

See the [observability documentation](https://docs.radapp.io/guides/operations/control-plane/logs/) for more logging guidance.

## The installation times out

`rad install kubernetes` waits for the control-plane resources to become ready and fails with an error like this if they do not:

```text
Error: failed to apply Radius Helm chart, err: failed to run Helm install, err: resource Deployment/radius-system/ucp not ready. status: InProgress, message: Available: 0/1
context deadline exceeded
```

The default budget is 10 minutes, which is ample for a typical install. A cold cluster has to pull every control-plane image before any deployment can become ready, so a slow or throttled network can exceed the default. Allow more time with `--timeout`:

```bash
rad install kubernetes --timeout 30m
```

`--timeout` accepts any Go duration (`30m`, `1h`). Negative values are rejected, and `0` means "use the default".

A timed-out install leaves a failed Helm release behind, and `rad install kubernetes` skips a cluster that already has a release. Retrying therefore needs `--reinstall`, otherwise the command reports `Found existing Radius installation` and exits without reapplying the chart:

```bash
rad install kubernetes --reinstall --timeout 30m
```

If the install still times out, check whether the pods are actually healthy using the steps above. Pods that are `Running` and `Ready` while the install reports a resource as `InProgress` point at the readiness watcher rather than your cluster — see [radius-project/radius#12975](https://github.com/radius-project/radius/issues/12975).

## cert-manager

Radius authorization depends on [cert-manager](https://cert-manager.io/). `rad install kubernetes` and `rad upgrade kubernetes` only manage cert-manager when authorization is enabled, by passing exactly one of `--set global.rbac.dryRun=true` or `--set global.rbac.enabled=true`. With authorization off (the default), `rad` does not look for, install, or change cert-manager.

### Supported versions

`rad` installs a pinned cert-manager chart (`CertManagerChartDefaultVersion` in `pkg/cli/helm/cert_manager.go`, currently `v1.21.2`) from `https://charts.jetstack.io` as the Helm release `cert-manager` in the `cert-manager` namespace, with CRDs enabled. An existing cert-manager must be at least `CertManagerMinimumVersion` (currently `v1.20.0`). Both constants track the [cert-manager supported releases](https://cert-manager.io/docs/releases/).

### Existing cert-manager installations

`rad` detects cert-manager by the `certificates.cert-manager.io` CRD and a controller Deployment in any namespace labeled `app.kubernetes.io/name=cert-manager,app.kubernetes.io/component=controller`. It reads the version from the controller's `app.kubernetes.io/version` label, or from its image tag.

| What `rad` finds                               | What `rad` does                                                            |
|------------------------------------------------|----------------------------------------------------------------------------|
| No CRD and no controller                       | Installs the pinned version and marks the release as owned by `rad`        |
| A supported version that `rad` did not install | Reuses it; never upgrades, modifies, or removes it                         |
| A version older than the minimum               | Stops before installing Radius and names the minimum version to upgrade to |
| The CRD without a controller (or the reverse)  | Fails; repair or remove the broken installation, then retry                |

### Ownership

`rad` marks a release it installed with the Helm release label `app.kubernetes.io/managed-by=rad`. `helm get metadata` does not show this label; inspect the Helm release Secrets instead:

```bash
kubectl get secret -n cert-manager -l owner=helm,name=cert-manager --show-labels
```

`rad` only upgrades a release it owns, and repairs an owned release that is not `deployed` or whose controller is missing.

### Uninstall

`rad uninstall kubernetes` removes an owned cert-manager release. The chart is installed with `crds.keep=false`, so this also deletes the cert-manager CRDs and every Certificate, CertificateRequest, Issuer, and ClusterIssuer in the cluster. To protect other workloads, `rad` first lists those resources and keeps cert-manager if any exist outside the Radius namespace (ClusterIssuers always count) or if the check fails:

```text
Keeping cert-manager: other workloads use it (2 resources, e.g. my-app/Certificate/my-cert)
```

A cert-manager that `rad` did not install is never removed.

### Recovering from a partial install or uninstall

- **cert-manager installed, Radius install failed:** fix the cause and rerun the same `rad install kubernetes` command (add `--reinstall` if a Radius release was left behind). `rad` reuses or repairs the cert-manager it owns. To back out instead, run `rad uninstall kubernetes`.
- **Uninstall interrupted or cert-manager left behind:** rerun `rad uninstall kubernetes`. If that is not possible, remove the release manually:

  ```bash
  helm uninstall cert-manager -n cert-manager
  ```

  Delete the cert-manager CRDs only when no other workload uses cert-manager, because deleting them removes every cert-manager resource in the cluster:

  ```bash
  kubectl get crd -o name | grep 'cert-manager.io' | xargs kubectl delete
  ```
