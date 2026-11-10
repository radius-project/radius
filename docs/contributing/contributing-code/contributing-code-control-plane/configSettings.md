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

| YAML section                     | Source of truth                                                                                                 | Current values or shape                                                                                                                                         |
|----------------------------------|-----------------------------------------------------------------------------------------------------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `environment`                    | [`hostoptions.EnvironmentOptions`](../../../../pkg/armrpc/hostoptions/options.go)                               | `name`, `roleLocation`                                                                                                                                          |
| `databaseProvider`               | [`databaseprovider.Options`](../../../../pkg/components/database/databaseprovider/options.go)                   | Provider is `apiserver`, `inmemory`, or `postgresql`                                                                                                            |
| `queueProvider`                  | [`queueprovider.QueueProviderOptions`](../../../../pkg/components/queue/queueprovider/options.go)               | Provider is `apiserver` or `inmemory`; includes a queue `name`                                                                                                  |
| `secretProvider`                 | [`secretprovider.SecretProviderOptions`](../../../../pkg/components/secret/secretprovider/options.go)           | Provider is `kubernetes` or `inmemory`                                                                                                                          |
| `metricsProvider`                | [`metricsservice.Options`](../../../../pkg/components/metrics/metricsservice/options.go)                        | `enabled`, `serviceName`, and nested `prometheus.path` / `prometheus.port`                                                                                      |
| `server`, `workerServer`, `ucp`  | [`pkg/armrpc/hostoptions`](../../../../pkg/armrpc/hostoptions/)                                                 | HTTP listener, async worker, and UCP connection settings                                                                                                        |
| `authorization`                  | [`authz.Options`](../../../../pkg/authz/mode.go)                                                                | `mode` is `off` (default), `dryRun`, or `enforce`; also `podSecurityLevel`, `controlPlaneNamespace`, `controlPlaneSecrets`, `certificateDirectory`, `namespace` |
| `callerAuthentication`           | [`authorization.RequestHeaderOptions`](../../../../pkg/ucp/authorization/requestheader.go)                      | UCP-only request-header CA file, allowed proxy names, and Kubernetes issuer                                                                                     |
| `kubernetesCallerAuthentication` | [`authorization.KubernetesRequestHeaderOptions`](../../../../pkg/ucp/authorization/kubernetes_requestheader.go) | UCP-only startup discovery of the cluster's request-header trust; enabled by the chart in every internal mode                                                   |

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

Kubernetes deployments use the chart-rendered UCP connection instead of the local endpoint. When the authorization mode is `off`, the chart renders `kind: kubernetes`. Otherwise it renders a `direct` connection to UCP's internal mTLS port, `https://ucp.<namespace>.svc:9444/apis/api.ucp.dev/v1alpha3`, for Applications RP, Dynamic RP, and the controller.

When the authorization mode is not `off`, a `direct` endpoint must use `https`, and the service presents the certificate from `authorization.certificateDirectory`. Keep the mode `off` for local process debugging over `http`.

The `authorization.mode` setting is read by UCP, Applications RP, Dynamic RP, and the controller, and each logs `authz mode=<mode>` at startup. The chart renders it from `global.rbac.enabled` and `global.rbac.dryRun`: `dryRun=true` renders `dryRun`, `enabled=true` renders `enforce`, and setting both fails rendering. In `off` mode the chart omits the section so older service binaries can still load the config.

> [!NOTE]
> Besides the admission controls below, the mode controls mutual TLS between control-plane components. When the mode is not `off`, the chart mounts each component's `<component>-mtls` Secret at `/var/run/radius/mtls` and renders `certificateDirectory` (that path, holding `tls.crt`, `tls.key`, and `ca.crt`) and `namespace` (the release namespace, used to check peer certificate identities) for UCP, Applications RP, Dynamic RP, and the controller. UCP serves internal traffic on port 9444; its aggregated API on 9443 is unchanged.
>
> In `dryRun`, the mTLS listeners accept both TLS and plain HTTP and log requests they would deny. In `enforce`, they require a client certificate issued by the Radius CA. A missing, untrusted, or expired client certificate fails the initial TLS handshake before an HTTP response can be returned. If a previously authenticated connection fails a request-time certificate or trust check, the server returns HTTP 401 `InvalidAuthenticationInfo`. The resource providers accept only UCP's identity and return HTTP 403 `AuthorizationFailed` for any other component. Certificate validity is checked on every request, including on reused connections.

When the mode is not `off`, UCP presents its certificate when it calls Applications RP and Dynamic RP over `https`, and the RPs and the controller present theirs when they call UCP on port 9444. The deployment engine still calls UCP through port 9443 without a client certificate.

Before sending a request, component clients close connections outside the validity period shared by their client certificate and the verified server certificate chains. This can interrupt active HTTP/2 response streams; it is not a timer that closes connections at the exact expiry instant. A successful certificate or CA-bundle reload applies to new client handshakes. Removing a server CA from that bundle does not immediately revoke already-open client connections.

When the mode is not `off`, the chart also renders `podSecurityLevel` (from `global.rbac.podSecurityLevel`, `baseline` by default or `restricted`) and `controlPlaneNamespace` (the release namespace, which is never labeled). `dryRun` adds the `pod-security.kubernetes.io/warn` and `audit` labels; `enforce` also adds `enforce`. Labels that an operator already set on a namespace are never overwritten.

Labeling is additive: changing the mode or level does not rewrite or remove existing labels, including labels Radius previously added. The new setting applies to newly created namespaces and missing label keys when an environment or application is updated. To change the policy on an already labeled namespace, review and update its Pod Security labels explicitly; switching Radius to `off` does not disable existing namespace enforcement.

The chart also installs the `radius-control-plane-isolation` ValidatingAdmissionPolicy when the mode is not `off` and the cluster serves the `admissionregistration.k8s.io/v1` ValidatingAdmissionPolicy API (Kubernetes 1.30+). It checks Deployments and Pods that the identities in `global.rbac.admission.applicationIdentities` (default: the `applications-rp` service account) create with the `radapp.io/resource-type` label, and flags workloads in the control-plane namespace that use a control-plane service account (`control-plane-service-account`), reference a control-plane Secret (`control-plane-secret`), or run there at all (`control-plane-namespace`). `dryRun` binds it with `Warn` and `Audit`; `enforce` with `Deny`, and the message starts with `AdmissionPolicyDenied: <rule>`. The service account and Secret names come from the `radius.authz.controlPlaneServiceAccounts` and `radius.authz.controlPlaneSecrets` chart helpers.

Applications RP applies the same rules itself. It checks an `Applications.Core` or `Radius.Core` environment, or an application `kubernetesNamespace` extension, whose namespace is `controlPlaneNamespace` (rule `control-plane-namespace`), before it creates or labels the namespace. In `enforce`, this namespace check rejects the request immediately with HTTP 403 and `AdmissionPolicyDenied` naming the rule.

Before it applies the output resources of a deployment, Applications RP checks any Kubernetes object in `controlPlaneNamespace`, including a rendered Namespace object with that name: rule `control-plane-secret` when the object is, or its pod spec references, one of `controlPlaneSecrets` (volumes, projected volumes, `envFrom`, `secretKeyRef`, `imagePullSecrets`), otherwise `control-plane-namespace`. The chart renders `controlPlaneSecrets` from the `radius.authz.controlPlaneSecrets` helper. In `enforce`, this deployment output check fails the asynchronous operation before any outputs are applied; it does not return the denial in the original request's response. Follow the `Azure-AsyncOperation` URL until the operation reaches `Failed` with an `AdmissionPolicyDenied` error naming the rule. For both checks, `dryRun` logs a would-deny and allows the request.

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

`global.rbac.certificates.mode` selects `selfSigned` (default), `caSecret` or `issuerRef`. In `caSecret` mode, set `certificates.caSecret` to an existing operator-owned signing Secret in the release namespace and `certificates.caBundle` to exactly one public PEM certificate matching the dedicated signing CA, without its parent chain or private key. In `issuerRef` mode, set `certificates.issuerRef` to an existing `cert-manager.io` Issuer or ClusterIssuer and provide the same dedicated public `caBundle`; the signing key remains with the external issuer. Both modes use the chart-owned `radius-mtls-ca` ConfigMap for `ca.crt` in each component's projected volume; only `tls.crt` and `tls.key` come from its leaf Secret. Direct Helm rendering checks PEM shape, not CA validity or a matching signing key; direct Helm operators must perform those checks before installation.

`issuerRef` also requires live verification of operator-managed approver-policy v0.29.0, cert-manager with default auto-approval disabled, fail-closed cert-manager admission webhooks, exact bounded Radius request policies and effective requestor/approver RBAC. Resource references live under `certificates.requestPolicy`; offline rendering cannot replace that evidence with a boolean. See [the chart's certificate-mode contract](../../../../deploy/Chart/README.md#internal-service-certificate-modes) for values, supported controller configuration, canonical policy specs, ownership and trust boundaries. These settings do not migrate UCP's aggregated API certificate or implement mode switching or CA rotation.
For a fresh authorization-enabled installation with compatible proof-capable images, `rad install kubernetes --set global.rbac.dryRun=true --set global.prometheus.enabled=true --set de.ucpMTLS.enabled=true --ca-secret operator-signing-ca` validates an existing namespace-local `kubernetes.io/tls` Secret and bootstraps aggregation using its dedicated public CA. Alternatively, supply `--ca-cert /path/to/ca.pem --ca-key /path/to/ca.key` to import a dedicated signing CA. The Secret's `tls.crt` must start with the dedicated signing CA, optionally followed by its issuance parent chain, and `tls.key` must match that first certificate. CLI validation checks CA basic constraints, signing key usage, validity dates, parent signatures and path lengths, and ECDSA P-256/P-384 or RSA keys of at least 2048 bits. A CA expiring within 30 days produces a warning.

For an existing external issuer, use `rad install kubernetes --set global.rbac.dryRun=true --cert-issuer Issuer/radius-ca --ca-bundle /path/to/public-ca.pem`, or use `ClusterIssuer/radius-ca`. The group is fixed to `cert-manager.io`. The public file must contain exactly one currently valid dedicated signing CA, without a private key or parent chain; the CLI validates its CA constraints, signing usage and permitted public-key algorithm, but never reads the issuer's signing Secret or discovers trust from a returned leaf. The operator must supply the CA that the issuer actually uses. The three selection methods are mutually exclusive and cannot be combined with `global.rbac.certificates` selection overrides. `certificates.requestPolicy` reference overrides remain available with the flags, for example to select controllers in a different namespace.

External issuer installation requires the operator to provision the Radius namespace and the chart's supported controller, admission, policy and RBAC prerequisites first. Install, upgrade, reinstall and `--preflight-only` render the actual selected chart using live Kubernetes lookups before any dependency, Secret or release mutation. An older chart without the shared verifier is rejected. Verification uses the effective stored/current values and the selected kubeconfig/context, requires five component policies for managed mTLS or only UCP for external mTLS, and rechecks the live prerequisites on every invocation rather than storing a verification boolean. External issuer mode never invokes cert-manager installation, repair or upgrade: those prerequisites remain operator-managed. This verifies the supported configuration, not the behavior of an adversarial controller.

File import creates a Secret named `radius-imported-ca-<first-16-SHA256-bytes-of-CA-DER>` outside Helm ownership, with no release annotations or owner references. The key never enters Helm values or release history. The Secret is retained on installation failure and uninstall. Retry the same files after a failure; an existing import is reused only when its certificate and key bytes match, and is never updated or deleted. Back up the key and remove the Secret manually only after no issuer uses it. Existing operator Secrets are read-only and are never adopted.

The CLI stores only the signing Secret name and exactly one public dedicated CA certificate in Helm values. The chart-owned `radius-mtls-ca` ConfigMap supplies `ca.crt` in each component's projected volume; only `tls.crt` and `tls.key` come from its leaf Secret. This avoids trusting an unrestricted organization root from cert-manager's returned chain. Direct Helm rendering checks only the name and PEM shape, so direct Helm operators must perform the cryptographic validation themselves. See [the chart's certificate-mode contract](../../../../deploy/Chart/README.md#internal-service-certificate-modes) for exact values and prerequisites.

`rad upgrade kubernetes` and install with `--reinstall` preserve stored certificate settings when the flags are omitted, and validate the referenced CA before changes. `--reset-values` rejects stored certificate settings rather than silently reverting them. Certificate flags can confirm unchanged settings; an unchanged `--cert-issuer` can reuse the stored public bundle without a file. Ordinary operations reject certificate mode, signing Secret, issuer reference and CA trust changes; use the explicit coordinated CA phases instead. `--skip-preflight` does not bypass certificate or live issuer-policy validation, and upgrade with `--preflight-only` changes neither resources nor Helm history. Public-issuer transactions rerun the chosen chart's verifier with the pinned issuer, public CA and managed/external scope before issuance or resume.

Fresh enabled installation records a separate `radius-aggregation-bootstrap` intent. Retry the identical settings after interruption; changing the chart, issuer or selected CA is not recovery. A ready Secret or successful Helm install does not complete bootstrap: actual aggregation serving/process evidence and an authenticated aggregated-API request are required, plus all five component proofs in managed scope. External scope requires aggregation evidence without creating component PKI. Conflicting lifecycle operations, including disabling authorization or uninstalling, reject unfinished bootstrap. Default Off installation remains unchanged. See [bootstrap prerequisites and recovery](../../../../deploy/Chart/README.md#internal-service-certificate-modes).

For an installed chart and images supporting the required loaded-trust protocol, run `rad upgrade kubernetes --ca-rotation-phase publish --ca-secret next-radius-ca`, followed by separate `adopt`, `issue` and `retire` invocations without target flags. Managed scope requires all five component proofs; external scope verifies aggregation independently. Omit target flags in `publish` to migrate legacy aggregation to the installed CA. Radius owns public trust; cert-manager still issues and renews leaves, and shared cainjector remains untouched. `issue` does not automatically retire old trust. Expiry or compromise recovery requires explicit `--ca-rotation-phase recover` and accepts an outage while establishing new-only trust. See [the chart's rotation, recovery and retention contract](../../../../deploy/Chart/README.md#radius-owned-public-trust) before using these commands. Do not mix phases with release configuration changes, use old charts, or roll back managed trust.

Mutating CA phases acquire the namespace's `radius-identity-operation` Lease and reject unfinished provider-transition intent. Writes recheck the Lease and provider journal; a completed provider journal requires its full integrated validator. Handled errors release the exact acquired Lease version, but a crash leaves it for operator investigation rather than automatic timeout takeover. `--preflight-only` never acquires a Lease.

UCP reads `aws.fingerprintKeyFile`, the file that holds the HMAC key for fingerprints of write-only AWS property values. It defaults to `/var/secrets/aws-fingerprint/key`, where the chart mounts the `radius-aws-fingerprint-key` Secret, so the chart does not render it. Set it when running UCP as a local process with a key file of at least 32 random bytes. When the file is missing or invalid, UCP still starts and logs an error naming the Secret.

#### Configure verified Kubernetes caller identity

UCP rejects requests carrying any `X-Remote-*` headers with HTTP 401 `InvalidAuthenticationInfo` unless aggregation-proxy trust is configured and the TLS peer is a verified Kubernetes front proxy. This authentication boundary applies regardless of internal `authorization.mode` or workload authorization mode; `off`, `Disabled`, and `DryRun` do not permit spoofed identities. Headerless requests retain compatibility but have **no authenticated caller identity**. Protected endpoints must require `authorization.IdentityFromContext`; they must never fall back to a claimed user or treat an unidentified request as authenticated. The internal component listener on port 9444 rejects all forwarded identity headers and announced identity trailers, even from a peer trusted by the aggregation CA; component certificates never establish a user identity.

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

Trust is a startup snapshot. **Restart every UCP replica after rotating the CA bundle or changing allowed names or issuer.** A ConfigMap or projected file update alone does not replace the active trust snapshot. Invalid trust configuration or configured authentication without server TLS fails startup. The aggregation listener requests, but does not require, a client certificate: headerless deployment-engine access on port 9443 remains available until the engine migrates to the internal listener. This integration establishes user identity separately from internal component TLS; it does not add policy endpoints or user-role enforcement.

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
