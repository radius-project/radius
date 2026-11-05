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

| YAML section                    | Source of truth                                                                                       | Current values or shape                                                                                                    |
|---------------------------------|-------------------------------------------------------------------------------------------------------|----------------------------------------------------------------------------------------------------------------------------|
| `environment`                   | [`hostoptions.EnvironmentOptions`](../../../../pkg/armrpc/hostoptions/options.go)                     | `name`, `roleLocation`                                                                                                     |
| `databaseProvider`              | [`databaseprovider.Options`](../../../../pkg/components/database/databaseprovider/options.go)         | Provider is `apiserver`, `inmemory`, or `postgresql`                                                                       |
| `queueProvider`                 | [`queueprovider.QueueProviderOptions`](../../../../pkg/components/queue/queueprovider/options.go)     | Provider is `apiserver` or `inmemory`; includes a queue `name`                                                             |
| `secretProvider`                | [`secretprovider.SecretProviderOptions`](../../../../pkg/components/secret/secretprovider/options.go) | Provider is `kubernetes` or `inmemory`                                                                                     |
| `metricsProvider`               | [`metricsservice.Options`](../../../../pkg/components/metrics/metricsservice/options.go)              | `enabled`, `serviceName`, and nested `prometheus.path` / `prometheus.port`                                                 |
| `server`, `workerServer`, `ucp` | [`pkg/armrpc/hostoptions`](../../../../pkg/armrpc/hostoptions/)                                       | HTTP listener, async worker, and UCP connection settings                                                                   |
| `authorization`                 | [`authz.Options`](../../../../pkg/authz/mode.go)                                                      | `mode` is `off` (default), `dryRun`, or `enforce`; also `podSecurityLevel`, `controlPlaneNamespace`, `controlPlaneSecrets` |

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

Use a `direct` UCP connection only for local process debugging:

```yaml
ucp:
  kind: direct
  direct:
    endpoint: http://localhost:9000/apis/api.ucp.dev/v1alpha3
```

Kubernetes deployments use the chart-rendered UCP connection instead of the local endpoint.

The `authorization.mode` setting is read by UCP, Applications RP, Dynamic RP, and the controller, and each logs `authz mode=<mode>` at startup. The chart renders it from `global.rbac.enabled` and `global.rbac.dryRun`: `dryRun=true` renders `dryRun`, `enabled=true` renders `enforce`, and setting both fails rendering. In `off` mode the chart omits the section so older service binaries can still load the config.

> [!NOTE]
> No component authorization checks use this setting yet. Its only runtime effects are the admission controls below: Pod Security Admission labels, the admission policy, and the Applications RP control-plane namespace and Secret checks.

When the mode is not `off`, the chart also renders `podSecurityLevel` (from `global.rbac.podSecurityLevel`, `baseline` by default or `restricted`) and `controlPlaneNamespace` (the release namespace, which is never labeled). `dryRun` adds the `pod-security.kubernetes.io/warn` and `audit` labels; `enforce` also adds `enforce`. Labels that an operator already set on a namespace are never overwritten.

Labeling is additive: changing the mode or level does not rewrite or remove existing labels, including labels Radius previously added. The new setting applies to newly created namespaces and missing label keys when an environment or application is updated. To change the policy on an already labeled namespace, review and update its Pod Security labels explicitly; switching Radius to `off` does not disable existing namespace enforcement.

The chart also installs the `radius-control-plane-isolation` ValidatingAdmissionPolicy when the mode is not `off` and the cluster serves the `admissionregistration.k8s.io/v1` ValidatingAdmissionPolicy API (Kubernetes 1.30+). It checks Deployments and Pods that the identities in `global.rbac.admission.applicationIdentities` (default: the `applications-rp` service account) create with the `radapp.io/resource-type` label, and flags workloads in the control-plane namespace that use a control-plane service account (`control-plane-service-account`), reference a control-plane Secret (`control-plane-secret`), or run there at all (`control-plane-namespace`). `dryRun` binds it with `Warn` and `Audit`; `enforce` with `Deny`, and the message starts with `AdmissionPolicyDenied: <rule>`. The service account and Secret names come from the `radius.authz.controlPlaneServiceAccounts` and `radius.authz.controlPlaneSecrets` chart helpers.

Applications RP applies the same rules itself. It checks an `Applications.Core` or `Radius.Core` environment, or an application `kubernetesNamespace` extension, whose namespace is `controlPlaneNamespace` (rule `control-plane-namespace`), before it creates or labels the namespace. In `enforce`, this namespace check rejects the request immediately with HTTP 403 and `AdmissionPolicyDenied` naming the rule.

Before it applies the output resources of a deployment, Applications RP checks any Kubernetes object in `controlPlaneNamespace`, including a rendered Namespace object with that name: rule `control-plane-secret` when the object is, or its pod spec references, one of `controlPlaneSecrets` (volumes, projected volumes, `envFrom`, `secretKeyRef`, `imagePullSecrets`), otherwise `control-plane-namespace`. The chart renders `controlPlaneSecrets` from the `radius.authz.controlPlaneSecrets` helper. In `enforce`, this deployment output check fails the asynchronous operation before any outputs are applied; it does not return the denial in the original request's response. Follow the `Azure-AsyncOperation` URL until the operation reaches `Failed` with an `AdmissionPolicyDenied` error naming the rule. For both checks, `dryRun` logs a would-deny and allows the request.

Applications RP also checks the Kubernetes Secret that a legacy `Applications.Core/secretStores` resource reads or writes directly: the namespace and name in `properties.resource`, or the application or environment namespace when that property is empty. It checks a PUT or PATCH before creating the namespace or writing the Secret, and the `listSecrets` action before reading Secret data. A Secret in `controlPlaneNamespace` that is one of `controlPlaneSecrets` violates `control-plane-secret`; any other Secret there violates `control-plane-namespace`. With `authorization.mode` unset or `off` (the default), this check is skipped and existing secret store behavior is unchanged. `dryRun` logs a would-deny and allows the operation. Only opting into `enforce` makes a prohibited request fail immediately with HTTP 403 and `AdmissionPolicyDenied` naming the rule, including updates and `listSecrets` on existing stores; references outside the control-plane namespace remain allowed. The check evaluates only the Secret's namespace and name, so Secret values never appear in admission errors, logs, or denial responses. Allowed `listSecrets` responses still return values, including in `dryRun`. Delete is not checked, so a secret store created before enforcement can still be removed; delete removes only Secrets carrying the `radapp.io/resource-type` label. Secret stores always use the control-plane cluster, so `RADIUS_TARGET_KUBECONFIG` never exempts them and no target identity is read.

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
