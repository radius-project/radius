---
type: docs
title: "Use Dapr state management with Azure SQL Server"
linkTitle: "Azure SQL"
description: "Learn how to use Dapr state management with Azure SQL Server and Radius"
---

## Dapr state management with Azure SQL Server

Radius components for Dapr state management with [Azure SQL Server](https://azure.microsoft.com/en-us/services/sql-database/campaign/) offers:

- Managed deployment and management of the underlying server
- Setup and configuration of connection strings for consuming components
- Creation and configuration of the Dapr component spec

## Create a Dapr state store with Azure SQL Server

To add a new managed Dapr state store with Azure SQL Server, add the following Radius component:

```sh
resource pubsub 'Components' = {
  name: 'pubsub'
  kind: 'dapr.io/State@v1alpha1'
  properties: {
    config: {
      kind: 'state.sqlserver'
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
  uses: [
    {
      binding: statestore.properties.bindings.default
    }
  ]
  traits: [
    {
      kind: 'dapr.io/App@v1alpha1'
      appId: 'nodeapp'
      appPort: 50051
    }
  ]
}
```
