---
type: docs
title: "Radius resource provider configuration schemas"
linkTitle: "Configuration schemas"
description: "Schema docs for the resource provider configuration files"
weight: 40
---

## Summary 
Configuration schemas are used to define the service configuration for the resource provider's execution. The default configurations use the Applications.Core RP but configurations can also be set to run the Applications.Link RP for private preview and dev/test purposes. 

If you wanted to locally run Radius with specific configurations, `yaml` files can be created and stored in the `cmd` folder for the corresponding UCP or resource provider. 

![Local Config](./configExamples/localConfig.png)

If you wanted to run Radius on Kubernetes with specific configurations, `yaml` files can be created and stored in the `deploy/Chart/charts` folder for the the `Applications.Core RP`, `Applications.Link RP`, or the `UCP`

![Kubernetes Config](./configExamples/kubeConfig.png)


## Schema

The following properties can be specified: 
| Key | Description | Example | 
|-----|-------------|---------|
| environment | Environment name and its role location | [**See below**](#environment) |
| identity | AAD APP authentication for the resource provider | [**See below**](#identity) |
| storageprovider | Configuration options for the data storage provider | [**See below**](#storageprovider) |
| queueprovider | Configuration options for the provider to create and manage the queue client | [**See below**](#queueprovider) |
| server | Configuration options for the HTTP server bootstrap | [**See below**](#server) |
| workerserver | Configuration options for the worker server | [**See below**](#workerserver) |
| metricsprovider | Configuration options of the providers for publishing metrics | [**See below**](#metricsProvider) |

The following are properties that can be specified on the UCP: 
| Key | Description |
|-----|-------------|
| secretprovider | Configuration options for the secret provider | [**See below**](#secretprovider)
| plane | Configuration options for the UCP plane | [**See below**](#plane)
 

### environment
| Key | Description | Example |
|-----|-------------|---------|
| name | The name of the environment | `Dev` | 
| location | The role location of the environment | `West US` |

### identity

| Key | Description | Example |
|-----|-------------|---------|
| clientId | Client ID of the Azure AAD App  | `set-client-ID` | 
| instance | The identity provider instance | `https://login.windows.net` |
| tenantId | Tenant ID of the Azure AAD App | `set-tenant-ID` |
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
| provider | The type of queue provider. | `apiServer` | 
| apiServer |  Object containing properties for Kubernetes APIServer store | [**See below**](#apiserver) |
| inMemoryQueue | Object containing properties for InMemory Queue client | |

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

### secretProvider

| Key | Description | Example |
|-----|-------------|---------|
| provider | The type of secret provider. | `etcd` | 
| etcd | Object containing properties for ETCD secret store | [**See below**](#etcd) |  

### plane

| Key | Description | Example |
|-----|-------------|---------|
| id | The ID of the UCP plane | `/planes/radius/local` | 
| type | The type of UCP plane | `System.Planes/radius` |
| name | The name of the UCP plane | `ucp` |
| properties | The properties specified on the plane (i.e. resource providers, kind, URL) | [**See below**](#properties) |

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

Below are completed examples of possible configurations. 

### AppCoreRP and AppLinkRP 


### UCP 
