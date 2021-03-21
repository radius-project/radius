---
type: docs
title: "Azure ServiceBus Tutorial"
linkTitle: "Azure ServiceBus Tutorial"
description: "Learn Project Radius by authoring templates and deploying a working application using Azure ServiceBus"
weight: 20
---

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

You can find the source code for the sender and receiver applications [here](https://github.com/Azure/radius/tree/main/test/azure-servicebus/apps). You can make changes to this app and build a new image as follows from the respective sender/receiver app directory:-


Here is a diagram of the complete application:

<img src="https://raw.githubusercontent.com/dapr/quickstarts/v1.0.0/hello-world/img/Architecture_Diagram_B.png" alt="The complete application" width=800>

### Receiver Application

The receiver application is a simple listener that listens to an Azure ServiceBus queue named "radius-queue1" and prints out the messages received. If you wish to change the queue name, you can modify the application code and create a new image as follows:-
```
cd <Radius Path>/test/azure-servicebus/apps/servicebus-receiver
docker build -t <your docker hub>/servicebus-receiver .
docker push <your docker hub>/servicebus-receiver
```

Note: You need to reference your new image as the container image in the deployment template:-
```
instance receiver 'radius.dev/Container@v1alpha1' = {
    name: 'servicebus-receiver'
    properties: {
      run: {
        container: {
          image: '<your docker hub>/servicebus-receiver:latest'
        }
      }
    .....
```

### Sender Application

The sender application sends messages over an Azure ServiceBus queue named "radius-queue1" with a delay of 1s. If you wish to change the queue name, you can modify the application code and create a new image as follows:-
```
cd <Radius Path>/test/azure-servicebus/apps/servicebus-sender
docker build -t <your docker hub>/servicebus-sender .
docker push <your docker hub>/servicebus-sender
```

Note: You need to reference your new image as the container image in the deployment template:-
```
instance sender 'radius.dev/Container@v1alpha1' = {
    name: 'servicebus-sender'
    properties: {
      run: {
        container: {
          image: '<your docker hub>/servicebus-sender:latest'
        }
      }
    .....
```

### Azure Service Bus
Radius will create a new ServiceBus namespace if one does not already exist in the resource group and add the queue name "radius-queue1" as specified in the deployment template:-
 instance sbq 'azure.com/ServiceBusQueue@v1alpha1' = {
    name: 'sbq'
    properties: {
        config: {
            managed: true
            queue: 'radius-queue1'
        }
    }
  }

  Note: The Node.js sender and receiver applications listen on the queue name specified here ("radius-queue1"). In case you want to use a different name, the queue name should be changed here accordingly to match the application spec.


### Deploy application

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

{{% alert title="⚠️ Temporary" color="warning" %}}
Run this command at the commandline, which is temporary pending additions to the rad CLI:

```sh
rad env merge-credentials --name azure 
```

{{% /alert %}}

To see the sender and receiver applications working, you can check logs:

```sh
kubectl logs <sender pod name> -n azure-servicebus
kubectl logs <receiver pod name> -n azure-servicebus
```

You should see the sender sending messages and the receiver receiving them as below:-

```txt
Messages: Cool Message 1



Messages: Cool Message 2



Messages: Cool Message 3
```

You have completed this tutorial!

## Step 3: Cleanup (optional)

When you are ready to clean up and delete the resources you can delete your environment. This will delete:

- The resource group
- Your Radius environment
- The application you just deployed

```sh
rad env delete azure --yes
```

## Related links

- [Dapr documentation](https://docs.dapr.io/)
- [Dapr Hello World](https://github.com/dapr/quickstarts/tree/v1.0.0/hello-world)
