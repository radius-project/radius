---
type: docs
title: "MongoDB database connector"
linkTitle: "MongoDB"
description: "Learn how to use a MongoDB connector in your application"
---

The `mongodb.com/MongoDatabase` connector is a [portable connector]({{< ref connectors >}}) which can be deployed to any [Radius platform]({{< ref platforms >}}).

## Supported resources

- [MongoDB container](https://hub.docker.com/_/mongo/)
- [Azure CosmosDB API for MongoDB](https://docs.microsoft.com/en-us/azure/cosmos-db/mongodb-introduction)

## Resource format

### Properties

A `resource` or set of `secrets` must be set to configure a MongoDB connector. Azure resources use `resource`, where properties and secrets are automatically generated from the Azure resource. Other workloads must manually configure `secrets`.

| Property | Description | Example |
|----------|-------------|---------|
| resource | The ID of a CosmosDB with Mongo API database to use for this connector. | `account::mongodb.id`
| secrets  | Configuration used to manually specify a Mongo container or other service providing a MongoDB. | See [secrets](#secrets) below.

#### Secrets

Secrets are used when defining a MongoDB connector with a non-Azure Mongo service, such as a container.

| Property | Description | Example |
|----------|-------------|---------|
| connectionString | The connection string to the MongoDB. Recommended to use parameters and variables to craft. | `mongodb://${userName}:${password}@${container.spec.hostname}:...`
| username | The username to use when connecting to the MongoDB. | `admin`
| password | The password to use when connecting to the MongoDB. | `password`

### Provided data

#### Functions

Secrets must be accessed via Bicep functions to ensure they're not leaked or logged.

| Bicep function | Description | Example |
|----------------|-------------|---------|
| `connectionString()` | Returns the connection string for the MongoDB. | `mongodb.connectionString()` |
| `username()` | Returns the username for the MongoDB. | `mongodb.username()` |
| `password()` | Returns the password for the MongoDB. | `mongodb.password()` |

## Starter

You can get up and running quickly with a Mongo Database by using a [starter]({{< ref starter-templates >}}):

{{< rad file="snippets/starter.bicep" embed=true >}}

### Container

The Mongo Database container starter uses a mongo container and can run on any Radius platform.

```
br:radius.azurecr.io/starters/mongo:latest
```

#### Input parameters

| Parameter | Description | Required | Default |
|-----------|-------------|:--------:|---------|
| radiusApplication | The application resource to use as the parent of the Mongo Database | Yes | - |
| dbName | The name for your Mongo Database | No | `deployment().name` (module name)` |
| username | The username for your Mongo Database | No | `'admin'` |
| password | The password for your Mongo Database | No | `newGuid()` |

#### Output parameters

| Parameter | Description | Type |
|----------|-------------|------|
| mongoDB | The Mongo Database resource | `radius.dev/Application/mongo.com.MongoDatabase@v1alpha3` |

### Microsoft Azure

The Mongo Database Azure starter uses an Azure CosmosDB and can run only on Azure.

```
br:radius.azurecr.io/starters/mongo-azure:latest
```

#### Input parameters

| Parameter | Description | Required | Default |
|-----------|-------------|:--------:|---------|
| radiusApplication | The application resource to use as the parent of the Mongo Database | Yes | - |
| dbName | The name for your Mongo Database | No | `deployment().name` (module name) |
| accountName | The name for your Azure CosmosDB | No | `'cosmos-${uniqueString(resourceGroup().id, deployment().name)}'` |
| location | The Azure region to deploy the Azure CosmosDB | No | `resourceGroup().location` |
| dbThroughput | The throughput for your Azure CosmosDB | No | `400` |

#### Output parameters

| Parameter | Description | Type |
|----------|-------------|------|
| mongoDB | The Mongo Database resource | `radius.dev/Application/mongo.com.MongoDatabase@v1alpha3` |
