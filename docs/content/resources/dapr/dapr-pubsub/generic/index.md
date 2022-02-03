---
type: docs
title: "Generic Dapr Pub/Sub Component"
linkTitle: "Generic"
description: "Learn how to use a generic Dapr Pub/Sub components in Radius"
weight: 400
slug: "pubsub"
---

Radius supports a generic Dapr Pub/Sub component kind which allows you to deploy a Dapr pub/sub component of a type for which there is no first class support within Radius.

## Component format

The following example shows a generic Dapr Pub/Sub topic Component of type Apache Kafka.
{{< rad file="snippets/kafka.bicep" embed=true marker="//SAMPLE" >}}

| Property | Description | Example |
|----------|-------------|---------|
| name | The name of the state store | `my-pubsub` |


### Pub/Sub settings

| Property | Description | Example |
|----------|-------------|---------|
| kind | The kind of Dapr Pub/Sub component | `generic` |
| type | The type of Dapr Pub/Sub component as mentioned in Dapr component specs | `pubsub.kafka`
| metadata | The metadata key/value pairs as mentioned in Dapr component specs | `authRequired: false`
| version | The Dapr Pub/Sub component version | `v1` |

Note that the type, metadata and version should match the [Dapr documentation](https://docs.dapr.io/reference/components-reference/) and Radius will be unable to perform validation for these values.