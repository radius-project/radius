---
type: docs
title: "Use Dapr Pub/Sub over Azure ServiceBus with Radius"
linkTitle: "Azure ServiceBus"
description: "Learn how to use Dapr Pub/Sub with Azure ServiceBus and Radius"
weight: 200
---

## Dapr Pub/Sub with Azure ServiceBus

Radius components for Dapr Pub/Sub with Azure ServiceBus offers:

- Managed deployment and management of the underlying Azure ServiceBus
- Setup and configuration of Managed Identities and RBAC for consuming components
- Creation and configuration of the Dapr component spec

## Using a Radius-managed ServiceBus topic

This example sets the property `managed: true` for the Dapr PubSub Component. When `managed` is set to true, Radius will manage the lifecycle of the underlying ServiceBus namespace and topic.

{{< rad file="managed.bicep">}}

## Using a user-managed ServiceBus topic

This example sets the `resource` property to a ServiceBus topic for the Dapr PubSub Component. Setting `managed: false` or using the default value allows you to explicitly specify a link to an Azure resource that you managed. When you supply your own `resource` value, Radius will not change or delete the resource you provide. 

In this example the ServiceBus resources are configured as part of the same `.bicep` template.

{{< rad file="unmanaged.bicep">}}

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
- [Install Kubectl](https://kubernetes.io/docs/tasks/tools/)

No prior knowledge of Radius is needed, this tutorial will walk you through authoring the deployment template and deploying a microservices application from first principles.

If you are using Visual Studio Code with the Project Radius extension you should see syntax highlighting. If you have the offical Bicep extension installed, you should disable it for this tutorial. The instructions will refer to VS Code features like syntax highlighting and the problems windows - however, you can complete this tutorial with just a basic text editor.

### Understanding the application

The application you will be deploying is a simple publisher-subscriber application using Dapr PubSub over Azure ServiceBus topics for communication. It has three components:

- A publisher written in Python
- A subscriber written in Node.js
- A Dapr PubSub component that uses Azure Service Bus

You can find the source code for the sender and receiver applications [here](https://github.com/Azure/radius/tree/main/examples/dapr-examples/dapr-pubsub-azure/apps).

#### Subscriber application

The subscriber application listens to a pubsub component named "pubsub" and an Azure ServiceBus topic named "TOPIC_A" and prints out the messages received. If you wish to modify the application code, you can do so and create a new image as follows:-

```bash
cd <Radius Path>/test/dapr-pubsub-azure/apps/nodesubscriber
docker build . -t <your docker hub>/dapr-pubsub-nodesubscriber:latest
docker push <your docker hub>/dapr-pubsub-nodesubscriber:latest
```

Note: You need to reference your new image as the container image in the deployment template:-
```
  resource nodesubscriber 'Components' = {
    name: 'nodesubscriber'
    kind: 'radius.dev/Container@v1alpha1'
    properties: {
      run: {
        container: {
          image: '<your docker hub>/dapr-pubsub-nodesubscriber:latest'
        }
      }
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
  }
```

The environment variables `SB_PUBSUBNAME` and `SB_TOPIC` are injected into the container by Radius. These correspond to the pubsub name and topic name specified in the Dapr PubSub component spec

#### Publisher application

The publisher application sends messages to a pubsub component named "pubsub" and an Azure ServiceBus topic named "TOPIC_A". If you wish to modify the application code, you can do so and create a new image as follows:-

```bash
cd <Radius Path>/test/dapr-pubsub-azure/apps/pythonpublisher
docker build . -t <your docker hub>/dapr-pubsub-pythonpublisher:latest
docker push <your docker hub>/dapr-pubsub-pythonpublisher:latest
```

Note: You need to reference your new image as the container image in the deployment template:-
```
  resource pythonpublisher 'Components' = {
    name: 'pythonpublisher'
    kind: 'radius.dev/Container@v1alpha1'
    properties: {
      run: {
        container: {
          image: '<your docker hub>/dapr-pubsub-pythonpublisher:latest'
        }
      }
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
          appId: 'pythonpublisher'
        }
      ]
    }
  }
```

The environment variables `SB_PUBSUBNAME` and `SB_TOPIC` are injected into the container by Radius. These correspond to the pubsub name and topic name specified in the Dapr PubSub component spec

#### Dapr pubsub component

Radius will create a new ServiceBus namespace if one does not already exist in the resource group and add the topic name "TOPIC_A" as specified in the deployment template below.

{{% alert title="Note" color="warning" %}}
Note that the name 'pubsub' used below should match the names used by the publisher and sender applications:-
{{% /alert %}}

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

#### Pre-requisites

- Make sure you have an active [Radius environment]({{< ref create-environment.md >}})
- Ensure you are logged into Azure using `az login`

#### Deploy template file

Submit the Radius template to Azure using:

```sh
rad deploy template.bicep
```

This will deploy the application, create the ServiceBus queue and launch the containers.

To see the publisher and subscriber applications working, you can check logs:

```sh
rad logs dapr-pubsub pythonpublisher
rad logs dapr-pubsub nodesubscriber
```

You should see the publisher sending messages and the subscriber receiving them as below:-

```txt
TOPIC_A :  hello world
TOPIC_A :  hello world
TOPIC_A :  hello world
TOPIC_A :  hello world
TOPIC_A :  hello world
TOPIC_A :  hello world
TOPIC_A :  hello world
TOPIC_A :  hello world
```

You have completed this tutorial!

Note: If you're done with testing, you can use the rad CLI to [delete an environment]({{< ref rad_env_delete.md >}}) to **prevent additional charges in your subscription**. 
