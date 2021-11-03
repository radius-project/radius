---
type: docs
title: "Radius Volume"
linkTitle: "Volume"
description: "Learn about the Radius Persistent Volume"
weight: 100
---

`Volume` provides an abstraction for a persistent volume to be referenced and mounted by a [ContainerComponent]({{< ref container >}}). Persistent volumes have lifecycles that are separate from the container. ContainerComponents "attach" to another resource which contains the volume.

## Supported resources

| Resource | kind |
|-----------|------|
| [Azure File Share](https://docs.microsoft.com/en-us/azure/storage/files/storage-files-introduction) | `'azure.com.fileshare'`

## Component format

{{< rad file="snippets/volume.bicep" embed=true marker="//SAMPLE" >}}

### Properties

| Key  | Required | Description | Example |
|------|:--------:|-------------|---------|
| kind | y | The kind of persistent volume. See [supported volumes](#supported-resources). | `'azure.com.fileshare'`
| managed | n | If set to 'true', Radius will manage the lifecycle and configuration of the underlying resource. Defaults to 'false'. | `'true'`, `'false'`
| resource | n | Resource ID for the existing resource. Used for an unmanaged resource. | `'share.id'`, `'/subscriptions/<subscription>/resourceGroups/<rg/providers/Microsoft.Storage/storageAccounts/<storageAccountName>/fileServices/default/shares/<fileshareName>'`
