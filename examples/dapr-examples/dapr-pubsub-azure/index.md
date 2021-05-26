---
type: docs
title: "Dapr PubSub component using Azure ServiceBus"
linkTitle: "Dapr PubSub using Azure ServiceBus"
description: "Sample application running with Dapr PubSub on Azure ServiceBus"
weight: 50
---

## Using a Radius-managed ServiceBus topic

This example sets the property `managed: true` for the Dapr PubSub Component. When `managed` is set to true, Radius will manage the lifecycle of the underlying ServiceBus namespace and topic.

{{< rad file="managed.bicep">}}

## Using a user-managed ServiceBus topic

This example sets the `resource` property to a ServiceBus topic for the Dapr PubSub Component. Setting `managed: false` or using the default value allows you to explicitly specify a link to an Azure resource that you managed. When you supply your own `resource` value, Radius will not change or delete the resource you provide. 

In this example the ServiceBus resources are configured as part of the same `.bicep` template.

{{< rad file="unmanaged.bicep">}}