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

Internal public trust is mounted separately at `/var/run/radius/ca`; no signing key or other component's leaf Secret is mounted. Only selfSigned bootstrap may project `radius-ca/ca.crt` until the lifecycle stages the independent `radius-mtls-ca` ConfigMap. BYO uses that public ConfigMap immediately. Do not configure CSI with a startup source bundle that does not exist yet. See [csi-driver-spiffe installation troubleshooting](../../docs/contributing/contributing-code/contributing-code-control-plane/troubleshooting-installation.md#csi-driver-spiffe).

`global.rbac.tls.provider=external` uses HTTP for internal component calls under operator-provided encryption. Both providers use the same receiver-scoped ServiceAccount tokens and caller checks; external components do not mount or read internal CSI certificates or the Radius CA bundle. UCP's independent aggregated API TLS remains enabled and still requires cert-manager. The Helm key is `global.rbac.tls.provider`; it renders the runtime configuration field `authorization.tlsProvider`. Unknown providers, the retired `global.rbac.mtls` settings, and the unsupported Helm alias `global.rbac.tlsProvider` fail rendering rather than silently selecting Radius TLS.

### Changing the authorization mode or upgrading from an mTLS build

Turning authorization on or off, and upgrading from an edge build that authenticated components with client certificates, changes how every component authenticates. Helm rolls ucp, applications-rp, dynamic-rp, and the controller one at a time, so for about the time the rollout takes, calls between an old and a new pod fail TLS verification or authentication and return errors. Radius does not fall back to weaker authentication to avoid this window. Plan these changes for a maintenance window:

1. Pause deployments that call Radius, such as `rad deploy` pipelines.
1. Use a CLI build with the paired certificate validation and lifecycle layers before changing CA, issuer or certificate mode. This chart layer does not implement those transitions. `rad rollback kubernetes` also refuses older charts that issue component client Certificates from `radius-ca-issuer` or `radius-mtls-issuer`.
1. Retry any deployment that failed during the window.

A plain `helm upgrade --wait` waits only for the workloads; check the APIService with `kubectl get apiservice v1alpha3.api.ucp.dev` before resuming.

The aggregation source is permanently the public-only `radius-apiserver-trust` Secret, with the **annotation** `cert-manager.io/allow-direct-injection: "true"`. Never delete it to return injection to a signing Secret. On a fresh selfSigned Helm bootstrap, cert-manager first generates the two CAs. A second **cluster-connected** Helm render can then read the public aggregation CA and create this trust Secret. An offline render cannot know the newly generated anchor; the APIService cannot become Available until the public source is staged. Internal selfSigned bootstrap still works through the public-only signing-Secret projection. To avoid a bootstrap gap, the paired lifecycle must pre-stage the sources before workload issuance.

Existing public sources are preserved byte-for-byte, carry `helm.sh/resource-policy: keep`, and must not be overwritten to switch CA or mode. Active or legacy `radius-ca-rotation` journals fail chart rendering. The schema4 completed-snapshot guard requires prior plane snapshots, exact pinned public-source content, and a chart-supported target issuer whose live UID and configuration digest still match; it does not execute or authorize transitions. Completed snapshots tolerate unrelated metadata updates while retaining source and issuer identity, ownership, journal binding and exact public bytes; active phases still require their observed resourceVersion. Issuer specifications are hashed, not copied into public journals or restored onto operator resources. Legacy aggregation-bootstrap journals and namespace intents also fail closed, including Off renders. Normal CLI aggregation reconciliation uses the permanent public source rather than deleting it or returning injection to a signing Secret. cainjector v1.20/v1.21 merges CAs by default, so configuring the source alone does not prove that stale anchors have retired from the APIService.

Fresh authorization-enabled CLI installation stages the selected chart's selfSigned root Issuer and CA Certificates, waits for their current Ready observations and matching signing Secrets, then publishes independent public trust before applying the full chart. The Radius provider stages both planes; the external provider stages aggregation only. BYO modes publish their configured public anchors without adopting operator signing resources. The public-only `radius-certificate-bootstrap` journal records `preparing`, `published`, and `complete` stages under the shared identity-operation Lease. A full chart render rejects an unfinished publication or a changed pending intent; it preserves creation markers and the exact staged source bytes. Completed bootstrap records are history, not overrides of subsequent CA lifecycles.

Automatic CA rotation remains blocked: the CLI lacks an approved identity for authenticated outbound loaded-trust observation. Fresh initial publication uses Kubernetes/cert-manager authority and does not need that workload identity. Initial installation still checks exact aggregation trust convergence, exact-Pod serving certificates and APIService availability; these checks do not prove outbound trust, token readiness or every API-server process's loaded trust. Local controller fixtures and isolated API-server tests cover execution and recovery, not live cert-manager issuance or installed-product acceptance. See [the CLI boundaries and retry guidance](../../docs/contributing/contributing-code/contributing-code-control-plane/configSettings.md#2-update-shared-service-options).

### Internal service certificate modes

`global.rbac.certificates.mode` selects the issuer and public anchor for both planes. `selfSigned` (default) retains separate Radius-owned internal and aggregation CAs. `caSecret` creates two distinct namespaced Issuers, `radius-ca-issuer` and `radius-apiserver-issuer`, referencing the **same operator-owned signing Secret**. `issuerRef` uses the selected existing `cert-manager.io` Issuer or ClusterIssuer for both profiles; the chart neither renders nor owns it. With `global.rbac.tls.provider=external`, only aggregation DNS issuance remains: no internal CA, CSI mount or receiver CertificateRequest-create Role is rendered.

The CLI certificate flags select these modes for either provider. It validates complete PEM input, CA/key/chain cryptography, and URI/DNS constraint feasibility using the selected chart's real issuer/receiver inventory. Both the actual chart's live policy render and cainjector prerequisite inspection run for BYO modes before resource changes when the controllers already exist. Public target planning does not authorize rotation or prove that an external issuer can issue a leaf. See [CLI certificate validation and lifecycle boundaries](../../docs/contributing/contributing-code/contributing-code-control-plane/configSettings.md#2-update-shared-service-options).

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

When the gate is active, the chart removes the engine's legacy anonymous/plain-HTTP Prometheus scrape annotations because they no longer describe a usable authenticated scrape. Operators must configure scraping with a token for audience `bicep-de` and, for Radius TLS, verify the engine's exact SPIFFE identity and public CA. There is no anonymous health/metrics exception or second business listener. This layer does not configure proof observers or mint their tokens; authenticated observer integration and composed live evidence remain pending.

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
