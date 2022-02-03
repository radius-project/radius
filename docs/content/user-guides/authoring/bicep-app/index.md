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

## (optional) Add portable connectors

If your application needs to be portable across [Radius platforms]({{< ref platforms >}}), you can use connectors to add an abstraction layer for each resource. Connectors present common values like `host`, `port` and `connectionString` that Service resources (like containers) can use to connect to the related API or service. The underlying infrastructure type can then be swapped out.

{{< button text="Connectors library" page="connectors" >}}

For example, the [mongo.com.mongoDatabase]({{< ref mongodb >}}) connector allows either an Azure CosmosDB and a MongoDB container to bind to it.

A MongoDB connector can be modeled as:

{{< rad file="snippets/mongo.bicep" embed=true replace-key-cosmos="//COSMOS" replace-value-cosmos="resource cosmos 'Microsoft.DocumentDB/databaseAccounts@2021-04-15' existing = {...}" >}}

## Add services

Now that you have an application resource defined you can add [services]({{< ref services >}}) to it.

{{< button text="Service library" page="services" >}}

For example, you can add a [container]({{< ref container >}}):

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
