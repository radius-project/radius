---
type: docs
title: "Use Dapr Pub/Sub with Radius"
linkTitle: "Pub/sub topic"
description: "Learn how to use Dapr Pub/Sub components in Radius"
weight: 300
slug: "pubsub"
---

## Overview

The `dapr.io/PubSubTopic` component represents a [Dapr pub/sub](https://docs.dapr.io/developing-applications/building-blocks/pubsub/pubsub-overview/) topic.

This component will automatically:
- Ensure the Dapr control plane is initialized
- Deploy and manage the underlying resource
- Create and deploy the Dapr component spec

## Platform resources

The following resources can act as a `dapr.io.PubSubTopic` resource:

| Platform | Resource |
|----------|----------|
| [Microsoft Azure]({{< ref azure>}}) | [Azure Service Bus](https://docs.microsoft.com/en-us/azure/service-bus-messaging/service-bus-messaging-overview)
| [Microsoft Azure]({{< ref azure>}}) | Generic
| [Kubernetes]({{< ref kubernetes >}}) | Generic