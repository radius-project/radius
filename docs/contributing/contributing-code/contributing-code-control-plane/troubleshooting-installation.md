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

## A chart download fails

`rad install kubernetes` and `rad upgrade kubernetes` download the Radius chart from `oci://ghcr.io/radius-project/helm-chart` and the Contour chart from `https://projectcontour.github.io/helm-charts`, unless you install from local charts. These downloads don't use the Helm repositories you added with `helm repo add` or their cached indexes, so you don't need to run `helm repo update` first, and a missing or broken repository in your Helm configuration doesn't affect them. The Radius chart download still uses your registry credentials from `helm registry login` or `docker login`.

Each request times out after 30 seconds. A download that fails with a temporary error, such as a timeout, a dropped connection, or an HTTP 408, 429, 500, 502, 503, or 504 response, is retried for up to 3 minutes, so it gives up within about 4 minutes. Each retry prints the error that caused it:

```text
Downloading the contour chart failed (attempt 1), retrying: Get "https://...": context deadline exceeded (Client.Timeout exceeded while awaiting headers)
```

When the retries run out, or the error isn't temporary, the command stops with `error downloading helm chart from the registry` followed by the last error. A timeout or connection error means that the machine can't reach the chart host, so check its network, proxy, and firewall settings. A `401` or `403` response points at the registry credentials. For a `403` from `ghcr.io`, the command reports `received 403 unauthorized` instead and suggests running `docker logout ghcr.io`.

On a network that can't reach the chart hosts, download the charts on a machine that can, copy them over, and install from the local files. For the Contour chart, use the version that `ContourChartDefaultVersion` sets in [`pkg/cli/helm/contour.go`](../../../../pkg/cli/helm/contour.go):

```bash
helm pull contour --repo https://projectcontour.github.io/helm-charts --version <version>
rad install kubernetes --contour-chart ./contour-<version>.tgz
```

To install Radius from a local chart as well, add `--chart` with the path of the Radius chart, such as `deploy/Chart` in your clone.

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
