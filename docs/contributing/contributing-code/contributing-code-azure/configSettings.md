---
type: docs
title: "Radius resource provider configuration schemas"
linkTitle: "Configuration schemas"
description: "Schema docs for the resource provider configuration files"
weight: 40
---

## Summary 
Configuration schemas are used to define the service configuration for the resource provider's execution. The default configurations use the Applications.Core RP but configurations can be set to also run the Applications.Link RP for private preview and dev/test purposes. 

If you wanted to locally run Radius with specific configurations, `yaml` files can be created and stored in the `cmd` folder for the corresponding UCP or resource provider. 

<img width="600px" src="~/configExamples/localConfig.png" alt="local config files">

If you wanted to run Radius on Kubernetes with specific configurations, `yaml` files can be created and stored in the `deploy/Chart/charts` folder for the the `Applications.Core RP` or the `UCP`

<img width="600px" src="~/configExamples/kubeConfig.png" alt="Kubernetes config files">

## Schema

The following properties can be specified: 
| Key | Description |
|-----|-------------|
| environment | Environment name and its role location |
| identity | AAD APP authentication for the resource provider | 
| [**storageProvider**](#storageprovider) | Configuration options for the data storage provider |
| [**queueProvider**](#queueprovider) | Configuration options for the provider to create and manage the queue client |
| [**server**](#server) | Configuration options for the HTTP server bootstrap | 
| [**workerServer**](#workerserver) | Configuration options for the worker server | 
| [**metricsProvider**](#metricsprovider) | Configuration options of the providers for publishing metrics | 

The following are properties that can be specified on the UCP: 
| Key | Description |
|-----|-------------|
| [**secretProvider**](#secretprovider) | Configuration options for the secret provider 
| [**plane**](#plane) | Configuration options for the UCP plane
 
#### storageProvider

| Key | Example |
|-----|---------|
| apiServer | |
| cosmosdb | |
| etcd | |

#### queueProvider

| Key | Example |
|-----|---------|
| apiServer | |
| inMemoryQueue | |

#### secretProvider

| Key | Example |
|-----|---------|
| etcd | | 
| kubernetes | | 

#### server

| Key | Description | Example | 
|-----|-------------|---------|
| host | Domain name of the server | ```host: "0.0.0.0"``` |
| port | HTTP port | ```port: 8080``` |
| pathBase | HTTPRequest PathBase | ```pathBase: ""``` |
| authType | The environmnet authentification type (e.g. client ceritificate, etc) | ```authType: "ClientCertificate"``` |
| armMetadataEndpoint | Endpoint that provides the client certification | ```armMetadataEndpoint: "https://admin.api-dogfood.resources.windows-int.net/metadata/authentication?api-version=2015-01-01"``` |
| enableArmAuth | If set, the ARM client authentifictaion is performed | ```enableArmAuth: true``` |

#### workerServer

| Key | Description | Example |
|-----|-------------|---------|
| port | the localhost port which provides system-level info | ```port: 2222``` |
| maxOperationConcurrency | The maximum concurrency to process async request operations | ```maxOperationConcurrency: 3``` |
| maxOperationRetryCount | The maximum retry count to process async request operation | ```maxOperationRetryCount: 2``` |

#### metricsProvider

| Key | Description | Example |
|-----|-------------|---------|
| enabled | Specified whether to publish metrics | ```enabled: true``` |
| port | The connection port | ```path: "/metrics"``` |
| path | The endpoint name where the metrics are posted | ```port: 2222``` |

#### plane

| Key | Description | Example |
|-----|-------------|---------|
| id | The ID of the UCP plane | ```id: "/planes/radius/local"```
| type | The type of UCP plane | ```type: ""``` |
| name | The name of the UCP plane | ```name: ucp``` |
| properties | The properties specified on the plane (i.e. resource providers, kind, URL) | |