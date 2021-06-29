---
type: docs
title: "Azure CosmosDB SQL"
linkTitle: "Azure CosmosDB SQL"
description: "Sample application running on an Azure CosmosDB with SQL"
---

This application showcases how Radius can use a managed Azure CosmosDB with SQL API.

## Configuration

| Property | Description | Example(s) |
|----------|-------------|---------|
| managed | Indicates if the resource is Radius-managed. If no, a `Resource` must be specified. | `true`, `false`
| resource | The ID of the user-managed CosmosDB with SQL API to use for this Component. | `account::sqldb.id`

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

## Example

{{< rad file="snippets/azure-cosmos-sql-manged.bicep" embed=true replace-key-hide="//HIDE" replace-value-hide="run: {...}" >}}
