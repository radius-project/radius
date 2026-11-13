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

`rad install kubernetes` and `rad upgrade kubernetes` download the Radius chart from `oci://ghcr.io/radius-project/helm-chart` and the Contour chart from `https://projectcontour.github.io/helm-charts`, unless you install from local charts. When authorization requires installing or upgrading cert-manager, they also download its chart from `https://charts.jetstack.io`. These downloads don't use the Helm repositories you added with `helm repo add` or their cached indexes, so you don't need to run `helm repo update` first, and a missing or broken repository in your Helm configuration doesn't affect them. The Radius chart download still uses your registry credentials from `helm registry login` or `docker login`.

Each request times out after 30 seconds. A download that fails with a temporary error, such as a timeout, a dropped connection, or an HTTP 408, 429, 500, 502, 503, or 504 response, is retried for up to 3 minutes, so it gives up within about 4 minutes. Each retry prints the error that caused it:

```text
Downloading the contour chart failed (attempt 1), retrying: Get "https://...": context deadline exceeded (Client.Timeout exceeded while awaiting headers)
```

When the retries run out, or the error isn't temporary, the command stops with `error downloading helm chart from the registry` followed by the last error. A timeout or connection error means that the machine can't reach the chart host, so check its network, proxy, and firewall settings. A `401` or `403` response points at the registry credentials. For a `403` from `ghcr.io`, the command reports `received 403 unauthorized` instead and suggests running `docker logout ghcr.io`.

On a network that can't reach the chart hosts, download the charts on a machine that can, copy them over, and install from the local files. For the Contour chart, use the version that `ContourChartDefaultVersion` sets in [`pkg/cli/helm/contour.go`](../../../../pkg/cli/helm/contour.go):

```bash
helm pull contour --repo https://projectcontour.github.io/helm-charts --version <version>
rad install kubernetes --reinstall --contour-chart ./contour-<version>.tgz
```

An earlier attempt may have installed Radius before the Contour download failed, and without `--reinstall` the command then stops at `Found existing Radius installation` instead of installing Contour. `--reinstall` upgrades Radius, keeping the values you set before, and installs Contour, and on a cluster without Radius it installs as usual.

The command still downloads the Radius chart. To install Radius from a local chart as well, add `--chart` to it with the path of the Radius chart, such as `deploy/Chart` in your clone.

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

cert-manager has a separate, fixed 10-minute Helm timeout; `--timeout` does not extend it.

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

## cert-manager

**Warning:** Radius RBAC is a work in progress. Full functionality is not guaranteed. The `global.rbac.enabled` and `global.rbac.dryRun` Helm settings are experimental; do not rely on RBAC for production access control. Install, reinstall, and upgrade print this warning before managing cert-manager whenever the effective settings enable RBAC, including when they reuse stored release values.

Radius authorization depends on [cert-manager](https://cert-manager.io/). `rad install kubernetes` and `rad upgrade kubernetes` only manage cert-manager when authorization is enabled, by passing exactly one of `--set global.rbac.dryRun=true` or `--set global.rbac.enabled=true`, or by reusing those values stored on the Radius release during upgrade or `--reinstall`. Current flags override stored values; `rad upgrade kubernetes --reset-values` discards stored values. With authorization off (the default), install and upgrade do not look for, install, or change cert-manager.

### Supported versions

`rad` installs a pinned cert-manager chart (`CertManagerChartDefaultVersion` in `pkg/cli/helm/cert_manager.go`, currently `v1.21.2`) from `https://charts.jetstack.io` as the Helm release `cert-manager` in the `cert-manager` namespace, with CRDs enabled. It limits cert-manager's default approver with `approveSignerNames` to the Radius bootstrap and API server Issuers (`radius-selfsigned-bootstrap` and `radius-apiserver-issuer`) and to ClusterIssuers, so that only Radius's csi-driver-spiffe approves requests to `radius-ca-issuer`. An existing cert-manager must be at least `CertManagerMinimumVersion` (currently `v1.20.0`). Both constants track the [cert-manager supported releases](https://cert-manager.io/docs/releases/).

### Existing cert-manager installations

`rad` detects cert-manager by the `certificates.cert-manager.io` CRD and a controller Deployment in any namespace labeled `app.kubernetes.io/name=cert-manager,app.kubernetes.io/component=controller`. It reads the version from the controller's `app.kubernetes.io/version` label, or from its image tag.

Before reusing an installation, `rad` checks that the controller and webhook Deployments have observed their current generation and have the desired updated and available replicas. If either is missing, scaled to zero, or not ready, repair cert-manager or wait for its rollout before retrying Radius.

| What `rad` finds                               | What `rad` does                                                            |
|------------------------------------------------|----------------------------------------------------------------------------|
| No CRD and no controller                       | Installs the pinned version and marks the release as owned by `rad`        |
| A supported version that `rad` did not install | Reuses it; never upgrades, modifies, or removes it                         |
| A version older than the minimum               | Stops before installing Radius and names the minimum version to upgrade to |
| The CRD without a controller (or the reverse)  | Fails; repair or remove the broken installation, then retry                |

### Ownership

`rad` marks a release it installed with the Helm release label `app.kubernetes.io/managed-by=rad`. Helm 4 (used by Radius) shows release labels in `helm get metadata cert-manager -n cert-manager`; Helm 3 does not. You can also inspect the Helm release Secrets:

```bash
kubectl get secret -n cert-manager -l owner=helm,name=cert-manager --show-labels
```

`rad` only upgrades a release it owns, and repairs an owned release that is not `deployed`, whose controller is missing, or that lacks the `approveSignerNames` setting, for example because an older `rad` installed it. It never downgrades a release whose chart version is newer than its target pin: a healthy newer installation is reused, and an unhealthy newer installation must be repaired at its current version. Kubernetes API errors are reported rather than silently triggering a repair.

### Uninstall

`rad uninstall kubernetes` removes an owned cert-manager release. The chart is installed with `crds.keep=false`, so this also deletes the cert-manager CRDs and every Certificate, CertificateRequest, Issuer, ClusterIssuer, ACME Order, and ACME Challenge in the cluster. To protect other workloads, `rad` first lists those resources and keeps cert-manager if any exist outside the Radius namespace (ClusterIssuers always count) or if ownership or resource checks fail. Missing resource types count as empty lists so an interrupted uninstall can be retried:

```text
Keeping cert-manager: other workloads use it (2 resources, e.g. my-app/Certificate/my-cert)
```

A cert-manager that `rad` did not install is never removed.

### Recovering from a partial install or uninstall

- **cert-manager installed, Radius install failed:** fix the cause and rerun the same `rad install kubernetes` command (add `--reinstall` if a Radius release was left behind). `rad` reuses or repairs the cert-manager it owns. To back out instead, run `rad uninstall kubernetes`.
- **Uninstall interrupted or cert-manager left behind:** rerun `rad uninstall kubernetes`. If that is not possible, check for other workloads **before** removing the release manually:

  ```bash
  kubectl get certificates,certificaterequests,issuers,orders.acme.cert-manager.io,challenges.acme.cert-manager.io -A
  kubectl get clusterissuers
  ```

  **Warning:** because `rad` installed the chart with `crds.keep=false`, `helm uninstall` itself deletes all six CRDs and their resources cluster-wide. Do not run it while other workloads use cert-manager; keep the installation instead. Only after confirming that no other workload depends on these resources, remove the release:

  ```bash
  helm uninstall cert-manager -n cert-manager
  ```

  If an interrupted uninstall left CRDs behind, delete only these six cert-manager CRDs, after the same dependency check:

  ```bash
  kubectl delete crd --ignore-not-found \
    certificates.cert-manager.io certificaterequests.cert-manager.io \
    issuers.cert-manager.io clusterissuers.cert-manager.io \
    orders.acme.cert-manager.io challenges.acme.cert-manager.io
  ```

## csi-driver-spiffe

When authorization is enabled, `rad install kubernetes` and `rad upgrade kubernetes` also install Radius's own [csi-driver-spiffe](https://cert-manager.io/docs/usage/csi-driver-spiffe/) after cert-manager. It issues server certificates to Radius components through a CSI volume. `rad` installs a pinned chart (`CSIDriverSPIFFEChartDefaultVersion` in `pkg/cli/helm/csi_driver_spiffe.go`) from `https://charts.jetstack.io` as the Helm release `cert-manager-csi-driver-spiffe` in the Radius namespace, labeled `app.kubernetes.io/managed-by=rad`. It configures the trust domain and CSI driver name from `global.rbac.spiffe` and limits the driver's approver to the `radius-ca-issuer` Issuer. With authorization off, `rad` does not look for or install it.

`rad` runs every check below except the default approver check before it changes the cluster, so those conflicts leave it unchanged. It checks cert-manager's default approver after it installs, repairs, or reconfigures the cert-manager it owns, because that step sets `approveSignerNames`, but before it installs or upgrades csi-driver-spiffe or Radius. A conflict there can leave an owned cert-manager reconfigured, which is safe to keep.

- **Another csi-driver-spiffe runs on the cluster:** a cluster can run only one, because every instance registers the same kubelet plugin path, `/var/lib/kubelet/plugins/cert-manager-csi-driver-spiffe`, even when installed with `nameOverride`. `rad` finds any DaemonSet that mounts or registers that path, stops, and names the DaemonSet to remove.
- **The CSI driver name is taken:** a `CSIDriver` with the name in `global.rbac.spiffe.csiDriverName` exists and Radius's release did not create it. Delete it or choose another name.
- **A `cert-manager-csi-driver-spiffe` release that `rad` did not install:** `rad` does not change it. Uninstall it, or let `rad` own the installation, and rerun the command.
- **The cluster does not serve ValidatingAdmissionPolicy:** the Radius chart's `radius-certificate-issuance` policy needs `admissionregistration.k8s.io/v1`, which Kubernetes 1.30 and later serve. The policy lets only the cert-manager controller request certificates from the bootstrap and API server Issuers, and limits the Radius service accounts to `radius-ca-issuer`.
- **cert-manager's default approver can approve requests to `radius-ca-issuer`:** an existing cert-manager approves CertificateRequests for every issuer unless its default approver is turned off or limited. Because the Radius components may create CertificateRequests for `radius-ca-issuer`, only csi-driver-spiffe's approver, which checks the SPIFFE ID, may approve them. `rad` stops and prints a `helm upgrade` command that sets `approveSignerNames` to the bootstrap and API server Issuers (`radius-selfsigned-bootstrap` and `radius-apiserver-issuer`) and to ClusterIssuers. If the default approver cannot approve the bootstrap or API server Issuer, `rad` warns, because the Radius CAs and UCP's aggregated API certificate are then issued only if another approver approves them.
- **`rad rollback kubernetes` refuses a revision that issues component client Certificates:** earlier Radius charts with authorization on issued component client Certificates (`<component>-cert`) from `radius-ca-issuer` or `radius-mtls-issuer`. Rollback keeps cert-manager's default approver limited to the current signers, which do not include either issuer, so those certificates could never be issued or renewed after the rollback. Roll back to a revision that does not create them, or keep the current version.
- **Components fail to load `/var/run/radius/ca/ca.crt`, or pods wait on volume `radius-ca`:** every component reads the Radius CA bundle from the `radius-ca` Secret rather than from the csi-driver-spiffe volume, which holds only `tls.crt` and `tls.key`. Check that cert-manager issued Certificate `radius-ca` in the Radius namespace with `kubectl get certificate radius-ca -n radius-system`. Plain Helm users should not configure csi-driver-spiffe with a source CA bundle from `radius-ca`: the driver needs it at startup, before the chart creates that Secret.
- **cert-manager's cainjector is missing or cannot inject the APIService:** with authorization on, cert-manager's cainjector copies `ca.crt` of the `radius-apiserver-ca` Secret, annotated `cert-manager.io/allow-direct-injection: "true"` through its Certificate's `secretTemplate`, into the `v1alpha3.api.ucp.dev` APIService and updates it when cert-manager renews that CA. Without the cainjector, the API server keeps the old CA certificate and stops trusting UCP once it expires. The APIService never takes the `ca.crt` of UCP's certificate Secret, which can hold a root that the API server should not trust. `rad` stops before changing the cluster if the cainjector Deployment is missing or not ready, runs with `--enable-apiservices-injectable=false`, watches only another namespace with `--namespace`, or ignores the Radius namespace with `--ignore-namespaces`.
- **The UCP APIService is not usable after install, upgrade, or rollback:** `rad` waits for cert-manager to issue `radius-apiserver-ca`, points the APIService at it and sets the `caBundle`, checks that the `caBundle` holds no other unexpired certificate, and then waits for the APIService to report `Available=True`. If it has no CA bundle, check the Secret with `kubectl get secret radius-apiserver-ca -n radius-system -o yaml` and the cainjector logs. If it is not Available, check the ucp pods and run `kubectl get apiservice v1alpha3.api.ucp.dev -o yaml`.
- **The upgrade stops before Helm runs:** when an upgrade or rollback turns authorization on, `rad` first applies the `radius-selfsigned-bootstrap` and `radius-apiserver-issuer` Issuers and the `radius-apiserver-ca` Certificate, labeled for adoption by the Radius Helm release, and waits for cert-manager to issue the CA. If it is not issued in time, `rad` fails before calling Helm and suggests `kubectl describe certificate radius-apiserver-ca -n radius-system`. Fix the issuance and run the command again; it reuses the objects it applied. If one of those objects exists without the Radius release's Helm metadata, `rad` refuses to adopt it and suggests deleting it.
- **The upgrade stops while UCP moves to the new CA:** when Radius moves from the Helm-generated `ucp-ca` to `radius-apiserver-ca`, `rad` trusts both CAs through the `radius-apiserver-trust` Secret before Helm reissues UCP's certificate. It decides this from the CAs in the APIService `caBundle`, not from its `cert-manager.io/inject-ca-from-secret` annotation, which an earlier switch to authorization off leaves in place. It then waits until the `ucp` Deployment has rolled out and its certificate in `ucp-apiserver-tls` chains to `radius-apiserver-ca`, and only then points the APIService at `radius-apiserver-ca` and deletes that Secret, which removes `ucp-ca`. If that does not happen within five minutes, `rad` fails and leaves both CAs trusted, so UCP stays reachable. Fix the rollout, check it with `kubectl rollout status deployment/ucp -n radius-system`, and run the command again to remove `ucp-ca`.
- **`CertificateRequestDenied` admission errors:** the `radius-certificate-issuance` ValidatingAdmissionPolicy lets only the users in `global.rbac.certManager.requesters` request certificates from `radius-apiserver-issuer` and `radius-selfsigned-bootstrap`, and limits other requesters in the Radius namespace to `radius-ca-issuer`. `rad` sets the requesters to the ServiceAccounts of the cert-manager controllers it finds. If you install the chart with plain Helm or move cert-manager, set `global.rbac.certManager.requesters` to its controller's username, such as `system:serviceaccount:cert-manager:cert-manager`. The policy needs Kubernetes 1.30 or later, and `rad` stops on older clusters.

`rad uninstall kubernetes` removes the csi-driver-spiffe release it owns before cert-manager, and lists it in the confirmation prompt. A release that `rad` did not install is never removed. The kubelet needs the driver to unmount a terminating Pod's certificate volume, so `rad` first waits up to three minutes for every Pod that mounts a volume from the driver to be deleted. If some remain, or `rad` cannot check ownership or Pods, it keeps the driver and cert-manager, which the driver needs, warns, and names one of the Pods. Run the uninstall again once they are gone. With `--purge`, `rad` then stops after uninstalling Radius and Contour, without deleting namespaces, the Radius CRDs, or the UCP APIService, because deleting `radius-system` would remove the driver before the kubelet unmounts those Pods' volumes. It prints the steps that finish the purge.

`rad rollback kubernetes` rolls back only the Radius release. It first refuses a target revision that issues component client Certificates, as described above, before it changes anything. Otherwise, before it rolls back, `rad` makes cert-manager and csi-driver-spiffe match the target revision's authorization settings: it installs or reconfigures them, for example to the target's trust domain or CSI driver name, with the same checks as an install. If the target has authorization off, it leaves them in place.
