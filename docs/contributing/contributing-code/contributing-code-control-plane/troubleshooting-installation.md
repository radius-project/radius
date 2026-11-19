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

These settings support fresh enabled installations and preserving an existing enabled mode. Turning authorization from Off to DryRun or Enforce on an existing installation is rejected before mutation until observer-backed lifecycle support can prove that consumers loaded the new trust. Installing cert-manager, scheduling a maintenance window, or using `--reinstall` does not remove that block.

Authorization-enabled install and upgrade, and supported Off transitions or cleanup through install, upgrade and rollback, also require `create` on `pods/portforward` in `radius-system` (or the configured Radius namespace) for exact-Pod serving-certificate checks. If the API server rejects the tunnel, ask the cluster administrator to grant this permission to the CLI's Kubernetes identity. Check the selected context with `kubectl --context CONTEXT auth can-i create pods --subresource=portforward -n radius-system`, replacing `CONTEXT` and the namespace as needed. Port-forward access does not supply the missing production loaded-trust observations or enable CA rotation.

Cert-manager preflight, certificate Secret operations, and Helm use the same CLI kubeconfig selection: merge the files in `KUBECONFIG`, or use `~/.kube/config` when it is unset, and honor the selected `--kubecontext`. Missing entries in a merged list are skipped; malformed or unusable configurations and unknown contexts fail explicitly rather than selecting a different cluster. This CLI behavior does not change server-side kubeconfig loading.

### Supported versions

`rad` installs a pinned cert-manager chart (`CertManagerChartDefaultVersion` in `pkg/cli/helm/cert_manager.go`, currently `v1.21.2`) from `https://charts.jetstack.io` as the Helm release `cert-manager` in the `cert-manager` namespace, with CRDs enabled. An existing cert-manager must be at least `CertManagerMinimumVersion` (currently `v1.20.0`). Both constants track the [cert-manager supported releases](https://cert-manager.io/docs/releases/).

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

`rad` only upgrades a release it owns, and repairs an owned release that is not `deployed` or whose controller is missing, except while a Helm operation is pending. It never downgrades a release whose chart version is newer than its target pin: a healthy newer installation is reused, and an unhealthy newer installation must be repaired at its current version. Kubernetes API errors are reported rather than silently triggering a repair.

### Uninstall

`rad uninstall kubernetes` removes an owned cert-manager release. The chart is installed with `crds.keep=false`, so this also deletes the cert-manager CRDs and every Certificate, CertificateRequest, Issuer, ClusterIssuer, ACME Order, and ACME Challenge in the cluster. To protect other workloads, `rad` first lists those resources and keeps cert-manager if any exist outside the Radius namespace (ClusterIssuers always count) or if ownership or resource checks fail. Missing resource types count as empty lists so an interrupted uninstall can be retried:

```text
Keeping cert-manager: other workloads use it (2 resources, e.g. my-app/Certificate/my-cert)
```

A cert-manager that `rad` did not install is never removed.

A self-signed upgrade that turns authorization on creates the `radius-selfsigned-bootstrap` and `radius-apiserver-issuer` Issuers and the `radius-apiserver-ca` Certificate before Helm adopts them into the release. If the upgrade stopped before Helm ran, `helm uninstall` leaves them, so `rad uninstall kubernetes` deletes each one that still carries the Radius release's Helm ownership metadata and has not changed since `rad` read it. It leaves any other object with those names and logs a warning with the `kubectl delete` command. After an upgrade that turns authorization off, `rad` deletes these staged objects the same way, except any that the applied chart still declares, such as the `radius-selfsigned-bootstrap` Issuer of an earlier chart. `rad uninstall kubernetes` also deletes a `ucp-cert` Secret that an interrupted switch to authorization off staged, and the `radius-apiserver-ca` and `ucp-apiserver-tls` Secrets that cert-manager issued for the release's Certificates, which have no owner reference, under the same ownership and unchanged-since-read rules.

### Recovering from a partial install or uninstall

- **Controller strategy migration failed:** on an installed release, `rad` checks the selected chart and certificate settings before preparing dependencies. It then migrates the controller before rendering the reinstall or upgrade action (including `--reset-values`) for certificate staging. If migration fails, this attempt has not imported a signing Secret, staged UCP trust, or applied a Radius revision. Dependencies prepared earlier may remain. Resolve the reported controller error before retrying. A fresh install skips migration even with `--reinstall`; `--preflight-only` renders the upgrade without migrating or staging.
- **Pending cert-manager Helm release:** install, `--reinstall`, and upgrade stop before changing the configured cert-manager release when its latest revision is `pending-install`, `pending-upgrade`, or `pending-rollback`, whether or not `rad` owns it. The error identifies the release, namespace, revision, state and kubeconfig context, with context-bound `helm status`, `helm history`, and `kubectl` inspection commands. An operation may still be running; pending status alone does not prove it was interrupted. Wait for an active operation to finish. Before recovery, confirm that no Helm operation is active and coordinate with the installation's operator. A pending install may have no successful revision to roll back to, so inspect partially created resources and plan recovery without removing shared resources. For a pending upgrade or rollback, review the history and the resource changes before considering the suggested rollback command; replace `REVISION` with a known-good revision, not the pending one. A rollback can change or delete shared resources and issuer configuration. Back up release history, issuer keys and certificate Secrets first. Do not uninstall cert-manager or delete CRDs or Helm release Secrets as a shortcut: this can destroy other workloads' certificates or orphan resources. `rad` never performs recovery automatically or takes ownership of an operator-managed release. Retry the same `rad` command after the operator resolves the pending state.
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

### Server certificates and the Radius CA bundle

With authorization enabled and the default self-signed Radius provider, ucp, applications-rp, and dynamic-rp read their server certificate from Secret `<component>-spiffe`, mounted at `/var/run/radius/spiffe`, and ucp, applications-rp, dynamic-rp, and controller read the Radius CA bundle from `ca.crt` of Secret `radius-ca`, mounted at `/var/run/radius/ca`. Both are required volumes, so a pod that waits on volume `spiffe` or `radius-ca`, or a component that fails to load `/var/run/radius/spiffe/tls.crt` or `/var/run/radius/ca/ca.crt`, means cert-manager has not issued that Certificate yet. Check it, for example with `kubectl get certificate radius-ca ucp-spiffe applications-rp-spiffe dynamic-rp-spiffe -n radius-system`, and look at the cert-manager controller logs for the reason.

### UCP API server certificate

For `caSecret` and `issuerRef`, inspect the APIService's injection annotation: it should name `<release-namespace>/radius-apiserver-trust`. This is a kept public CA, not a transient overlap Secret. Do not delete it or point injection to the serving Secret. For managed component TLS check `radius-mtls-ca` and the receiver's `-spiffe` Certificate; the external provider has neither. For issuer readiness and operator approval requirements, see the [chart certificate settings](../../../../deploy/Chart/README.md).

With authorization on and self-signed issuance, UCP's aggregated API on port 9443 serves Certificate `ucp-cert` (Secret `ucp-apiserver-tls`) from Issuer `radius-apiserver-issuer` and its own CA, `radius-apiserver-ca`. Only the Kubernetes API server trusts that CA; no Radius component mounts it.

- **cert-manager's cainjector is missing or cannot inject the APIService:** with authorization on, cert-manager's cainjector copies `ca.crt` of the `radius-apiserver-ca` Secret, annotated `cert-manager.io/allow-direct-injection: "true"` through its Certificate's `secretTemplate`, into the `v1alpha3.api.ucp.dev` APIService and updates it when cert-manager renews that CA. Without the cainjector, the API server keeps the old CA certificate and stops trusting UCP once it expires. The APIService never takes the `ca.crt` of UCP's certificate Secret, which can hold a root that the API server should not trust. `rad` stops before changing the cluster if the cainjector Deployment is missing or not ready, runs with `--enable-apiservices-injectable=false`, watches only another namespace with `--namespace`, or ignores the Radius namespace with `--ignore-namespaces`. It also reads the configuration file that `--config` names from its ConfigMap or Secret volume and stops if the file sets `enableInjectableConfig.apiServices: false` or another `namespace`, unless a flag overrides that setting. If `rad` cannot read the file, it prints a warning and the command fails later if the CA certificate never reaches the APIService. `rad` checks the container whose name, image or command is `cainjector` or `cert-manager-cainjector`, or the only container. If the Deployment has several containers and none matches, `rad` cannot tell which runs cainjector, so it skips the flag checks and prints a warning instead of reading another container's flags.
- **The UCP APIService is not usable after install, upgrade, or rollback:** `rad` waits for cert-manager to issue `radius-apiserver-ca`, points the APIService at it and sets the `caBundle`, checks that the `caBundle` holds no other unexpired certificate, and then waits for the APIService to report `Available=True`. If it has no CA bundle, check the CA with `kubectl describe certificate radius-apiserver-ca -n radius-system` and `kubectl get secret radius-apiserver-ca -n radius-system -o jsonpath='{.data.ca\.crt}'`, and the cainjector logs. Don't print the whole Secret: it holds the CA's private key. If it is not Available, check the ucp pods and run `kubectl get apiservice v1alpha3.api.ucp.dev -o yaml`.
- **The upgrade stops while UCP moves to the new CA:** when Radius moves from the Helm-generated `ucp-ca` to `radius-apiserver-ca`, `rad` trusts both CAs through the `radius-apiserver-trust` Secret before Helm reissues UCP's certificate. It decides this from the CAs in the APIService `caBundle`, not from its `cert-manager.io/inject-ca-from-secret` annotation, which an earlier switch to authorization off leaves in place. It then waits until the `ucp` Deployment has rolled out and its certificate in `ucp-apiserver-tls` chains to `radius-apiserver-ca`, and only then points the APIService at `radius-apiserver-ca` and deletes the `radius-apiserver-trust` Secret, which removes `ucp-ca`. If that does not happen within five minutes, `rad` fails and leaves both CAs trusted, so UCP stays reachable. Fix the rollout, check it with `kubectl rollout status deployment/ucp -n radius-system`, and run the command again to remove `ucp-ca`. `rad` uses, changes and deletes the `radius-apiserver-trust` Secret only when it holds only `ca.crt` and has the `cert-manager.io/allow-direct-injection: "true"` annotation, as `rad` and the chart README's plain-Helm steps create it, and deletes it only if it is unchanged since `rad` read it. If another Secret has that name, the upgrade stops before Helm runs and suggests a `kubectl delete secret` command, and other commands leave it and print a warning.
- **The upgrade or rollback that turns authorization off stops while UCP moves to `ucp-cert`:** with authorization off, the chart serves UCP from the `ucp-cert` Secret signed by the Helm-generated `ucp-ca` and sets the APIService `caBundle` to `ucp-ca` alone. Before calling Helm, `rad` applies the `ucp-cert` Secret that the target release renders, labeled for adoption by the Radius Helm release, adds its CA to the APIService `caBundle` next to `radius-apiserver-ca`, mounts it in the `ucp` Deployment, and waits until every UCP pod serves it. Helm then keeps that Secret, or a rollback applies the same one, so the `caBundle` it sets trusts what UCP serves. If UCP does not roll out within five minutes, `rad` fails before calling Helm with both CAs trusted. Fix the rollout, check it with `kubectl rollout status deployment/ucp -n radius-system`, and run the command again.
- **After authorization turns off, the APIService still trusts `radius-apiserver-ca`:** Helm cannot remove the `cert-manager.io/inject-ca-from-secret` annotation that `rad` set, and cert-manager keeps the `radius-apiserver-ca` Secret after Helm removes its Certificate, so cainjector would keep adding that CA to the `caBundle`. After Helm, `rad` waits until every UCP pod serves `ucp-cert`, then removes the cert-manager inject annotations and sets the `caBundle` to the `ucp-cert` CA in one patch that applies only to the APIService version it read, and repeats the patch if cainjector writes first. It then deletes the `radius-apiserver-ca`, `ucp-apiserver-tls` and `radius-apiserver-trust` Secrets, but only those it owns (issued by cert-manager for the expected Certificate, or holding only the `ca.crt` that `rad` writes) and only if they are unchanged since it read them. It leaves any other Secret with those names and prints a warning with a `kubectl delete secret` command. Turning authorization on again issues a new `radius-apiserver-ca`. If `rad` stops before this, the API server trusts both CAs, which still verifies UCP; run the same command again, or `rad install kubernetes --reinstall`, `rad upgrade kubernetes` or `rad rollback kubernetes` to an authorization-off release, to finish. A plain `rad install kubernetes` on an existing installation does not change the release and does not finish it. Check the result with `kubectl get apiservice v1alpha3.api.ucp.dev -o yaml`.
