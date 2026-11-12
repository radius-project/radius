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

| YAML section                    | Source of truth                                                                                       | Current values or shape                                                                                                                                                                                         |
|---------------------------------|-------------------------------------------------------------------------------------------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `environment`                   | [`hostoptions.EnvironmentOptions`](../../../../pkg/armrpc/hostoptions/options.go)                     | `name`, `roleLocation`                                                                                                                                                                                          |
| `databaseProvider`              | [`databaseprovider.Options`](../../../../pkg/components/database/databaseprovider/options.go)         | Provider is `apiserver`, `inmemory`, or `postgresql`                                                                                                                                                            |
| `queueProvider`                 | [`queueprovider.QueueProviderOptions`](../../../../pkg/components/queue/queueprovider/options.go)     | Provider is `apiserver` or `inmemory`; includes a queue `name`                                                                                                                                                  |
| `secretProvider`                | [`secretprovider.SecretProviderOptions`](../../../../pkg/components/secret/secretprovider/options.go) | Provider is `kubernetes` or `inmemory`                                                                                                                                                                          |
| `metricsProvider`               | [`metricsservice.Options`](../../../../pkg/components/metrics/metricsservice/options.go)              | `enabled`, `serviceName`, and nested `prometheus.path` / `prometheus.port`                                                                                                                                      |
| `server`, `workerServer`, `ucp` | [`pkg/armrpc/hostoptions`](../../../../pkg/armrpc/hostoptions/)                                       | HTTP listener, async worker, and UCP connection settings                                                                                                                                                        |
| `authorization`                 | [`authz.Options`](../../../../pkg/authz/mode.go)                                                      | `mode` is `off` (default), `dryRun`, or `enforce`; also `podSecurityLevel`, `controlPlaneNamespace`, `controlPlaneSecrets`, `certificateDirectory`, `caDirectory`, `tokenDirectory`, `trustDomain`, `namespace` |

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
> Besides the admission controls below, the mode controls authentication between control-plane components. When the mode is not `off`, UCP, Applications RP, and Dynamic RP mount a csi-driver-spiffe server certificate at `/var/run/radius/spiffe`, callers mount projected ServiceAccount tokens at `/var/run/radius/tokens/<receiver>/token`, and every component, including the controller, mounts only `ca.crt` from the `radius-ca` Secret at `/var/run/radius/ca`. The chart renders `certificateDirectory`, `caDirectory`, `tokenDirectory`, `trustDomain` (`global.rbac.spiffe.trustDomain`), and `namespace` (the release namespace) for each of them. UCP serves internal traffic on port 9444; its aggregated API on 9443 is unchanged.
>
> In `dryRun`, the internal listeners accept both TLS and plain HTTP and log requests they would deny, except that they reject every request with HTTP 503 `AuthorizationUnavailable` while the signing keys could not be refreshed. In `enforce`, they require TLS and a ServiceAccount token whose audience is the receiver's name. A missing, invalid, or wrong-audience token returns HTTP 401 `InvalidAuthenticationInfo`, and signing keys that could not be refreshed return HTTP 503 `AuthorizationUnavailable`. The resource providers accept only UCP's identity and return HTTP 403 `AuthorizationFailed` for any other component. Callers send a token only after verifying the receiver's SPIFFE ID.

When the mode is not `off`, UCP sends its tokens when it calls Applications RP and Dynamic RP over `https`, and the RPs and the controller send theirs when they call UCP on port 9444. The deployment engine still calls UCP through port 9443 without a token.

Before sending a request, component clients close connections whose verified server certificate chain has expired. This can interrupt active HTTP/2 response streams; it is not a timer that closes connections at the exact expiry instant. A successful CA-bundle reload applies to new client handshakes. Removing a server CA from that bundle does not immediately revoke already-open client connections. Clients re-read a token file when it changes, and at least once a minute.

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

UCP reads `aws.fingerprintKeyFile`, the file that holds the HMAC key for fingerprints of write-only AWS property values. It defaults to `/var/secrets/aws-fingerprint/key`, where the chart mounts the `radius-aws-fingerprint-key` Secret, so the chart does not render it. Set it when running UCP as a local process with a key file of at least 32 random bytes. When the file is missing or invalid, UCP still starts and logs an error naming the Secret.

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
