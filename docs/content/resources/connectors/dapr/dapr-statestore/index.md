---
type: docs
title: "Use Dapr State Management with Radius"
linkTitle: "State store"
description: "Learn how to use Dapr state management components in Radius"
weight: 200
slug: "statestore"
---

## Overview

The `dapr.io/StateStore` connector represents a [Dapr state store](https://docs.dapr.io/developing-applications/building-blocks/state-management/state-management-overview/) database.

This connector will automatically:
- Ensure the Dapr control plane is initialized
- Deploy and manage the underlying resource
- Setup and configuration of connection strings for consuming resources
- Create and configure of the Dapr component spec

## Platform resources

| Platform | Resource | Kind |
|----------|----------|------|
| [Microsoft Azure]({{< ref azure>}}) | [Azure Table Storage](#azure-table-storage) | `'state.azure.tablestorage'`
| [Microsoft Azure]({{< ref azure>}}) | [Azure SQL](#azure-table-storage) | `'state.sqlserver'`
| [Kubernetes]({{< ref kubernetes >}}) | [Redis]({{< ref redis >}}) | `'state.redis'`

Additionally, the `any` kind will automatically choose a resource based on the platform. For Azure it is Table Storage, and for Kubernetes it is Redis.

{{% alert title="Warning" color="warning" %}}
The `any` kind is meant for dev/test purposes only. It is not recommended for production use.
{{% /alert %}}

## Resource spec

{{< tabs Managed User-managed >}}

{{% codetab %}}
In the following example a State Store connector is defined, where the underlying resource is provided by the platform.
{{< rad file="snippets/managed.bicep" embed=true marker="//SAMPLE" >}}
{{% /codetab %}}

{{% codetab %}}
{{% alert title="🚧 Under construction" color="warning" %}}
User-managed resources are not yet supported for Dapr State Stores. Check back soon for updates.
{{% /alert %}}
{{% /codetab %}}

{{< /tabs >}}

| Property | Description | Example |
|----------|-------------|---------|
| name | The name of the state store | `my-state-store` |

### Resource lifecycle

| Property | Description | Example |
|----------|-------------|---------|
| kind | The kind of the underlying state store resource. See [State Store kinds](#platform-resources) for more information. | `state.azure.tablestorage`
| managed | Indicates if the resource is Radius-managed. For now only true is accepted for this resource. | `true`

## Starter

You can get up and running quickly with a Dapr state store by using a [starter]({{< ref starter-templates >}}):

{{< rad file="snippets/starter.bicep" embed=true >}}

### Container

The Dapr StateStore container starter uses a Redis container and can run on any Radius platform.

```
br:radius.azurecr.io/starters/dapr/statestore:latest
```

#### Input parameters

| Parameter | Description | Required | Default |
|-----------|-------------|:--------:|---------|
| radiusApplication | The application resource to use as the parent of the PubSub Topic | Yes | - |
| stateStoreName | The name of the State Store | No | `deployment().name` (module name) |

#### Output parameters

| Parameter | Description | Type |
|-----------|-------------|------|
| stateStore | The StateStore resource | `radius.dev/Application/dapr.io.StateStore@v1alpha3` |

### Microsoft Azure

The Dapr StateStore Azure starter uses Azure Table Storage and can run only on Microsoft Azure.

```
br:radius.azurecr.io/starters/dapr/statestore-azure-table:latest
```

### Input parameters

| Parameter | Description | Required | Default |
|-----------|-------------|:--------:|---------|
| radiusApplication | The application resource to use as the parent of the State Store | Yes | - |
| stateStoreName | The name of the State Store | No | `depployment().name` (module name) |
| storageAccountName | The name of the underlying Azure storage account | No | `'storage-${uniqueString(resourceGroup().id, deployment().name)}'` |
| tableName | The name of the underlying Azure storage table | No | `'dapr'` |
| location | The Azure region to deploy the Azure storage account and table | No | `resourceGroup().location` |

### Output parameters

| Resource | Description | Type |
|----------|-------------|------|
| stateStore | The StateStore resource | `radius.dev/Application/dapr.io.StateStore@v1alpha3` |
