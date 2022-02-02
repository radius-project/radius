---
type: docs
title: "SQL Server Dapr State Store Component"
linkTitle: "SQL Server"
description: "Learn how to use SQL Server Dapr State Store components in Radius"
weight: 420
slug: "statestore"
---

This section shows how to use a SQL Server Dapr State Store component in a Radius Application.

## Component format

{{< tabs "Radius-managed" "User-managed" >}}

{{% codetab %}}
The following example shows a fully managed Dapr State Store Component, where the underlying infrastructure is managed by Radius:
{{< rad file="snippets/managed.bicep" embed=true marker="//SAMPLE" >}}
{{% /codetab %}}

{{% codetab %}}
First define your State Store resource. In this example we're using a SQL Server:
{{< rad file="snippets/user-managed.bicep" embed=true marker="//BICEP" >}}
Then you can connect a Dapr State Store Component to the Bicep resource:
{{< rad file="snippets/user-managed.bicep" embed=true marker="//SAMPLE" >}}
{{% /codetab %}}

{{< /tabs >}}

| Property | Description | Example |
|----------|-------------|---------|
| name | The name of the state store | `my-statestore` |

### Resource lifecycle

| Property | Description | Example |
|----------|-------------|---------|
| kind | The kind of the underlying state store resource. See [Platform resources](#platform-resources) for more information. | `state.sqlserver`
| managed | Indicates if the resource is Radius-managed. | `true`
| resource | Points to the user-managed resource, if used. | `tablestorage.id`
