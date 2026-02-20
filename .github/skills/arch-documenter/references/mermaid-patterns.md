# Mermaid Diagram Patterns

Reusable Mermaid templates for architecture documentation. Copy and adapt these patterns.

## High-Level System Overview (Top-Down Flowchart)

```mermaid
graph TD
    subgraph External["External Clients"]
        CLI["rad CLI"]
        API["API Clients"]
    end

    subgraph ControlPlane["Control Plane"]
        UCP["UCP<br/>Universal Control Plane"]
    end

    subgraph ResourceProviders["Resource Providers"]
        AppRP["Applications RP"]
        DynRP["Dynamic RP"]
    end

    subgraph Infrastructure["Infrastructure"]
        K8s["Kubernetes"]
        Azure["Azure"]
        AWS["AWS"]
    end

    CLI --> UCP
    API --> UCP
    UCP -->|routes requests| AppRP
    UCP -->|routes requests| DynRP
    AppRP -->|deploys to| K8s
    AppRP -->|deploys to| Azure
    DynRP -->|deploys to| K8s
```

## Sequence Diagram (Request Flow)

```mermaid
sequenceDiagram
    participant Client
    participant Frontend as Frontend<br/>(HTTP Handler)
    participant Controller as ARM RPC<br/>Controller
    participant Backend as Backend<br/>(Async Worker)
    participant Store as Data Store

    Client->>Frontend: PUT /resource
    Frontend->>Frontend: Validate request
    Frontend->>Controller: CreateOrUpdate()
    Controller->>Store: Save resource (Queued)
    Controller-->>Frontend: 201 Created (async)
    Frontend-->>Client: 201 + Operation-Location

    Note over Backend: Async processing
    Backend->>Store: Get resource
    Backend->>Backend: Execute operation
    Backend->>Store: Update resource (Succeeded/Failed)
```

## Component Diagram (Entity Relationships)

```mermaid
classDiagram
    class Interface {
        <<interface>>
        +Method() error
    }
    class ConcreteImpl {
        -field Type
        +Method() error
    }
    class Dependency {
        +HelperMethod() Result
    }

    Interface <|.. ConcreteImpl : implements
    ConcreteImpl --> Dependency : uses
```

## Package Dependency Diagram

```mermaid
graph LR
    subgraph cmd["cmd/"]
        main["main.go"]
    end

    subgraph pkg["pkg/"]
        frontend["frontend/"]
        backend["backend/"]
        datamodel["datamodel/"]
        api["api/"]
    end

    main --> frontend
    main --> backend
    frontend --> datamodel
    frontend --> api
    backend --> datamodel
    api --> datamodel
```

## State Diagram (Resource Lifecycle)

```mermaid
stateDiagram-v2
    [*] --> Provisioning: Create
    Provisioning --> Succeeded: Operation complete
    Provisioning --> Failed: Error

    Succeeded --> Updating: Update
    Updating --> Succeeded: Operation complete
    Updating --> Failed: Error

    Succeeded --> Deleting: Delete
    Failed --> Deleting: Delete
    Deleting --> [*]: Removed
    Deleting --> Failed: Error
```

## Deployment Topology (Subgraphs)

```mermaid
graph TD
    subgraph Namespace["radius-system namespace"]
        UCP["UCP Pod"]
        AppRP["Applications RP Pod"]
        DynRP["Dynamic RP Pod"]
        Controller["Controller Pod"]
    end

    subgraph UserNS["user namespace"]
        App["Application Resources"]
    end

    subgraph External["External"]
        Cloud["Cloud Providers"]
    end

    Controller -->|watches| App
    UCP -->|proxy| AppRP
    UCP -->|proxy| DynRP
    AppRP -->|manages| App
    AppRP -->|provisions| Cloud
```

## Tips for Effective Diagrams

### Keep It Readable

- **Max ~15 nodes** per diagram. Split complex systems into multiple diagrams.
- Use **subgraphs** to group related components and reduce visual clutter.
- Use **short labels** on arrows — one or two words.
- Prefer **top-down** (`TD`) for hierarchical relationships and **left-right** (`LR`) for data flows.

### Make It Accurate

- Use **actual names** from the code (package names, type names, function names).
- Show **real relationships** — don't invent connections that don't exist in the code.
- Include a **legend** or note if using non-obvious conventions.

### Sequence Diagram Tips

- Name participants after their **actual role** in the code (e.g., "FrontendController" not "Server").
- Use `Note over` to explain non-obvious steps.
- Use `activate`/`deactivate` to show which participant is processing.
- Show **error paths** with `alt`/`else` blocks when they are architecturally significant.

### Class Diagram Tips

- Use `<<interface>>` stereotypes for Go interfaces.
- Show only **architecturally significant** fields and methods, not every field.
- Use composition (`*--`) vs. aggregation (`o--`) vs. dependency (`-->`) appropriately.
