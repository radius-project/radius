---
type: docs
title: "Azure file share"
linkTitle: "Azure file share"
description: "Learn about Radius persistent Azure file share volumes"
weight: 200
---

Radius supports mounting an Azure file share persistent volume to a container. See the [Azure file share docs](https://docs.microsoft.com/azure/storage/files/storage-files-introduction) for more information on the file share service.

## Component format

{{< rad file="snippets/volume-fileshare.bicep" embed=true marker="//SAMPLE" >}}

### Properties

The following properties are available on the `Volume` resource which the container attaches to:

| Key  | Required | Description | Example |
|------|:--------:|-------------|---------|
| kind | y | The kind of persistent volume. Should be 'azure.com.fileshare' for Azure FileShare persistent volumes | `'azure.com.fileshare'`
| managed | n | If set to 'true', Radius will manage the lifecycle and configuration of the underlying resource. Defaults to 'false'. | `'true'`, `'false'`
| resource | n | Resource ID for the existing resource. Used for an unmanaged resource. | `'share.id'`, `'/subscriptions/<subscription>/resourceGroups/<rg/providers/Microsoft.Storage/storageAccounts/<storageAccountName>/fileServices/default/shares/<fileshareName>'`