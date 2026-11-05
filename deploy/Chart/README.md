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

### Deployment Engine mTLS integration prerequisite

`de.ucpMTLS.enabled` defaults to `false` independently of the global authorization mode. This is preparatory wiring for the paired [azure-octo/deployment-engine](https://github.com/azure-octo/deployment-engine) transport change, not a declaration that the default engine image supports mTLS. The engine transport implementation, a published compatible image, a paired Radius image reference and end-to-end deployment verification are still pending. Do not enable this setting with an image that ignores the new environment variables: rendering the chart cannot verify an image's behavior.

With an explicitly selected compatible engine image (`de.image` and `de.tag`), enabling `de.ucpMTLS.enabled` requires `global.rbac.dryRun=true` or `global.rbac.enabled=true`. Both modes render the same engine transport contract:

| Environment variable             | Rendered value                                               |
|----------------------------------|--------------------------------------------------------------|
| `UCP_MTLS_ENABLED`               | `true`                                                       |
| `UCP_MTLS_CERTIFICATE_DIRECTORY` | `/var/run/radius/mtls`                                       |
| `UCP_MTLS_NAMESPACE`             | The Helm release namespace                                   |
| `RADIUSBACKENDURL`               | `https://ucp.<namespace>.svc:9444/apis/api.ucp.dev/v1alpha3` |

Only the engine's own `bicep-de-mtls` Secret supplies its leaf certificate and key, read-only at `/var/run/radius/mtls`. In `selfSigned` mode it also supplies `ca.crt`; in `caSecret` mode a projected volume supplies `ca.crt` from the public `radius-mtls-ca` ConfigMap instead. The mount has no `subPath`, so updates can propagate. Its certificate names `bicep-de.<namespace>.svc`. The paired engine must use a coherent certificate/key/CA snapshot for every handshake, verify the canonical UCP hostname and dedicated CA, and fail closed on invalid or missing material even in DryRun. All outbound UCP SDK callbacks, nested deployment requests and operation polling must use that transport without an unauthenticated fallback. `ARM_AUTH_METHOD=UCPCredential` is unchanged; the engine's existing Kubernetes Secret access for Azure credentials is separate from this UCP transport.

The scoped mount does not establish Kubernetes Secret isolation: the engine ServiceAccount retains its existing `cluster-admin` binding for resource provisioning and can therefore read other component Secrets through the Kubernetes API. Restricting that provisioning authority is separate from this transport wiring.

Without the opt-in, the engine mounts no component credentials and keeps the published image's existing `https://ucp.radius-system:443/apis/api.ucp.dev/v1alpha3` endpoint, even in DryRun or Enforce. UCP verifies aggregation-proxy credentials before trusting forwarded user headers, while its headerless, certificate-less engine compatibility path remains open pending the verified engine migration. A component certificate is not an aggregation-proxy credential. These settings alone do not complete internal authentication or execution authorization.

## Configuration Options

### Internal service certificate modes

With component authorization off, certificate settings have no effect. In DryRun or Enforce, `global.rbac.certificates.mode=selfSigned` remains the default. It creates the Radius CA, the namespaced `radius-ca-issuer`, and five service Certificates. Service certificates have one canonical DNS SAN, `<component>.<release-namespace>.svc`, an ECDSA P-256 key, client/server usages, a requested 24-hour lifetime and renewal 8 hours before expiry.

For `caSecret`, prepare an operator-owned `kubernetes.io/tls` Secret in the release namespace. Its `tls.crt` must start with the dedicated signing CA certificate, optionally followed by its parent chain; `tls.key` must match that first certificate. The chart neither renders nor adopts that Secret, and does not mount the signing key into any component. The existing namespaced `radius-ca-issuer` references it; no bootstrap issuer or CA Certificate is created.

Set `global.rbac.certificates.caSecret` to that Secret's name and `global.rbac.certificates.caBundle` to **only the public dedicated signing CA certificate**. For a fresh installation, preview the resources without contacting a cluster:

```console
helm template radius deploy/Chart -n radius-system \
  --set global.rbac.dryRun=true \
  --set global.rbac.certificates.mode=caSecret \
  --set global.rbac.certificates.caSecret=operator-signing-ca \
  --set-file global.rbac.certificates.caBundle=/path/to/dedicated-ca.pem
```

`caBundle` accepts exactly one PEM `CERTIFICATE` block, with surrounding whitespace trimmed. Do not supply a private key, the parent chain, or the organization root. The chart checks the PEM shape only: it does not parse X.509, inspect the existing Secret, or verify a matching CA/key. Before installation, verify that the public certificate's DER equals the first certificate in the Secret's `tls.crt`, that it is a currently valid CA with a matching allowed key, and that it is dedicated to Radius. Use a path length constraint of zero and a permitted DNS subtree of `<release-namespace>.svc` where possible. CLI flags and cryptographic install/upgrade validation are pending A9; do not interpret a successful render as that validation.

The public certificate is stored in the chart-owned `radius-mtls-ca` ConfigMap. Each component's projected volume combines only `tls.crt` and `tls.key` from its own leaf Secret with `ca.crt` from this ConfigMap. The leaf Secret's `ca.crt` is deliberately excluded: it may contain a broader organization chain. Receivers therefore trust the configured dedicated CA, not every CA in the leaf's chain. This is separate from `global.rootCA`, which configures outbound system trust.

`issuerRef` remains **disabled**. Rendering fails with an explicit prerequisite error, including if a value asserts that approver-policy is installed. A9 must establish that an approver-policy controller is running, cert-manager's default auto-approval is disabled, and an effective policy restricts the selected issuer and Radius identities to authorized certificate requests. A CRD, policy object, or caller-supplied boolean alone does not establish that protection. Direct Helm installs must also remain fail closed until the paired chart/preflight enforcement can provide equivalent protection.

This layer does not implement mode switching, CA replacement, expiry alerts, or coordinated rotation. Do not change the mode or replace a CA on an existing installation using these values alone: A10 must publish old/new trust, wait for consumers, reissue certificates, and explicitly retire old trust. UCP's aggregated API certificate and APIService `caBundle` on port 9443 remain separately managed and unchanged. Local chart and cryptographic tests do not establish cert-manager issuance or intermediate-CA installation on a live cluster.

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
