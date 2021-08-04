---
type: docs
title: "Use Dapr Pub/Sub with Azure ServiceBus"
linkTitle: "Azure ServiceBus"
description: "Learn how to use Dapr Pub/Sub with Azure ServiceBus"
weight: 200
---

Radius components for Dapr Pub/Sub with Azure ServiceBus offers:

- Managed deployment and management of the underlying Azure ServiceBus
- Setup and configuration of Managed Identities and RBAC for consuming components
- Creation and configuration of the Dapr component spec

## Using a Radius-managed ServiceBus topic

This example sets the property `managed: true` for the Dapr PubSub Component. When `managed` is set to true, Radius will manage the lifecycle of the underlying ServiceBus namespace and topic.

{{< rad file="snippets/managed.bicep" embed=true >}}

## Using a user-managed ServiceBus topic

This example sets the `resource` property to a ServiceBus topic for the Dapr PubSub Component. Setting `managed: false` or using the default value allows you to explicitly specify a link to an Azure resource that you managed. When you supply your own `resource` value, Radius will not change or delete the resource you provide. 

In this example the ServiceBus resources are configured as part of the same `.bicep` template.

{{< rad file="snippets/usermanaged.bicep" embed=true >}}

## Access from a container

To access the Dapr PubSub component from a container, add the following traits and dependencies:

```sh
resource nodesubscriber 'Components' = {
  name: 'nodesubscriber'
  kind: 'radius.dev/Container@v1alpha1'
  properties: {
  uses: [
    {
      binding: pubsub.properties.bindings.default
      env: {
        SB_PUBSUBNAME: pubsub.properties.bindings.default.pubSubName
        SB_TOPIC: pubsub.properties.bindings.default.topic
      }
    }
  ]
  traits: [
    {
      kind: 'dapr.io/App@v1alpha1'
      appId: 'nodesubscriber'
      appPort: 50051
    }
  ]
}
```

## Tutorial

### Pre-requisites

To begin this tutorial you should have already completed the following steps:

- [Install Radius CLI]({{< ref install-cli.md >}})
- [Create an environment]({{< ref create-environment.md >}})
- [Install VS Code extension]({{< ref setup-vscode >}})

No prior knowledge of Radius is needed, this tutorial will walk you through authoring the deployment template and deploying a microservices application from first principles.

### Download the Radius application

#### Radius-managed

{{< rad file="snippets/managed.bicep" download=true >}}

#### User-managed

{{< rad file="snippets/managed.bicep" download=true >}}

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
