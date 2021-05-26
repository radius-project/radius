---
type: docs
title: "Use Dapr Pub/Sub with Radius"
linkTitle: "Pub/Sub"
description: "Learn how to use Dapr Pub/Sub in Radius"
weight: 300
---

## Create a Dapr pub/sub component

To add a new managed Dapr Pub/Sub component, add the following Radius component:

```sh
resource pubsub 'Components' = {
  name: 'pubsub'
  kind: 'dapr.io/PubSubTopic@v1alpha1'
  properties: {
    config: {
      kind: '<PUBSUB_KIND>'
      topic: 'TOPIC_A'
      managed: true
    }
  }
}
```

## Supported Dapr pub/sub kinds
