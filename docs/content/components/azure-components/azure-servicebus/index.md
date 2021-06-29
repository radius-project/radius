---
type: docs
title: "Azure ServiceBus Component"
linkTitle: "ServiceBus"
description: "Deploy and orchestrate Azure KeyVault using Radius"
---

## Overview

The Azure ServiceBus component offers to the user:

- Managed resource deployment and lifecycle of the ServiceBus Queue
- Automatic configuration of Azure Managed Identities and RBAC between consuming components and the ServiceBus
- Injection of connection information into connected containers
- Automatic secret injection for configured components

## Configuration

| Property | Description | Example(s) |
|----------|-------------|---------|
| managed | Indicates if the resource is Radius-managed. If no, a `Resource` must be specified. | `true`, `false`
| queue | The name of the queue=
| resource | The ID of the user-managed CosmosDB with Mongo API to use for this Component. | `account::mongodb.id`

## Bindings

### default

The `default` Binding of kind `azure.com/ServiceBusQueue` represents the the Service Bus resource, and all APIs it offers.

| Property | Description |
|----------|-------------|
| `connectionString` | The Service Bus connection string used to connect to the resource.
| `namespace` | The namespace of the Service Bus.
| `queue` | The message queue to which you are connecting.

### Example

A ServiceBus Queue resource can be modeled with the `azure.com/ServiceBusQueue@v1alpha1` kind:

{{< rad file="snippets/azure-servicebus-managed.bicep" embed=true marker="//SAMPLE" replace-key-hide="//HIDE" replace-value-hide="run: {...}">}}

## Tutorial

### Pre-requisites

To begin this tutorial you should have already completed the following steps:

- [Install Radius CLI]({{< ref install-cli.md >}})
- [Create an environment]({{< ref create-environment.md >}})
- (optional) [Install Radius VSCode extension]({{< ref setup-vscode >}})

### Understand the application

The application you will be deploying is a simple sender-receiver application using Azure ServiceBus queue for communication between the send and receiver. It has three components:

- A sender written in Node.js
- A receiver written in Node.js
- An Azure ServiceBus queue

#### Azure Service Bus

Radius will create a new ServiceBus namespace if one does not already exist in the resource group and add the queue name `radius-queue1` as specified in the deployment template below. If you change the queue name, it is automatically injected into the sender/receiver app containers and they start sending/listening on the new queue accoridingly.

{{< rad file="snippets/azure-servicebus-managed.bicep" embed=true marker="//BUS" >}}

#### Receiver application

The receiver application is a simple listener that listens to an Azure ServiceBus queue named `radius-queue1` and prints out the messages received:

{{< rad file="snippets/azure-servicebus-managed.bicep" embed=true marker="//RECEIVER" >}}

#### Sender application

The sender application sends messages over an Azure ServiceBus queue named `radius-queue1` with a delay of 1s:

{{< rad file="snippets/azure-servicebus-managed.bicep" embed=true marker="//SENDER" >}}

### Deploy application

#### Download Bicep file

{{< rad file="snippets/azure-servicebus-managed.bicep" download=true >}}

Alternately, you can create a new file named `azure-servicebus-managed.bicep` and paste the above components into an `app` resource.  

#### Deploy template file

Submit the Radius template to Azure using:

```sh
rad deploy azure-servicebus-managed.bicep
```

This will deploy the application, create the ServiceBus queue, and launch the containers.

### Access the application

To see the sender and receiver working, you can check the logs for those two components of the "radius-servicebus" application:

```sh
rad component logs sender --application radius-servicebus 
```

```sh
rad component logs receiver --application radius-servicebus 
```

You should see the sender sending messages and the receiver receiving them as below:

```
Messages: Cool Message 1

Messages: Cool Message 2

Messages: Cool Message 3
```

You have completed this tutorial!

{{% alert title="Cleanup" color="warning" %}}
If you're done with testing, you can use the rad CLI to [delete an environment]({{< ref rad_env_delete.md >}}) to **prevent additional charges in your subscription**.
{{% /alert %}}
