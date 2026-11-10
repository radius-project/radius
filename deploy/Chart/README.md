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

### External-provider identity

`global.rbac.mtls.provider` defaults to `radius`. With DryRun or Enforce, selecting `external` uses receiver-bound Kubernetes ServiceAccount tokens over plain internal HTTP, relying on the operator's network stack for encryption and integrity. Protect **every connection, including storage and queues**; Radius cannot verify this protection. An explicit CLI provider selector with Off is rejected. An omitted upgrade selector retains the stored provider when turning authorization Off: the chart emits no token projections or discovery grants, Go clients use normal Off routing, and the engine receives `external`/`off` with its existing HTTPS aggregation backend. No verifier, discovery or outbound token work runs in Off.

Active external mode requires `de.externalIdentity.enabled=true` and explicit `de.image`/`de.tag` selecting the paired token-capable engine. These are compatibility assertions, not proof of an image's behavior; no compatible published image or full deployment result is implied. Do not select the default engine image for DryRun or Enforce. `de.ucpMTLS.enabled` and component TLS startup proof cannot be combined with external identity.

UCP receives tokens at `http://ucp.<namespace>.svc:9444/apis/api.ucp.dev/v1alpha3` and sends distinct tokens to Applications RP (5443), Dynamic RP (8082), and the engine (6443). RPs, the controller, and the engine each receive only their own UCP-audience projection. Every projection expires after 600 seconds, has audience `<receiver>.<namespace>.svc`, and is mounted without `subPath` at `/var/run/radius/tokens/<receiver>`. Tokens are read on every request; incoming bearer credentials are consumed and replaced for the next receiver. Correlation metadata is retained; `x-remote-*` and `X-Forwarded-Client-Cert` cannot establish component identity.

The engine receives `RADIUS_INTERNAL_AUTH_PROVIDER=external`, `RADIUS_INTERNAL_AUTH_MODE=dryRun|enforce`, `RADIUS_INTERNAL_AUTH_NAMESPACE=<namespace>`, and `RADIUS_INTERNAL_AUTH_UCP_TOKEN_FILE=/var/run/radius/tokens/ucp`. `RADIUSBACKENDURL` is the canonical UCP HTTP endpoint above. Its discovery configuration is `RADIUS_INTERNAL_AUTH_API_SERVER=https://kubernetes.default.svc:443`, `RADIUS_INTERNAL_AUTH_API_CA_FILE=/var/run/secrets/kubernetes.io/serviceaccount/ca.crt`, and `RADIUS_INTERNAL_AUTH_API_TOKEN_FILE=/var/run/secrets/kubernetes.io/serviceaccount/token`. Incoming engine requests require a UCP ServiceAccount token for `bicep-de.<namespace>.svc`; outgoing engine requests carry a separate UCP-audience token.

Receivers use verified Kubernetes API-server TLS and only `/.well-known/openid-configuration` and `/openid/v1/jwks`; neither token headers nor discovery's `jwks_uri` can select another network destination. The issuer is pinned after initial discovery. Complete validated keysets replace the cache every 5 minutes, with unknown-key requests throttled to 30 seconds. Last-good keys become unusable at 15 minutes stale (`AuthorizationUnavailable`, even in DryRun). Valid algorithms are RS256 with RSA 2048–8192-bit keys, ES256 and ES384. Invalid claims/signatures are never used as identity; Enforce returns `InvalidAuthenticationInfo`, while DryRun logs would-deny.

UCP aggregation TLS remains Radius-managed independently of the internal provider. External mode supports `selfSigned`, `caSecret` and `issuerRef` for aggregation only; it creates no component Certificates, component TLS proofs or engine mTLS opt-in. Fresh enabled installs use the selected aggregation CA through the bootstrap workflow below, including in `selfSigned` mode. Self-signed and CA Secret issuance still require cert-manager; issuer mode uses the operator-managed prerequisites. The aggregation-only CA rotation and explicit recovery workflows remain available without a provider change. Completed aggregation journals and their trust survive ordinary upgrades and dormant Off settings; an unfinished bootstrap, CA transaction or provider journal blocks conflicting operations.

Existing-install provider changes use a resumable outage transaction, not an ordinary Helm value change. The installed chart must advertise `radius.io/provider-transition-protocol: "1"` and the selected CA must have a completed aggregation lifecycle, including version 3 bootstrap or compatible version 1/2 rotation records. First install compatible component and paired engine images in a separate upgrade. Keep authorization in DryRun or Enforce and enable the component management listeners. Then run one of these commands with the same Kubernetes context and namespace used for installation:

```bash
# Inspect without prompting, preparing credentials or stopping workloads.
rad upgrade kubernetes --mtls-provider external --set de.externalIdentity.enabled=true --preflight-only
# The interactive outage confirmation defaults to No. Use --yes only deliberately.
rad upgrade kubernetes --mtls-provider external --set de.externalIdentity.enabled=true --yes
# Return to managed component mTLS with the same selected aggregation CA.
rad upgrade kubernetes --mtls-provider radius --set de.ucpMTLS.enabled=true --yes
```

Changing provider cannot accompany a chart, version, image, authorization-mode, CA or unrelated settings change. `--skip-preflight` does not bypass identity safety checks. Omitted upgrade flags preserve the provider and selected CA. Do not flip the Helm value directly, delete journals, steal an expired Lease, bridge through Off or run Helm rollback during an unfinished transaction. Aggregation-proxy authentication, execution authorization and the existing engine `cluster-admin` provisioning isolation limitation remain separate boundaries; external internal-token checks do not complete them.

Resume is fenced to the original target Deployment specifications, images, chart and Helm history. A deterministic defect in a target image cannot be repaired by replacing that image within the pending transaction. There is no supported provider abort, rollback or replan operation; retaining source credentials is not a recovery guarantee. Validate compatible images before starting the transaction. A new acceptance installation is not recovery of a failed transaction, and a second namespace alone does not isolate the cluster-scoped aggregation APIService.

With `issuerRef`, install the target scope's bounded request policies before requesting the transition. Returning to managed mTLS requires all five component policies as well as aggregation policy. External mode requires only aggregation policy and permits the canonical narrow component policies to remain dormant; it does not issue component credentials until an explicit transition prepares a managed target.

Active external receivers expose `GET /healthz/internal-identity?challenge=<UUID>` on their internal listener. The request requires a valid receiver-bound token and an allowed caller even in DryRun. The response binds the canonical, nonzero UUID challenge to the verified caller's full ServiceAccount subject, configured namespace/provider/mode, `RADIUS_INTERNAL_AUTH_POD_UID` from the Downward API, process boot ID and current UTC timestamp. It is not cached, contains no credentials, and invokes no business operation. Missing process identity or stale signing keys fail closed. UCP returns the receiver assertion; the RPs and paired engine additionally exercise their actual configured UCP client and return its validated receipt as `upstream`. The controller provides the same protected outgoing-client probe on its existing metrics listener when enabled, requiring a UCP caller token with audience `controller.<namespace>.svc`. No caller receives another component's projected credentials.

Each outgoing probe rereads the UCP token projection, rejects redirects, bounds the complete exchange to five seconds and the receipt to 8 KiB, and checks its exact schema, challenge, namespace, mode, caller and UTC observation time with at most 30 seconds of skew. A missing, invalid or unavailable upstream response returns 503; previous success is never reused. These are assertions over operator-secured HTTP, not cryptographic server authentication. A transition observer must independently bind both receipts to the expected current pods and processes, alongside separate aggregation TLS evidence.

The CLI observation implementation uses an authenticated Kubernetes exact-pod port-forward, not pod proxy: the receiver's bearer token must remain separate from Kubernetes API authentication. It requires pod-read, pod-port-forward and ServiceAccount TokenRequest permissions for the canonical callers. Observer tokens request a 600-second lifetime, bind to the inspected caller pod and target only the exact receiver audience; the observer never reads workload token projections. It rechecks pod readiness and container identity and directly probes the reported UCP pod to compare process boot IDs. Transition verification covers every current replica and checks the fleet again after observation; managed targets instead require their loaded-TLS proofs. Both targets independently require real aggregation serving evidence and an authenticated aggregated-API probe.

Transition preparation renders separate component ConfigMaps using the reserved `global.rbac.providerGeneration` value. All five workloads, plus optional BuildKit configuration, reference that generation. Configuration preparation is create-only: it checks all known ownership/content conflicts before writing and reuses only exact owned preparations after interruption. Managed targets prepare canonical component Certificates under the unchanged CA; external targets prepare only fixed signing-key discovery permissions. Native API dry runs capture admitted target Deployment specifications without modifying the source. The reserved `global.rbac.providerReplicas` preserves all five replica counts, including through the final Helm upgrade; each component must have at least one replica and the controller must have exactly one.

The durable phases are `stopping`, `staging`, `starting`, `verifying`, `recording` and `complete`. All source pods must disappear before any target starts. Each phase uses the shared `radius-identity-operation` Lease and fences the observed journal, CA, workloads, configuration and permissions. Verification cannot complete from rendered settings, a ready Secret or generic Pod Ready. Recording runs a real Helm upgrade pinned to the original chart, source revision and target settings, with a fence before each API write. Only pending revisions owned by that exact transaction can be recovered. External completion removes the source component Certificates and deletes only the five pinned former component Secret identities; it never deletes aggregation or signing material.

After an interruption, repeat the same command and target flags. There is no automatic rollback or dual-authentication interval. The original source Helm revision, unchanged CA history and prepared configuration identities remain pinned across retries; replacing them requires investigation, not journal deletion. Private API-server tests simulate issuance/controller status for orchestration, while separate local HTTP tests exercise token verification and client receipts. Neither establishes published-image, real-cluster or HA acceptance.

### Engine loaded-trust evidence

With an engine image implementing the startup evidence contract, `de.ucpMTLS.startupProof.enabled=true` enables bounded public status on the existing engine listener at `GET /healthz/ucp-mtls`. It requires `de.ucpMTLS.enabled=true` and `de.ucpMTLS.startupProof.expectedTrustSHA256`. Both opt-ins default to false; no default image or endpoint changes. The expected digest is lowercase SHA-256 of the UTF-8 bytes of sorted, unique lowercase SHA-256 CA DER fingerprints joined by newline, without a trailing newline. It is not the hash of the PEM file.

The chart sets exactly `UCP_MTLS_STARTUP_PROOF_ENABLED`, `UCP_MTLS_EXPECTED_TRUST_SHA256` and Downward API `UCP_MTLS_POD_UID`. The engine initializes the actual shared UCP transport before listening and pins its trusted CA set for that process. Valid leaf renewals under those same roots remain supported; no expected-leaf or trust-generation environment variable is used. Do not select this setting with an older image that lacks the contract.

The versioned response reports `schemaVersion: 1`, `ready`, `proofKind: "startup-tls-initialized"`, `podUid`, `bootId`, `initializedAt`, `trustBundleSha256`, `caSha256`, `leafSha256`, `notBefore`, `notAfter` and `peerHandshakeVerified: false`. Initialization time is historical; the leaf and validity bounds describe current loaded material, including every loaded CA. Failure returns only `{"schemaVersion":1,"ready":false}` with HTTP 503. Status contains no private keys, PEM or source paths.

Observe this endpoint through an authenticated Kubernetes exact-pod SPDY port-forward. The CLI observer needs namespace-scoped `get` on `pods` and `create` on `pods/portforward`, in addition to the lifecycle's resource permissions; it does not need `pods/proxy` permission or a workload token for this public proof. These permissions belong to the operator running the CLI, not to the component ServiceAccounts. Kubernetes credentials authenticate only the API-server connection and are never forwarded to the proof endpoint. The temporary listener binds only to `127.0.0.1`; observation is bounded to ten seconds, rejects redirects and responses exceeding 8 KiB, and closes the tunnel on success, error or cancellation. Independently bind the response to the live pod UID and running container identity before and after the request, compare expected roots and the current public leaf, and re-list all consumers before a phase transition. A replacement or restart invalidates earlier observations. Generic Pod Ready, projected files and this initialization response do not prove a successful UCP handshake, completion of old streams, or termination of old processes.

These image and network capabilities are prerequisites for the [explicit rotation phases](#radius-owned-public-trust). The CLI checks all five component deployments, their ReplicaSets, immutable public trust generations, exact-pod evidence and a second inventory after observation. The default engine image is not implicitly upgraded or assumed to support this contract.

### Go component loaded-trust evidence

`global.rbac.startupProof.enabled=true` enables the same public evidence contract in UCP, Applications RP, Dynamic RP and the controller. Set `global.rbac.startupProof.expectedTrustSHA256` to the canonical digest described above. The chart supplies the same three proof environment variables, including the actual pod UID. This setting does not enable the separate deployment-engine opt-in or select an engine image.

The endpoint is added beside `/metrics` on the existing metrics listener. `global.prometheus.port` (default 9090) configures the runtime listener, named container port and scrape annotation for all four Go components; bootstrap and rotation discover that named port. A pod-template checksum rolls the processes to load the port setting, including when upgrading an older Dynamic RP installation that listened on 9092 despite declaring 9090. The deployment engine keeps its separate observation listener. Proof mode requires component mTLS and enabled metrics on a fixed valid port. It never silently enables metrics, adds a listener or makes the application API public. Without the opt-in, no proof endpoint is mounted and existing listener/client lifetimes remain unchanged.

Proof observation uses the API-server/kubelet port-forward channel, not direct HTTP from the API server to the pod's metrics port. It does not add plaintext mesh exemptions or widen the bundled network policy. The cluster must support authenticated SPDY port-forwarding to the selected pod and port; a timeout, denied permission or unsupported upgrade fails observation without a pod-proxy, Service or direct-network fallback. This transport proves access to the selected process, not that component-to-component mesh traffic succeeds; external identity and application-path checks remain separate.

In proof mode, the component's incoming listener and all outgoing component-client factories use the same actual pinned TLS Source. This also covers the clients used by workers sharing the RP or UCP process. Startup validates the component identity, permitted keys and both server/client certificate usages; renewals are validated through that same Source. The report therefore describes material installed for actual use, not a separately parsed copy of the files. A ready report is still only local TLS initialization: it does not assert that every hosted service is healthy, a remote handshake succeeded, or old processes and streams have stopped. UCP's separate aggregation serving certificate is not covered by this component-trust report.

## Configuration Options

### Internal service certificate modes

Before managed rotation, certificate settings have no effect with component authorization off. In DryRun or Enforce, `global.rbac.certificates.mode=selfSigned` remains the default. It creates the Radius CA and, in managed scope, five service Certificates. Existing unmigrated releases use the namespaced `radius-ca-issuer`; bootstrap and rotation use a pinned transaction-specific issuer. Service certificates have one canonical DNS SAN, `<component>.<release-namespace>.svc`, an ECDSA P-256 key, client/server usages, a requested 24-hour lifetime and renewal 8 hours before expiry. After bootstrap or rotation, turning authorization off does not restore legacy aggregation trust or delete the certificate resources needed by the managed aggregation endpoint.

For `caSecret`, prepare an operator-owned `kubernetes.io/tls` Secret in the release namespace. Its `tls.crt` must start with the dedicated signing CA certificate, optionally followed by its parent chain; `tls.key` must match that first certificate. The chart neither renders nor adopts that Secret, and does not mount the signing key into any component. The existing namespaced `radius-ca-issuer`, or the bootstrap/rotation transaction issuer, references it; no self-signed bootstrap issuer or CA Certificate is created.

Set `global.rbac.certificates.caSecret` to that Secret's name and `global.rbac.certificates.caBundle` to **only the public dedicated signing CA certificate**. Fresh enabled installations require the live bootstrap workflow, not an offline Helm render. With compatible process-evidence images selected, the CLI can validate an existing signing Secret and derive its public CA:

```console
rad install kubernetes --chart deploy/Chart \
  --set global.rbac.dryRun=true \
  --set global.prometheus.enabled=true \
  --set de.ucpMTLS.enabled=true \
  --ca-secret operator-signing-ca
```

`caBundle` accepts exactly one PEM `CERTIFICATE` block, with surrounding whitespace trimmed. Do not supply a private key, the parent chain, or the organization root. The chart checks the PEM shape only: it does not parse X.509, inspect the existing Secret, or verify a matching CA/key. The public certificate's DER must equal the first certificate in the Secret's `tls.crt`, be a currently valid CA with a matching allowed key, and be dedicated to Radius. Use a path length constraint of zero and a permitted DNS subtree of `<release-namespace>.svc` where possible. The CLI performs cryptographic validation with `--ca-secret` or `--ca-cert`/`--ca-key`, and for `caSecret` Helm overrides and stored values, before changing the cluster. It allows ECDSA P-256/P-384 or RSA keys of at least 2048 bits, validates signing constraints and validity, checks any ordered parent chain, and warns within 30 days of CA expiry. See [certificate CLI configuration and import lifecycle](../../docs/contributing/contributing-code/contributing-code-control-plane/configSettings.md#2-update-shared-service-options).

Existing installations initially store the public certificate in the chart-owned `radius-mtls-ca` ConfigMap; bootstrap and rotation use immutable `radius-mtls-ca-<digest>` generations. Each managed component's projected volume combines only `tls.crt` and `tls.key` from its own leaf Secret with `ca.crt` from the selected ConfigMap. The leaf Secret's `ca.crt` is deliberately excluded: it may contain a broader organization chain. Receivers therefore trust the configured dedicated CA, not every CA in the leaf's chain. This is separate from `global.rootCA`, which configures outbound system trust.

`issuerRef` references an existing `cert-manager.io` `Issuer` in the release namespace or a `ClusterIssuer`. It issues the same service certificates, but creates no issuer or signing-key Secret and never reads the issuer's private key. Set `certificates.issuerRef.group=cert-manager.io`, `kind=Issuer` or `ClusterIssuer`, `name` to the existing issuer, and `caBundle` to the public dedicated signing CA, with the same single-certificate semantics as `caSecret`. The issuer must return certificates chaining to that dedicated CA. Vault and other backends configured through cert-manager's built-in Issuer types can keep the signing key external; third-party issuer API groups are not supported by this profile.

For the CLI, use `--cert-issuer Issuer/name` or `--cert-issuer ClusterIssuer/name` with `--ca-bundle /path/to/public-dedicated-ca.pem` on a fresh install. The CLI validates the public CA cryptographically without reading the issuer's key, then runs this chart's shared live verifier before any cluster changes. It never installs, repairs or upgrades cert-manager in this mode. The Radius namespace and operator-managed prerequisites must already exist. Omitted upgrade/reinstall flags preserve the stored issuer, public CA and prerequisite references, and the same live verifier runs again; `--skip-preflight` cannot bypass it and `--preflight-only` writes no resources or history. Use `--set global.rbac.certificates.requestPolicy...` to change prerequisite references without combining selection flags with certificate selection overrides.

Ordinary upgrade and reinstall preserve stored certificate settings and reject mode, signing Secret and CA trust changes, including attempts to discard stored settings with `--reset-values`. Use the explicit phases below to migrate existing legacy aggregation trust or select a replacement CA; do not overwrite a signing Secret or substitute Helm values. Fresh enabled installations use their selected CA for aggregation from the start. Default Off installation remains unchanged. UCP reloads valid serving-certificate renewals without a restart, but renewal alone does not update APIService trust or prove aggregator adoption. There is no continuous CA-expiry monitor or automatic root rotation.

Fresh enabled bootstrap requires a chart advertising `radius.io/aggregation-bootstrap-protocol: "1"`, an enabled fixed UCP management listener, and compatible aggregation-proof images. Managed scope additionally requires the deployment-engine mTLS opt-in and all five component proof implementations; external scope creates no component Certificates. These requirements do not make an older/default image compatible. The CLI records public intent in `radius-aggregation-bootstrap` before issuance, stages the selected chart's root resources for `selfSigned`, and pins the actual CA and signing identity. `caSecret` validates the existing or imported signer; `issuerRef` revalidates the live public issuer policy without accessing its key. A transaction-specific aggregation Certificate must yield a cryptographically valid leaf before public APIService trust or workload projection is installed. No legacy `ucp-cert` Secret is generated.

The bootstrap phases are `preparing`, `installing`, `verifying` and `complete`. Retry the identical install settings after interruption; do not replace the chart, issuer, CA or recorded resources. Namespace creation retains the initial public intent if the first journal write is interrupted. Surviving Helm resources must match the intent and desired specification before a retry can update them. The shared identity Lease and unfinished journal block conflicting CA, provider, upgrade, rollback and uninstall operations, including attempts to switch Off. Helm success and a ready Secret are insufficient: completion requires converged exact-pod serving evidence, an available APIService and an authenticated aggregated-API probe, plus all required component evidence in managed scope. The completed bootstrap creates a version 3 initial CA lifecycle record with no invented predecessor CA; later rotation and provider transitions can consume it.

### Radius-owned public trust

The A10 implementation retains cert-manager for issuance and renewal but gives Radius ownership of public trust, including its APIService `caBundle`. It does not configure cainjector for Radius's APIService and does not disable shared cainjector. This replaces the earlier injector-fence approach; production admission-policy activation is not a rotation prerequisite.

The installed chart must advertise `radius.io/ca-rotation-protocol: "1"`. First install compatible images, enable component TLS and both proof opt-ins, and verify exact-pod port-forward access as described above. Then use the installed chart without combining rotation with software, authorization, Contour or other configuration changes:

```console
rad upgrade kubernetes --ca-rotation-phase publish --ca-secret next-radius-ca --preflight-only
rad upgrade kubernetes --ca-rotation-phase publish --ca-secret next-radius-ca
rad upgrade kubernetes --ca-rotation-phase adopt
rad upgrade kubernetes --ca-rotation-phase issue
rad upgrade kubernetes --ca-rotation-phase retire
```

`publish` selects an existing dedicated signing Secret, imports `--ca-cert`/`--ca-key`, or selects `--cert-issuer Issuer/name|ClusterIssuer/name` with `--ca-bundle`. It records public intent and publishes old/new trust. Omit target flags to migrate legacy aggregation to the installed CA. Public issuer selection preserves the configured request-policy references, pins the live issuer UID and specification, and reuses the installed chart's live policy verifier on entry, resume and issuance without reading the issuer's private key. Configure prerequisite references before starting the transaction; rotation cannot also change release configuration. This also supports a `selfSigned` installation moving to a supplied `caSecret`; it does not generate a new bootstrap root when switching back. Selecting the installed self-signed CA creates a retained, create-only signing snapshot so renewal of `radius-ca` cannot silently change the transaction's issuer. A replacement CA must use a different key; a renewed certificate with the old key cannot retire that key.

For managed component TLS, `adopt` rolls all five consumers onto immutable overlap trust and waits for their actual process evidence. New version 2 transactions then prepare a separate aggregation certificate and retain old UCP backends while probing a candidate through the authenticated aggregated API, checking the response's exact pod UID. A failed probe restores the old route only with the version observed when that route changed. Only after aggregation adoption succeeds does `issue` change component Certificate issuer references and wait for new leaf adoption. External-provider transactions use aggregation-only scope: they do not inspect component Certificates, change component TLS configuration or accept an empty component fleet as evidence. Existing version 1 transactions retain their original phase ordering. Successful issuance stops at `ready`; it never authorizes retirement automatically.

`retire` rechecks consumers and aggregation, records explicit retirement, rolls consumers onto new-only trust, removes old APIService trust, and requires final evidence before completion. It then records final settings through the current Helm chart. The operation probes the configured Kubernetes endpoint, not every hidden HA API server or cached aggregation transport. It does not prove that a force-deleted process has stopped or that old streams have drained.

The independently owned `radius-ca-rotation` ConfigMap records bounded public intent, resource identities and phase. Each writer reads its mutable target before authoritative intent and retains that target's original resourceVersion. An older queued write therefore cannot overwrite a later successful update to the same target. Retrying means rereading and recomputing, not assigning a fresh version to stale intent. The chart preserves completed trust, issuer references and mounts; it rejects active or missing journals. Ordinary lifecycle operations are guarded during rotation, and CLI rollback is disabled after managed rotation. Use a guarded forward upgrade instead.

Chart rendering preserves legacy version 1 journals, version 2 scoped rotation journals and version 3 completed bootstrap journals. Scoped records preserve separate aggregation proof settings and check a completed external issuer's live UID and specification, public CA and policy references before rendering issuance resources. External-provider rendering does not inspect or emit component Certificates. The CLI supports both scopes, including explicit incident recovery.

Each mutating CA phase holds the namespace-scoped `radius-identity-operation` Lease and rechecks its UID, resourceVersion and holder before writes, including signing imports and final Helm recording. The same guard rejects an unfinished `radius-provider-transition` journal between invocations. An existing completed provider journal also requires the integrated provider validator; completion alone is not readiness evidence. Handled errors release only the exact acquired Lease version. An abrupt crash can leave a Lease that blocks retries: investigate the interrupted operation before explicit operator recovery. Radius never steals a Lease because time elapsed or the holder appears dead.

Retry an interrupted phase without changing its target. `--preflight-only` is read-only, including signing imports and Leases; `--skip-preflight` does not bypass rotation checks. If trust has expired, a CA is compromised, or an interrupted target must be replaced, explicitly select a fresh signing CA with `rad upgrade kubernetes --ca-rotation-phase recover --ca-secret replacement-radius-ca`. Recovery is outage-capable: it publishes new-only trust, reissues leaves and replaces consumers without restoring old trust. It still requires all final evidence before completion. Its historical journal can be inspected after expiry, but expired material is never permission to publish trust. Recovery retains at most eight historical CA certificates and eight predecessor revisions, not a permanent denylist of all retired keys.

If final Helm recording fails after retirement, retry `retire`; do not roll trust back. A pending Helm operation requires operator recovery of that operation before retrying; the CLI does not steal its lock or delete release records. Standalone journals, immutable trust generations, transaction issuers and Certificates, and imported signing Secrets are retained on uninstall. Reinstallation cannot adopt a journal bound to a deleted APIService. Before a deliberate fresh installation, back up the completed journal and signing material, confirm no remaining installation, issuer or workload uses the retained resources, and explicitly remove only the retired installation's resources. Never delete an operator-owned signing Secret as implicit cleanup.

Existing injection annotations or known injector managed-field history cause an explicit handover error. Removing annotations is not sufficient because an already submitted injector write may still arrive. No automatic handover is implemented. Replaying historical manifests, using unguarded older charts, stripping ownership history or bypassing the protocol with administrator writes is not supported. Local crypto, simulated-controller and private API-server tests do not establish live cert-manager issuance, image compatibility or HA acceptance.

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
