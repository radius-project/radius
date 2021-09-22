---
type: docs
title: "RabbitMQ message broker component"
linkTitle: "RabbitMQ"
description: "Learn how to use a RabbitMQ component in your application"
---

The `rabbitmq.com/MessageQueue` component is a Kubernetes specific component for message brokering.

## Platform resources

| Platform | Resource |
|----------|----------|
| [Microsoft Azure]({{< ref azure>}}) | Not compatible |
| [Kubernetes]({{< ref kubernetes >}}) | [RabbitMQ](https://hub.docker.com/_/rabbitmq/) service |

## Configuration

| Property | Description | Example(s) |
|----------|-------------|---------|
| managed | Indicates if the resource is Radius-managed. For now only true is accepted for this Component.| `true`
| queue | The name of the queue

## Resource lifecycle

A `rabbitmq.com/MessageQueue` component can be Radius-managed. For more information read the [Components docs]({{< ref "components-model#resource-lifecycle" >}}).

### Radius managed

{{< rad file="snippets/managed.bicep" embed=true marker="//SAMPLE" >}}

### User managed

{{% alert title="Warning" color="warning" %}}
Currently user-managed RabbitMQ components are not supported.
{{% /alert %}}

## Bindings

### rabbitmq

The `default` Binding of kind `rabbitmq.com/MessageQueue` represents the the RabbitMQ resource, and all APIs it offers.

| Property | Description | Example(s) |
|----------|-------------|------------|
| `connectionString` | The RabbitMQ connection string used to connect to the resource. | amqp://rabbitmq:5672/ |
| `queue` | The message queue to which you are connecting.
