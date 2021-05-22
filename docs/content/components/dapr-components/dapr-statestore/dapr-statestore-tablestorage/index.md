---
type: docs
title: "Use Dapr state management with Azure Table Storage"
linkTitle: "Azure Table Storage"
description: "Learn how to use Dapr state management with Azure Table Storage and Radius"
---

## Dapr state management with Azure Table Storage

Radius components for Dapr state management with Azure Table Storage offers:

- Managed deployment and management of the underlying Azure Storage Account
- Setup and configuration of Managed Identities and RBAC for consuming components
- Creation and configuration of the Dapr component spec

## Create a Dapr state store with Azure Table Storage

To add a new managed Dapr state store with Azure Table Storage, add the following Radius component:

```sh
resource pubsub 'Components' = {
  name: 'pubsub'
  kind: 'dapr.io/State@v1alpha1'
  properties: {
    config: {
      kind: 'state.azure.tablestorage'
      managed: true
    }
  }
}
```

## Access from a container

To access the Dapr state store component from a container, add the following traits and dependencies:

```sh
resource nodeapp 'Components' = {
  name: 'nodeapp'
  kind: 'radius.dev/Container@v1alpha1'
  properties: {...}
  dependsOn: [
    {
      name: 'statestore'
      kind: 'dapr.io/State'
    }
  ]
  traits: [
    {
      kind: 'dapr.io/App@v1alpha1'
      properties: {
        appId: 'nodeapp'
        appPort: 50051
      }
    }
  ]
}
```
