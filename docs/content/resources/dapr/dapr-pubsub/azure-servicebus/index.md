---
type: docs
title: "Azure Service Bus Dapr Pub/Sub Component"
linkTitle: "Azure Service Bus"
description: "Learn how to use Azure Service Bus Dapr Pub/Sub components in Radius"
weight: 400
slug: "pubsub"
---

This section shows how to use an [Azure Service Bus](https://docs.microsoft.com/en-us/azure/service-bus-messaging/service-bus-messaging-overview) Dapr Pub/Sub component in a Radius Application.

## Component format

{{< tabs "Radius-managed" "User-managed" >}}

{{% codetab %}}
The following example shows a fully managed Dapr Pub/Sub topic Component, where the underlying infrastructure is managed by Radius:
{{< rad file="snippets/managed.bicep" embed=true marker="//SAMPLE" >}}
{{% /codetab %}}

{{% codetab %}}
First define your Pub/Sub message broker. In this example we're using an Azure Service Bus:
{{< rad file="snippets/user-managed.bicep" embed=true marker="//BICEP" >}}
Then you can connect a Dapr Pub/Sub Component to the Bicep resource:
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
