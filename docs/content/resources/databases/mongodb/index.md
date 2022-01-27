---
type: docs
title: "MongoDB database component"
linkTitle: "MongoDB"
description: "Learn how to use a MongoDB component in your application"
---

The `mongodb.com/MongoDB` component is a [portable component]({{< ref components-model >}}) which can be deployed to any [Radius platform]({{< ref platforms >}}).

## Platform resources

| Platform | Resource |
|----------|----------|
| [Microsoft Azure]({{< ref azure>}}) | [Azure CosmosDB API for MongoDB](https://docs.microsoft.com/en-us/azure/cosmos-db/mongodb-introduction)
| [Kubernetes]({{< ref kubernetes >}}) | [MongoDB Docker image](https://hub.docker.com/_/mongo/)

## Component format

Mongo databases can be either managed by Radius or provided by the user:

{{< tabs Radius-managed User-managed >}}

{{% codetab %}}
Simply create a 'mongo.com.MongoDatabase' and specify `managed: true`:

{{< rad file="snippets/managed.bicep" embed=true marker="//SAMPLE" >}}
{{% /codetab %}}

{{% codetab %}}
Begin by defining a CosmosDB with Mongo API in Bicep, either as part of the template or reference an `existing` resource, and then specify it as part of the 'mongo.com.MongoDatabase':

{{< rad file="snippets/user-managed.bicep" embed=true marker="//SAMPLE" >}}
{{% /codetab %}}

{{< /tabs >}}

### Properties

| Property | Description | Example(s) |
|----------|-------------|---------|
| managed | Indicates if the resource is Radius-managed. If no, a `Resource` must be specified. | `true`, `false`
| resource | The ID of the user-managed CosmosDB with Mongo API to use for this Component. | `account::mongodb.id`


## Provided data

| Bicep function | Description | Example |
|----------------|-------------|---------|
| connectionString() | Returns the connection string for the MongoDB. | `mongodb.connectionString()` |