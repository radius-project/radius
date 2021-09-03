---
type: docs
title: "Redis cache component"
linkTitle: "Redis"
description: "Learn how to use a Redis component in your application"
---

The `redislabs.com/Redis` component is a [portable component]({{< ref components-model >}}) which can be deployed to any [Radius platform]({{< ref environments >}}).

## Platform resources

| Platform | Resource |
|----------|----------|
| [Microsoft Azure]({{< ref azure-environments >}}) | [Azure Cache for Redis](https://docs.microsoft.com/en-us/azure/azure-cache-for-redis/cache-overview)
| [Kubernetes]({{< ref kubernetes-environments >}}) | Redis service

## Configuration

| Property | Description | Example(s) |
|----------|-------------|---------|
| managed | Indicates if the resource is Radius-managed. If `false`, a `Resource` must be specified. | `true`, `false`

## Resource lifecycle

A `redislabs.com/Redis` component can be Radius-managed. For more information read the [Components docs]({{< ref "components-model#resource-lifecycle" >}}).

### Radius managed

{{< rad file="snippets/managed.bicep" embed=true marker="//SAMPLE" >}}

### User managed

{{% alert title="Warning" color="warning" %}}
Currently user-managed Redis components are not supported.
{{% /alert %}}

## Bindings

### redis

The `redis` Binding of kind `redislabs.com/Redis` represents the Redis.

| Property | Description | Example(s) |
|----------|-------------|------------|
| `connectionString` | The Redis connection string used to connect to the redis cache. | `myrediscache.redis.cache.windows.net:6380`, `redis.default.svc.cluster.local:6379`
| `host` | The host name of the redis cache to which you are connecting. | `myrediscache.redis.cache.windows.net`,  `redis.default.svc.cluster.local`
| `port` | The port value of the redis cache to which you are connecting.| `6380`, `6379`
| `primaryKey` | The primary access key for connecting to the redis cache. Can be used for password and can be empty. | `d2Y2ba...`
| `secondaryKey` | The secondary access key for connecting to the redis cache. | `d2Y2ba...`
