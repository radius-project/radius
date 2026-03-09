# Service Interaction Map

This document explains how the main executables in this repository fit
together at runtime. Use it as the top-level map before diving into a specific
service.

```mermaid
graph TD
    CLI[rad CLI\ncmd/rad]
    UCP[UCP\ncmd/ucpd]
    DYNRP[dynamic-rp\ncmd/dynamic-rp]
    CTRL[controller\ncmd/controller]
    DE[deployment-engine\nexternal repo]
    DB[(database.Client)]
    Q[(queue.Client)]
    S[(secret.Client)]
    K8S[Kubernetes API]

    CLI --> UCP
    CLI --> K8S
    UCP --> DYNRP
    UCP --> K8S
    DYNRP --> DB
    DYNRP --> Q
    DYNRP --> S
    CTRL --> K8S
    CTRL --> UCP
    DYNRP --> DE
```

## Components

- **`rad`** is the user-facing CLI. It loads workspace and connection config,
  builds clients, and invokes Radius APIs or Kubernetes/Helm operations.
- **`ucpd`** is the Universal Control Plane. It is the main routing point for
  control-plane API requests.
- **`dynamic-rp`** is the main authoring surface for Radius resource types and
  generic resource lifecycle behavior.
- **`controller`** runs Kubernetes reconcilers and webhooks for Radius custom
  resources and related workflows.
- **Deployment Engine** is not implemented in this repository, but several
  flows cross that boundary.

This map is intentionally focused on the current contributor path for new work.
Some legacy provider processes still exist in the runtime, but new authoring
work should target Radius resource types through `dynamic-rp`.
`dynamic-rp`.

## Main Runtime Patterns

### CLI to service path

Most user-initiated operations begin in `rad`, which resolves the active
workspace and connection, then sends requests either to UCP or directly to the
cluster for install/debug workflows.

### UCP as the control-plane hub

UCP receives the request, identifies the target plane or provider, and either:

- serves UCP-native behavior itself
- proxies to `dynamic-rp`
- adapts the request for an external control plane such as AWS

### Shared state model

The provider processes share pluggable abstractions for:

- resource state in `database.Client`
- async work in `queue.Client`
- sensitive values in `secret.Client`

Those abstractions are described in
[state-persistence.md](state-persistence.md).

### Reconciliation path

The controller does not replace the resource providers. Instead it watches
Kubernetes resources, coordinates Kubernetes-native workflows, and uses Radius
clients to drive backend operations through the control plane.

## Typical Flows

### Deploy through the CLI

```mermaid
sequenceDiagram
    participant User
    participant CLI as rad
    participant UCP
  participant RP as dynamic-rp
    participant Queue
    participant Worker as async worker

    User->>CLI: run command
    CLI->>UCP: send control-plane request
    UCP->>RP: route or proxy request
    RP->>Queue: enqueue async work if needed
    RP-->>UCP: return ARM-style async response
    UCP-->>CLI: return status URL / operation state
    Worker->>RP: process queued operation
```

### Reconcile inside the cluster

```mermaid
sequenceDiagram
    participant K8S as Kubernetes API
    participant CTRL as controller
    participant UCP
    participant RP as Radius backend

    K8S->>CTRL: watch event for CRD or source change
    CTRL->>UCP: invoke Radius API
    UCP->>RP: route request
    RP-->>UCP: return result
    UCP-->>CTRL: response
    CTRL->>K8S: update status or emit events
```

## Boundaries That Matter When Changing Code

- If the change is about **routing, plane selection, or protocol translation**,
  start in UCP.
- If the change is about **authoring or handling Radius resource types**,
  start in `dynamic-rp`.
- If the change is about **Kubernetes watch/reconcile/webhook behavior**,
  start in `controller`.
- If the change is about **user experience, config, or command orchestration**,
  start in `rad`.

## Start Reading in Code

- [cmd/ucpd/main.go](../../cmd/ucpd/main.go)
- [cmd/ucpd/cmd/root.go](../../cmd/ucpd/cmd/root.go)
- [cmd/dynamic-rp/main.go](../../cmd/dynamic-rp/main.go)
- [cmd/dynamic-rp/cmd/root.go](../../cmd/dynamic-rp/cmd/root.go)
- [cmd/controller/main.go](../../cmd/controller/main.go)
- [cmd/controller/cmd/root.go](../../cmd/controller/cmd/root.go)
- [cmd/rad/main.go](../../cmd/rad/main.go)
- [cmd/rad/cmd/root.go](../../cmd/rad/cmd/root.go)