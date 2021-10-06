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

## Component format

{{< rad file="snippets/managed.bicep" embed=true marker="//SAMPLE" >}}

### Resource lifecycle

A `rabbitmq.com/MessageQueue` component can be Radius-managed. For more information read the [Components docs]({{< ref "components-model#resource-lifecycle" >}}).

| Property | Description | Example(s) |
|----------|-------------|---------|
| managed | Indicates if the resource is Radius-managed. For now only true is accepted for this Component.| `true`

## Queue information

| Property | Description | Example(s) |
|----------|-------------|---------|
| queue | The name of the queue. | `'orders'` |

## Provided data

### Functions

| Property | Description | Example |
|----------|-------------|---------|
| `connectionString()` | The RabbitMQ connection string used to connect to the resource. | amqp://rabbitmq:5672/ |

### Properties

| Property | Description | Example |
|----------|-------------|---------|
| `queue` | The message queue to which you are connecting. | `'orders'`
