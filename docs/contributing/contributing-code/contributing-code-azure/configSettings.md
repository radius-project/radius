## Summary 
Configuration schemas are used to define the service configuration for the resource provider's execution. The default configurations use the `Applications.Core RP` but configurations can also be set to run the `Applications.Link RP` for private preview and dev/test purposes. 

If you wanted to locally run Radius with specific configurations, `yaml` files can be created and stored in the `cmd` folder for the corresponding UCP or resource provider. 

![Local Config](./configExamples/localConfig.png)

If you wanted to run Radius on Kubernetes with specific configurations, `yaml` files can be created and stored in the `deploy/Chart/charts` folder for `Applications.Core RP`, `Applications.Link RP`, or `UCP`.

![Kubernetes Config](./configExamples/kubeConfig.png)


## Schema

The following properties can be specified in configuration for all services: 
| Key | Description | Example | 
|-----|-------------|---------|
| environment | Environment name and its role location | [**See below**](#environment) |
| identity | AAD APP authentication for the resource provider | [**See below**](#identity) |
| storageProvider | Configuration options for the data storage provider | [**See below**](#storageprovider) |
| queueProvider | Configuration options for the provider to create and manage the queue client | [**See below**](#queueprovider) |
| secretProvider | Configuration options for the provider to manage credential | [**See below**](#secretprovider) |
| server | Configuration options for the HTTP server bootstrap | [**See below**](#server) |
| workerServer | Configuration options for the worker server | [**See below**](#workerserver) |
| metricsProvider | Configuration options of the providers for publishing metrics | [**See below**](#metricsProvider) |

-----

The following are properties that can be specified for the `Applications.Core RP` and the `Applications.Link RP`: 
| Key | Description | Example |
|-----|-------------|---------|
| ucp | Configuration options for connecting to UCP's API | [**See below**](#ucp)

----

The following are properties that can be specified for UCP: 
| Key | Description | Example |
|-----|-------------|---------|
| secretProvider | Configuration options for the secret provider | [**See below**](#secretprovider)
| plane | Configuration options for the UCP plane | [**See below**](#plane)
 

### environment
| Key | Description | Example |
|-----|-------------|---------|
| name | The name of the environment | `Dev` | 
| location | The role location of the environment | `West US` |

### identity
| Key | Description | Example |
|-----|-------------|---------|
| clientId | Client ID of the Azure AAD App  | `your-client-ID` | 
| instance | The identity provider instance | `https://login.windows.net` |
| tenantId | Tenant ID of the Azure AAD App | `your-tenant-ID` |
| armEndpoint | ARM endpoint | `https://management.azure.com:443` |
| audience | The recipient of the certificate | `https://management.core.windows.net` |
| pemCertPath | Path to certificate file | `/var/certs/rp-aad-app.pem` |

### storageProvider
| Key | Description | Example |
|-----|-------------|---------|
| provider | The type of storage provider | `apiServer` | 
| apiServer | Object containing properties for Kubernetes APIServer store | [**See below**](#apiserver) | 
| cosmosdb | Object containing properties for CosmosDB | [**See below**](#cosmosdb) | 
| etcd | Object containing properties for ETCD store | [**See below**](#etcd)|

### queueProvider
| Key | Description | Example |
|-----|-------------|---------|
| provider | The type of queue provider | `apiServer` | 
| apiServer |  Object containing properties for Kubernetes APIServer store | [**See below**](#apiserver) |
| inMemoryQueue | Object containing properties for InMemory Queue client | |

### secretProvider
| Key | Description | Example |
|-----|-------------|---------|
| provider | The type of queue provider | `etcd`, `kubernetes` |
| etcd | Object containing properties for ETCD store | [**See below**](#etcd)|

### server
| Key | Description | Example | 
|-----|-------------|---------|
| host | Domain name of the server | `0.0.0.0` |
| port | HTTP port | `8080` |
| pathBase | HTTPRequest PathBase | `""` |
| authType | The environmnet authentification type (e.g. client ceritificate, etc) |`ClientCertificate` |
| armMetadataEndpoint | Endpoint that provides the client certification | `https://admin.api-dogfood.resources.windows-int.net/metadata/authentication?api-version=2015-01-01` |
| enableArmAuth | If set, the ARM client authentifictaion is performed (must be `true`/`false`) | `true` |

### workerServer
| Key | Description | Example |
|-----|-------------|---------|
| port | the localhost port which provides system-level info | `2222` |
| maxOperationConcurrency | The maximum concurrency to process async request operations | `3` |
| maxOperationRetryCount | The maximum retry count to process async request operation | `2` |

### metricsProvider
| Key | Description | Example |
|-----|-------------|---------|
| enabled | Specified whether to publish metrics (must be `true`/`false`) | `true` |
| port | The connection port | `/metrics` |
| path | The endpoint name where the metrics are posted | `2222` |

### ucp

This section configures the connection from either the `Applications.Core RP` or the `Applications.Link RP` to UCP's API. As the UCP service does not need to connect to itself, these settings do not apply in UCP's configuration files.

| Key | Description | Example |
|-----|-------------|---------|
| kind | Specifies how to connect and authenticate with UCP. Either `kubernetes` or `direct`. Kubernetes should always be used for production scenarios. Use `direct` for a local debugging configuration | `kubernetes` |
| direct | Settings that are applied when `kind==direct` | `{ }`|
| direct.endpoint | The URL endpoint used to connect to to UCP. | `http://localhost:9000` |

Example production use:

```yaml
ucp:
  kind: kubernetes
```

Example development use:

```yaml
ucp:
  kind: direct
  direct:
    endpoint: 'http://localhost:9000' # Tell RP that UCP is listening on port 9000 locally
```

### secretProvider
| Key | Description | Example |
|-----|-------------|---------|
| provider | The type of secret provider | `etcd` | 
| etcd | Object containing properties for ETCD secret store | [**See below**](#etcd) |  

### plane
| Key | Description | Example |
|-----|-------------|---------|
| id | The ID of the UCP plane | `/planes/radius/local` | 
| type | The type of UCP plane | `System.Planes/radius` |
| name | The name of the UCP plane | `ucp` |
| properties | The properties specified on the plane | [**See below**](#properties) |

## Available providers

### apiServer
| Key | Description | Example |
|-----|-------------|---------|
| inCluster | Configures the APIServer store to use "in-cluster" credentials (must be `true`/`false`) | `true` |
| context | The Kubernetes context name to use for the connection | `myContext` |
| namespace | The Kubernetes namespace used for data-storage | `radius-system` |

### etcd
| Key | Description | Example |
|-----|-------------|---------|
| inMemory | Configures the etcd store to run in-memory with the resource provider (must be `true`/`false`) | `true` |

### cosmosdb
| Key | Description | Example |
|-----|-------------|---------|
| url | URL of CosmosDB account | `https://radius-eastus-test.documents.azure.com:443/` |
| database | Name of the database in account | `applicationscore` |
| masterKey | All access key token for database resources | `your-master-key` |
| CollectionThroughput | Throughput of database | `400` |

## Plane properties

## properties
| Key | Description | Example |
|-----|-------------|---------|
| resourceProviders | Resource Providers for UCP Native Plane | `http://appcore-rp.radius-system:5443` |
| kind | The kind of plane | `Azure` |
| url | URL to forward requests to for non UCP Native Plane | `http://localhost:7443` |

## Example configuration files 

Below are completed examples of possible configurations: 

### Applications.Core and Applications.Link
```yaml
environment:
  name: self-hosted
  roleLocation: "global"
storageProvider:
  provider: "apiserver"
  apiserver:
    incluster: true
    context: ""
    namespace: "radius-system"
queueProvider:
  provider: "apiserver"
  apiserver:
    incluster: true
    context: ""
    namespace: "radius-system"
metricsProvider:
  prometheus:
    enabled: true
    path: "/metrics"
    port: 2222
server:
  host: "0.0.0.0"
  port: 5443
workerServer:
  maxOperationConcurrency: 3
  maxOperationRetryCount: 2
ucp:
  kind: kubernetes
```

### UCP 
```yaml
storageProvider:
  provider: "apiserver"
  apiserver:
    incluster: true
    context: ""
    namespace: "radius-system"
secretProvider:
  provider: "kubernetes"
planes:
  - id: "/planes/radius/local"
    properties:
      resourceProviders:
        Applications.Core: "http://appcore-rp.radius-system:5443"
        Applications.Link: "http://appcore-rp.radius-system:5444"
      kind: "UCPNative"
  - id: "/planes/deployments/local"
    properties:
      resourceProviders:
        Microsoft.Resources: "http://de-api.radius-system:6443"
      kind: "UCPNative"
  - id: "/planes/aws/aws"
    properties:
      kind: "AWS"
metricsProvider:
  prometheus:
    enabled: true
    path: "/metrics"
    port: 2222
```