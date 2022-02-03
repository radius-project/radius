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

## Provided data

| Property | Description | Example(s) |
|----------|-------------|------------|
| `host`  | The Redis host name. | `redis.hello.com`
| `port` | The Redis port. | `4242`
| `username` | The username for connecting to the redis cache. |
| `password()` | The password for connecting to the redis cache. Can be used for password and can be empty. | `d2Y2ba...`
