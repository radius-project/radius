# State Persistence in Radius

Radius persists state through three pluggable subsystems: a **database** for
resource data, a **secret store** for sensitive values, and a **message queue**
for asynchronous operations. Each subsystem is defined by a Go interface in
`pkg/components/` and has multiple implementations that can be selected at
startup via configuration.

```mermaid
graph TD
    subgraph "Control Plane Services"
        UCP["UCP<br/>cmd/ucpd"]
        CoreRP["Applications RP<br/>cmd/applications-rp"]
        DynRP["Dynamic RP<br/>cmd/dynamic-rp"]
        Controller["Controller<br/>cmd/controller"]
    end

    subgraph "Provider Layer"
        DBProvider["DatabaseProvider<br/>databaseprovider"]
        SecretProvider["SecretProvider<br/>secretprovider"]
        QueueProvider["QueueProvider<br/>queueprovider"]
    end

    subgraph "Interfaces"
        DBClient["database.Client"]
        SecretClient["secret.Client"]
        QueueClient["queue.Client"]
    end

    subgraph "Database Implementations"
        APIServer["APIServerClient<br/>apiserverstore"]
        Postgres["PostgresClient<br/>postgres"]
        InMemDB["InMemory Client<br/>inmemory"]
    end

    subgraph "Secret Implementations"
        K8sSecret["Kubernetes Client<br/>kubernetes"]
        InMemSecret["InMemory Client<br/>inmemory"]
    end

    subgraph "Queue Implementations"
        APIServerQ["APIServer Queue<br/>apiserver"]
        InMemQ["InMemory Queue<br/>inmemory"]
    end

    UCP --> DBProvider
    UCP --> SecretProvider
    UCP --> QueueProvider
    CoreRP --> DBProvider
    CoreRP --> SecretProvider
    CoreRP --> QueueProvider
    DynRP --> DBProvider
    DynRP --> SecretProvider
    Controller --> DBProvider

    DBProvider -->|"GetClient()"| DBClient
    SecretProvider -->|"GetClient()"| SecretClient
    QueueProvider -->|"GetClient()"| QueueClient

    DBClient -.->|implements| APIServer
    DBClient -.->|implements| Postgres
    DBClient -.->|implements| InMemDB

    SecretClient -.->|implements| K8sSecret
    SecretClient -.->|implements| InMemSecret

    QueueClient -.->|implements| APIServerQ
    QueueClient -.->|implements| InMemQ
```

## Interfaces

### `database.Client`

**Package:** `pkg/components/database`
**File:** [pkg/components/database/client.go](../../pkg/components/database/client.go)

The primary interface for persisting and querying resource data.
It understands ARM/UCP concepts like scopes, resource types, and resource IDs.
It provides optimistic concurrency control (OCC) via ETags.

```go
type Client interface {
    Query(ctx context.Context, query Query, options ...QueryOptions) (*ObjectQueryResult, error)
    Get(ctx context.Context, id string, options ...GetOptions) (*Object, error)
    Delete(ctx context.Context, id string, options ...DeleteOptions) error
    Save(ctx context.Context, obj *Object, options ...SaveOptions) error
}
```

#### Methods

| Method | Purpose |
|--------|---------|
| `Query` | Executes a scoped query filtered by root scope, resource type, and optional filters. Returns a paginated list of `Object` values. |
| `Get` | Retrieves a single resource by its fully-qualified resource ID. Returns `ErrNotFound` if the resource does not exist. |
| `Delete` | Removes a single resource by ID. Supports OCC via an optional ETag. |
| `Save` | Creates or updates a resource (logical PUT). Computes and sets the ETag on the object after writing. Supports OCC via an optional ETag. |

#### Key Types

- **`Object`** — Wraps a `Metadata` (ID + ETag) and a `Data` field (`any`) that is marshaled to/from JSON.
- **`Query`** — Specifies `RootScope`, `ResourceType`, optional `ScopeRecursive`, `RoutingScopePrefix`, `IsScopeQuery`, and `Filters`.
- **`QueryFilter`** — Field/value pair for property-level filtering (e.g., `properties.application`).

#### Error Types

| Error | Meaning |
|-------|---------|
| `ErrNotFound` | The resource with the given ID does not exist. |
| `ErrConcurrency` | An OCC conflict: the resource was modified or deleted since the ETag was read. |
| `ErrInvalid` | A programming error — invalid arguments were passed. |

### `secret.Client`

**Package:** `pkg/components/secret`
**File:** [pkg/components/secret/client.go](../../pkg/components/secret/client.go)

A simpler key-value interface for storing sensitive data (e.g., credentials,
connection strings). Values are opaque byte slices. The package also provides
generic helper functions `SaveSecret[T]` and `GetSecret[T]` that handle
JSON marshaling/unmarshaling.

```go
type Client interface {
    Save(ctx context.Context, name string, value []byte) error
    Delete(ctx context.Context, name string) error
    Get(ctx context.Context, name string) ([]byte, error)
}
```

| Method | Purpose |
|--------|---------|
| `Save` | Creates or updates a secret by name. |
| `Delete` | Deletes a secret by name. Returns `ErrNotFound` if absent. |
| `Get` | Retrieves a secret's value by name. Returns `ErrNotFound` if absent. |

### `queue.Client`

**Package:** `pkg/components/queue`
**File:** [pkg/components/queue/client.go](../../pkg/components/queue/client.go)

The interface for the asynchronous work queue used by the async operation
framework. Long-running operations (deployments, recipe execution) are enqueued
as messages and processed by backend workers.

```go
type Client interface {
    Enqueue(ctx context.Context, msg *Message, opts ...EnqueueOptions) error
    Dequeue(ctx context.Context, cfg QueueClientConfig) (*Message, error)
    FinishMessage(ctx context.Context, msg *Message) error
    ExtendMessage(ctx context.Context, msg *Message) error
}
```

| Method | Purpose |
|--------|---------|
| `Enqueue` | Adds a message to the queue. |
| `Dequeue` | Retrieves the next available message, respecting lease semantics. |
| `FinishMessage` | Acknowledges and removes a message after successful processing. |
| `ExtendMessage` | Extends the visibility timeout / lease on a message. |

## Implementations

### `database.Client` Implementations

```mermaid
classDiagram
    class Client {
        <<interface>>
        +Query(ctx, query, ...options) (*ObjectQueryResult, error)
        +Get(ctx, id, ...options) (*Object, error)
        +Delete(ctx, id, ...options) error
        +Save(ctx, obj, ...options) error
    }

    class APIServerClient {
        -client: controller-runtime Client
        -namespace: string
        +Query(ctx, query, ...options) (*ObjectQueryResult, error)
        +Get(ctx, id, ...options) (*Object, error)
        +Delete(ctx, id, ...options) error
        +Save(ctx, obj, ...options) error
    }

    class PostgresClient {
        -api: PostgresAPI
        +Query(ctx, query, ...options) (*ObjectQueryResult, error)
        +Get(ctx, id, ...options) (*Object, error)
        +Delete(ctx, id, ...options) error
        +Save(ctx, obj, ...options) error
    }

    class InMemoryClient {
        -mutex: sync.Mutex
        -resources: map~string, entry~
        +Query(ctx, query, ...options) (*ObjectQueryResult, error)
        +Get(ctx, id, ...options) (*Object, error)
        +Delete(ctx, id, ...options) error
        +Save(ctx, obj, ...options) error
    }

    Client <|.. APIServerClient
    Client <|.. PostgresClient
    Client <|.. InMemoryClient
```

#### 1. Kubernetes APIServer (`apiserverstore.APIServerClient`)

**Package:** `pkg/components/database/apiserverstore`
**Provider key:** `"apiserver"`

Stores resource data as Kubernetes Custom Resources (CRDs) of type
`ucp.dev/v1alpha1/Resource`. This is the default production backend when
Radius runs on Kubernetes.

**How it works:**

- Each UCP resource is stored as an entry inside a Kubernetes `Resource` object.
  The Kubernetes object name is derived from `<resource-name>.<SHA1-hash-of-id>`.
- Labels on the Kubernetes object encode the root scope segments and resource
  type, enabling efficient list queries via label selectors.
- Hash collisions are handled by storing multiple entries in a single Kubernetes
  object and using optimistic concurrency (Kubernetes resource versions) with
  retry logic (up to 10 retries).
- Queries use Kubernetes label selectors as "hints" and then post-filter results
  in-process against the full query criteria.

**Configuration:**

```yaml
database:
  provider: apiserver
  apiserver:
    context: ""          # Kubernetes context (optional, for testing)
    namespace: radius-system
```

#### 2. PostgreSQL (`postgres.PostgresClient`)

**Package:** `pkg/components/database/postgres`
**Provider key:** `"postgresql"`

Stores resources in a PostgreSQL `resources` table with columns for the
normalized ID, original ID, resource type, root scope, routing scope, ETag,
and JSON resource data.

**How it works:**

- Uses SQL `INSERT ... ON CONFLICT DO UPDATE` for upserts and conditional
  `DELETE ... WHERE etag = $2` for OCC.
- Queries are a single parameterized SQL statement with optional filters for
  scope recursion, routing scope prefix, and pagination (timestamp-based
  continuation tokens).
- Property-level filters (`QueryFilter`) are applied in-process after rows are
  fetched, keeping the SQL simple and compatible with the other implementations.

**Configuration:**

```yaml
database:
  provider: postgresql
  postgresql:
    url: "postgres://user:password@host:5432/radius"
```

#### 3. In-Memory (`inmemory.Client`)

**Package:** `pkg/components/database/inmemory`
**Provider key:** `"inmemory"`

Holds all data in a Go `map[string]entry` protected by a `sync.Mutex`.
Suitable for testing and single-process development.

**How it works:**

- All CRUD operations lock the mutex and operate directly on the map.
- Deep copies are made on read and write to prevent callers from mutating
  stored data.
- Queries iterate the full map and filter entries by scope, resource type,
  routing scope prefix, and query filters.

### `secret.Client` Implementations

```mermaid
classDiagram
    class SecretClient {
        <<interface>>
        +Save(ctx, name, value) error
        +Delete(ctx, name) error
        +Get(ctx, name) ([]byte, error)
    }

    class KubernetesSecretClient {
        +K8sClient: controller-runtime Client
        +Save(ctx, name, value) error
        +Delete(ctx, name) error
        +Get(ctx, name) ([]byte, error)
    }

    class InMemorySecretClient {
        -lock: sync.Mutex
        -data: map~string, []byte~
        +Save(ctx, name, value) error
        +Delete(ctx, name) error
        +Get(ctx, name) ([]byte, error)
    }

    SecretClient <|.. KubernetesSecretClient
    SecretClient <|.. InMemorySecretClient
```

#### 1. Kubernetes Secrets (`kubernetes.Client`)

**Package:** `pkg/components/secret/kubernetes`
**Provider key:** `"kubernetes"`

Stores each secret as a Kubernetes `Secret` resource in the `radius-system`
namespace. The secret value is stored under the key `ucp_secret` in the
Secret's data map.

#### 2. In-Memory (`inmemory.Client`)

**Package:** `pkg/components/secret/inmemory`
**Provider key:** `"inmemory"`

Holds secrets in a `map[string][]byte` protected by a `sync.Mutex`. For
testing and development only.

### `queue.Client` Implementations

#### 1. Kubernetes APIServer (`apiserver` queue)

**Package:** `pkg/components/queue/apiserver`
**Provider key:** `"apiserver"`

Uses Kubernetes Custom Resources as a message queue with lease-based
dequeue semantics.

#### 2. In-Memory (`inmemory` queue)

**Package:** `pkg/components/queue/inmemory`
**Provider key:** `"inmemory"`

A named in-memory queue for testing and development.

## Provider / Factory Pattern

Each subsystem uses a **provider** that acts as a lazy-initializing factory.
The provider is configured once at startup and returns the same client instance
for the lifetime of the process.

```mermaid
sequenceDiagram
    participant Service as Control Plane Service
    participant Provider as DatabaseProvider
    participant Factory as Factory Function
    participant Client as database.Client

    Service->>Provider: GetClient(ctx)
    alt First call
        Provider->>Factory: factory(ctx, options)
        Factory-->>Provider: client
        Provider-->>Service: client
    else Subsequent calls
        Provider-->>Service: cached client
    end
    Service->>Client: Save / Get / Query / Delete
```

### `DatabaseProvider`

**Package:** `pkg/components/database/databaseprovider`

```go
// Production: reads provider type from configuration
provider := databaseprovider.FromOptions(options)

// Testing: uses an ephemeral in-memory store
provider := databaseprovider.FromMemory()

// Testing: injects a mock or pre-built client
provider := databaseprovider.FromClient(mockClient)

// Retrieve the client (initializes on first call)
client, err := provider.GetClient(ctx)
```

The factory map in
[databaseprovider/factory.go](../../pkg/components/database/databaseprovider/factory.go)
maps provider type strings to initialization functions:

| Key | Factory | Client |
|-----|---------|--------|
| `"apiserver"` | `initAPIServerClient` | `apiserverstore.APIServerClient` |
| `"inmemory"` | `initInMemoryClient` | `inmemory.Client` |
| `"postgresql"` | `initPostgreSQLClient` | `postgres.PostgresClient` |

### `SecretProvider`

**Package:** `pkg/components/secret/secretprovider`

Similar pattern, but uses `NewSecretProvider` and `SetClient`. Factory map in
[secretprovider/factory.go](../../pkg/components/secret/secretprovider/factory.go):

| Key | Factory | Client |
|-----|---------|--------|
| `"kubernetes"` | `initKubernetesSecretClient` | `kubernetes.Client` |
| `"inmemory"` | `initInMemorySecretClient` | `inmemory.Client` |

### `QueueProvider`

**Package:** `pkg/components/queue/queueprovider`

Similar pattern, but uses `New` and `SetClient`. Factory map in
[queueprovider/factory.go](../../pkg/components/queue/queueprovider/factory.go):

| Key | Factory | Client |
|-----|---------|--------|
| `"apiserver"` | `initAPIServer` | `apiserver` queue client |
| `"inmemory"` | `initInMemory` | `inmemory` named queue |

## How to Create a New Implementation

This section walks through adding a new `database.Client` implementation. The
same pattern applies to `secret.Client` and `queue.Client`.

### Step 1: Create the implementation package

Create a new package under `pkg/components/database/<backend>/`. For example,
for a hypothetical DynamoDB backend:

```text
pkg/components/database/dynamodb/
├── client.go
└── client_test.go
```

### Step 2: Implement the `database.Client` interface

Your struct must satisfy `database.Client`. Add a compile-time check:

```go
package dynamodb

import (
    "context"
    "github.com/radius-project/radius/pkg/components/database"
)

var _ database.Client = (*Client)(nil)

type Client struct {
    // backend-specific fields (connection pool, SDK client, etc.)
}

func NewClient(/* backend-specific params */) *Client {
    return &Client{ /* ... */ }
}

func (c *Client) Query(ctx context.Context, query database.Query, options ...database.QueryOptions) (*database.ObjectQueryResult, error) {
    // 1. Validate ctx and query (call query.Validate())
    // 2. Translate query.RootScope, ResourceType, Filters, etc. into backend query
    // 3. Return *ObjectQueryResult with matching items
}

func (c *Client) Get(ctx context.Context, id string, options ...database.GetOptions) (*database.Object, error) {
    // 1. Validate ctx and id (parse with resources.Parse)
    // 2. Normalize the ID with databaseutil.ConvertScopeIDToResourceID
    // 3. Fetch from backend, return ErrNotFound if absent
}

func (c *Client) Delete(ctx context.Context, id string, options ...database.DeleteOptions) error {
    // 1. Validate ctx and id
    // 2. Normalize the ID
    // 3. Read ETag from options (database.NewDeleteConfig)
    // 4. Delete with OCC if ETag provided, return ErrConcurrency on conflict
    // 5. Return ErrNotFound if absent
}

func (c *Client) Save(ctx context.Context, obj *database.Object, options ...database.SaveOptions) error {
    // 1. Validate ctx and obj
    // 2. Normalize the ID
    // 3. Read ETag from options (database.NewSaveConfig)
    // 4. Compute new ETag: obj.ETag = etag.New(marshaledData)
    // 5. Upsert with OCC if ETag provided, return ErrConcurrency on conflict
}
```

Key implementation rules:

- **Normalize IDs** using `databaseutil.ConvertScopeIDToResourceID` and
  `databaseutil.NormalizePart` for consistent case-insensitive lookups.
- **Compute ETags** using `pkg/ucp/util/etag.New(raw)` on the JSON-marshaled
  data before writing.
- **Return correct error types**: `ErrNotFound`, `ErrConcurrency`, `ErrInvalid`.
- **Handle scope queries**: When `query.IsScopeQuery` is true, use
  `databaseutil.ConvertScopeTypeToResourceType` to normalize the resource type.
- **Apply `QueryFilter` values**: Use `Object.MatchesFilters` or implement
  equivalent filtering logic.

### Step 3: Add a provider type constant

In `pkg/components/database/databaseprovider/types.go`:

```go
const (
    TypeAPIServer  DatabaseProviderType = "apiserver"
    TypeInMemory   DatabaseProviderType = "inmemory"
    TypePostgreSQL DatabaseProviderType = "postgresql"
    TypeDynamoDB   DatabaseProviderType = "dynamodb"  // <-- new
)
```

### Step 4: Add configuration options

In `pkg/components/database/databaseprovider/options.go`, add a new options
struct and field:

```go
type DynamoDBOptions struct {
    Region    string `yaml:"region"`
    TableName string `yaml:"tableName"`
}

type Options struct {
    Provider   DatabaseProviderType `yaml:"provider"`
    APIServer  APIServerOptions     `yaml:"apiserver,omitempty"`
    InMemory   InMemoryOptions      `yaml:"inmemory,omitempty"`
    PostgreSQL PostgreSQLOptions    `yaml:"postgresql,omitempty"`
    DynamoDB   DynamoDBOptions      `yaml:"dynamodb,omitempty"`  // <-- new
}
```

### Step 5: Register the factory function

In `pkg/components/database/databaseprovider/factory.go`:

```go
import "github.com/radius-project/radius/pkg/components/database/dynamodb"

var databaseClientFactory = map[DatabaseProviderType]databaseClientFactoryFunc{
    TypeAPIServer:  initAPIServerClient,
    TypeInMemory:   initInMemoryClient,
    TypePostgreSQL: initPostgreSQLClient,
    TypeDynamoDB:   initDynamoDBClient,  // <-- new
}

func initDynamoDBClient(ctx context.Context, opt Options) (store.Client, error) {
    if opt.DynamoDB.Region == "" {
        return nil, errors.New("failed to initialize DynamoDB client: region is required")
    }
    return dynamodb.NewClient(opt.DynamoDB.Region, opt.DynamoDB.TableName), nil
}
```

### Step 6: Add shared conformance tests

The shared test suite in `test/ucp/storetest/shared.go` contains
`RunTest(t, client, clearFunc)` which validates all `database.Client`
behaviors including OCC, scope queries, filters, and error semantics. Wire it
into your test file:

```go
package dynamodb_test

import (
    "testing"
    "github.com/radius-project/radius/test/ucp/storetest"
)

func TestDynamoDBClient(t *testing.T) {
    client := /* create your client pointing at test infrastructure */
    storetest.RunTest(t, client, func(t *testing.T) {
        // Clear all data in the test table
    })
}
```

### Step 7: Configure and use

Set the provider in your Radius configuration YAML:

```yaml
database:
  provider: dynamodb
  dynamodb:
    region: us-west-2
    tableName: radius-resources
```

No other code changes are needed. The `DatabaseProvider` will read the
`provider` field and call the matching factory function to produce the client.

## Notable Details

- **Single client instance**: Each provider initializes the client exactly
  once using double-checked locking. All concurrent callers share the same
  client.
- **Optimistic concurrency**: All three `database.Client` implementations
  support ETag-based OCC. The APIServer implementation additionally
  leverages Kubernetes resource versions with retry logic.
- **ID normalization**: Resource IDs are normalized to lowercase with
  leading/trailing slashes. Scope-type IDs (e.g., resource groups) are
  converted to resource-type IDs for uniform storage.
- **Shared conformance tests**: The `test/ucp/storetest` package provides a
  comprehensive test suite that all `database.Client` implementations must
  pass, ensuring behavioral consistency across backends.
- **Generated mocks**: Both `database.Client` and `secret.Client` have
  `mockgen`-generated mocks (`mock_client.go`) for use in unit tests.
