---
type: docs
title: "MongoDB database component"
linkTitle: "MongoDB"
description: "Learn how to use a MongoDB component in your application"
---

The `mongodb.com/MongoDB` component is a [portable component]({{< ref components-model >}}) which can be deployed to any [Radius platform]({{< ref environments >}}).

## Platform resources

| Platform | Resource |
|----------|----------|
| [Microsoft Azure]({{< ref azure-environments >}}) | [Azure CosmosDB API for MongoDB](https://docs.microsoft.com/en-us/azure/cosmos-db/mongodb-introduction)
| [Kubernetes]({{< ref kubernetes-environments >}}) | [MongoDB Docker image](https://hub.docker.com/_/mongo/)

## Configuration

| Property | Description | Example(s) |
|----------|-------------|---------|
| managed | Indicates if the resource is Radius-managed. If no, a `Resource` must be specified. | `true`, `false`
| resource | The ID of the user-managed CosmosDB with Mongo API to use for this Component. | `account::mongodb.id`

## Resource lifecycle

A `mongodb.com/MongoDB` component can be Radius-managed and user-managed. For more information read the [Components docs]({{< ref "components-model#resource-lifecycle" >}}).

{{% alert title="Warning" color="warning" %}}
At this time user-managed MongoDB components are only supported in Azure environments.
{{% /alert %}}

### Radius managed

{{< rad file="snippets/managed.bicep" embed=true marker="//SAMPLE" >}}

### User managed

#### Radius component

{{< rad file="snippets/user-managed.bicep" embed=true marker="//SAMPLE" >}}

#### Bicep resource

{{< rad file="snippets/user-managed.bicep" embed=true marker="//BICEP" >}}

## Bindings

### mongo

The `mongo` Binding of kind `mongodb.com/Mongo` represents the Mongo API offered by the CosmosDB resource.

| Property | Description |
|----------|-------------|
| `connectionString` | The MongoDB connection string used to connect to the database.
| `database` | The name of the database to which you are connecting.

