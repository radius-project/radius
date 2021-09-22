---
type: docs
title: "Azure CosmosDB SQL"
linkTitle: "SQL API"
description: "Sample application running on an Azure CosmosDB with SQL"
weight: 100
---

This application showcases how Radius can use a managed Azure CosmosDB with SQL API.

## Platform resources

| Platform | Resource |
|----------|----------|
| [Microsoft Azure]({{< ref azure>}}) | [Azure CosmosDB API with Core/SQL API](https://docs.microsoft.com/en-us/azure/cosmos-db/choose-api#coresql-api)
| [Kubernetes]({{< ref kubernetes >}}) | Not compatible

## Configuration

| Property | Description | Example(s) |
|----------|-------------|---------|
| managed | Indicates if the resource is Radius-managed. If no, a `Resource` must be specified. | `true`, `false`
| resource | The ID of the user-managed CosmosDB with SQL API to use for this Component. | `account::sqldb.id`

## Resource lifecycle

An `azure.com/CosmosDBSQL` can be Radius-managed. For more information read the [Components docs]({{< ref "components-model#resource-lifecycle" >}}).

{{< rad file="snippets/managed.bicep" embed=true marker="//SAMPLE" >}}

## Bindings

### cosmos

The `cosmos` Binding of kind `azure.com/CosmosDBSQL` represents the the CosmosDB resource itself, and all APIs it offers.

| Property | Description |
|----------|-------------|
| `connectionString` | The SQL connection string used to connect to the database.
| `database` | The name of the database to which you are connecting.

### sql

The `sql` Binding of kind `microsoft.com/SQL` represents the SQL API offered by the CosmosDB resource.

| Property | Description |
|----------|-------------|
| `connectionString` | The SQL connection string used to connect to the database.
| `database` | The name of the database to which you are connecting.
