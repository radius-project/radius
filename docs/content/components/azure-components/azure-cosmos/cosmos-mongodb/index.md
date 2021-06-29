---
type: docs
title: "Azure CosmosDB Mongo"
linkTitle: "Azure CosmosDB Mongo"
description: "Sample application running MongoDB through Azure CosmosDB API"
---

The `azure.com/CosmosDBMongo` Component defines an [Azure CosmosDB](https://azure.microsoft.com/en-us/services/cosmos-db/) configured with a MongoDB API.

## Resource lifecycle

An `azure.com/CosmosDBMongo` can be either Radius-managed or user-managed. For more information read the [Components docs]({{< ref "components-model#resource-lifecycle" >}})

## Configuration

| Property | Description | Example(s) |
|----------|-------------|---------|
| managed | Indicates if the resource is Radius-managed. If no, a `Resource` must be specified. | `true`, `false`
| resource | The ID of the user-managed CosmosDB with Mongo API to use for this Component. | `account::mongodb.id`

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

## Example

{{< tabs "Radius Managed" "User Managed" >}}

{{% codetab %}}
{{< rad file="snippets/azure-cosmos-mongo-managed.bicep" embed=true marker="//SAMPLE" replace-key-hide="//HIDE" replace-value-hide="run: {...}" >}}
{{% /codetab %}}

{{% codetab %}}
In this example, `Microsoft.DocumentDB/databaseAccounts` and `mongodbDatabases` resources are defined in Bicep, and then referenced in a Radius application.

You can also use Bicep's [existing functionality](https://docs.microsoft.com/en-us/azure/azure-resource-manager/bicep/resource-declaration?tabs=azure-powershell#reference-existing-resources) to reference a resource that has previously been deployed.

{{< rad file="snippets/azure-cosmos-mongo-usermanaged.bicep" embed=true marker="//SAMPLE" replace-key-hide="//HIDE" replace-value-hide="run: {...}" replace-key-properties="//PROPERTIES" replace-value-properties="properties: {...}" >}}
{{% /codetab %}}

{{< /tabs >}}
