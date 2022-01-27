---
type: docs
title: "Use Dapr Pub/Sub with Radius"
linkTitle: "Pub/sub topic"
description: "Learn how to use Dapr Pub/Sub components in Radius"
weight: 300
slug: "pubsub"
---

## Overview

The `dapr.io/PubSubTopic` connector represents a [Dapr pub/sub](https://docs.dapr.io/developing-applications/building-blocks/pubsub/pubsub-overview/) topic.

This connector will automatically:
- Ensure the Dapr control plane is initialized
- Deploy and manage the underlying resource
- Create and deploy the Dapr component spec

## Platform resources

The following resources can act as a `dapr.io.PubSubTopic` resource:

| Platform | Resource |
|----------|----------|
| [Microsoft Azure]({{< ref azure>}}) | [Azure Service Bus](https://docs.microsoft.com/en-us/azure/service-bus-messaging/service-bus-messaging-overview)
| [Kubernetes]({{< ref kubernetes >}}) | Not yet compatible

## Resource format

{{< tabs "Radius-managed" "User-managed" >}}

{{% codetab %}}
The following example shows a fully managed Dapr Pub/Sub topic resource, where the underlying infrastructure is managed by Radius:
{{< rad file="snippets/managed.bicep" embed=true marker="//SAMPLE" >}}
{{% /codetab %}}

{{% codetab %}}
First define your Pub/Sub message broker. In this example we're using an Azure Service Bus:
{{< rad file="snippets/user-managed.bicep" embed=true marker="//BICEP" >}}
Then you can connect a Dapr Pub/Sub connector to the Bicep resource:
{{< rad file="snippets/user-managed.bicep" embed=true marker="//SAMPLE" >}}
{{% /codetab %}}

{{< /tabs >}}

| Property | Description | Example |
|----------|-------------|---------|
| name | The name of the state store | `my-pubsub` |

### Resource lifecycle

| Property | Description | Example |
|----------|-------------|---------|
| kind | The kind of the underlying pub/sub resource. See [Platform resources](#platform-resources) for more information. | `pubsub.azure.servicebus`
| managed | Indicates if the resource is Radius-managed. | `true`
| resource | Points to the user-managed resource, if used. | `namespace::topic.id`

### Pub/Sub settings

| Property | Description | Example |
|----------|-------------|---------|
| topic | The name of the topic to create for this Pub/Sub broker | `TOPIC_A`

## Starter

You can get up and running quickly with a Dapr Pub/Sub topic by using a [starter]({{< ref starter-templates >}}):

{{< rad file="snippets/starter.bicep" embed=true >}}

### Container

The Dapr Pub/Sub container starter uses a Redis container and can run on any Radius platform.

```
br:radius.azurecr.io/starters/dapr/pubsub:latest
```

#### Input parameters

| Parameter | Description | Required | Default |
|-----------|-------------|:--------:|---------|
| radiusApplication | The application resource to use as the parent of the PubSub Topic | Yes | - |
| pubSubName | The name of the PubSub Topic | No | `deployment().name` (module name) |

#### Output parameters

| Parameter | Description | Type |
|----------|-------------|------|
| pubSub | The PubSub Topic resource | `radius.dev/Application/dapr.io.PubSubTopic@v1alpha3` |

### Microsoft Azure

The Dapr Pub/Sub Azure Service Bus starter uses an Azure Service Bus Topic and can run only on Azure.

```txt
br:radius.azurecr.io/starters/dapr/pubsub-azure-servicebus:latest
```

### Input parameters

| Parameter | Description | Required | Default |
|-----------|-------------|:--------:|---------|
| radiusApplication | The application resource to use as the parent of the PubSub Topic | Yes | - |
| pubSubName | The name of the PubSub Topic | No | `deployment().name` (module name) |
| serviceBusName | The name of the underlying Azure Service Bus namespace | No | `'servicebus-${uniqueString(resourceGroup().id, deployment().name)}'` |
| queueName | The name of the underlying Azure Service Bus queue | No | `'dapr'` |
| location | The Azure region to deploy the Azure Service Bus | No | `resourceGroup().location` |

### Output parameters

| Parameter | Description | Type |
|-----------|-------------|------|
| pubSub | The PubSub Topic resource | `radius.dev/Application/dapr.io.PubSubTopic@v1alpha3` |
