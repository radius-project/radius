# Radius control-plane configuration

## Purpose

This guide explains where Radius control-plane configuration lives and how to change it for local development or a Helm-based installation. The Go option types are the schema source of truth; the checked-in development YAML and Helm templates show complete configurations.

## Prerequisites

- Complete the [repository prerequisites](../contributing-code-prerequisites/README.md).
- For local process debugging, complete the setup in [Running and debugging the control plane locally](../contributing-code-debugging/radius-os-processes-debugging.md).
- For Kubernetes configuration changes, install Helm.

## Steps

### 1. Choose the configuration surface

Use the development YAML for a service that runs as a local process:

| Service         | Development configuration                      |
|-----------------|------------------------------------------------|
| UCP             | `cmd/ucpd/ucp-dev.yaml`                        |
| Applications RP | `cmd/applications-rp/applications-rp-dev.yaml` |
| Dynamic RP      | `cmd/dynamic-rp/dynamicrp-dev.yaml`            |
| Controller      | `cmd/controller/controller-dev.yaml`           |

For Kubernetes installations, configuration is rendered from `deploy/Chart/templates/<service>/configmaps.yaml` with values from `deploy/Chart/values.yaml`. Do not add configuration under `deploy/Chart/charts`; that directory does not exist.

### 2. Update shared service options

The development files combine shared host options with service-specific settings. Use the option types linked below instead of treating this page as a manually duplicated schema:

| YAML section                     | Source of truth                                                                                                 | Current values or shape                                                                                                                                                                                         |
|----------------------------------|-----------------------------------------------------------------------------------------------------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `environment`                    | [`hostoptions.EnvironmentOptions`](../../../../pkg/armrpc/hostoptions/options.go)                               | `name`, `roleLocation`                                                                                                                                                                                          |
| `databaseProvider`               | [`databaseprovider.Options`](../../../../pkg/components/database/databaseprovider/options.go)                   | Provider is `apiserver`, `inmemory`, or `postgresql`                                                                                                                                                            |
| `queueProvider`                  | [`queueprovider.QueueProviderOptions`](../../../../pkg/components/queue/queueprovider/options.go)               | Provider is `apiserver` or `inmemory`; includes a queue `name`                                                                                                                                                  |
| `secretProvider`                 | [`secretprovider.SecretProviderOptions`](../../../../pkg/components/secret/secretprovider/options.go)           | Provider is `kubernetes` or `inmemory`                                                                                                                                                                          |
| `metricsProvider`                | [`metricsservice.Options`](../../../../pkg/components/metrics/metricsservice/options.go)                        | `enabled`, `serviceName`, and nested `prometheus.path` / `prometheus.port`                                                                                                                                      |
| `server`, `workerServer`, `ucp`  | [`pkg/armrpc/hostoptions`](../../../../pkg/armrpc/hostoptions/)                                                 | HTTP listener, async worker, and UCP connection settings                                                                                                                                                        |
| `authorization`                  | [`authz.Options`](../../../../pkg/authz/mode.go)                                                                | `mode` is `off` (default), `dryRun`, or `enforce`; also `podSecurityLevel`, `controlPlaneNamespace`, `controlPlaneSecrets`, `certificateDirectory`, `caDirectory`, `tokenDirectory`, `trustDomain`, `namespace` |
| `callerAuthentication`           | [`authorization.RequestHeaderOptions`](../../../../pkg/ucp/authorization/requestheader.go)                      | UCP-only request-header CA file, allowed proxy names, and Kubernetes issuer                                                                                                                                     |
| `kubernetesCallerAuthentication` | [`authorization.KubernetesRequestHeaderOptions`](../../../../pkg/ucp/authorization/kubernetes_requestheader.go) | UCP-only startup discovery of the cluster's request-header trust; enabled by the chart in every internal mode                                                                                                   |

For example, local services commonly use Kubernetes API server storage and expose Prometheus settings in this shape:

```yaml
databaseProvider:
  provider: apiserver
  apiserver:
    context: ""
    namespace: radius-testing

queueProvider:
  provider: apiserver
  name: radius
  apiserver:
    context: ""
    namespace: radius-testing

secretProvider:
  provider: kubernetes

metricsProvider:
  enabled: false
  serviceName: ucp
  prometheus:
    path: /metrics
    port: 9091
```

For local process debugging with authorization mode `off`, use a `direct` UCP connection to the local HTTP endpoint:

```yaml
ucp:
  kind: direct
  direct:
    endpoint: http://localhost:9000/apis/api.ucp.dev/v1alpha3
```

Kubernetes deployments use the chart-rendered UCP connection instead of the local endpoint. When the authorization mode is `off`, the chart renders `kind: kubernetes`. Otherwise it renders a `direct` connection to UCP's internal port, `https://ucp.<namespace>.svc:9444/apis/api.ucp.dev/v1alpha3`, for Applications RP, Dynamic RP, and the controller.

When the authorization mode is not `off`, a `direct` endpoint must use `https`; the service verifies UCP's certificate against `ca.crt` in `authorization.caDirectory` (or `authorization.certificateDirectory` when it is unset) and sends the token from `authorization.tokenDirectory`. Keep the mode `off` for local process debugging over `http`.

The `authorization.mode` setting is read by UCP, Applications RP, Dynamic RP, and the controller, and each logs `authz mode=<mode>` at startup. The chart renders it from `global.rbac.enabled` and `global.rbac.dryRun`: `dryRun=true` renders `dryRun`, `enabled=true` renders `enforce`, and setting both fails rendering. In `off` mode the chart omits the section so older service binaries can still load the config.

> [!NOTE]
> With `global.rbac.tls.provider=radius`, UCP, Applications RP, and Dynamic RP mount a csi-driver-spiffe server certificate at `/var/run/radius/spiffe`; the gated compatible deployment engine does so too. Callers mount projected tokens at `/var/run/radius/tokens/<receiver>/token`. Internal public trust is separate at `/var/run/radius/ca`: BYO uses `radius-mtls-ca/ca.crt`; only selfSigned bootstrap may read public `radius-ca/ca.crt` until the lifecycle stages an independent source. No CA key is mounted. The chart renders `certificateDirectory`, `caDirectory`, `tokenDirectory`, `trustDomain` and `namespace`. External TLS omits the internal certificate/trust mounts but retains token authentication. UCP's aggregated API on 9443 uses the `ucp-cert` Certificate and permanently injects public `radius-apiserver-trust/ca.crt` into the APIService. selfSigned keeps its aggregation CA independent; BYO uses the provided compatible CA/issuer for both profiles, with distinct requester restrictions.
>
> In `dryRun`, the internal listeners accept both TLS and plain HTTP and log ordinary authentication failures they would deny. In `enforce`, they require TLS and a ServiceAccount token whose audience is the receiver's name. A missing, invalid, or wrong-audience token returns HTTP 401 `InvalidAuthenticationInfo`. Signing keys that have never loaded or have gone at least 15 minutes without a successful refresh return HTTP 503 `AuthorizationUnavailable` in both `dryRun` and `enforce`; no caller identity is created. The resource providers accept only UCP's identity and return HTTP 403 `AuthorizationFailed` for any other component in `enforce`. Callers send a token only after verifying the receiver's SPIFFE ID in exactly one URI SAN; certificates with additional DNS, IP, or email SANs are rejected before sending the token.

When the mode is not `off`, UCP sends its tokens when it calls Applications RP and Dynamic RP over `https`, and the RPs and the controller send theirs when they call UCP on port 9444. The deployment engine still calls UCP through port 9443 without a token.

Before sending a request, component clients close connections whose verified server certificate chain has expired or no longer chains to the loaded trust generation. This can interrupt active HTTP/2 response streams; it is not a timer that closes connections at the exact expiry instant. Invalid projected material fails closed, and repairing it does not restore invalidated pooled connections. Each TLS handshake uses one coherent snapshot. Serving requests also check their original handshake certificate chain against current trust and validity, rather than substituting a renewed certificate. Clients re-read a token file when it changes, and at least once a minute.

When the mode is not `off`, the chart also renders `podSecurityLevel` (from `global.rbac.podSecurityLevel`, `baseline` by default or `restricted`) and `controlPlaneNamespace` (the release namespace, which is never labeled). `dryRun` adds the `pod-security.kubernetes.io/warn` and `audit` labels; `enforce` also adds `enforce`. Labels that an operator already set on a namespace are never overwritten.

Labeling is additive: changing the mode or level does not rewrite or remove existing labels, including labels Radius previously added. The new setting applies to newly created namespaces and missing label keys when an environment or application is updated. To change the policy on an already labeled namespace, review and update its Pod Security labels explicitly; switching Radius to `off` does not disable existing namespace enforcement.

The chart also installs the `radius-control-plane-isolation` ValidatingAdmissionPolicy when the mode is not `off` and the cluster serves the `admissionregistration.k8s.io/v1` ValidatingAdmissionPolicy API (Kubernetes 1.30+). It checks Deployments and Pods that the identities in `global.rbac.admission.applicationIdentities` (default: the `applications-rp` service account) create with the `radapp.io/resource-type` label, and flags workloads in the control-plane namespace that use a control-plane service account (`control-plane-service-account`), reference a control-plane Secret (`control-plane-secret`), or run there at all (`control-plane-namespace`). `dryRun` binds it with `Warn` and `Audit`; `enforce` with `Deny`, and the message starts with `AdmissionPolicyDenied: <rule>`. The service account and Secret names come from the `radius.authz.controlPlaneServiceAccounts` and `radius.authz.controlPlaneSecrets` chart helpers.

Applications RP applies the same rules itself. It checks an `Applications.Core` or `Radius.Core` environment, or an application `kubernetesNamespace` extension, whose namespace is `controlPlaneNamespace` (rule `control-plane-namespace`), before it creates or labels the namespace. In `enforce`, this namespace check rejects the request immediately with HTTP 403 and `AdmissionPolicyDenied` naming the rule.

Before it applies the output resources of a deployment, Applications RP checks any Kubernetes object in `controlPlaneNamespace`, including a rendered Namespace object with that name: rule `control-plane-secret` when the object is, or its pod spec references, one of `controlPlaneSecrets` (volumes, projected volumes, `envFrom`, `secretKeyRef`, `imagePullSecrets`), otherwise `control-plane-namespace`. It applies the same rules to Secrets that an object in another namespace references in `controlPlaneNamespace`: a Contour HTTPProxy that a gateway renders can name a TLS, client-validation, or upstream-validation Secret as `<namespace>/<name>`. The chart renders `controlPlaneSecrets` from the `radius.authz.controlPlaneSecrets` helper. In `enforce`, this deployment output check fails the asynchronous operation before any outputs are applied; it does not return the denial in the original request's response. Follow the `Azure-AsyncOperation` URL until the operation reaches `Failed` with an `AdmissionPolicyDenied` error naming the rule. For both checks, `dryRun` logs a would-deny and allows the request.

Applications RP also checks the Kubernetes Secret used by a legacy `Applications.Core/secretStores` resource:

- **Checked operations:** PUT and PATCH before namespace or Secret writes; `listSecrets` and DELETE before Secret reads or deletion.
- **Rules:** in `controlPlaneNamespace`, one of `controlPlaneSecrets` violates `control-plane-secret`; any other Secret violates `control-plane-namespace`. Other namespaces are allowed.
- **Modes:** unset or `off` (the default) skips the check. `dryRun` logs a would-deny. `enforce` returns HTTP 403 `AdmissionPolicyDenied`, including for existing stores.
- **Secret values:** they never appear in admission errors, logs, or denials. Allowed `listSecrets` responses still return them.
- **Delete:** `enforce` blocks deletion of stores referencing control-plane Secrets, even if the Secret is missing. For allowed requests in every mode, Radius deletes the backing Secret only when this store recorded its UID at creation and the live UID still matches. A UID precondition protects against replacement during deletion.
- **Existing/adopted Secrets:** without verified ownership, DELETE removes only the Radius record and retains the Secret, including Secrets with a `radapp.io/resource-type` label. Updates do not claim ownership of an existing Secret. Secrets referenced by older records are retained because those records lack ownership evidence; review retained Secrets before removing them directly. A second store referencing a Secret does not become its owner or prevent its creating store from cleaning it up.
- **Targets:** `RADIUS_TARGET_KUBECONFIG` never exempts secret stores because they use the control-plane cluster.

These protections apply to the cluster hosting the control plane, not an unrelated same-named namespace on a deployment target. With `RADIUS_TARGET_KUBECONFIG` configured, Radius compares the live `kube-system` namespace UIDs before exempting a protected-name target namespace. Different kubeconfig files or API-server addresses alone do not establish separation. The control-plane and target credentials must allow reading that namespace for these protected-name checks; a failed identity read fails the request explicitly in `dryRun` and `enforce`. Ordinary application namespaces and `off` mode need no identity lookup. A verified distinct target receives normal Pod Security labels even when its namespace has the control-plane namespace's name. Legacy `Applications.Core` environment namespace creation still uses the control-plane cluster and remains protected, regardless of target configuration.

The Applications RP rollout checksum covers its authorization mode and, when active, its effective Pod Security level, control-plane namespace, and protected Secret names, so changing these settings restarts the provider with the updated policy. Settings omitted from the runtime configuration in `off` mode do not affect its checksum.

`global.rbac.certificates.mode` selects `selfSigned` (default), `caSecret` or `issuerRef`. `caSecret` uses an operator-owned signing Secret through separate internal and aggregation Issuers. `issuerRef` uses an existing `cert-manager.io` Issuer or ClusterIssuer without owning its key. Both take exactly one public selected signing CA in `certificates.caBundle`, not a private key or parent chain. With `global.rbac.tls.provider=radius`, the selected CA must support the actual receiver SPIFFE URI trust domain and exact `ucp.<release-namespace>.svc` DNS profile; `external` requires DNS only. The certificate flags work with either provider, without substituting a Radius-owned CA. Rendering checks PEM shape; the CLI validates CA/key/chain cryptography and profile feasibility. CSI provides the internal leaf/key, not component leaf Secrets.

`issuerRef` requires a connected render verifying approver-policy v0.29.0 for the exact cert-manager UCP DNS requester, plus the pinned CSI approver for annotated own-Pod URI requests in managed mode. Default approval of the selected shared signer is forbidden. `certificates.requestPolicy` identifies the controllers/webhook; a boolean is not evidence. The public aggregation source is permanent, not deleted after bootstrap. A fresh selfSigned Helm bootstrap needs a second connected render after CA issuance, or lifecycle pre-staging; the current CLI baseline must be adapted before composing this chart. Existing rotation journals or conflicting public sources fail closed rather than switching modes. See [the chart contract](../../../../deploy/Chart/README.md#internal-service-certificate-modes) for bytes, profiles, policy, ownership and paired lifecycle requirements.

For a fresh authorization-enabled installation, `rad install kubernetes --set global.rbac.dryRun=true --ca-secret operator-signing-ca` validates an existing namespace-local `kubernetes.io/tls` Secret and sets the public certificate values. Alternatively, supply `--ca-cert /path/to/ca.pem --ca-key /path/to/ca.key` to import a dedicated signing CA. These flags are mutually exclusive with each other and with `global.rbac.certificates` selection overrides. The Secret's `tls.crt` must start with the dedicated signing CA, optionally followed by its ordered issuance parent chain, and `tls.key` must match that first certificate. CLI validation checks the entire PEM input, rejects duplicate chain certificates, and checks CA basic constraints, signing key usage, validity dates, parent signatures and path lengths, and ECDSA P-256/P-384 or RSA keys of at least 2048 bits. Every supplied CA's URI/DNS constraints must permit the selected profiles, including constraints on supplied parents. A CA expiring within 30 days produces a warning.

For an existing external issuer, use `rad install kubernetes --set global.rbac.dryRun=true --cert-issuer Issuer/radius-ca --ca-bundle /path/to/public-ca.pem`, or use `ClusterIssuer/radius-ca`. The group is fixed to `cert-manager.io`. The public file must contain exactly one currently valid dedicated signing CA, without a private key or parent chain; the CLI validates its CA constraints, signing usage and permitted public-key algorithm, but never reads the issuer's signing Secret or discovers trust from a returned leaf. The operator must supply the CA that the issuer actually uses. The three selection methods are mutually exclusive and cannot be combined with `global.rbac.certificates` selection overrides. `certificates.requestPolicy` reference overrides remain available with the flags, for example to select controllers in a different namespace.

External issuer installation requires the operator to provision the Radius namespace and the chart's supported controller, admission, policy and RBAC prerequisites first. Before dependency, signing Secret or release mutation, the CLI runs the selected chart's actual `DryRunServer` verifier and the shared read-only cainjector configuration/discovery checks. An existing `caSecret` installation also requires this inspection; when cert-manager is absent, its normal installation path remains available. The issuer's signing key remains external; local X.509 constraint probes and live prerequisite checks do not prove actual external issuance or successful CA injection. The pinned CSI approver requires four certificate usages, including client auth, but Radius authenticates callers with tokens and never requests or presents component client certificates.

File import creates a Secret named `radius-imported-ca-<first-16-SHA256-bytes-of-CA-DER>` outside Helm ownership, with no release annotations or owner references. The key never enters Helm values or release history. The Secret is retained on installation failure and uninstall. Retry the same files after a failure; an existing import is reused only when its certificate and key bytes match, and is never updated or deleted. Back up the key and remove the Secret manually only after no issuer uses it. Existing operator Secrets are read-only and are never adopted.

The CLI stores only the signing Secret name and exactly one public dedicated CA certificate in Helm values. It never trusts the leaf's returned organization chain.

`rad upgrade kubernetes` and install with `--reinstall` preserve stored certificate settings when the flags are omitted, and validate the referenced CA before changes. `--reset-values` rejects stored certificate settings rather than silently reverting them. Certificate flags can confirm unchanged settings; an unchanged `--cert-issuer` can reuse the stored public bundle without a file. Ordinary upgrades still reject mode, signing Secret, issuer reference and trust changes. A read-only public target plan is available before that guard for the paired lifecycle: it contains selected chart values, applicable planes, actual issuers/receivers, planned public sources and public CA/Secret identity pins, never the private import payload. New selfSigned CAs remain unresolved until staged. A transition import requires the lifecycle's active Lease write guard; this layer does not implement that lease or authorize a transition. `--skip-preflight` cannot bypass validation, and `--preflight-only` changes neither resources nor Helm history. A10 must revalidate pinned targets/history and adapt permanent public trust staging, journal resolution and convergence before composing the lifecycle; those operations and actual issuance acceptance are not established by this CLI layer.

Fresh authorization-enabled CLI installation uses a two-stage bootstrap. With selfSigned, it first stages the selected chart's root Issuer and CA Certificates and waits for current Ready conditions and matching signing Secrets. It then creates the independent public `radius-mtls-ca` ConfigMap and permanent `radius-apiserver-trust` Secret before applying the full chart. The external provider stages only aggregation. BYO modes use the configured public anchors; operator signing resources remain read-only. Installation completes only after the existing exact aggregation-set, serving-Pod and APIService availability checks pass. No component token or protected workload report is used for initial publication.

The public-only `radius-certificate-bootstrap` journal binds the namespace UID, selected chart and values, observed root/issuer/signer identities, and exact public-source bytes. Its `preparing`, `published`, and `complete` stages use the shared identity-operation Lease, journal resource-version checks and cancellation guards, including guards on Helm mutation requests. Retry the same chart and settings after interruption; use `--reinstall` if Helm already recorded the release. A root or source created before a failed journal update is reused only with its matching transaction marker and contents. Replaced resources, changed active signer revisions and unrelated releases fail closed. A retained Lease or pending Helm revision requires operator investigation; the executor neither steals locks nor marks an unfinished Helm operation successful. Do not delete the journal or permanent trust sources to bypass recovery.

Local controller fixtures and isolated Kubernetes API servers exercise these stages and retries, but do not establish live cert-manager issuance, CSI behavior or installed-product acceptance. Legacy `radius-aggregation-bootstrap` journals and namespace intents still require explicit recovery with the matching pre-token release. Default Off installation is unchanged. See [the certificate-mode contract](../../../../deploy/Chart/README.md#internal-service-certificate-modes).

Lifecycle preparation reads both installed trust planes and pins their prior mode, public source bytes/UID/revision, issuer identity/configuration digest, and local signer's public certificate identity. Schema4 recovery rechecks the durable journal, APIService ownership, public sources, target issuers and local signing material without writing or claiming workload readiness. Active phases require exact revisions; completed states tolerate unrelated metadata changes, but not replacement objects, changed issuer configuration or changed anchors. An affected `issuerRef` plane requires a distinct staged issuer. Recovery never copies an operator's issuer specification or private key into the public journal, and never restores operator resources.

The schema2 evidence consumer validates bounded, complete JSON against the exact selected anchors, their current validity intersection, expected UCP SPIFFE ID, and independently observed Pod/container identity before and after the read. It rejects legacy or client-leaf fields and restarted containers. This is data validation for a future authenticated observer, not an authentication mechanism or phase-advance authorization. Its source-bound local fixtures do not establish installed-product acceptance; the CLI still has no permitted way to acquire the protected workload report.

`rad upgrade kubernetes --rotate-ca` selects replacement of both Radius-owned selfSigned CAs with the Radius provider, or aggregation only with the external provider. Explicit internal-only plans must leave aggregation unchanged. This flag and `--ca-rotation-phase` currently fail before transition writes because the CLI has no approved identity for authenticated loaded-trust observation. They do not execute rotation, retirement, expiry recovery or compromise recovery. The CLI must not mint or extract component tokens, impersonate UCP, forward Kubernetes API credentials to a component, or use an anonymous proof endpoint. A serving TLS handshake proves the selected serving chain only; it cannot substitute for outbound loaded-trust acknowledgment. Schema4 preparation and exact anchor-set checks are not a completed lifecycle executor.

Supported guarded writes, including removal of stale aggregation injection annotations in Off mode, use the namespace's `radius-identity-operation` Lease and reject conflicting lifecycle intent. Write guards recheck cancellation and Lease ownership; APIService updates also use resource-version preconditions. Installed enabled operations check the selected aggregation source before changing dependencies; rollback checks the target manifest before reconciling its CSI driver. These guards do not authorize the blocked CA transition. `--preflight-only` never acquires a Lease. Exact observed convergence is not a permanent guarantee against later shared-controller writes, and CA replacement alone is not full incident response or execution-record revocation.

UCP reads `aws.fingerprintKeyFile`, the file that holds the HMAC key for fingerprints of write-only AWS property values. It defaults to `/var/secrets/aws-fingerprint/key`, where the chart mounts the `radius-aws-fingerprint-key` Secret, so the chart does not render it. Set it when running UCP as a local process with a key file of at least 32 random bytes. When the file is missing or invalid, UCP still starts and logs an error naming the Secret.

#### Configure verified Kubernetes caller identity

UCP rejects requests carrying any `X-Remote-*` headers with HTTP 401 `InvalidAuthenticationInfo` unless aggregation-proxy trust is configured and the TLS peer is a verified Kubernetes front proxy. This authentication boundary applies regardless of internal `authorization.mode` or workload authorization mode; `off`, `Disabled`, and `DryRun` do not permit spoofed identities. Headerless aggregation requests retain compatibility but have **no authenticated user identity**. Protected user endpoints must require `authorization.IdentityFromContext`; they must never fall back to a claimed user or treat an unidentified request as authenticated. The internal component listener on port 9444 rejects all forwarded identity headers and announced identity trailers, including requests with a valid component token. Internal TLS does not request client certificates. Verified tokens establish a separate `authz.Identity`, never a Kubernetes user identity.

The Helm chart configures `kubernetesCallerAuthentication` in every authorization mode, including the default install. At startup, UCP uses its in-cluster ServiceAccount to read `kube-system/extension-apiserver-authentication` once. The chart grants `get` on that named ConfigMap, not general ConfigMap access. UCP uses `requestheader-client-ca-file` and `requestheader-allowed-names`, never the general `client-ca-file`. Missing, unreadable or malformed trust fails startup without a fallback. Only the canonical username/group/extra header configuration (`X-Remote-User`, `X-Remote-Group`, `X-Remote-Extra-`, case-insensitive) is supported; another header configuration fails startup explicitly.

Set the Helm value `ucp.kubernetesIssuer` to a stable `kubernetes:<trust-domain>` issuer; its default is `kubernetes:local`. Choose distinct trust domains when policies distinguish clusters. The issuer is an identity label, not a CA trust source. Changing this value rolls UCP. The rendered configuration is:

```yaml
kubernetesCallerAuthentication:
  kubernetesIssuer: kubernetes:local
```

For a local process or explicitly managed trust, configure UCP with a TLS server certificate directory and the cluster's dedicated request-header trust instead. `callerAuthentication` and `kubernetesCallerAuthentication` are mutually exclusive:

```yaml
server:
  tlsCertificateDirectory: /etc/radius/tls
callerAuthentication:
  caFile: /etc/radius/requestheader-ca.crt
  allowedNames:
    - front-proxy-client
  kubernetesIssuer: kubernetes:<cluster-id>
```

For file-based configuration in a Pod, copy the `requestheader-client-ca-file` PEM bundle from the `kube-system/extension-apiserver-authentication` ConfigMap into a ConfigMap in UCP's Helm release namespace, then mount that copy at `caFile`. ConfigMap volumes can only reference ConfigMaps in the Pod's own namespace; UCP cannot directly mount the source in `kube-system`. For a local UCP process, export the bundle to a local file instead and set `caFile` to that file's path. Copy the source ConfigMap's JSON `requestheader-allowed-names` array into `allowedNames`. Do **not** substitute the general Kubernetes client CA: its clients must not be able to assert other users. Names are exact, case-sensitive certificate common names. An empty allowed-name list follows Kubernetes semantics: any client certificate name signed by the dedicated request-header CA is accepted. Copy the cluster configuration deliberately rather than guessing a proxy name. Use a stable, cluster-specific issuer, normally `kubernetes:` followed by the UID of the `kube-system` namespace; a bare `kubernetes` issuer is rejected. Default Helm installs use in-cluster discovery and need no manual CA copy.

UCP requests a TLS client certificate and independently validates its chain, validity period, client-authentication usage, and allowed name before consuming `X-Remote-User` and each `X-Remote-Group` value. The leaf certificate must explicitly include the `clientAuth` extended key usage (EKU); an absent EKU or `anyExtendedKeyUsage` alone is rejected. This is stricter than Kubernetes' own certificate check; kubeadm and standard front-proxy client certificates already include `clientAuth`. Service-account user names become workload principals; other names become user principals. Anonymous and `system:unauthenticated` identities are rejected. Verified identities use the configured issuer; consumed `X-Remote-*` headers do not reach downstream handlers. Component certificates and forwarded certificate headers do not establish Kubernetes user identity.

Identity assertions in request trailers are never accepted. An announced `X-Remote-*` trailer is rejected with HTTP 401 `InvalidAuthenticationInfo` before body consumption or proxying, including HTTP/1.1 chunked and HTTP/2 requests from verified peers in every mode. Trailer names are checked case-insensitively; they are never used to authenticate a caller. Unrelated trailers remain supported.

Aggregation-proxy trust is a startup snapshot. **Restart every UCP replica after rotating the proxy CA bundle or changing allowed names or issuer.** A ConfigMap or projected file update alone does not replace the active proxy trust snapshot. Invalid trust configuration or configured authentication without server TLS fails startup. The aggregation listener requests, but does not require, a client certificate: headerless deployment-engine access on port 9443 remains available until the engine migrates to the internal listener. This does not bypass the separate internal listener's token checks. Serving-certificate reloads preserve the aggregation proxy's client-certificate policy. This integration establishes user identity separately from internal component tokens; it does not add policy endpoints or user-role enforcement.

### 3. Configure supported environment overrides

Some behavior is read directly from the process environment:

| Environment variable                 | Purpose                                                                         |
|--------------------------------------|---------------------------------------------------------------------------------|
| `SKIP_ARM`                           | Set to `true` to disable Azure Resource Manager integration                     |
| `ARM_AUTH_METHOD`                    | Selects `UCPCredential`, `Managed`, `ServicePrincipal`, or `Cli` authentication |
| `AZURE_CLIENT_ID`                    | Service-principal client ID                                                     |
| `AZURE_CLIENT_SECRET`                | Service-principal client secret                                                 |
| `AZURE_TENANT_ID`                    | Service-principal tenant ID                                                     |
| `MSI_ENDPOINT` / `IDENTITY_ENDPOINT` | Signals that managed identity is available                                      |
| `RADIUS_LOGGING_JSON`                | Selects the `development` or `production` log profile                           |
| `RADIUS_LOGGING_LEVEL`               | Overrides the configured log level                                              |

The logging environment variables take precedence over the equivalent `logging.json` and `logging.level` YAML values. See [Logging](./logging.md) for logging conventions.

### 4. Render or run the changed configuration

For a Helm change, render the chart and inspect the generated ConfigMaps:

```bash
helm template radius deploy/Chart
```

For a local development change, restart the affected process through the debug workflow:

```bash
make debug-stop
make debug-start
make debug-status
```

## Verification

- `helm template radius deploy/Chart` succeeds after a chart configuration change.
- `make debug-start` starts every local component after a development configuration change.
- `make debug-status` reports the affected component as running.
- The service log contains no YAML decoding or unknown-provider errors.

## Troubleshooting

- **The service rejects a provider name.** Check the provider constants linked in [Update shared service options](#2-update-shared-service-options); provider names are lowercase.
- **A local service cannot reach UCP.** Compare its `ucp.direct.endpoint` with `cmd/ucpd/ucp-dev.yaml`; the development endpoint includes `/apis/api.ucp.dev/v1alpha3`.
- **A Helm edit does not change the installed configuration.** Confirm you changed the appropriate `deploy/Chart/templates/<service>/configmaps.yaml` template or the value that feeds it, then render the chart before reinstalling.
- **Logging ignores the YAML value.** Unset `RADIUS_LOGGING_JSON` or `RADIUS_LOGGING_LEVEL`; environment values take precedence.
- **Aggregated requests return `InvalidAuthenticationInfo`.** Verify that UCP can read `kube-system/extension-apiserver-authentication` and receives the proxy's TLS client certificate. For a local process, configure `callerAuthentication` from that ConfigMap instead. Restart all replicas after changing trust. Headerless local requests remain unidentified; disabling authorization does not make forwarded identity headers trustworthy.
