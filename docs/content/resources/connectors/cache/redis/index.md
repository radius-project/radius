---
type: docs
title: "Redis cache connector"
linkTitle: "Redis"
description: "Learn how to use a Redis connector in your application"
---

The `redislabs.com/Redis` connector is a [portable connector]({{< ref connectors >}}) which can be deployed to any [Radius platform]({{< ref platforms >}}).

## Platform resources

| Platform | Resource |
|----------|----------|
| [Microsoft Azure]({{< ref azure>}}) | [Azure Cache for Redis](https://docs.microsoft.com/en-us/azure/azure-cache-for-redis/cache-overview)
| [Kubernetes]({{< ref kubernetes >}}) | Redis service

## Resource format

{{< rad file="snippets/managed.bicep" embed=true marker="//SAMPLE" >}}

The following top-level information is available in the resource:

| Key  | Required | Description | Example |
|------|:--------:|-------------|---------|
| name | y | The name of your resource. Used to provide status and visualize the resource. | `cache`

### Resource lifecycle

A `redislabs.com/Redis` connector can be Radius-managed. For more information read the [Components docs]({{< ref "components-model#resource-lifecycle" >}}).

| Property | Description | Example(s) |
|----------|-------------|---------|
| managed | Indicates if the resource is Radius-managed. If `false`, a `Resource` must be specified. | `true`, `false`

## Provided data

| Property | Description | Example(s) |
|----------|-------------|------------|
| `host`  | The Redis host name. | `redis.hello.com`
| `port` | The Redis port. | `4242`
| `username` | The username for connecting to the redis cache. |
| `password()` | The password for connecting to the redis cache. Can be used for password and can be empty. | `d2Y2ba...`

## Starter

You can get up and running quickly with a Redis cache by using a [starter]({{< ref starter-templates >}}):

{{< rad file="snippets/starter.bicep" embed=true >}}

### Container

The Redis cache container starter uses a Redis container and can run on any Radius platform.

```
br:radius.azurecr.io/starters/redis:latest
```

#### Input parameters

| Parameter | Description | Required | Default |
|-----------|-------------|:--------:|---------|
| radiusApplication | The application resource to use as the parent of the RabbitMQ Broker | Yes | - |
| cacheName | The name for your Redis Cache container | No | `deployment().name` (module name) |

#### Output parameters

| Resource | Description | Type |
|----------|-------------|------|
| redisCache | The Redis Cache resource | `radius.dev/Application/redislabs.com.RedisCache@v1alpha3` |

### Microsoft Azure

The Redis cache Azure starter uses an Azure Cache for Redis and can run only on Azure.

```txt
br:radius.azurecr.io/starters/redis-azure:latest
```

#### Input parameters

| Parameter | Description | Required | Default |
|-----------|-------------|:--------:|---------|
| radiusApplication | The application resource to use as the parent of the RabbitMQ Broker | Yes | - |
| cacheName | The name for your Redis Cache container | No | `'redis-${uniqueString(resourceGroup().id, deployment().name)}'` |
| redisCacheSku | The SKU of the Redis Cache | No | `'Basic'` |
| redisCacheFamily | The family of the Azure Redis Cache | No | `'C'` |
| redisCacheCapacity | The capacity of the Azure Redis Cache | No | `1` |
| location | The Azure region to deploy the Azure Redis Cache | No | `resourceGroup().location` |

#### Output parameters

| Parameter | Description | Type |
|-----------|-------------|------|
| redisCache | The Redis Cache resource | `radius.dev/Application/redislabs.com.RedisCache@v1alpha3` |
