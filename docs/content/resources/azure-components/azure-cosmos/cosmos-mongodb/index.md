---
type: docs
title: "Azure CosmosDB Mongo"
linkTitle: "MongoDB API"
description: "Sample application running MongoDB through Azure CosmosDB API"
weight: 200
---

The `azure.com/CosmosDBMongo` Component defines an [Azure CosmosDB](https://azure.microsoft.com/en-us/services/cosmos-db/) configured with a MongoDB API.

## Platform resources

| Platform | Resource |
|----------|----------|
| [Microsoft Azure]({{< ref azure>}}) | [Azure CosmosDB API for MongoDB](https://docs.microsoft.com/en-us/azure/cosmos-db/mongodb-introduction)
| [Kubernetes]({{< ref kubernetes >}}) | Not compatible

## Configuration

| Property | Description | Example(s) |
|----------|-------------|---------|
| managed | Indicates if the resource is Radius-managed. If no, a `Resource` must be specified. | `true`, `false`
| resource | The ID of the user-managed CosmosDB with Mongo API to use for this Component. | `account::mongodb.id`

## Resource lifecycle

An `azure.com/CosmosDBMongo` can be either Radius-managed or user-managed. For more information read the [Components docs]({{< ref "components-model#resource-lifecycle" >}})

### Radius managed

{{< rad file="snippets/managed.bicep" embed=true marker="//SAMPLE" >}}

### User managed

#### Radius component

{{< rad file="snippets/user-managed.bicep" embed=true marker="//SAMPLE" >}}

#### Bicep resource

{{< rad file="snippets/user-managed.bicep" embed=true marker="//BICEP" >}}

## Bindings

### cosmos

The `cosmos` Binding of kind `azure.com/CosmosDBMongo` represents the the CosmosDB resource itself, and all APIs it offers.

| Property | Description |
|----------|-------------|
| `connectionString` | The MongoDB connection string used to connect to the database.
| `database` | The name of the database to which you are connecting.

### mongo

The `mongo` Binding of kind `mongodb.com/Mongo` represents the Mongo API offered by the CosmosDB resource.

| Property | Description |
|----------|-------------|
| `connectionString` | The MongoDB connection string used to connect to the database.
| `database` | The name of the database to which you are connecting.

