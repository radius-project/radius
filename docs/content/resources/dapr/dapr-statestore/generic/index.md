---
type: docs
title: "Generic Dapr State Store Component"
linkTitle: "Generic"
description: "Learn how to use a generic Dapr State Store components in Radius"
weight: 410
slug: "statestore"
---

Radius supports a generic Dapr State Store component kind which allows you to deploy a Dapr State Store component of a type for which there is no first class support within Radius.

## Component format

The following example shows a generic Dapr Pub/Sub topic Component of type Apache Kafka.
{{< rad file="snippets/kafka.bicep" embed=true marker="//SAMPLE" >}}

| Property | Description | Example |
|----------|-------------|---------|
| name | The name of the state store | `my-statestore` |


### Pub/Sub settings

| Property | Description | Example |
|----------|-------------|---------|
| kind | The kind of Dapr State Store component | `generic` |
| type | The type of Dapr State Store component as mentioned in Dapr component specs | `state.zookeeper`
| metadata | The metadata key/value pairs as mentioned in Dapr component specs | `servers: zookeeper.default.svc.cluster.local:2181`
| version | The Dapr State Store component version | `v1` |

Note that the type, metadata and version should match the [Dapr documentation](https://docs.dapr.io/reference/components-reference/) and Radius will be unable to perform validation for these values. The user is responsible for deploying the infra pieces for the corresponding state store e.g. the user needs to install Zookeeper to the cluster for using the Zookeeper state store.