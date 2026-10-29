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

## Contour cannot use the Gateway API CRDs in the cluster

`rad install kubernetes` also installs Contour, which needs the Gateway API CRDs. When the cluster already has Gateway API CRDs from another installer, such as the Traefik chart that k3s and k3d install by default, Radius reuses them instead of letting Contour take them over. Before it changes the cluster, Radius checks that they serve the resource versions that Contour watches. The CRDs of k3s Traefik do not, so the command stops with a message like this one, and nothing is installed:

```text
Radius stopped before changing the cluster because Contour 1.32.0 cannot use the Gateway API CRDs in this cluster. Contour needs these resource versions:
  - tlsroutes.gateway.networking.k8s.io v1alpha2 (the CRD is not installed)
  - tcproutes.gateway.networking.k8s.io v1alpha2 (the CRD is not installed)
  - backendtlspolicies.gateway.networking.k8s.io v1alpha3 (the installed CRD serves v1)
```

Contour exits when it cannot watch one of these resources. To continue, either turn off the Contour features that use them, with the `--contour-set` value that the message suggests. Contour then ignores TLSRoute, TCPRoute, and BackendTLSPolicy resources, and `rad upgrade kubernetes` accepts the same value:

```bash
rad install kubernetes --contour-set "contour.extraArgs={--disable-feature=tlsroutes,--disable-feature=tcproutes,--disable-feature=backendtlspolicies}"
```

Or use a cluster without preinstalled Gateway API CRDs. For example, create the k3d cluster without Traefik:

```bash
k3d cluster create --k3s-arg "--disable=traefik@server:*"
```

k3s installs the Traefik CRDs in the background after the cluster starts. When `rad install kubernetes` runs right after the cluster is created, Radius logs `Waiting for k3s to finish installing the Traefik CRDs...` and waits up to three minutes, so that it checks the CRDs that the cluster ends up with. If k3s does not finish in time, the message says what Radius observed, for example that the pod of the k3s job restarted several times and is in `CrashLoopBackOff`, and shows the `kubectl` commands that list the pods of the job and show its logs. Radius does not wait, and logs a warning instead, when k3s cannot install the Traefik CRDs because a Gateway API CRD that they include belongs to something else, such as a `contour` release that an earlier Radius version installed, or when the k3s job finished but the `traefik-crd` release no longer exists, for example after `helm uninstall traefik-crd`.

If an earlier attempt failed while Helm installed Contour, the `contour` release in `radius-system` has a failed revision. Run `rad install kubernetes` again: after the same checks, Radius upgrades the failed release before it installs Radius. When Radius is already installed, the command prints the `rad install kubernetes --reinstall` command that repairs Contour. If the message instead says that the Gateway API CRDs have more than one owner, run the `kubectl annotate` command that it shows, and then run `rad install kubernetes` again.

The `kubectl` and `helm` commands in these messages name the Kubernetes context of the cluster that Radius checked, so they act on that cluster even when the current context of your kubeconfig selects another one.
