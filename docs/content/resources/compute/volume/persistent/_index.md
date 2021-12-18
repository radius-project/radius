---
type: docs
title: "Persistent volumes"
linkTitle: "Persistent"
description: "Learn about Radius persistent volumes"
weight: 201
---

Persistent volumes have lifecycles that are separate from the container. Containers "attach" to another resource which contains the persistent volume.

## Properties

A persistent volume can be mounted to a container by specifying the following `volumes` properties within the container definition:

| Key  | Required | Description | Example |
|------|:--------:|-------------|---------|
| name | y | A name key for the volume. | `tempstore`
| kind | y | The type of volume, either `ephemeral` or `persistent` | `persistent`
| mountPath | y | The container path to mount the volume to. | `\tmp\mystore`
| source | y | The resource if of the resource providing the volume. | `filestore.id`
| rbac | n | The role-based access control level for the file share. Allowed values are `'read'` and `'write'`. | `'read'`

### Supported resources

