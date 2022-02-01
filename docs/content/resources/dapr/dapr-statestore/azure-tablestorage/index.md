---
type: docs
title: "Azure Table Storage Dapr State Store Component"
linkTitle: "Azure Table Storage"
description: "Learn how to use Azure Table Storage Dapr State Store components in Radius"
weight: 400
slug: "statestore"
---

This section shows how to use an [Azure Table Storage](https://docs.microsoft.com/en-us/azure/storage/tables/table-storage-overview) Dapr State Store component in a Radius Application.

## Component format

{{< tabs "Radius-managed" "User-managed" >}}

{{% codetab %}}
The following example shows a fully managed Dapr State Store Component, where the underlying infrastructure is managed by Radius:
{{< rad file="snippets/managed.bicep" embed=true marker="//SAMPLE" >}}
{{% /codetab %}}

{{% codetab %}}
First define your State Store resource. In this example we're using an Azure Table Storage:
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
| kind | The kind of the underlying state store resource. See [Platform resources](#platform-resources) for more information. | `state.azure.tablestorage`
| managed | Indicates if the resource is Radius-managed. | `true`
| resource | Points to the user-managed resource, if used. | `namespace::tablestorage.id`

### State Store settings

| Property | Description | Example |
|----------|-------------|---------|
| topic | The name of the topic to create for this Pub/Sub broker | `TOPIC_A`
