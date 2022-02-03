---
type: docs
title: "RabbitMQ message broker connector"
linkTitle: "RabbitMQ"
description: "Learn how to use a RabbitMQ connector in your application"
---

The `rabbitmq.com/MessageQueue` connector is a Kubernetes specific connector for message brokering.

## Platform resources

| Platform | Resource |
|----------|----------|
| [Microsoft Azure]({{< ref azure>}}) | Not compatible |
| [Kubernetes]({{< ref kubernetes >}}) | [RabbitMQ](https://hub.docker.com/_/rabbitmq/) service |

## Resource format

{{< rad file="snippets/managed.bicep" embed=true marker="//SAMPLE" >}}

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
