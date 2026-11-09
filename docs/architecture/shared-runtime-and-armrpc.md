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

### Internal component authorization (mTLS)

ARM-RPC frontends serve through `mtls.ListenAndServe` in
[pkg/authz/mtls/serve.go](../../pkg/authz/mtls/serve.go), which applies the
authorization mode from the `authorization` config section (set by the chart
from `global.rbac.enabled` and `global.rbac.dryRun`):

- `off` (default): plain HTTP, behavior unchanged.
- `dryRun`: the same port accepts TLS and plain HTTP. Missing or invalid client
  certificates and disallowed callers are logged as would-deny
  (`PeerCertificateInvalid`, `AuthorizationFailed`), and every request is served.
- `enforce`: a client certificate issued by the Radius CA is required. Failed
  handshakes are closed and logged as `PeerCertificateInvalid`, and callers not
  in the server's allowed list get `403 AuthorizationFailed`. `applications-rp`
  and `dynamic-rp` allow only the `ucp` identity.

The certificate, key, and CA bundle are read from `/var/run/radius/mtls` and reloaded by polling the files (every 10 seconds) without a restart. In the default `selfSigned` mode all three files come from the component's `<component>-mtls` Secret. In `caSecret` and `issuerRef` modes a projected volume takes only the leaf certificate and key from that Secret and `ca.crt` from the chart's public `radius-mtls-ca` ConfigMap, so the receiver trusts the configured dedicated signing CA rather than the organization's root from a leaf's returned chain. The operator-owned signing Secret in `caSecret` mode is referenced only by the namespaced issuer, not mounted into services. `issuerRef` leaves the issuer and its signing key external; the chart verifies live bounded approver-policy/controller/admission/RBAC prerequisites before rendering issuance resources. See the [certificate-mode contract](../../deploy/Chart/README.md#internal-service-certificate-modes) for supported policy specs and administrative trust boundaries. New handshakes use the new material, and each request's client certificate is checked against the current CA bundle. Probes and metrics stay on their plain ports.

Every request also checks the current server certificate's full verified-chain validity. Cached peer verification lasts only until the earliest expiry in the verified chains or a material reload, whichever comes first. On an open connection, expired certificates or removed trust cause `enforce` to return `401 InvalidAuthenticationInfo` and close the connection; `dryRun` logs the failure and continues serving. Renewing the server certificate does not invalidate an otherwise trusted peer on an existing connection.

Outbound component transports check the local verified chain and accepted peer chains before reusing pooled HTTP/1.1 or HTTP/2 connections. A loaded CA bundle change or expired chain forces a fresh handshake; invalid current local material prevents dispatch. Closing an affected active connection can interrupt its in-flight requests. This is not a drain acknowledgment or an immediate revocation of every already acquired request. Projected material is read from one stable `..data` generation; plain directories require consecutive matching reads.

The [process TLS initialization evidence](../../pkg/authz/mtls/process_proof.go) can pin expected public roots and report the current leaf and validity without pinning normal leaf renewals. In this explicit opt-in mode only, incoming component listeners and outgoing client factories use one actual shared Source, including clients used by workers in the same process. Startup and renewal validate the component identity, permitted keys and both client/server usages. Use of the Source revalidates the files; invalid input clears usable material instead of retaining a last-good fallback. Normal-mode Source lifetimes remain unchanged.

The existing metrics listener exposes bounded public status at `/healthz/ucp-mtls` only in proof mode; disabled metrics or an unusable observation port fails startup rather than opening a new listener. The controller registers the route with its existing controller-runtime metrics server. No application mTLS policy is weakened. See the [chart opt-in](../../deploy/Chart/README.md#go-component-loaded-trust-evidence). The [exact-pod observer](../../pkg/cli/helm/certificate_proof.go) validates status through authenticated Kubernetes proxy and checks container identity before and after observation. This reports local TLS initialization, not hosted-service health, remote handshake success, process termination or drained streams. It does not enumerate every consumer or authorize a rotation transition; UCP's separate aggregation serving certificate still needs separate transition evidence.

UCP exposes that separate serving Source at `/healthz/ucp-aggregation-tls` when aggregation proof is enabled. Version 2 rotation [candidate observation](../../pkg/cli/helm/certificate_rotation_aggregation_inventory.go) verifies this evidence against the aggregation certificate and immutable public trust, not the component certificate. Candidate inventory permits retained old backends but rejects unknown consumers and changes during observation. Final inventory additionally requires deployment convergence and removal of old backends. A real request through the configured Kubernetes endpoint must reach an observed candidate before the rollout hold is released; this does not prove that unseen HA API-server transports have adopted trust.

The [aggregation deployment reconciler](../../pkg/cli/helm/certificate_rotation_workloads.go) can update serving projection and proof settings without changing component TLS configuration. Managed rollout applies both configurations in one Deployment update so a separate component rollout cannot remove the retained old backend. Failed candidate probes restore only the exact observed Service version; retries after routing completes recheck candidate evidence before releasing a remaining hold. New version 2 transactions prove aggregation adoption before switching component issuers. The CLI selects managed or aggregation-only scope from the installed provider, and both scopes support explicit incident recovery. External issuer transactions revalidate the pinned public issuer through the chosen chart's live policy verifier.

Fresh enabled installation uses a separate [bootstrap state machine](../../pkg/cli/helm/certificate_bootstrap_install.go), not a rotation with fictional predecessor identities. Its public intent precedes owned writes, and creation retries retain observed UID/specification pins. The selected chart supplies self-signed root resources; existing CA and public-issuer modes retain their ownership and policy boundaries. A verified aggregation leaf precedes public trust and workload installation. Exact-pod serving evidence, APIService availability and an authenticated aggregated request gate completion; managed scope also checks all five component processes. Bootstrap shares the CA/provider Lease and blocks conflicting lifecycle operations between retries. Its completed version 3 CA journal has no old CA and can feed subsequent rotation/provider transitions. Default Off installation retains its compatibility path. Private API-server and simulated-controller tests do not establish live issuer-controller or image acceptance.

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
