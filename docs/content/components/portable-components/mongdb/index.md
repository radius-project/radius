---
type: docs
title: "MongoDB"
linkTitle: "MongoDB"
description: "Documentation for the MongoDB component"
weight: 100
---

This application showcases how Radius can use a portable MongoDB compatible database.

## Configuration

| Property | Description | Example(s) |
|----------|-------------|---------|
| managed | Indicates if the resource is Radius-managed. If no, a `Resource` must be specified. | `true`, `false`
| resource | The ID of the user-managed CosmosDB with Mongo API to use for this Component. | `account::mongodb.id`

## Resource lifecycle

A `mongodb.com/MongoDB` component can be Radius-managed. For more information read the [Components docs]({{< ref "components-model#resource-lifecycle" >}}).

### Radius managed

{{< rad file="snippets/managed.bicep" embed=true marker="//SAMPLE" >}}

### User managed

{{% alert title="Warning" color="warning" %}}
Currently user-managed MongoDB components are only supported in the Azure environment.
{{% /alert %}}

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

