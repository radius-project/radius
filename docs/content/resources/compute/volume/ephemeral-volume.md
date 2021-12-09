---
type: docs
title: "Ephemeral Volumes"
linkTitle: "Ephemeral"
description: "Learn about the Radius Ephemeral Volume"
weight: 200
---

## Ephemeral volumes

Ephemeral volumes have the same lifecycle as the container, being deployed and deleted with the container. They create an empty directory on the host and mount it to the container.

{{< rad file="snippets/volume-ephemeral.bicep" embed=true marker="//SAMPLE" >}}

### Properties

An ephemeral volume can be mounted to a container by specifying the following properties in the container definition:-
| Key  | Required | Description | Example |
|------|:--------:|-------------|---------|
| name | y | A name key for the volume. | `tempstore`
| kind | y | The type of volume, either `ephemeral` or `persistent`. | `ephemeral`
| mountPath | y | The container path to mount the volume to. | `\tmp\mystore`
| managedStore | y | The backing storage medium. Either `disk` or `memory`. | `memory`
