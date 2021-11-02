---
type: docs
title: "Radius Volume"
linkTitle: "Volume"
description: "Learn about the Radius Persistent Volume"
weight: 100
---

`Volume` provides an abstraction for a persistent volume to be referenced and mounted by a container component. Persistent volumes have lifecycles that are separate from the container. ContainerComponents "attach" to another resource which contains the volume.

## Volume Properties

| Key  | Required | Description | Example |
|------|:--------:|-------------|---------|
| kind | y | The kind of persistent volume. Currently, the supported types are: Azure File Share | `azure.com.fileshare`
| managed | y | Volume is created and deleted by Radius or is an existing resource that is referenced by Radius. | `true`, `false`
| resource | n | Resource ID for the existing resource. Used for an unmanaged resource. | `/subscriptions/<subscription>/resourceGroups/<rg/providers/Microsoft.Storage/storageAccounts/<storageAccountName>/fileServices/default/shares/<fileshareName`
