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

| YAML section                    | Source of truth                                                                                       | Current values or shape                                                                        |
|---------------------------------|-------------------------------------------------------------------------------------------------------|------------------------------------------------------------------------------------------------|
| `environment`                   | [`hostoptions.EnvironmentOptions`](../../../../pkg/armrpc/hostoptions/options.go)                     | `name`, `roleLocation`                                                                         |
| `databaseProvider`              | [`databaseprovider.Options`](../../../../pkg/components/database/databaseprovider/options.go)         | Provider is `apiserver`, `inmemory`, or `postgresql`                                           |
| `queueProvider`                 | [`queueprovider.QueueProviderOptions`](../../../../pkg/components/queue/queueprovider/options.go)     | Provider is `apiserver` or `inmemory`; includes a queue `name`                                 |
| `secretProvider`                | [`secretprovider.SecretProviderOptions`](../../../../pkg/components/secret/secretprovider/options.go) | Provider is `kubernetes` or `inmemory`                                                         |
| `metricsProvider`               | [`metricsservice.Options`](../../../../pkg/components/metrics/metricsservice/options.go)              | `enabled`, `serviceName`, and nested `prometheus.path` / `prometheus.port`                     |
| `server`, `workerServer`, `ucp` | [`pkg/armrpc/hostoptions`](../../../../pkg/armrpc/hostoptions/)                                       | HTTP listener, async worker, and UCP connection settings                                       |
| `authorization`                 | [`authz.Options`](../../../../pkg/authz/mode.go)                                                      | `mode` is `off` (default), `dryRun`, or `enforce`                                              |
| `controller`                    | [`hostoptions.ControllerOptions`](../../../../pkg/armrpc/hostoptions/providerconfig.go)               | `flux.enabled` defaults to `false`; `flux.sources` selects exact namespace/name/UID identities |

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
> Services only read and log this setting for now. No authorization checks use it yet, so selecting `enforce` does not block requests.

#### Flux source configuration

`controller.flux` is independent of the reserved `authorization` mode. Both chart and local-process configurations default to disabled with an empty source list. Select only installation-trusted publishers, including every accepted executable dependency. This is not a per-namespace execution sandbox; the default deployment engine can manage arbitrary Kubernetes resources.

```yaml
controller:
  flux:
    enabled: true
    sources:
      - namespace: flux-system
        name: applications
        uid: "<read from the existing GitRepository>"
```

Read the live identity and generation with `kubectl get gitrepositories.source.toolkit.fluxcd.io applications -n flux-system -o json`. Do not copy a UID from a deleted object. Only active sources with a current-generation Ready artifact are processed; the controller does not write Flux-owned conditions.

Before enabling, inventory all user/group grants on the selected GitRepository, verification Secrets, included sources, controller ConfigMap, DeploymentTemplate specifications/status, and DeploymentResource specifications/status. Only administrators and the appropriate controllers should be able to modify these objects. Check actual lower-trust principals, including their groups, rather than assuming a namespace implies isolation:

```sh
kubectl auth can-i update gitrepositories.source.toolkit.fluxcd.io -n "$SOURCE_NAMESPACE" --as="$PRINCIPAL"
kubectl auth can-i patch secrets -n "$SOURCE_NAMESPACE" --as="$PRINCIPAL"
kubectl auth can-i patch configmaps -n radius-system --as="$PRINCIPAL"
kubectl auth can-i create deploymenttemplates.radapp.io -n "$TEMPLATE_NAMESPACE" --as="$PRINCIPAL"
kubectl auth can-i patch deploymenttemplates.radapp.io -n "$TEMPLATE_NAMESPACE" --as="$PRINCIPAL"
kubectl auth can-i update deploymenttemplates.radapp.io --subresource=status -n "$TEMPLATE_NAMESPACE" --as="$PRINCIPAL"
```

An administrator running impersonation checks must supply the relevant `--as-group` values and repeat the checks for create/update/patch/delete and resource-status permissions as applicable. Confirm denied mutations in a disposable test installation. A source name, signature, or caller-supplied attribution field does not replace Kubernetes write permissions.

Restart a local controller after configuration changes. Helm changes the controller checksum and uses a non-overlapping `Recreate` rollout, briefly pausing all three reconcilers. Malformed source configuration is a startup error even while disabled; run `helm template` with the intended values before upgrading. Old component image overrides are not compatible with the new settings.

For existing installations, apply the matching additive CRD and explicitly review/associate legacy source-managed templates before enabling Flux. Follow the [chart migration and recovery procedure](../../../../deploy/Chart/README.md#migrating-existing-templates); do not delete/recreate templates or remove held finalizers to bypass migration. `Held` retains operation/tracking state and does not claim a deployment succeeded. Source changes do not cancel previously accepted backend operations.

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
- **Flux does not reconcile after an upgrade.** Check the startup source-selection notice, the exact source UID, and current-generation Ready status. An empty or disabled selection performs no Flux work. A legacy template needs a reviewed `sourceRef`; schema errors require the matching chart CRD.
- **A template or resource remains `Held`.** Inspect its reasoned event and source identity/generation. Restore an approved source or perform a reviewed association. Holds preserve finalizers, including during deletion; avoid rolling back to a binary that predates these settings.
