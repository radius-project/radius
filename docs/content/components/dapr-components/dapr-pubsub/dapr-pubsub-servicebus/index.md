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

## Create a Dapr Pub/Sub with Azure ServiceBus

To add a new managed Dapr Pub/Sub with Azure ServiceBus, add the following Radius component:

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

## Access from a container

To access the Dapr PubSub component from a container, add the following traits and dependencies:

```sh
resource nodesubscriber 'Components' = {
  name: 'nodesubscriber'
  kind: 'radius.dev/Container@v1alpha1'
  properties: {...}
  dependsOn: [
    {
      name: 'pubsub'
      kind: 'dapr.io/PubSubTopic'
      setEnv: {
        SB_PUBSUBNAME: 'pubsubName'
        SB_TOPIC: 'topic'
      }
    }
  ]
  traits: [
    {
      kind: 'dapr.io/App@v1alpha1'
      properties: {
        appId: 'nodesubscriber'
        appPort: 50051
      }
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

```sh
resource nodesubscriber 'Components' = {
    name: 'nodesubscriber'
    kind: 'radius.dev/Container@v1alpha1'
    properties: {
      run: {
        container: {
          image: '<your docker hub>/dapr-pubsub-nodesubscriber:latest'
        }
      }
      dependsOn: [
        {
          name: 'pubsub'
          kind: 'dapr.io/PubSubTopic'
          setEnv: {
            SB_PUBSUBNAME: 'pubsubName'
            SB_TOPIC: 'topic'
          }
        }
      ]
      traits: [
        {
          kind: 'dapr.io/App@v1alpha1'
          properties: {
            appId: 'nodesubscriber'
            appPort: 50051
          }
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

```sh
resource pythonpublisher 'Components' = {
  name: 'pythonpublisher'
  kind: 'radius.dev/Container@v1alpha1'
  properties: {
    run: {
      container: {
        image: '<your docker hub>/dapr-pubsub-pythonpublisher:latest'
      }
    }
    dependsOn: [
      {
        name: 'pubsub'
        kind: 'dapr.io/PubSubTopic'
        setEnv: {
          SB_PUBSUBNAME: 'pubsubName'
          SB_TOPIC: 'topic'
        }
      }
    ]
    traits: [
      {
        kind: 'dapr.io/App@v1alpha1'
        properties: {
          appId: 'pythonpublisher'
        }
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

Now you are ready to deploy this application.

First, double-check that you are logged-in to Azure. Switch to your commandline and run the following command:

```sh
az login
```

Then after that completes, run:

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

### (optional) Cleanup

When you are ready to clean up and delete the resources you can delete your environment. This will delete:

- The resource group
- Your Radius environment
- The application you just deployed

```sh
rad env delete --name azure --yes
```
