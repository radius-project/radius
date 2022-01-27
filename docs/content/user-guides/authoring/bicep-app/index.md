---
type: docs
title: "Model your application and its services"
linkTitle: "Model application"
description: "Learn how to model your application and its services in Radius"
weight: 300
---

Now that your infrastructure is modeled in Bicep, you can model your app's services with Radius resources.

## Create a new Radius application

Create a new Bicep resource that represents your [application]({{< ref application-model >}}). This is the parent resource which will contain all of your services and relationships:

{{< rad file="snippets/blank.bicep" embed=true >}}

## (optional) Add portable components

If your application needs to be portable across [Radius platforms]({{< ref platforms >}}), you can use portable components to add an abstraction layer. For example, the [MongoDB]({{< ref mongodb >}}) and [Dapr]({{< ref dapr >}}) resources allow different infrastucture resources to bind to them, and then your services connect to these abstractions.

{{% alert title="Portable components" color="info" %}}
Portable components allow you to decouple infrastructure that provides an API from your application which consumes it. For example, a MongoDB can be provided by both Azure CosmosDB and a MongoDB container. Using a portable component allows your application services to use common values like `host`, `port` and `connectionString` to connect to the MongoDB, and the underlying infrastructure binding can be swapped out.
{{% /alert %}}

Add a [MongoDB]({{< ref mongodb >}}) component to your application and bind it to the CosmosDB with MongoDB resource you previously modeled:

{{< rad file="snippets/mongo.bicep" embed=true replace-key-cosmos="//COSMOS" replace-value-cosmos="resource cosmos 'Microsoft.DocumentDB/databaseAccounts@2021-04-15' existing = {...}" >}}

## Add services

Now that you have an application resource defined you can add services to it. For example, you can add a [container]({{< ref container >}}):

{{< rad file="snippets/service.bicep" embed=true >}}

## Connect to infrastructure

Relationships between Radius services and other resources can be defined through [connections]({{< ref connections-model >}}). Connections allow you to configure:

- Environment variables with resource properties and connection information
- Managed identities and role based acces control (where applicable)
- Scoping and least-privilege communication (where applicable)

{{< rad file="snippets/connection.bicep" embed=true replace-key-mongo="//MONGO" replace-value-mongo="resource mongo 'mongo.com.MongoDatabase' = {...}" replace-key-cosmos="//COSMOS" replace-value-cosmos="resource cosmos 'Microsoft.DocumentDB/databaseAccounts@2021-04-15' existing = {...}" replace-key-container="//CONTAINER" replace-value-container="container: {...}" >}}

## Next steps

Now that you have defined your infrastructure, application, services, and connections you can:

- [Deploy your application to a Radius-enabled platform]({{< ref deploying >}})
- [Break it up into modules so separate teams can work on different parts of the application]({{< ref bicep-modules >}})
