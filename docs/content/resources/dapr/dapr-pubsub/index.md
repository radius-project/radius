---
type: docs
title: "Use Dapr Pub/Sub with Radius"
linkTitle: "Pub/sub topic"
description: "Learn how to use Dapr Pub/Sub components in Radius"
weight: 300
slug: "pubsub"
---

## Overview

The `dapr.io/PubSubTopic` component represents a [Dapr pub/sub](https://docs.dapr.io/developing-applications/building-blocks/pubsub/pubsub-overview/) topic.

This component will automatically:
- Ensure the Dapr control plane is initialized
- Deploy and manage the underlying resource
- Create and deploy the Dapr component spec

## Platform resources

The following resources can act as a `dapr.io.PubSubTopic` resource:

| Platform | Resource |
|----------|----------|
| [Microsoft Azure]({{< ref azure>}}) | [Azure Service Bus](https://docs.microsoft.com/en-us/azure/service-bus-messaging/service-bus-messaging-overview)
| [Kubernetes]({{< ref kubernetes >}}) | Not yet compatible

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
