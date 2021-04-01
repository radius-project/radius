---
type: docs
title: "Azure ServiceBus application"
linkTitle: "Azure ServiceBus"
description: "Sample application that deploys Azure ServiceBus"
weight: 20
---

This application showcases how Radius can deploy Azure ServiceBus.

## Prerequisites

To begin this tutorial you should have already completed the following steps:

- [Install Radius CLI]({{< ref install-cli.md >}})
- [Create an environment]({{< ref create-environment.md >}})
- [Install Kubectl](https://kubernetes.io/docs/tasks/tools/)

No prior knowledge of Radius is needed, this tutorial will walk you through authoring the deployment template and deploying a microservices application from first principles.

If you are using Visual Studio Code with the Project Radius extension you should see syntax highlighting. If you have the offical Bicep extension installed, you should disable it for this tutorial. The instructions will refer to VS Code features like syntax highlighting and the problems windows - however, you can complete this tutorial with just a basic text editor.

## Understanding the application

The application you will be deploying is a simple sender-receiver application using Azure ServiceBus queue for communication between the send and receiver. It has three components:

- A sender written in Node.js
- A receiver written in Node.js
- An Azure ServiceBus queue 

You can find the source code for the sender and receiver applications at the [here](https://github.com/Azure/radius/tree/main/examples/azure-examples/azure-servicebus/apps).

### Receiver application

The receiver application is a simple listener that listens to an Azure ServiceBus queue named `radius-queue1` and prints out the messages received:

```
resource receiver 'Components' = {
    name: 'receiver'
    kind: 'radius.dev/Container@v1alpha1'
    properties: {
      run: {
        container: {
          image: 'vinayada/servicebus-receiver:latest'
        }
      }
      dependsOn: [
        {
          name: 'sbq'
          kind: 'azure.com/ServiceBusQueue'
          setEnv: {
            SB_CONNECTION: 'connectionString'
            SB_NAMESPACE: 'namespace'
            SB_QUEUE: 'queue'
          }
        }
      ]
    }
  }
```

#### (optional) Updating the application

If you wish to modify the application code, you can do so and create a new image as follows:

```bash
cd <Radius Path>/examples/azure-examples/azure-servicebus/apps/servicebus-receiver
docker build -t <your docker hub>/servicebus-receiver .
docker push <your docker hub>/servicebus-receiver
```

Make sure to update the container images in the receiver resource of your deployment template if you create your own image.

### Sender application

The sender application sends messages over an Azure ServiceBus queue named `radius-queue1` with a delay of 1s:

```
resource sender 'Components' = {
    name: 'sender'
    kind: 'radius.dev/Container@v1alpha1'
    properties: {
      run: {
        container: {
          image: 'vinayada/servicebus-sender:latest'
        }
      }
      dependsOn: [
        {
          name: 'sbq'
          kind: 'azure.com/ServiceBusQueue'
          setEnv: {
            SB_CONNECTION: 'connectionString'
            SB_NAMESPACE: 'namespace'
            SB_QUEUE: 'queue'
          }
        }
      ]
    }
  }
```

#### (optional) Updating the application

If you wish to modify the application code, you can do so and create a new image as follows:

```bash
cd <Radius Path>/examples/azure-examples/azure-servicebus/apps/servicebus-sender
docker build -t <your docker hub>/servicebus-sender .
docker push <your docker hub>/servicebus-sender
```

Make sure to update the container images in the sender resource of your deployment template if you create your own image.

### Azure Service Bus

Radius will create a new ServiceBus namespace if one does not already exist in the resource group and add the queue name `radius-queue1` as specified in the deployment template below. If you change the queue name, it is automatically injected into the sender/receiver app containers and they start sending/listening on the new queue accoridingly.

```
resource sbq 'Components' = {
  name: 'sbq'
  kind: 'azure.com/ServiceBusQueue@v1alpha1'
  properties: {
      config: {
          managed: true
          queue: 'radius-queue1'
      }
  }
}
```

## Deploy application

### Pre-requisites

- Make sure you have an active [Radius environment]({{< ref create-environment.md >}}).
- Ensure you are logged into Azure using `az login`

### Download Bicep file

Begin by creating a file named `template.bicep` and pasting the above components. Alternately you can download it [below](#bicep-file).

### Deploy template file

Submit the Radius template to Azure using:

```sh
rad deploy template.bicep
```

This will deploy the application, create the ServiceBus queue, and launch the containers.

### Access the application

{{% alert title="⚠️ Temporary" color="warning" %}}
To gain access to the application now that it is deployed, make sure to merge the underlying AKS cluster into your Kubectl config:
```sh
rad env merge-credentials --name azure 
```
This step will eventually be removed.
{{% /alert %}}

To see the sender and receiver applications working, you can check logs:

```sh
kubectl logs <sender pod name> -n azure-servicebus
kubectl logs <receiver pod name> -n azure-servicebus
```

You should see the sender sending messages and the receiver receiving them as below:

```
Messages: Cool Message 1

Messages: Cool Message 2

Messages: Cool Message 3
```

You have completed this tutorial!

## Cleanup (optional)

When you are ready to clean up and delete the resources you can delete your environment. This will delete:

- The resource group
- Your Radius environment
- The application you just deployed

```sh
rad env delete azure --yes
```


## Bicep file

{{< rad file="azure-bicep/template.bicep">}}