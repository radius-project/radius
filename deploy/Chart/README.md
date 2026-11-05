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

### Engine loaded-trust evidence

With an engine image implementing the startup evidence contract, `de.ucpMTLS.startupProof.enabled=true` enables bounded public status on the existing engine listener at `GET /healthz/ucp-mtls`. It requires `de.ucpMTLS.enabled=true` and `de.ucpMTLS.startupProof.expectedTrustSHA256`. Both opt-ins default to false; no default image or endpoint changes. The expected digest is lowercase SHA-256 of the UTF-8 bytes of sorted, unique lowercase SHA-256 CA DER fingerprints joined by newline, without a trailing newline. It is not the hash of the PEM file.

The chart sets exactly `UCP_MTLS_STARTUP_PROOF_ENABLED`, `UCP_MTLS_EXPECTED_TRUST_SHA256` and Downward API `UCP_MTLS_POD_UID`. The engine initializes the actual shared UCP transport before listening and pins its trusted CA set for that process. Valid leaf renewals under those same roots remain supported; no expected-leaf or trust-generation environment variable is used. Do not select this setting with an older image that lacks the contract.

The versioned response reports `schemaVersion: 1`, `ready`, `proofKind: "startup-tls-initialized"`, `podUid`, `bootId`, `initializedAt`, `trustBundleSha256`, `caSha256`, `leafSha256`, `notBefore`, `notAfter` and `peerHandshakeVerified: false`. Initialization time is historical; the leaf and validity bounds describe current loaded material, including every loaded CA. Failure returns only `{"schemaVersion":1,"ready":false}` with HTTP 503. Status contains no private keys, PEM or source paths.

Observe this endpoint through authenticated Kubernetes exact-pod proxy, with pod-read and pod-proxy permissions. Independently bind the response to the live pod UID and running container identity before and after the request, compare expected roots and the current public leaf, and re-list all consumers before a phase transition. A replacement or restart invalidates earlier observations. Generic Pod Ready, projected files and this initialization response do not prove a successful UCP handshake, completion of old streams, or termination of old processes.

This is a rotation prerequisite, not a rotation command or completed consumer barrier. The Go TLS evidence primitives and CLI exact-pod observer do not yet cover every running Radius role. RP listeners require component mTLS and cannot simply expose this status to plaintext pod proxy. Full consumer lifecycle wiring, safe aggregation migration and a server-enforced injector write fence remain required.

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

`caBundle` accepts exactly one PEM `CERTIFICATE` block, with surrounding whitespace trimmed. Do not supply a private key, the parent chain, or the organization root. The chart checks the PEM shape only: it does not parse X.509, inspect the existing Secret, or verify a matching CA/key. Before a direct Helm installation, verify that the public certificate's DER equals the first certificate in the Secret's `tls.crt`, that it is a currently valid CA with a matching allowed key, and that it is dedicated to Radius. Use a path length constraint of zero and a permitted DNS subtree of `<release-namespace>.svc` where possible. The CLI performs cryptographic validation with `--ca-secret` or `--ca-cert`/`--ca-key`, and for `caSecret` Helm overrides and stored values, before changing the cluster. It allows ECDSA P-256/P-384 or RSA keys of at least 2048 bits, validates signing constraints and validity, checks any ordered parent chain, and warns within 30 days of CA expiry. See [certificate CLI configuration and import lifecycle](../../docs/contributing/contributing-code/contributing-code-control-plane/configSettings.md#2-update-shared-service-options).

The public certificate is stored in the chart-owned `radius-mtls-ca` ConfigMap. Each component's projected volume combines only `tls.crt` and `tls.key` from its own leaf Secret with `ca.crt` from this ConfigMap. The leaf Secret's `ca.crt` is deliberately excluded: it may contain a broader organization chain. Receivers therefore trust the configured dedicated CA, not every CA in the leaf's chain. This is separate from `global.rootCA`, which configures outbound system trust.

`issuerRef` references an existing `cert-manager.io` `Issuer` in the release namespace or a `ClusterIssuer`. It issues the same service certificates, but creates no issuer or signing-key Secret and never reads the issuer's private key. Set `certificates.issuerRef.group=cert-manager.io`, `kind=Issuer` or `ClusterIssuer`, `name` to the existing issuer, and `caBundle` to the public dedicated signing CA, with the same single-certificate semantics as `caSecret`. The issuer must return certificates chaining to that dedicated CA. Vault and other backends configured through cert-manager's built-in Issuer types can keep the signing key external; third-party issuer API groups are not supported by this profile.

For the CLI, use `--cert-issuer Issuer/name` or `--cert-issuer ClusterIssuer/name` with `--ca-bundle /path/to/public-dedicated-ca.pem` on a fresh install. The CLI validates the public CA cryptographically without reading the issuer's key, then runs this chart's shared live verifier before any cluster changes. It never installs, repairs or upgrades cert-manager in this mode. The Radius namespace and operator-managed prerequisites must already exist. Omitted upgrade/reinstall flags preserve the stored issuer, public CA and prerequisite references, and the same live verifier runs again; `--skip-preflight` cannot bypass it and `--preflight-only` writes no resources or history. Use `--set global.rbac.certificates.requestPolicy...` to change prerequisite references without combining selection flags with certificate selection overrides.

This layer does not implement mode switching, CA replacement, continuous expiry monitoring, or coordinated rotation. The CLI preserves stored certificate settings on upgrade and reinstall and rejects mode, signing Secret and CA trust changes, including attempts to discard stored settings with `--reset-values`. Do not replace a CA on an existing installation using Helm values or direct Secret edits: A10 must publish old/new trust, wait for consumers, reissue certificates, and explicitly retire old trust. UCP's aggregated API certificate and APIService `caBundle` on port 9443 remain separately managed by the legacy chart. UCP now reloads a valid aggregation serving certificate from its mounted Secret without a restart; this neither changes the APIService trust nor proves that the aggregator has loaded it. Local chart and cryptographic tests do not establish cert-manager issuance or intermediate-CA installation on a live cluster.

The selected future injection source is a separate public-only Secret containing the intended Radius aggregation trust, not a leaf Secret's potentially broader `ca.crt`. It is not enabled here. Supported cainjector versions can merge still-valid old roots, and a queued force-apply can restore an old root even after an annotation is removed and a resource-version-checked update succeeds. Updating the source, trimming once, sleeping or restarting a shared injector is not a safe retirement protocol. Keep CA changes disabled until a bounded write fence and recovery path are implemented and verified.

#### Issuer policy prerequisites

The chart's [`radius.authz.issuerPolicy.validate`](templates/_issuer-policy.tpl) helper is the shared source of policy checks for direct Helm installation and the A9 CLI preflight. It performs live Kubernetes reads; a plain offline `helm template` cannot establish these prerequisites and fails. A caller-supplied boolean, a policy CRD, or the presence of a policy object is not proof of enforcement. The chart neither installs nor reconfigures shared controllers, policies, webhooks, issuers or their RBAC.

The supported operator-managed profile uses [approver-policy v0.29.0](https://github.com/cert-manager/approver-policy/tree/v0.29.0), API `policy.cert-manager.io/v1alpha1`, and cert-manager v1.20 or newer. Before installing Radius, an operator must configure these prerequisites, accounting for the effect on other certificate users:

- Disable cert-manager's default approval controller using its documented [`disableAutoApproval=true` chart setting](https://cert-manager.io/docs/policy/approval/approver-policy/installation/). The running controller must have exactly one `--controllers=-certificaterequests-approver` or `--controllers=*,-certificaterequests-approver` argument, with no competing controller/config-file flag. Remove its signer-approval RBAC too, as the upstream chart does for this setting.
- Run the official controller binaries with inspectable version tags, one container per Pod, a separate explicit ServiceAccount for each controller, a completed current Deployment rollout and matching Running/Ready Pods. The bounded profile rejects custom commands, config files, injected containers and unrecognized arguments. It allows the cert-manager `POD_NAMESPACE` downward API environment variable, not arbitrary environment overrides. Image name/version inspection is not image provenance verification; the operator remains responsible for trusted images and infrastructure namespaces.
- Keep cert-manager's mutating and validating admission webhooks enabled and fail closed. They must cover request creation and status updates in the Radius namespace without object exclusions or match conditions, use `/mutate` and `/validate` on the configured service with a CA bundle, and have Ready official webhook Pods at the endpoint addresses. Namespace selectors are evaluated against the actual existing Radius namespace, including the upstream `cert-manager.io/disable-validation NotIn ["true"]` default; do not disable validation on that namespace. These webhooks bind `CertificateRequest.spec.username` to the actual API requestor and authorize approval writes; disabling them defeats policy/RBAC evidence.
- Install the exact policies below, wait for each policy and issuer to report `Ready=True` for its current generation, and grant the actual cert-manager controller ServiceAccount policy `use` permission. This is **not** the UCP/component ServiceAccount: cert-manager creates the CertificateRequests on behalf of Certificate objects. Both namespace RoleBindings and ClusterRoleBindings are checked, including group and wildcard permissions. `resourceNames: ["*"]` is a literal RBAC name, not a wildcard grant.
- Grant the approver-policy ServiceAccount its upstream cluster-wide informer, policy/request status, SubjectAccessReview and selected-signer approval permissions. Namespaced signer names use `issuers.cert-manager.io/<release-namespace>.<issuer-name>`; ClusterIssuers use `clusterissuers.cert-manager.io/<issuer-name>`. The upstream signer-family wildcard is also recognized. Other non-administrative approval subjects, including a residual default approver, are rejected.

The default resource references are:

```yaml
global:
  rbac:
    certificates:
      mode: issuerRef
      issuerRef:
        group: cert-manager.io
        kind: Issuer
        name: external-ca
      requestPolicy:
        certManager:
          namespace: cert-manager
          deployment: cert-manager
        approverPolicy:
          namespace: cert-manager
          deployment: cert-manager-approver-policy
        webhook:
          namespace: cert-manager
          service: cert-manager-webhook
          configuration: cert-manager-webhook
```

For `radius-system`, a controller requestor `system:serviceaccount:cert-manager:cert-manager`, and issuer `external-ca`, the UCP policy must have this exact spec. Install equivalent policies named `radius-radius-system-<component>` for `applications-rp`, `dynamic-rp`, `controller` and `bicep-de`, substituting the component in the SAN and request-name prefix. Substitute the release namespace and actual controller requestor consistently. Do not add plugins, other identity fields, wildcard SANs, broader durations or algorithms, or extra CEL rules.

```yaml
apiVersion: policy.cert-manager.io/v1alpha1
kind: CertificateRequestPolicy
metadata:
  name: radius-radius-system-ucp
spec:
  selector:
    issuerRef:
      group: cert-manager.io
      kind: Issuer
      name: external-ca
    namespace:
      matchNames: [radius-system]
  allowed:
    dnsNames:
      values: [ucp.radius-system.svc]
      required: true
      validations:
        - rule: "cr.username == 'system:serviceaccount:cert-manager:cert-manager' && cr.namespace == 'radius-system' && cr.name.startsWith('ucp-cert-')"
    isCA: false
    usages: [digital signature, server auth, client auth]
  constraints:
    minDuration: 24h
    maxDuration: 24h
    privateKey:
      algorithm: ECDSA
      minSize: 256
      maxSize: 256
```

Bind policy use to that exact ServiceAccount, for example with a ClusterRole whose rule has `apiGroups: ["policy.cert-manager.io"]`, `resources: ["certificaterequestpolicies"]`, `verbs: ["use"]` and the five exact policy names in `resourceNames`, plus a ClusterRoleBinding to `cert-manager/cert-manager`. A namespace RoleBinding to that ClusterRole in `radius-system` is also supported: approver-policy evaluates `use` in the **request namespace**, even though policies are cluster-scoped. The chart does not create these operator-owned resources.

Policy approval is [OR across applicable policies](https://cert-manager.io/docs/policy/approval/approver-policy/): one permissive policy can bypass another restrictive policy. The verifier examines every policy whose issuer selector could match, even if it is not Ready or is not currently bound to this requestor, and rejects noncanonical specs rather than trying to prove arbitrary CEL safe. It also rejects a permissive policy for the selected issuer in another namespace: a certificate containing a Radius SAN is accepted based on the dedicated CA and SAN, not the namespace where it was requested. Exact unrelated issuer selectors can coexist, but a different issuer reference alone does not prove a different CA. The operator must keep the Radius CA dedicated and must not expose another issuer or external signing path capable of minting Radius identities.

The canonical policy envelope contains all five Radius identities. The required subset comes from `radius.authz.certificateComponents`: managed mTLS requires all five; the external-provider integration requires only UCP for aggregation. Unused exact canonical policies may remain installed; they do not create component Certificates. Approver-policy permits subsets of `allowed.usages`, so the UCP policy also permits the server-only aggregation profile. A10's distinct aggregation Certificate names must start with `ucp-cert-aggregation`, making their CertificateRequest names satisfy the same UCP prefix without colliding with component Certificate `ucp-cert`. This helper does not implement the external-provider transport or the aggregation migration; those remain A11/A10 work.

This is an install-time inspection of a bounded RBAC-based configuration, not continuous admission enforcement or proof against a hostile cluster administrator, arbitrary authorization webhooks, or future policy/controller changes. Operators must restrict creation/update of Certificates, CertificateRequests, issuers, policy/RBAC objects and controller configuration to trusted identities. The verifier rejects explicit component certificate/issuer creation or modification grants, including matching User and ServiceAccount-group bindings, but the existing engine `cluster-admin` provisioning binding remains an administrative trust boundary, not isolation. Existing signed certificates or previously approved requests are not revoked by adding a policy. Protect the CA from its creation and use coordinated rotation after any exposure.

After operator preparation, inspect the chart against the selected cluster without installing Radius:

```console
helm template radius deploy/Chart -n radius-system --dry-run=server \
  --set global.rbac.dryRun=true \
  --set global.rbac.certificates.mode=issuerRef \
  --set global.rbac.certificates.issuerRef.name=external-ca \
  --set-file global.rbac.certificates.caBundle=/path/to/dedicated-ca.pem
```

The rendering identity needs get/list access to the existing Radius namespace, referenced issuers, policies, Deployments, Pods, admission configurations, webhook Endpoints, Roles, ClusterRoles and their bindings. Forbidden API reads and missing or unsupported prerequisites fail explicitly before issuance resources are rendered. Run `make test-helm` and `go test ./pkg/cli/helm -run '^TestIssuerPolicyChart$'` for local checks. The latter runs the real REST-backed Helm engine against read-only API fixtures; it does not execute controller/admission code. Live-cluster acceptance must still demonstrate allowed component and aggregation issuance, denial of wrong requestor/SAN/namespace/CA/usages/key/duration requests, and failure when the controller, webhook or approval protection is removed. No live-cluster acceptance is claimed by these local tests.

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
