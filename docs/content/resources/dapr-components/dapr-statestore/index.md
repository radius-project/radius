---
type: docs
title: "Use Dapr State Management with Radius"
linkTitle: "State"
description: "Learn how to use Dapr state management components in Radius"
weight: 100
---

## Overview

The `dapr.io/StateStore` component represents a [Dapr state store](https://docs.dapr.io/developing-applications/building-blocks/state-management/state-management-overview/) database.

This component will automatically:
- Ensure the Dapr control plane is initialized
- Deploy and manage the underlying resource
- Setup and configuration of connection strings for consuming components
- Creation and configuration of the Dapr component spec

## Configuration

| Property | Description | Example(s) |
|----------|-------------|---------|
| name | The name of the state store | `my-state-store` |
| kind | The kind and version of Radius component, in this case `dapr.io/StateStore@v1alpha1` | `dapr.io/StateStire@v1alpha1`
| properties.config.kind | The kind of the underlying state store resource. See [State Store kinds](#state-store-kinds) for more information. | `state.azure.tablestorage`
| properties.config.managed | Indicates if the resource is Radius-managed. For now only true is accepted for this Component. | `true`

To add a new managed Dapr statestore component, add the following Radius component:

```sh
resource orderstore 'Components' = {
  name: 'orderstore'
  kind: 'dapr.io/StateStore@v1alpha1'
  properties: {
    config: {
      kind: '<STATESTORE_KIND>'
      managed: true
    }
  }
```

## State Store kinds

The following resources can act as a `dapr.io/StateStore` state store:

### any (determined by platform)

The `any` kind lets the platform choose the best state store for the given platform. This provides portability across the various [Radius platforms]({{< ref platforms >}}).

{{% alert title="Warning" color="warning" %}}
The `any` kind is meant for dev/test purposes only. It is not recommended for production use.
{{% /alert %}}

| Platform | Resource |
|----------|----------|
| [Microsoft Azure]({{< ref azure>}}) | [Azure Table Storage](#azure-table-storage) |
| [Kubernetes]({{< ref kubernetes >}}) | [Redis]({{< ref redis >}})]

```sh
resource orderstore 'Components' = {
  name: 'orderstore'
  kind: 'dapr.io/StateStore@v1alpha1'
  properties: {
    config: {
      kind: 'any'
      managed: true
    }
  }
```

### Azure Table Storage

The `state.azure.tablestorage` kind represents an [Azure Table Storage](https://azure.microsoft.com/en-us/services/storage/tables/) account that is configured as a Dapr state store.

| Platform | Resource |
|----------|----------|
| [Microsoft Azure]({{< ref azure>}}) | [Azure Table Storage](https://azure.microsoft.com/en-us/services/storage/tables/)
| [Kubernetes]({{< ref kubernetes >}}) | Not supported

```sh
resource orderstore 'Components' = {
  name: 'orderstore'
  kind: 'dapr.io/StateStore@v1alpha1'
  properties: {
    config: {
      kind: 'state.azure.tablestorage'
      managed: true
    }
  }
}
```

### Azure SQL Server

The `state.sqlserver` represents an [Azure SQL Server](https://azure.microsoft.com/en-us/services/sql-database/campaign/) that is configured as a Dapr state store.

| Platform | Resource |
|----------|----------|
| [Microsoft Azure]({{< ref azure>}}) | [Azure SQL Server](https://azure.microsoft.com/en-us/services/sql-database/campaign/)
| [Kubernetes]({{< ref kubernetes >}}) | Not supported

```sh
resource pubsub 'Components' = {
  name: 'pubsub'
  kind: 'dapr.io/StateStore@v1alpha1'
  properties: {
    config: {
      kind: 'state.sqlserver'
      managed: true
    }
  }
}
```
