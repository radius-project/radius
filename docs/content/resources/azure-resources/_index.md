---
type: docs
title: "Add Azure resources to your Radius application"
linkTitle: "Microsoft Azure"
description: "Learn how to model and deploy Azure resources as part of your application"
weight: 400
---

Radius applications are able to connect to and leverage every Azure resource with Bicep. Simply model your Azure resources in Bicep and add a connection from your Radius resources.

## Resource library

Visit [the Microsoft docs](https://docs.microsoft.com/azure/templates/) to see every Azure resource and how to represent it in Bicep.

{{< button text="Azure resource library" link="https://docs.microsoft.com/azure/templates/" >}}

## Connections

To connect to an Azure resource from a Radius resource, use the `azure` connection kind.

### Properties

| Property | Description | Required | Default | Example |
|----------|-------------|:--------:|---------|---------|
| kind | The kind of connection. Set to 'azure' for connections to Azure resources. | Y | - | `'azure'` |
| source | The ID of the resource to connect to. | Y | - | `cache.id` |
| roles | List of [Azure Active Directory (AAD) role-based access control (RBAC) definitions](https://docs.microsoft.com/azure/role-based-access-control/built-in-roles) to assign on the Azure resource from the connecting resource. | N | Blank (no assignment) | `['Redis Cache Contributor']` |

### Example

In the following example, a [Container]({{< ref container >}}) is connecting to an Azure Cache for Redis resource. The Container is assigned the `Redis Cache Contributor` role:

{{< rad file="snippets/azure-connection.bicep" embed=true >}}
