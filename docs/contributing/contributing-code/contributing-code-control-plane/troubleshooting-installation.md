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

Contour exits when it cannot watch one of these resources. To continue, turn off the Contour features that use them with the `--contour-set` value that the message suggests. Contour then ignores TLSRoute, TCPRoute, and BackendTLSPolicy resources, and `rad upgrade kubernetes` accepts the same value:

```bash
rad install kubernetes --contour-set "contour.extraArgs={--disable-feature=tlsroutes,--disable-feature=tcproutes,--disable-feature=backendtlspolicies}"
```

This is a CRD compatibility remedy, not a complete ingress configuration. On k3s with Traefik and ServiceLB, also address the separate port conflict described below. Disabling these features means Contour cannot reconcile Gateway API TLS passthrough routes, TCP routes, or backend TLS policies; a successful HTTP request does not prove equivalent TLS/TCP behavior. Do not silently disable features an application needs, replace the shared CRDs, or transfer their ownership to Contour.

If you set the Contour arguments with `--contour-set-file`, Radius applies the file after every `--contour-set` value, so the message instead lists the `--disable-feature` entries to add to the list in the file, or tells you to replace the `--contour-set-file` flag with the suggested `--contour-set` value.

Or use a cluster without preinstalled Gateway API CRDs. For example, create the k3d cluster without Traefik:

```bash
k3d cluster create --k3s-arg "--disable=traefik@server:*"
```

Or keep Traefik as your ingress controller and install Radius without Contour. Radius gateways use Contour, so they do not work without it. If you use `rad init`, run this command first, and then run `rad init` again:

```bash
rad install kubernetes --skip-contour-install
```

k3s installs the Traefik CRDs in the background after the cluster starts. When `rad install kubernetes` runs right after the cluster is created, Radius logs `Waiting for k3s to finish installing the Traefik CRDs...` and waits up to three minutes, so that it checks the CRDs that the cluster ends up with. If k3s does not finish in time, the message says what Radius observed, for example that the pod of the k3s job restarted several times and is in `CrashLoopBackOff`, and shows the `kubectl` commands that list the pods of the job and show its logs. Radius does not wait, and logs a warning instead, when k3s cannot install the Traefik CRDs because a Gateway API CRD that they include belongs to something else, such as a `contour` release that an earlier Radius version installed, or when the k3s job finished but the `traefik-crd` release no longer exists, for example after `helm uninstall traefik-crd`.

If an earlier attempt failed while Helm installed Contour, the `contour` release in `radius-system` has a failed revision. Run `rad install kubernetes` again: after the same checks, Radius upgrades the failed release before it installs Radius. When Radius is already installed, the command prints the `rad install kubernetes --reinstall` command that repairs Contour. If the message instead says that the Gateway API CRDs have more than one owner, run the `kubectl annotate` command that it shows, and then run `rad install kubernetes` again.

When you pass `--kubecontext`, the `kubectl` and `helm` commands in these messages name that context, so they act on the cluster that Radius checked even when the current context of your kubeconfig selects another one. Without `--kubecontext`, Radius and the commands both use the current context of your kubeconfig.

## Contour is ready, but external HTTP reaches Traefik or times out

CRD compatibility and external reachability are separate checks. In a default k3s/k3d cluster, Traefik's LoadBalancer Service uses ports 80/443. k3s ServiceLB reserves those ports on the nodes. Contour's Envoy LoadBalancer Service requests the same ports by default, so its ServiceLB pods can remain Pending with `FailedScheduling` events reporting that nodes "didn't have free ports". Contour can be Ready and the Radius Gateway can report `Programmed=True` while external requests reach Traefik (often returning 404) or cannot reach Envoy at all.

Check the Service address, Service ports, and scheduling events, not just the Gateway:

```bash
kubectl get services -n kube-system traefik
kubectl get services -n radius-system contour-envoy
kubectl get pods -n kube-system -o wide
kubectl get events -n kube-system --sort-by=.metadata.creationTimestamp
kubectl get gateway radius -n radius-system -o yaml
```

### Fresh k3d installation with both controllers

Keep Traefik enabled and leave its configuration unchanged. Explicitly expose Envoy on Service ports 8080/8443 with `envoy.hostNetwork=false`. Publish those **node-side** ports through k3d. Changing only a laptop port (for example `18081:80`) still targets node port 80 and therefore Traefik; it does not fix the conflict.

For a disposable cluster, the following example uses k3s `v1.35.5-k3s1`, whose bundled Traefik CRDs require the feature-disable remedy above for Contour 1.32.0. Use a `rad` build containing that remedy and working `--contour-set` propagation. Choose unused host ports and a new cluster name:

```bash
k3d cluster create radius-coexistence \
  --image rancher/k3s:v1.35.5-k3s1 \
  --kubeconfig-update-default=false --kubeconfig-switch-context=false \
  --port '127.0.0.1:18080:80@loadbalancer' \
  --port '127.0.0.1:18443:443@loadbalancer' \
  --port '127.0.0.1:18081:8080@loadbalancer' \
  --port '127.0.0.1:18444:8443@loadbalancer' \
  --wait --timeout 180s
coexistence_kubeconfig="$(mktemp)"
k3d kubeconfig get radius-coexistence >"${coexistence_kubeconfig}"

KUBECONFIG="${coexistence_kubeconfig}" rad install kubernetes \
  --contour-set "contour.extraArgs={--disable-feature=tlsroutes,--disable-feature=tcproutes,--disable-feature=backendtlspolicies}" \
  --contour-set envoy.hostNetwork=false,envoy.service.ports.http=8080,envoy.service.ports.https=8443
```

Traefik remains reachable on host ports 18080/18443; Contour is exposed on 18081/18444. The HTTPS mapping does not enable the disabled TLSRoute feature or configure certificates. Do not enable Envoy host networking: that would bind its listener ports directly on the node and reintroduce the conflict. The Service ports change, not the Radius Gateway's listener ports (80/443).

Radius still creates the Contour `GatewayClass` and the `radius-system/radius` Gateway; default HTTPRoutes, including those emitted by the `Radius.Compute/routes` recipe, remain attached to Contour, not Traefik. Legacy `Applications.Core/gateways` uses Contour HTTPProxy instead of HTTPRoute; the Gateway API feature-disable flags do not describe HTTPProxy's TLS capabilities. This workaround is not a switch to Traefik for either API.

Verify a known application response through each published HTTP port with the appropriate Host header, for example `curl --fail -H 'Host: radius.test' http://127.0.0.1:18081/` for an HTTPRoute with that hostname. Require `Accepted=True` and `ResolvedRefs=True` on the route's Contour parent, a populated Envoy LoadBalancer address, and the expected response body. The [opt-in coexistence smoke test](../contributing-code-tests/running-functional-tests.md#test-contour-and-traefik-coexistence) creates both routes and checks them before cleaning up.

Delete only this disposable cluster and its temporary kubeconfig when finished:

```bash
k3d cluster delete radius-coexistence
rm "${coexistence_kubeconfig}"
```

### Changing an already deployed Contour release

`rad install kubernetes --reinstall` reinstalls Radius and can repair a failed Contour release, but leaves an already deployed Contour release and its values unchanged. It is not the command for changing a healthy Envoy Service's ports.

Use `rad upgrade kubernetes` in the workspace for the intended cluster to apply new Contour values. This also upgrades Radius; choose the appropriate Radius version or local chart for your installation. Review the active workspace before running it:

```bash
rad workspace show
rad upgrade kubernetes \
  --contour-set "contour.extraArgs={--disable-feature=tlsroutes,--disable-feature=tcproutes,--disable-feature=backendtlspolicies}" \
  --contour-set envoy.hostNetwork=false,envoy.service.ports.http=8080,envoy.service.ports.https=8443
```

Upgrade preserves stored Helm overrides unless `--reset-values` is specified. The `contour.extraArgs` list above replaces that list; preserve any additional arguments your installation needs, including arguments supplied through `--contour-set-file`. Updating Service ports alone does not add missing k3d host port mappings: arrange matching exposure separately, or recreate a disposable development cluster with the mappings above. Do not delete an existing cluster containing workloads just to apply this example.
