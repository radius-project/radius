---
type: docs
title: "Redis cache component"
linkTitle: "Redis"
description: "Learn how to use a Redis component in your application"
---

The `redislabs.com/Redis` component is a [portable component]({{< ref components-model >}}) which can be deployed to any [Radius platform]({{< ref platforms >}}).

## Platform resources

| Platform | Resource |
|----------|----------|
| [Microsoft Azure]({{< ref azure>}}) | [Azure Cache for Redis](https://docs.microsoft.com/en-us/azure/azure-cache-for-redis/cache-overview)
| [Kubernetes]({{< ref kubernetes >}}) | Redis service

## Component format

{{< rad file="snippets/managed.bicep" embed=true marker="//SAMPLE" >}}

The following top-level information is available in the Component:

| Key  | Required | Description | Example |
|------|:--------:|-------------|---------|
| name | y | The name of your Component. Used to provide status and visualize the Component. | `cache`

### Resource lifecycle

A `redislabs.com/Redis` component can be Radius-managed. For more information read the [Components docs]({{< ref "components-model#resource-lifecycle" >}}).

| Property | Description | Example(s) |
|----------|-------------|---------|
| managed | Indicates if the resource is Radius-managed. If `false`, a `Resource` must be specified. | `true`, `false`

## Provided data

| Property | Description | Example(s) |
|----------|-------------|------------|
| `connectionString()` | The Redis connection string used to connect to the redis cache. | `myrediscache.redis.cache.windows.net:6380`, `redis.default.svc.cluster.local:6379`
| `primaryKey()` | The primary access key for connecting to the redis cache. Can be used for password and can be empty. | `d2Y2ba...`
| `secondaryKey()` | The secondary access key for connecting to the redis cache. | `d2Y2ba...`
