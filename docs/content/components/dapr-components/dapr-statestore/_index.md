---
type: docs
title: "Use Dapr State Management with Radius"
linkTitle: "State management"
description: "Learn how to use Dapr state management components in Radius"
weight: 100
---

## Create a Dapr statestore component

To add a new managed Dapr statestore component, add the following Radius component:

```sh
resource orderstore 'Components' = {
  name: 'orderstore'
  kind: 'dapr.io/Component@v1alpha1'
  properties: {
    config: {
      kind: '<STATESTORE_KIND>'
      managed: true
    }
  }
```

## Supported Dapr state management kinds
