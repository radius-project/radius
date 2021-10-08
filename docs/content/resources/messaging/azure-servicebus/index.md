---
type: docs
title: "Azure ServiceBus Queue Component"
linkTitle: "Service Bus Queue"
description: "Deploy and orchestrate Azure Service Bus Queues using Radius"
---

## Overview

The Azure ServiceBus Queue component offers to the user:

- Managed resource deployment and lifecycle of the ServiceBus Queue
- Automatic configuration of Azure Managed Identities and RBAC between consuming components and the ServiceBus
- Injection of connection information into connected containers
- Automatic secret injection for configured components

## Platform resources

| Platform                             | Resource                                                                                                               |
| ------------------------------------ | ---------------------------------------------------------------------------------------------------------------------- |
| [Microsoft Azure]({{< ref azure>}})  | [Azure Service Bus Queue](https://docs.microsoft.com/en-us/azure/service-bus-messaging/service-bus-messaging-overview) |
| [Kubernetes]({{< ref kubernetes >}}) | Not compatible                                                                                                         |

## Component format

{{< tabs Radius-managed User-managed >}}

{{% codetab %}}
{{< rad file="snippets/managed.bicep" embed=true marker="//BUS" >}}
{{% /codetab %}}

{{% codetab %}}
{{% alert title="🚧 Under construction" color="warning" %}}
User-managed resources are not yet supported. Check back soon for updates.
{{% /alert %}}
{{% /codetab %}}

{{< /tabs >}}

| Property | Description           | Example    |
| -------- | --------------------- | ---------- |
| name     | Name of the Component | `'orders'` |

### Resource lifecycle

| Property | Description                                                                                      | Example |
| -------- | ------------------------------------------------------------------------------------------------ | ------- |
| managed  | Indicates if the resource is Radius-managed. For now only `true` is accepted for this Component. | `true`  |

### Queue properties

| Property | Description           | Example    |
| -------- | --------------------- | ---------- |
| queue    | The name of the queue | `'orders'` |

## Provided data

### Functions

| Property             | Description                                                        |
| -------------------- | ------------------------------------------------------------------ |
| `connectionString()` | The Service Bus connection string used to connect to the resource. |

### Properties

| Property                    | Description                                                         |
| --------------------------- | ------------------------------------------------------------------- |
| `namespace`                 | The namespace of the Service Bus.                                   |
| `namespaceConnectionString` | The Service Bus connection string used to connect to the namespace. |
| `queue`                     | The message queue to which you are connecting.                      |
| `queueConnectionString`     | The Service Bus connection string used to connect to the queue.     |
