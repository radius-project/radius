---
type: docs
title: "Use Dapr Pub/Sub with Radius"
linkTitle: "Pub/Sub"
description: "Learn how to use Dapr Pub/Sub components in Radius"
weight: 300
---

## Overview

The `dapr.io/PubSubTopic` component represents a [Dapr pub/sub](https://docs.dapr.io/developing-applications/building-blocks/pubsub/pubsub-overview/) topic.

This component will automatically:
- Ensure the Dapr control plane is initialized
- Deploy and manage the underlying resource
- Setup and configuration of connection strings for consuming components
- Creation and configuration of the Dapr component spec

## Configuration

| Property | Description | Example(s) |
|----------|-------------|---------|
| name | The name of the state store | `my-pubsub` |
| kind | The kind and version of Radius component, in this case `dapr.io/PubSubTopic@v1alpha1` | `dapr.io/PubSubTopic@v1alpha1`
| properties.config.kind | The kind of the underlying pub/sub resource. See [Pub/Sub  kinds](#pubsub-kinds) for more information. | `pubsub.azure.servicebus`
| properties.config.managed | Indicates if the resource is Radius-managed. | `true`
| properties.config.resource | Points to the user-managed resource, if used. | `namespace::topic.id`
| properties.config.topic | The name of the topic to create for this Pub/Sub broker | `TOPIC_A`

To add a new managed Dapr Pub/Sub component, add the following Radius component:

```sh
resource pubsub 'Components' = {
  name: 'pubsub'
  kind: 'dapr.io/PubSubTopic@v1alpha1'
  properties: {
    config: {
      kind: '<PUBSUB_KIND>'
      topic: 'TOPIC_A'
      managed: true
    }
  }
}
```

## Pub/Sub kinds

The following resources can act as a `dapr.io/PubSubTopic` resource:

### Azure Service Bus

The `pubsub.azure.servicebus` kind represents an [Azure Service Bus](https://docs.microsoft.com/en-us/azure/service-bus-messaging/service-bus-messaging-overview) account that is configured as a Dapr pubsub broker.

| Platform | Resource |
|----------|----------|
| [Microsoft Azure]({{< ref azure-environments >}}) | [Azure Service Bus](https://docs.microsoft.com/en-us/azure/service-bus-messaging/service-bus-messaging-overview)
| [Kubernetes]({{< ref kubernetes-environments >}}) | Not compatible

#### Using a Radius-managed ServiceBus topic

This example sets the property `managed: true` for the Dapr PubSub Component. When `managed` is set to true, Radius will manage the lifecycle of the underlying ServiceBus namespace and topic.

{{< rad file="snippets/servicebus-managed.bicep" download=true >}}

#### Using a user-managed ServiceBus topic

This example sets the `resource` property to a ServiceBus topic for the Dapr PubSub Component. Setting `managed: false` or using the default value allows you to explicitly specify a link to an Azure resource that you managed. When you supply your own `resource` value, Radius will not change or delete the resource you provide. 

In this example the ServiceBus resources are configured as part of the same `.bicep` template.

{{< rad file="snippets/servicebus-usermanaged.bicep" download=true >}}

## Tutorial

### Pre-requisites

To begin this tutorial you should have already completed the following steps:

- [Install Radius CLI]({{< ref install-cli.md >}})
- [Create an environment]({{< ref create-environment.md >}})
- [Install VS Code extension]({{< ref setup-vscode >}})

No prior knowledge of Radius is needed, this tutorial will walk you through authoring the deployment template and deploying a microservices application from first principles.

### Download the Radius application

#### Radius-managed

{{< rad file="snippets/servicebus-managed.bicep" download=true >}}

#### User-managed

{{< rad file="snippets/servicebus-managed.bicep" download=true >}}

### Understanding the application

The application you will be deploying is a simple publisher-subscriber application using Dapr PubSub over Azure ServiceBus topics for communication. It has three components:

1. A publisher written in Python
1. A subscriber written in Node.js
1. A Dapr PubSub component that uses Azure Service Bus

#### Subscriber application

The subscriber application listens to a pubsub component named "pubsub" and an Azure ServiceBus topic named "TOPIC_A" and prints out the messages received. 

The environment variables `SB_PUBSUBNAME` and `SB_TOPIC` are injected into the container by Radius. These correspond to the pubsub name and topic name specified in the Dapr PubSub component spec

#### Publisher application

The publisher application sends messages to a pubsub component named "pubsub" and an Azure ServiceBus topic named "TOPIC_A".

The environment variables `SB_PUBSUBNAME` and `SB_TOPIC` are injected into the container by Radius. These correspond to the pubsub name and topic name specified in the Dapr PubSub component spec

#### Dapr pubsub component

Radius will create a new ServiceBus namespace if one does not already exist in the resource group and add the topic name "TOPIC_A" as specified in the deployment template below.

```sh
resource pubsub 'Components' = {
  name: 'pubsub'
  kind: 'dapr.io/PubSubTopic@v1alpha1'
  properties: {
    config: {
      kind: 'pubsub.azure.servicebus'
      topic: 'TOPIC_A'
      managed: true
    }
  }
}
```

### Deploy the application

#### Deploy template file

Submit the Radius template to Azure using:

```sh
rad deploy template.bicep
```

This will deploy the application, create the ServiceBus queue and launch the containers.

To see the publisher and subscriber components working, you can check their logs.

{{< tabs Publisher Subscriber >}}

{{% codetab %}}
```sh
rad component logs pythonpublisher --application dapr-pubsub 
```

You should see the publisher sending messages:

```
{'id': 1, 'message': 'hello world'}
{'id': 2, 'message': 'hello world'}
{'id': 3, 'message': 'hello world'}
{'id': 4, 'message': 'hello world'}
{'id': 5, 'message': 'hello world'}
```

{{% /codetab %}}

{{% codetab %}}
```sh
rad component logs nodesubscriber --application dapr-pubsub 
```

You should see the subscriber receiving messages from the publisher:

```
TOPIC_A :  hello world
TOPIC_A :  hello world
TOPIC_A :  hello world
TOPIC_A :  hello world
TOPIC_A :  hello world
```
{{% /codetab %}}

{{< /tabs >}}

You have completed this tutorial!

Note: If you're done with testing, you can use the rad CLI to [delete an environment]({{< ref rad_env_delete.md >}}) to **prevent additional charges in your subscription**. 
