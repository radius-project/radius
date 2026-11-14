# Shared Runtime And ARM-RPC

This document explains the common service runtime and ARM-RPC framework used by
the Go control-plane services in this repository. Read this before changing the
service startup flow, HTTP request handling, builder registration, or async
operation machinery.

The shared runtime provides a consistent model for loading service
configuration, building long-running services, wiring HTTP servers and
middleware, registering API handlers by namespace and operation, and processing
ARM-style async operations. It exists so UCP and `dynamic-rp` do not each
invent their own hosting, routing, validation, and async patterns.

## Quick Reference

| Topic                      | Start Here                                                 |
|----------------------------|------------------------------------------------------------|
| Host lifecycle             | `pkg/components/hosting/hosting.go`                        |
| Shared HTTP server         | `pkg/armrpc/frontend/server/server.go`                     |
| Namespace/API registration | `pkg/armrpc/builder/builder.go`                            |
| Async status               | `pkg/armrpc/asyncoperation/statusmanager/statusmanager.go` |
| Async worker               | `pkg/armrpc/asyncoperation/worker/...`                     |

| Test Focus                       | Packages                                                   |
|----------------------------------|------------------------------------------------------------|
| Shared runtime and builder logic | `./pkg/armrpc/...`                                         |
| Shared API/async host usage      | `./pkg/server/...`                                         |
| Broad downstream impact check    | `./pkg/ucp/...`, `./pkg/corerp/...`, `./pkg/dynamicrp/...` |

## Main Building Blocks

| Package                                   | Responsibility                                                     |
|-------------------------------------------|--------------------------------------------------------------------|
| `pkg/armrpc/hostoptions`                  | turns config files and environment into runtime options            |
| `pkg/components/hosting`                  | starts and supervises one or more long-running services            |
| `pkg/armrpc/frontend/server`              | shared HTTP server creation, middleware, version and health routes |
| `pkg/armrpc/builder`                      | namespace-based API and async controller registration              |
| `pkg/armrpc/frontend/controller`          | controller contracts for HTTP operations                           |
| `pkg/armrpc/asyncoperation/statusmanager` | creates and updates operation status resources and queue messages  |
| `pkg/armrpc/asyncoperation/worker`        | dequeues and executes async controllers                            |

## End-To-End Shape

```mermaid
graph TD
    Config[config file + env] --> Options[hostoptions]
    Options --> Root[service root command]
    Root --> Host[hosting.Host]

    Host --> HTTP[frontend server]
    Host --> Worker[async worker]

    Builder[builder.Builder] -.registers.-> HTTP
    Builder -.registers.-> Worker

    HTTP --> API[API controllers]
    API --> DB[(database.Client)]
    API --> Status[statusmanager]
    Status --> DB
    Status --> Queue[(queue.Client)]
    Queue --> Worker
    Worker --> AsyncCtrl[async controllers]
    AsyncCtrl --> DB
```

## Startup Model

### 1. Root command loads config and builds options

Each service root command follows the same general pattern:

- read a config file
- construct runtime options
- create a logger
- create one or more long-running services
- hand them to shared hosting

Examples:

- [cmd/ucpd/cmd/root.go](../../cmd/ucpd/cmd/root.go)
- [cmd/dynamic-rp/cmd/root.go](../../cmd/dynamic-rp/cmd/root.go)

For services using the standard host options path, the main constructor is
[pkg/armrpc/hostoptions/hostoptions.go](../../pkg/armrpc/hostoptions/hostoptions.go).

### 2. Shared hosting starts services and manages shutdown

[pkg/components/hosting/hosting.go](../../pkg/components/hosting/hosting.go)
defines a small but important contract:

- each long-running subsystem implements `Name()` and `Run(ctx)`
- a `Host` starts all services
- duplicate service names are rejected
- shutdown is coordinated through context cancellation and a timeout

This abstraction is what lets one process host multiple subsystems, such as the
API service and async worker inside `dynamic-rp`.

## HTTP Runtime Model

### Shared HTTP server

[pkg/armrpc/frontend/server/server.go](../../pkg/armrpc/frontend/server/server.go)
creates the common HTTP server used by ARM-RPC-based services.

Its default behavior includes:

- panic recovery middleware
- request logging middleware
- API not-found and method-not-allowed handlers
- optional ARM client certificate validation
- request context creation through `servicecontext.ARMRequestCtx`
- built-in `/version` and `/healthz` endpoints
- OpenTelemetry HTTP instrumentation

That means service-specific code usually does not build raw HTTP stacks itself.
Instead, it supplies a `Configure` callback that registers routes on top of the
shared server.

### Internal component authorization

ARM-RPC frontends serve through `mtls.ListenAndServe` in
[pkg/authz/mtls/serve.go](../../pkg/authz/mtls/serve.go), which applies the
authorization mode from the `authorization` config section (set by the chart
from `global.rbac.enabled` and `global.rbac.dryRun`). Internal calls are
authenticated with projected Kubernetes ServiceAccount tokens; no component
presents a client certificate.

The Helm setting `global.rbac.tls.provider` renders `authorization.tlsProvider` (`authz.Options.TLSProvider`), independently of the authorization mode. The default `radius` provider uses the TLS behavior below. The `external` provider uses HTTP under operator-provided encryption, loads no component certificate or CA material, and applies the same receiver-token verification and allowed-caller checks. Ordinary DryRun denials remain advisory; unavailable signing keys still return 503 in both providers. Unknown providers fail configuration, and neither provider retries through an unauthenticated transport. UCP's separate aggregated API TLS is unaffected.

The internal UCP/RP listeners expose [schema1 caller receipts](../../pkg/authz/mtls/internal_identity.go) through the same token middleware, with strict authentication even in DryRun. The UCP receipt stays inside its existing forwarded-user rejection boundary and is not exposed on aggregation. RP receipts call UCP through their actual configured client. The wire caller remains the full verified ServiceAccount subject; the canonical FQDN is a separate internal identity. Neither a receipt nor its nested UCP exchange substitutes for managed server SPIFFE verification or loaded-trust adoption evidence. The controller remains trust-only, with no receipt listener or serving certificate.

- `off` (default): plain HTTP, behavior unchanged.
- `dryRun`: the same port accepts TLS and plain HTTP. Plain HTTP, missing or
  invalid tokens, and disallowed callers are logged as would-deny
  (`InvalidAuthenticationInfo`, `AuthorizationFailed`), and the request is
  served. Signing keys that could not be refreshed are the exception: every
  request gets `503 AuthorizationUnavailable`, as in `enforce`.
- `enforce`: TLS is required (plain HTTP gets `400`). A missing, invalid, or
  wrong-audience token gets `401 InvalidAuthenticationInfo`; signing keys that
  could not be refreshed give `503 AuthorizationUnavailable`; and callers not in
  the server's allowed list get `403 AuthorizationFailed`. `applications-rp`
  and `dynamic-rp` allow only the `ucp` identity.

Each receiver presents the csi-driver-spiffe certificate mounted at
`/var/run/radius/spiffe` (`tls.crt`, `tls.key`), whose only SAN is the
URI `spiffe://<trustDomain>/ns/<namespace>/sa/<service account>`. It requests no
client certificate. It verifies the `Authorization: Bearer` token offline with
[pkg/authz/token](../../pkg/authz/token/verifier.go), requiring the receiver's
name as the audience, maps the ServiceAccount to the caller's
`<component>.<namespace>.svc` identity, and removes the header before calling the
handler. Probes and metrics stay on their plain ports.

Callers build transports with `mtls.NewClientTransport` and
`mtls.NewUpgradeTransport` in [pkg/authz/mtls/client.go](../../pkg/authz/mtls/client.go).
They verify the server's chain against `ca.crt` in `/var/run/radius/ca` and require exactly
one SAN, a URI SAN with the receiver's SPIFFE ID, instead of matching DNS
names; a certificate with any DNS, IP, email, or other SAN is rejected. Only
after the server is verified do they send the token from
`/var/run/radius/tokens/<receiver>/token`, which they re-read when it changes.
Internal public trust is mounted separately at `/var/run/radius/ca`, never with a CA signing key. BYO uses ConfigMap `radius-mtls-ca`; only selfSigned bootstrap projects `radius-ca/ca.crt` until an independent source is staged. In selfSigned mode aggregation retains a separate CA. In caSecret mode distinct internal and aggregation Issuers share the operator's signing Secret; issuerRef selects the existing issuer for both request profiles. UCP's ordinary DNS Certificate and internal own-Pod SPIFFE CSI requests have separate approval restrictions, even under the same CA. Runtime callers never present component client certificates; pinned CSI's client-auth EKU is not caller authorization.

The APIService's permanent public source is Secret `radius-apiserver-trust`, with the direct-injection annotation, not a leaf Secret or organization root. The chart guards existing sources and rotation journals against unvalidated overwrite. A new selfSigned CA needs a second connected render or lifecycle pre-staging before this source exists. CLI crypto/profile validation and journal-aware lifecycle/convergence remain paired integrations; cainjector merging alone does not establish exact retirement of old anchors. See the [certificate-mode contract](../../deploy/Chart/README.md#internal-service-certificate-modes).

Certificate and CA files are reloaded by polling (every 10 seconds) without a
restart; new handshakes use the new material. Before each request, a client
closes every connection whose verified server chain has expired, including
connections in use, which aborts their in-flight requests.

The [source-bound trust report](../../pkg/authz/mtls/trust_proof.go) describes the actual outbound CA generation, not a client certificate or a successful peer handshake. Its schema2 selected-anchor validity bounds do not establish token readiness or inbound serving readiness. No new Radius observer route is enabled: explicitly enabling the legacy process or aggregation proof configuration fails startup until an authenticated observer contract is approved.

The CLI can independently measure an exact Pod's serving certificate through [TLS-only observation](../../pkg/cli/helm/certificate_serving_observation.go). Kubernetes credentials authenticate only the outer port-forward connection; no workload HTTP request or component token is sent. The selected CA and DNS or SPIFFE identity verify the inner TLS connection. This measures serving material, not the Pod's outbound loaded trust or every HA API server's trust.

The [token/CSI lifecycle preparation](../../pkg/cli/helm/certificate_lifecycle.go) separates internal and aggregation trust planes and rejects legacy journals rather than reinterpreting their Certificate UIDs as CSI observations. Fresh enabled CLI bootstrap stages public trust before Helm. Production rotation and provider transitions stop before mutation because source reporting, complete authority/cache coverage and authorized acquisition remain unresolved. A readiness credential alone cannot resolve those dependencies. Default Off installation remains available. See [the current CLI boundaries](../contributing/contributing-code/contributing-code-control-plane/configSettings.md#2-update-shared-service-options); isolated API-server fixtures are not live issuance or image acceptance.

### Request context and validation

The server layer applies middleware that creates an ARM-aware request context.
This is what makes downstream code able to reason about API version, operation
type, resource ID, identity, and correlation metadata in a consistent way.

OpenAPI validation is not hard-coded per route in every service. Instead it is
usually created per namespace through the builder layer.

## Builder Model

The `builder.Builder` type in
[pkg/armrpc/builder/builder.go](../../pkg/armrpc/builder/builder.go) is the key
registration contract between resource namespaces and the shared runtime.

### API registration

`ApplyAPIHandlers` does three things:

- computes the standard root scope paths
- creates subrouters for plane scope, resource-group scope, and resource scope
- registers handler/controller pairs for operations like `GET`, `PUT`, `PATCH`,
  `DELETE`, and list operations

It also registers default ARM-RPC-style operations such as:

- `<namespace>/operations`
- `<namespace>/operationstatuses`
- `<namespace>/operationresults`

### Validation registration

`NewOpenAPIValidator` loads the swagger-backed OpenAPI spec for a namespace and
returns middleware that validates requests for the namespace routes.

This is an important architectural rule: validation is attached at the routing
layer, not left to each controller to reimplement.

### Async registration

`ApplyAsyncHandler` registers async controllers into the worker controller
registry using the same builder registrations. That is why synchronous API
handlers and async backend handlers are usually wired from the same namespace
setup code.

## Async Operation Model

### Status manager

[pkg/armrpc/asyncoperation/statusmanager/statusmanager.go](../../pkg/armrpc/asyncoperation/statusmanager/statusmanager.go)
is the bridge between request-time handling and background execution.

When a request needs long-running processing, the status manager:

- creates an operation status resource in the database
- creates a queue message that captures operation metadata
- stores ARM-facing details such as the operation ID and retry behavior

This is what lets the API layer return an ARM-style async response while the
real work happens later.

### Worker service

[pkg/armrpc/asyncoperation/worker/service.go](../../pkg/armrpc/asyncoperation/worker/service.go)
starts the async worker with:

- a database client
- a queue client
- a status manager
- a controller registry

The controller registry in
[pkg/armrpc/asyncoperation/worker/registry.go](../../pkg/armrpc/asyncoperation/worker/registry.go)
maps operation type plus method to an async controller instance.

That registry can also hold a default factory for operations that do not have a
more specific controller.

## How Services Use This Framework

- UCP uses the shared hosting and HTTP runtime patterns, but its routing layer
  is specialized around module dispatch, proxying, and adaptation.
- `dynamic-rp` uses the same runtime concepts, but applies them to generic
  resource handling rather than a multi-namespace hosted-provider process.

## When To Change Which Layer

- Change `hostoptions` when the service bootstrap or config-to-runtime mapping
  changes.
- Change `hosting` when process lifecycle, startup ordering, or shutdown rules
  change.
- Change `frontend/server` when shared HTTP behavior, middleware, or standard
  endpoints change.
- Change `builder` when namespace route generation, default operations, or
  validation wiring changes.
- Change `statusmanager` or `worker` when async operation semantics change.

## Invariants And Constraints

- Shared runtime code should remain generic across services.
- Builder registration is the contract that binds a namespace into both the API
  layer and the async layer.
- The HTTP path should stay separate from the background worker path even when
  they are hosted in the same process.
- Operation status persistence and queueing must remain consistent with the ARM
  async protocol shape exposed by the API layer.

## Common Mistakes

- Re-implementing routing or validation inside a service instead of using the
  builder and shared server patterns.
- Registering API handlers for a namespace without also checking its async
  handler registration.
- Treating the worker as independent from the status manager and queue contract.
- Changing shared middleware without considering impact on every ARM-RPC-based
  service.

## Change This Safely

When changing the shared runtime, assume you are changing multiple binaries at
once.

### Packages That Usually Move Together

- `pkg/armrpc/frontend/server` and `pkg/middleware` when shared request handling
  changes
- `pkg/armrpc/builder` and namespace setup packages when route registration or
  validator behavior changes
- `pkg/armrpc/asyncoperation/statusmanager` and
  `pkg/armrpc/asyncoperation/worker` when async semantics change
- `pkg/components/hosting` and service root commands when lifecycle behavior
  changes

### Suggested Test Scope

- `go test ./pkg/armrpc/...`
- `go test ./pkg/server/...`
- `go test ./pkg/ucp/...`
- `go test ./pkg/dynamicrp/...`

If the change is broad enough to affect shared process lifecycle or cross-cutting
HTTP behavior, also run the repo unit-test target documented in
[../contributing/contributing-code/contributing-code-tests/README.md](../contributing/contributing-code/contributing-code-tests/README.md).

## Start Reading In Code

- [pkg/components/hosting/hosting.go](../../pkg/components/hosting/hosting.go)
- [pkg/armrpc/frontend/server/server.go](../../pkg/armrpc/frontend/server/server.go)
- [pkg/armrpc/frontend/server/doc.go](../../pkg/armrpc/frontend/server/doc.go)
- [pkg/armrpc/builder/builder.go](../../pkg/armrpc/builder/builder.go)
- [pkg/armrpc/asyncoperation/statusmanager/statusmanager.go](../../pkg/armrpc/asyncoperation/statusmanager/statusmanager.go)
- [pkg/armrpc/asyncoperation/worker/service.go](../../pkg/armrpc/asyncoperation/worker/service.go)
- [pkg/armrpc/asyncoperation/worker/registry.go](../../pkg/armrpc/asyncoperation/worker/registry.go)
- [pkg/server/apiservice.go](../../pkg/server/apiservice.go)
- [pkg/server/asyncworker.go](../../pkg/server/asyncworker.go)

## Related Docs

- [service-interaction-map.md](service-interaction-map.md)
- [ucp.md](ucp.md)
- [dynamic-rp.md](dynamic-rp.md)
- [state-persistence.md](state-persistence.md)
