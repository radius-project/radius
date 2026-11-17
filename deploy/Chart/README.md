# Introduction

The Radius helm chart deploys the Radius services on a Kubernetes cluster using Helm.

## Prerequisites

- Kubernetes cluster with RBAC enabled
- Helm 3

## Installing the Chart

To install the chart with the release name `radius`:

```console
helm upgrade --wait --install radius deploy/Chart -n radius-system
```

### Upgrading with component authorization settings

The v0.62.0 chart described `global.rbac.enabled` and `global.rbac.dryRun` as reserved settings with no runtime effect. In this chart, setting either one to `true` creates cert-manager Issuers and Certificates, so an upgrade that retains either setting now requires cert-manager v1.20.0 or later. With both settings `false` (the default), the chart does not create these resources and does not require cert-manager.

`rad install kubernetes` and `rad upgrade kubernetes` manage this prerequisite when a non-off mode is selected, including when upgrade reuses saved values. A plain `helm install` or `helm upgrade` does not install cert-manager: install a supported cert-manager, including its CRDs, and wait for its controller and webhook to be ready before applying the Radius chart with either setting enabled. Otherwise the upgrade fails because the cert-manager resource types are missing. See [cert-manager installation troubleshooting](../../docs/contributing/contributing-code/contributing-code-control-plane/troubleshooting-installation.md#cert-manager) for supported versions and ownership rules.

With either setting enabled and `global.rbac.tls.provider=radius` (the default), ucp, applications-rp, and dynamic-rp mount csi-driver-spiffe CSI volumes; the deployment engine does so only with its compatible-image gate enabled. The controller is caller-only. Plain Helm users must install csi-driver-spiffe v0.15.1 with the selected issuer, driver name, trust domain and 24-hour duration. Its approver must have `autoApproveNonSPIFFE=false`, and cert-manager's default approver must not have approval permission for the internal signer. In `selfSigned`, the default approver may approve the distinct bootstrap and aggregation signers. The chart's `radius-certificate-issuance` policy restricts selected-issuer creation to active receivers or the configured cert-manager requester; it denies CA requests from the leaf issuers in both DryRun and Enforce. Kubernetes 1.30+ admission is required for BYO modes.

Internal public trust is mounted separately at `/var/run/radius/ca`; no signing key or other component's leaf Secret is mounted. Steady-state selfSigned installations project only `radius-ca/ca.crt`, so same-key, same-subject renewal reaches consumers without another Helm run. The retained `radius-mtls-ca` ConfigMap is a historical identity pin, not the renewal feed. BYO uses that frozen public ConfigMap. Journal-bound rotation preserves its exact public-source contract. Do not configure CSI with a startup source bundle that does not exist yet. See [csi-driver-spiffe installation troubleshooting](../../docs/contributing/contributing-code/contributing-code-control-plane/troubleshooting-installation.md#csi-driver-spiffe).

`global.rbac.tls.provider=external` uses HTTP for internal component calls under operator-provided encryption. Both providers use the same receiver-scoped ServiceAccount tokens and caller checks; external components do not mount or read internal CSI certificates or the Radius CA bundle. UCP's independent aggregated API TLS remains enabled and still requires cert-manager. The Helm key is `global.rbac.tls.provider`; it renders the runtime configuration field `authorization.tlsProvider`. Unknown providers, the retired `global.rbac.mtls` settings, and the unsupported Helm alias `global.rbac.tlsProvider` fail rendering rather than silently selecting Radius TLS.

### Changing the authorization mode or upgrading from an mTLS build

Turning authorization from Off to DryRun or Enforce on an existing installation is rejected before mutation until an observer-backed lifecycle can prove that consumers loaded the new trust. A maintenance window, `--reinstall`, or a plain Helm upgrade does not bypass this requirement. Fresh authorization-enabled CLI installation follows the bootstrap flow described below.

For otherwise supported changes, such as turning authorization off or upgrading an authorization-enabled edge build that authenticated components with client certificates, Helm rolls ucp, applications-rp, dynamic-rp, and the controller one at a time. Calls between old and new pods can fail TLS verification or authentication during that rollout. Radius does not fall back to weaker authentication. Plan these changes for a maintenance window; a window does not authorize an unsupported CA or provider transition:

1. Pause deployments that call Radius, such as `rad deploy` pipelines.
1. Use a CLI build with the paired certificate validation and lifecycle layers before changing CA, issuer or certificate mode. This chart layer does not implement those transitions. `rad rollback kubernetes` also refuses older charts that issue component client Certificates from `radius-ca-issuer` or `radius-mtls-issuer`. When authorization turns off, `rad` first applies the `ucp-cert` Secret that the off release uses, trusts its CA alongside `radius-apiserver-ca`, and waits until every UCP pod serves it, so the `ucp-ca` `caBundle` that Helm then applies matches what UCP serves. After Helm, it removes the `cert-manager.io/inject-ca-from-secret` annotation it set, sets the `caBundle` to the `ucp-cert` CA alone, and deletes the `radius-apiserver-ca`, `ucp-apiserver-tls` and transient `radius-apiserver-trust` Secrets, which the off release no longer uses, but only when `rad` owns them and they are unchanged since it read them; otherwise it leaves them and prints a warning. A mode-annotated public trust copy is retained, not treated as transient.
1. Retry any deployment that failed during the window.

A plain `helm upgrade --wait` waits only for the workloads; check the APIService with `kubectl get apiservice v1alpha3.api.ucp.dev` before resuming.

In steady-state `selfSigned`, the APIService injects public `ca.crt` directly from the cert-manager-owned `radius-apiserver-ca` Secret, whose Certificate sets the **annotation** `cert-manager.io/allow-direct-injection: "true"`. cainjector follows same-key, same-subject CA renewal without another Helm or `rad` run. During an authorization switch-on, Helm instead preserves the transient `radius-apiserver-trust` union that `rad` stages, so the API server still trusts the old UCP certificate until UCP rolls out. Only a source with non-empty `ca.crt`, no other data, the direct-injection annotation and **no** `radius.io/certificate-mode` annotation qualifies as transient. No signing key is mounted, and the source is never UCP's leaf Secret. A fresh Helm bootstrap can wait for cert-manager issuance and injection without a second render. An existing A8-era `radius-apiserver-trust` copy remains untouched and unused for injection: its mode annotation and Helm `keep` policy distinguish it from transient overlap, and `rad` uses it as a historical identity pin. In `caSecret` and `issuerRef`, the injection source remains the frozen public-only `radius-apiserver-trust`; renewing the signing CA does not update that pinned copy.

Turning authorization on for an existing installation remains blocked, including `caSecret` and `issuerRef` transitions and rollbacks from Off. Fresh enabled installs use the initial certificate bootstrap instead. The inherited selfSigned staging code cannot authorize a BYO transition or overwrite its pinned public source with a transient union. Plain Helm can reuse a retained BYO source only with the same configured `caBundle`; a changed bundle fails with lifecycle guidance.

Off transitions reject bootstrap, rotation and provider journals before mutation, including completed journals whose public sources remain bound to that history. For installations without those records, one operation Lease spans pre-Helm staging, Helm and post-Helm cleanup. Each trust mutation rechecks cancellation, lifecycle exclusion and object identity; updates and deletes carry UID/resource-version preconditions. Exact-Pod serving checks precede removing old aggregation trust. Rollout status and APIService convergence are not proof that consumers loaded trust. Permanent public sources remain retained.

Outside the guarded completed-bootstrap uninstall path described below, uninstall and authorization-off cleanup retain every mode-annotated public trust Secret; uninstall does not override its Helm `keep` policy. By default cert-manager also keeps the signer Secret after Helm removes its Certificate, so reinstall with that same signer and pin passes. A later selfSigned `rad install` with the pin but no usable original signer stops before creating a replacement signer. Restore the original signer backup with `kubectl --context "<context>" -n radius-system apply -f radius-apiserver-ca-backup.yaml`. For an intentional fresh installation, first verify the previous release is removed, then explicitly discard its historical pin with `kubectl --context "<context>" -n radius-system delete secret radius-apiserver-trust` as certificate lifecycle cleanup. Use the actual context, namespace and backup file; never silently accept a replacement key against the stale pin. Transient, unannotated trust is deleted only with UID/resourceVersion preconditions, and a foreign or changed object is left untouched.

Existing public sources are preserved byte-for-byte, carry `helm.sh/resource-policy: keep`, and must not be overwritten to switch CA or mode. Active or legacy `radius-ca-rotation` journals fail chart rendering. The schema4 completed-snapshot guard requires prior plane snapshots, exact pinned public-source content, and a chart-supported target issuer whose live UID and configuration digest still match; it does not execute or authorize transitions. Completed snapshots tolerate unrelated metadata updates while retaining source and issuer identity, ownership, journal binding and exact public bytes; active phases still require their observed resourceVersion. Issuer specifications are hashed, not copied into public journals or restored onto operator resources. Legacy aggregation-bootstrap journals and namespace intents also fail closed, including Off renders. CLI aggregation reconciliation preserves historical public copies and follows the selected source: the signer in steady-state selfSigned, or the exact public source for BYO and journal-bound rotation. cainjector v1.20/v1.21 merges CAs by default, so configuring the source alone does not prove that stale anchors have retired from the APIService.

Fresh authorization-enabled CLI installation stages the selected chart's selfSigned root Issuer and CA Certificates, waits for their current Ready observations and matching signing Secrets, then publishes independent public trust before applying the full chart. The Radius provider stages both planes; the external provider stages aggregation only. BYO modes publish their configured public anchors without adopting operator signing resources. The public-only `radius-certificate-bootstrap` journal records `preparing`, `published`, and `complete` stages under the shared identity-operation Lease. A full chart render rejects an unfinished publication or a changed pending intent; it preserves creation markers and the exact staged source bytes. Completed bootstrap records are history, not overrides of subsequent CA lifecycles.

Completed selfSigned bootstrap journals use schema2 to record the original subject public key info and encoded subject. Existing completed schema1 records recover that identity from their historical certificate, even after it expires. A renewal must retain the signer UID, expected cert-manager ownership, TLS Secret type and CA constraints. Default uninstall may retire a renewed generated signer with current UID/resource-version preconditions; it never deletes operator or imported signing CAs. Active bootstrap and rotation revisions remain exact.

Automatic CA rotation remains blocked before any mutation by missing production observation coverage **and** authorized access. Schema4 execution mechanics support staged roots/issuers, independent overlap publication, selected-chart workload CAS, explicit retirement and final Helm reconciliation with injected observation fixtures. Temporary phase projections are render-only; ordinary Helm still rejects active journals and final reconciliation uses the original chart. Internal-only plans preserve the existing aggregation serving Secret. The frozen schema2 report covers UCP-server trust only, not UCP's other receiver authorities or kube-apiserver cache adoption. Separate lifecycle serving measurements now enumerate every selected CSI receiver/listener and aggregation Pod, pin their Kubernetes identities and require new-only chains; this is not loaded-trust acknowledgment. No new endpoint, identity, token minting or anonymous bypass is supplied.

Fresh initial publication uses Kubernetes/cert-manager authority and does not need workload observer credentials. Initial installation still checks exact aggregation trust convergence, exact-Pod serving certificates and APIService availability; these checks do not prove outbound trust, token readiness or every API-server process's loaded trust. Local controller fixtures and isolated API-server tests cover execution and recovery, not live cert-manager issuance or installed-product acceptance. See [the observation gaps and retry guidance](../../docs/contributing/contributing-code/contributing-code-control-plane/configSettings.md#2-update-shared-service-options).

Authorization-enabled CLI install and upgrade, and supported Off transitions or cleanup through install, upgrade and rollback, require the Kubernetes permission `create` on `pods/portforward` in `radius-system` (or the configured Radius namespace), in addition to the ordinary installation permissions. The CLI uses authenticated exact-Pod tunnels to check serving certificates. This permission does not provide missing loaded-trust reports or authorize CA rotation.

At connected rendering, Helm checks the selfSigned signer's TLS Secret type, required keys, Certificate annotation and expected issuer annotations. `rad`, the supported lifecycle path for experimental RBAC, also compares the actual subject public key info and encoded subject against the retained public copy and any prior managed APIService anchor. Historical pins may have expired; a changed serial or expiry with the same key and subject passes, but a foreign signer or unexpected key/subject change fails with a lifecycle error. These are command-boundary checks, not admission control: cainjector follows signer changes between commands, and `rad` detects unexpected identity changes at the next command. Plain Helm does not perform that cryptographic comparison.

The shared `publicSource` contract returns `kind/name/key/bundle/uid/resourceVersion/contentDigest/issuerDigest/digest/issuer`: steady-state selfSigned selects signer Secret `radius-apiserver-ca` for aggregation or `radius-ca` for internal trust, projecting only `ca.crt`. BYO and journal-bound rotation retain their independent public sources. An offline render cannot resolve a newly issued anchor or its digests; lifecycle operations that need those pins must stage issuance before a connected render.

### Internal service certificate modes

`global.rbac.certificates.mode` selects the issuer and public anchor for both planes. `selfSigned` (default) retains separate Radius-owned internal and aggregation CAs. `caSecret` creates two distinct namespaced Issuers, `radius-ca-issuer` and `radius-apiserver-issuer`, referencing the **same operator-owned signing Secret**. `issuerRef` uses the selected existing `cert-manager.io` Issuer or ClusterIssuer for both profiles; the chart neither renders nor owns it. With `global.rbac.tls.provider=external`, only aggregation DNS issuance remains: no internal CA, CSI mount or receiver CertificateRequest-create Role is rendered.

The CLI certificate flags select these modes for either provider. It validates complete PEM input, CA/key/chain cryptography, and URI/DNS constraint feasibility using the selected chart's real issuer/receiver inventory. Both the actual chart's live policy render and cainjector prerequisite inspection run for BYO modes before resource changes when the controllers already exist. Enabling authorization with a BYO CA or issuer, including on a fresh installation, requires the paired certificate lifecycle; ordinary install and upgrade reject it before mutations. Public target planning does not authorize rotation or prove that an external issuer can issue a leaf. See [CLI certificate validation and lifecycle boundaries](../../docs/contributing/contributing-code/contributing-code-control-plane/configSettings.md#2-update-shared-service-options).

```yaml
global:
  rbac:
    dryRun: true
    tls:
      provider: radius
    certificates:
      mode: caSecret
      caSecret: operator-signing-ca
      caBundle: |
        -----BEGIN CERTIFICATE-----
        <public selected signing CA, not its parent chain>
        -----END CERTIFICATE-----
```

`caBundle` contains exactly one public selected signing CA certificate, usually a dedicated intermediate, never a private key, organization root substituted for that intermediate, or whole organization chain. Input accepts LF or CRLF PEM and surrounding whitespace; a newly rendered source trims the surrounding whitespace and adds one LF. An existing matching source retains its exact bytes. Ambiguous signing chains must be resolved by selecting the actual signing CA, not by broadening trust. Render-time checks validate PEM shape only. The paired CLI validation owns strict DER/CA/key/date checks and signing-Secret matching, and must verify feasibility for every actual receiver URI SAN and `ucp.<namespace>.svc`. Name constraints must allow both the URI trust-domain host and aggregation DNS suffix for managed BYO; external needs only DNS. A shared CA does not grant shared requester authority.

Only UCP aggregation uses an ordinary leaf Certificate: `ucp-cert` writes `ucp-apiserver-tls`, with one DNS SAN, ECDSA P-256, 24-hour duration, 8-hour early renewal and digital-signature/server-auth usages. Internal receivers use CSI, not `<component>-mtls` Secrets. Pinned CSI v0.15.1 requests **key encipherment, digital signature, client auth and server auth**; its evaluator requires those usages and the requester's own exact SPIFFE URI. The client-auth EKU does not authorize callers: runtime callers never present component client certificates and still send receiver-audience ServiceAccount tokens.

For `issuerRef`, leave `caSecret` empty and supply:

```yaml
issuerRef:
  group: cert-manager.io
  kind: Issuer # or ClusterIssuer
  name: existing-external-issuer
requestPolicy:
  certManager:
    namespace: cert-manager
    deployment: cert-manager
  approverPolicy:
    namespace: cert-manager
    deployment: cert-manager-approver-policy
  spiffe:
    namespace: radius-system
    deployment: cert-manager-csi-driver-spiffe-approver
  webhook:
    namespace: cert-manager
    service: cert-manager-webhook
    configuration: cert-manager-webhook
```

These are fields under `global.rbac.certificates`. A connected render checks Ready current issuer/controller rollouts and Pods, fail-closed cert-manager webhooks, effective policy-use/signer RBAC, and approver-policy v0.29.0. Managed mode also checks the pinned CSI approver profile and its selected-signer permissions; external does not require the CSI approver. Unsupported arguments, unknown controller configuration, unavailable reads, extra overlapping policies and other selected-signer approval grants fail explicitly. No boolean can replace this evidence, and no default approver may approve the shared signer.

The only approver-policy profile is `radius-<namespace>-ucp`: selector matches the exact issuer and namespace; `allowed` permits only required DNS `ucp.<namespace>.svc`, `isCA: false` and usages `digital signature`, `server auth`. Its DNS validation rule is `cr.username == '<actual cert-manager username>' && cr.namespace == '<namespace>' && cr.name.startsWith('ucp-cert-')`; constraints require min/max duration `24h` and ECDSA min/max size `256`. The actual cert-manager requester must appear in `global.rbac.certManager.requesters` and alone receive `use` on this policy. Do not bind Pod identities to it: approver-policy's policies are OR-ed, and an applicable DNS policy can deny a CSI request. The separate CSI approver handles annotated own-Pod URI requests and must skip unannotated DNS requests. The Kubernetes administrative `cluster-admin` boundary is unchanged; these checks do not isolate the existing privileged engine from other Secrets.

The shared CLI helpers in `pkg/cli/helm/certificate_bundle.go` and `cainjector_prerequisites.go` provide strict whole-input PEM parsing and read-only injector configuration discovery. PEM parsing preserves order and duplicates and does not apply CA validity or historical-overlap policy. Injector discovery checks all identifiable Deployments and Pods, requires exactly one eligible current rollout, supports inspectable v1.20/v1.21 flags and an immutable mounted `CAInjectorConfiguration`, and returns configuration/source/Pod pins. Unknown, mutable or unreadable configuration fails with a diagnostic. These helpers alone are not successful live issuance, injection, cryptographic feasibility or exact-set convergence evidence; A9/A10 must consume them.

### Deployment Engine token authentication integration prerequisite

`de.internalAuth.enabled` defaults to `false` independently of the global authorization mode. Enable it only with an explicitly selected compatible engine image (`de.image` and `de.tag`) implementing the token/server-TLS contract in [azure-octo/deployment-engine#714](https://github.com/azure-octo/deployment-engine/pull/714). This chart does not select or claim a compatible published default image. Image publication, a paired image reference, and composed live deployment verification remain integration prerequisites; source tests and chart rendering do not establish them.

Enabling the gate requires `global.rbac.dryRun=true` or `global.rbac.enabled=true`. The chart passes `RADIUS_INTERNAL_AUTH_MODE`, `RADIUS_INTERNAL_AUTH_NAMESPACE`, and `RADIUS_INTERNAL_TLS_PROVIDER` from the global mode, release namespace, and provider. Engine callbacks use `RADIUSBACKENDURL=https://ucp.<namespace>.svc:9444/apis/api.ucp.dev/v1alpha3` with the Radius provider, or the same URL with `http` for external. UCP routes engine requests to `https://bicep-de.<namespace>.svc:6443` or `http://bicep-de.<namespace>.svc:6443`, respectively. Port 6443 remains the engine's sole business listener.

Both providers project a 600-second token for the bare audience `ucp` into the engine at `/var/run/radius/tokens/ucp/token`, passed as `RADIUS_INTERNAL_AUTH_UCP_TOKEN_FILE`. UCP receives its own token with audience `bicep-de` at `/var/run/radius/tokens/bicep-de/token`; `routing.authenticateDeploymentEngine` enables the matching downstream transport. Each sender replaces any incoming Authorization header with the receiver's token. Kubernetes API access uses separate credentials: `RADIUS_INTERNAL_AUTH_API_SERVER=https://kubernetes.default.svc:443`, `RADIUS_INTERNAL_AUTH_API_CA_FILE=/var/run/secrets/kubernetes.io/serviceaccount/ca.crt`, and `RADIUS_INTERNAL_AUTH_API_TOKEN_FILE=/var/run/secrets/kubernetes.io/serviceaccount/token`. The API token must not be used for engine/UCP business or proof requests.

For the Radius provider, the engine mounts only its own server certificate and key (`tls.crt`, `tls.key`) through CSI at `/var/run/radius/spiffe`, and public `ca.crt` separately at `/var/run/radius/ca`. The chart sets `RADIUS_INTERNAL_TLS_CERTIFICATE_DIRECTORY`, `RADIUS_INTERNAL_TLS_CA_DIRECTORY`, and `RADIUS_INTERNAL_TLS_TRUST_DOMAIN` accordingly. Callers verify the dedicated CA and the receiver's exact URI-only SPIFFE identity, `spiffe://<trustDomain>/ns/<namespace>/sa/<receiver>`, without presenting client certificates. The external provider mounts neither internal certificate material nor the CA bundle. Neither provider falls back to the unauthenticated aggregation endpoint after failure. DryRun keeps ordinary token/caller denials advisory, but unavailable signing keys fail closed with 503; managed TLS verification failures also fail closed.

The scoped mount does not establish Kubernetes Secret isolation: the engine ServiceAccount retains its existing `cluster-admin` binding for resource provisioning and can therefore read other component Secrets through the Kubernetes API. Restricting that provisioning authority is separate from this transport wiring.

When the gate is active, the chart removes the engine's legacy anonymous/plain-HTTP Prometheus scrape annotations. In Enforce mode, the engine accepts only the UCP ServiceAccount on `/metrics`; no supported Prometheus scrape identity exists. A token with audience `bicep-de` alone does not grant access. There is no anonymous health/metrics exception or second business listener. This layer does not configure proof observers or mint their tokens; authenticated observer integration and composed live evidence remain pending.

Without the opt-in, the engine mounts no internal component credentials and keeps the published image's existing `https://ucp.radius-system:443/apis/api.ucp.dev/v1alpha3` endpoint, even in DryRun or Enforce. Explicitly enabled `de.ucpMTLS.enabled` is rejected with a migration diagnostic, not silently disabled. Select a compatible image and migrate during a maintenance window, since changing the provider or gate rolls UCP and the engine separately. UCP verifies aggregation-proxy credentials before trusting forwarded user headers, while its headerless engine compatibility path remains open pending verified migration. A component's TLS identity is not an aggregation-proxy credential. These settings alone do not complete execution authorization.

## Configuration Options

### In-Cluster Container Image Builds

The `Radius.Compute/containerImages` resource type requires the opt-in rootless BuildKit sidecar:

```console
rad install kubernetes \
  --set dynamicrp.buildkit.enabled=true
```

Git build sources require no additional storage. To enable local filesystem build sources, create a PVC in the Radius release namespace containing the approved source directories, then configure the chart to mount it read-only:

```console
rad install kubernetes \
  --set dynamicrp.buildkit.enabled=true \
  --set dynamicrp.buildkit.localContexts.existingClaim=build-contexts
```

The chart does not create or populate the PVC. It mounts the claim only into `dynamic-rp` at `/var/radius/build-contexts`; local source paths must resolve beneath that directory.

`dynamic-rp` reads the mount as user ID `65532` and the Pod sets no `fsGroup`, so every directory and file on the claim must be readable by that user or world-readable. The Deployment runs a single replica with the default rolling update, so a `ReadWriteOnce` claim can block a rollout when the replacement Pod is scheduled to another node. Prefer a `ReadOnlyMany` or `ReadWriteMany` claim where the storage class supports it.

BuildKit defaults to one parallel OCI worker step across all image builds sharing the `dynamic-rp` Pod. This protects the sidecar's memory limit when multiple `containerImages` resources are deployed together. `dynamicrp.buildkit.maxParallelism` must be an integer between `1` and `2147483647`; BuildKit treats non-positive values as unlimited, so the chart rejects them before rendering the daemon configuration.

Increase `dynamicrp.buildkit.maxParallelism` only after profiling representative cold builds. Size `dynamicrp.buildkit.resources.requests` for the sustained working set and `dynamicrp.buildkit.resources.limits` with enough headroom for the configured parallelism. The limit bounds concurrent build steps but cannot make an individual build fit within an undersized memory limit.

Do not lower the global `workerServer.maxOperationConcurrency` to control image-build memory. That setting governs every Dynamic RP operation and does not constrain parallel steps within a single BuildKit solve.

### Async worker polling

UCP, Applications RP and Dynamic RP poll their queue for async operations. With the default API server queue, each poll is a request to the Kubernetes API server, so an idle installation polls about five times per second per component. Set `ucp.workerServer.dequeueInterval`, `rp.workerServer.dequeueInterval` or `dynamicrp.workerServer.dequeueInterval` to a duration such as `2s` to poll an empty queue less often. An empty value or a non-positive duration keeps the 200ms default.

The interval applies only after a poll finds the queue empty or fails; a worker polls again immediately after it dequeues an operation. A longer interval therefore delays the start of the first operation after an idle period by up to that interval, but does not slow down a burst of operations. The components read the setting at startup, so restart their Deployments after changing it on an existing installation.

### Custom Container Registry

By default, Radius pulls container images from GitHub Container Registry (ghcr.io). For air-gapped environments or when using private registries, you can configure a custom container registry using the `global.imageRegistry` parameter.

### Custom Image Tag

**Default image tags are version-pinned.** Final and patch charts use the full `Chart.AppVersion` (for example, `0.61.0` or `0.61.2`), including the Deployment Engine, dashboard, Bicep, readiness and pre-upgrade images. RC charts continue to use their full RC version. Historical charts keep their original tag behavior.

**Patch updates require a chart upgrade.** Restarting pods no longer picks up a later patch through the moving `major.minor` alias. Upgrade the Radius CLI and run `rad upgrade kubernetes`, or upgrade to the desired chart version with Helm. Channel aliases remain published for existing consumers.

An upgrade adopts the new defaults when the previous installation used chart defaults. Explicit `global.imageTag`, component tags, and tagged or digest-pinned image paths remain respected. To remove a stored global channel override while preserving other user settings, use `rad upgrade kubernetes --set global.imageTag=`. Clear component-specific tag overrides the same way. An empty image value renders an invalid image reference, so restore each overridden image to its default name from `values.yaml` instead, for example `--set rp.image=applications-rp`. `--reset-values` restores every default but also discards unrelated stored settings.

You can specify a custom tag for all Radius images using the `global.imageTag` parameter. This is useful when you want to deploy a specific version across all components or use custom-built images.

Main-branch Radius images use the mutable `edge` tag. The `latest` tag points to the most recent stable release. Use `edge` when you need builds from `main`.

The Deployment Engine and dashboard follow the same convention: edge charts use `edge`, and the Radius release finalizer promotes their `latest` aliases to the newest stable release. The Deployment Engine explicitly uses `imagePullPolicy: Always` for resolved `:edge` images so pod restarts do not reuse a stale main build; release-tag and digest-pinned pull behavior is unchanged.

#### Using a Custom Registry

```console
# Using Azure Container Registry
helm upgrade --wait --install radius deploy/Chart -n radius-system \
  --set global.imageRegistry=myregistry.azurecr.io

# Using AWS Elastic Container Registry
helm upgrade --wait --install radius deploy/Chart -n radius-system \
  --set global.imageRegistry=123456789.dkr.ecr.us-west-2.amazonaws.com

# Using a private registry with custom port
helm upgrade --wait --install radius deploy/Chart -n radius-system \
  --set global.imageRegistry=private.registry.com:5000
```

#### Using a Custom Image Tag

```console
# Use a specific version for all components
helm upgrade --wait --install radius deploy/Chart -n radius-system \
  --set global.imageTag=0.48

# Combine custom registry with custom tag
helm upgrade --wait --install radius deploy/Chart -n radius-system \
  --set global.imageRegistry=myregistry.azurecr.io \
  --set global.imageTag=0.48

# Override specific component while using global tag for others
helm upgrade --wait --install radius deploy/Chart -n radius-system \
  --set global.imageTag=0.48 \
  --set controller.tag=0.49
```

#### With rad CLI commands

The custom registry and tag configuration is also supported in rad CLI commands:

```console
# During initial installation with custom registry
rad install kubernetes \
  --set global.imageRegistry=myregistry.azurecr.io

# During initial installation with custom tag
rad install kubernetes \
  --set global.imageTag=0.48

# Combine custom registry and tag
rad install kubernetes \
  --set global.imageRegistry=myregistry.azurecr.io \
  --set global.imageTag=0.48

# During upgrade
rad upgrade kubernetes \
  --set global.imageRegistry=myregistry.azurecr.io \
  --set global.imageTag=0.48

# During initialization
rad init \
  --set global.imageRegistry=myregistry.azurecr.io \
  --set global.imageTag=0.48
```

#### Using with Private Registries and Certificates

For private registries that require custom CA certificates:

```console
# Install with custom CA certificate
rad install kubernetes \
  --set global.imageRegistry=private.registry.com \
  --set-file global.rootCA.cert=/path/to/ca-certificate.pem

# Or using Helm directly
helm upgrade --wait --install radius deploy/Chart -n radius-system \
  --set global.imageRegistry=private.registry.com \
  --set-file global.rootCA.cert=/path/to/ca-certificate.pem
```

#### Using with Private Registries and Authentication

For private registries that require authentication:

1. Create the docker-registry secret in the `radius-system` namespace:

```bash
kubectl create secret docker-registry regcred \
  --docker-server=myregistry.azurecr.io \
  --docker-username=<username> \
  --docker-password=<password> \
  --docker-email=<email> \
  -n radius-system
```

1. Reference the secret in your Helm values:

```console
helm upgrade --wait --install radius deploy/Chart -n radius-system \
  --set global.imageRegistry=myregistry.azurecr.io \
  --set-string 'global.imagePullSecrets[0].name=regcred'

# Or using values file:
# values.yaml:
# global:
#   imageRegistry: myregistry.azurecr.io
#   imagePullSecrets:
#     - name: regcred
```

1. With rad CLI:

```console
rad install kubernetes \
  --set global.imageRegistry=myregistry.azurecr.io \
  --set 'global.imagePullSecrets[0].name=regcred'
```

#### Air-gapped Environment Setup

For completely air-gapped environments, you'll need to:

1. Mirror all Radius images to your private registry
2. Configure Radius to use your private registry
3. Create and reference image pull secrets if authentication is required

Example of mirroring images (requires access to both registries):

```bash
# List of Radius images
IMAGES=(
  "controller"
  "ucpd"
  "applications-rp"
  "dynamic-rp"
  "deployment-engine"
  "dashboard"
  "bicep"
)

SOURCE_REGISTRY="ghcr.io/radius-project"
TARGET_REGISTRY="myregistry.azurecr.io"
VERSION="0.60"

# Mirror each image
for IMAGE in "${IMAGES[@]}"; do
  docker pull ${SOURCE_REGISTRY}/${IMAGE}:${VERSION}
  docker tag ${SOURCE_REGISTRY}/${IMAGE}:${VERSION} ${TARGET_REGISTRY}/${IMAGE}:${VERSION}
  docker push ${TARGET_REGISTRY}/${IMAGE}:${VERSION}
done
```

Then install Radius using your private registry:

```console
rad install kubernetes \
  --set global.imageRegistry=myregistry.azurecr.io
```

**Note:** When using a custom registry, images are pulled directly from `<registry>/<image-name>:<tag>` format. For example, with `myregistry.azurecr.io`, the controller image will be pulled from `myregistry.azurecr.io/controller:0.60`.

### Terraform Binary Pre-downloading

By default, Radius downloads Terraform binaries at runtime when Terraform recipes are executed. You can optionally configure Radius to pre-download Terraform binaries during pod startup to improve performance.

To enable Terraform pre-downloading:

```console
helm upgrade --wait --install radius deploy/Chart -n radius-system \
  --set global.terraform.enabled=true
```

This automatically downloads the latest Terraform version. For custom sources (private repositories, proxies, etc.), specify a complete download URL:

```console
helm upgrade --wait --install radius deploy/Chart -n radius-system \
  --set global.terraform.enabled=true \
  --set global.terraform.downloadUrl="https://my-artifactory.com/terraform_1.5.7_linux_amd64.zip"
```

#### Terraform Logging Configuration

You can configure the log level for Terraform execution to control verbosity:

```console
helm upgrade --wait --install radius deploy/Chart -n radius-system \
  --set global.terraform.loglevel="DEBUG"
```

Valid log levels are: `TRACE`, `DEBUG`, `INFO`, `WARN`, `ERROR`, `OFF`. Default is `ERROR`.

### Network Policies

Radius can optionally install Kubernetes `NetworkPolicy` resources that lock down
ingress to the control-plane namespace (`radius-system`). They are **disabled by
default** and gated behind `networkPolicies.enabled`.

When enabled, three policies are applied:

- `radius-default-deny-ingress` — denies all ingress to `radius-system` pods.
- `radius-allow-internal` — re-permits east-west traffic between Radius
  components (intra-namespace), matched by the immutable
  `kubernetes.io/metadata.name` namespace label.
- `radius-allow-control-plane` — allows the Kubernetes API server to reach UCP (APIService aggregation) on port `9443`, from the CIDRs in `networkPolicies.controlPlaneCIDRs`.

Only ingress is restricted; egress is left open so UCP can reach the Kubernetes
API server and pods can resolve DNS.

> **Enforcement requires a CNI that implements NetworkPolicy** (e.g. Calico,
> Cilium, Antrea). On CNIs that do not (e.g. kindnet, flannel, the default EKS
> VPC CNI) these objects are accepted by the API server but silently **not**
> enforced.

#### Setting `controlPlaneCIDRs`

The kube-apiserver reaches UCP (APIService aggregation) over the host network, so this traffic arrives with the **node's** IP rather than a pod IP and cannot be matched by a namespace/pod selector. You must supply the source CIDR(s) via `networkPolicies.controlPlaneCIDRs` — **this is required when `networkPolicies.enabled=true`; Helm rendering fails if it is empty** — otherwise the default-deny policy would block API aggregation and break the control plane.

Use your cluster's node/control-plane subnet(s), **not** individual node IPs
(a `/32` would exclude other control-plane addresses):

- **Managed clusters (AKS/EKS/GKE):** the node pool's VPC/subnet CIDR(s).
- **kubeadm / on-prem:** the node network CIDR.
- **KinD:** the Docker network subnet, e.g.
  `docker network inspect kind -f '{{range .IPAM.Config}}{{.Subnet}} {{end}}'`.

```console
helm upgrade --wait --install radius deploy/Chart -n radius-system \
  --set networkPolicies.enabled=true \
  --set 'networkPolicies.controlPlaneCIDRs={10.0.0.0/16}'
```

```console
# With the rad CLI
rad install kubernetes \
  --set networkPolicies.enabled=true \
  --set 'networkPolicies.controlPlaneCIDRs={10.0.0.0/16}'
```

## Verify the installation

Verify that the controller is running in the radius-system namespace:

```bash
kubectl get pods -n radius-system
```

## Uninstalling the Chart

To uninstall/delete the `radius` deployment:

```console
helm delete radius
```

The command removes all the Kubernetes components associated with the chart and deletes the release.

Uninstalling the chart will not delete any data stored by Radius. To clean up any remaining data, delete the radius-system namespace.
