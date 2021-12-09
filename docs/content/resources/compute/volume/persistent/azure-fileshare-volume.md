---
type: docs
title: "Azure FileShare"
linkTitle: "Azure FileShare"
description: "Learn about the Radius Persistent Azure FileShare Volume"
weight: 200
---

### Azure File Share
Radius supports mounting of an Azure File Share persistent volume to a container. See [Azure File Share](https://docs.microsoft.com/en-us/azure/storage/files/storage-files-introduction)


## Component format

{{< rad file="snippets/volume-fileshare.bicep" embed=true marker="//SAMPLE" >}}

### Properties

You need to specify the properties below on the volume resource to which the container attaches:-
| Key  | Required | Description | Example |
|------|:--------:|-------------|---------|
| kind | y | The kind of persistent volume. Should be 'azure.com.fileshare' for Azure FileShare persistent volumes | `'azure.com.fileshare'`
| managed | n | If set to 'true', Radius will manage the lifecycle and configuration of the underlying resource. Defaults to 'false'. | `'true'`, `'false'`
| resource | n | Resource ID for the existing resource. Used for an unmanaged resource. | `'share.id'`, `'/subscriptions/<subscription>/resourceGroups/<rg/providers/Microsoft.Storage/storageAccounts/<storageAccountName>/fileServices/default/shares/<fileshareName>'`