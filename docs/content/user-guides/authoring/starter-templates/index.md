---
type: docs
title: "Use pre-built templates in your application to quickly create and deploy resources"
linkTitle: "Starter templates"
description: "Learn how to quickly model and deploy Connectors in your application using Radius starters"
weight: 499
---

Starters allow you to quickly create and deploy [services]({{< ref services>}}), [connectors]({{< ref connectors >}}), and other resources in your application.

## Using a starter

A starter can be consumed as a Bicep module through a Bicep registry.

{{< rad file="snippets/mongo-starter.bicep" embed=true >}}

A MongoDB container is deployed into your application and can be referenced via `mongoDB.outputs.mongoDB`.

## Available starters

The following starter templates are available for use in your application. Visit each resource page to learn more about input and output parameters and schema.

### Connectors

| Connector | Container starter | Azure starter |
|-----------|:-----------------:|:-------------:|
| [Dapr Pub/Sub Topic]({{< ref "dapr-pubsub.md#starter" >}}) | ✅ | ✅ | 
| [Dapr State Store]({{< ref "dapr-statestore.md#starter" >}}) | ✅ | ✅ | 
| [Mongo Database]({{< ref "mongodb.md#starter" >}}) | ✅ | ✅ |
| [RabbitMQ Queue]({{< ref "rabbitmq.md#starter" >}}) | ✅ | ❌ |
| [Redis Cache]({{< ref "redis.md#starter" >}})    | ✅ | ✅ |
| [SQL Database]({{< ref "microsoft-sql.md#starter" >}})   | ✅ | ✅ |

## Source

Check back soon for updates on contributing to the starters repository!

## Create your own templates

Want to create your own starter templates and distribute via a Bicep registry? Check out [this guide]({{< ref bicep-templates >}}) to learn more.