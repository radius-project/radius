---
type: docs
title: "gRPC Route"
linkTitle: "gRPC Route"
description: "Learn how to define gRPC communication with a gRPC Route"
weight: 200
---

## Overview

`GrpcRoute` defines gRPC communication between two [compute Components]({{< ref container >}}).

## Route format

A gRPC Route is defined as a resource within your Application, defined at the same lavel as the Components providing and consuming the gRPC communication.

{{< rad file="snippets/grpc.bicep" embed=true marker="//ROUTE" >}}

## Provided Data

The following data is available for use from the consuming Component:

### Properties

| Property | Description | Example |
|----------|-------------|-------------|
| host | The hostname of the gRPC endpoint | `example.com` |
| port | The port of the gRPC endpoint | `80` |
| scheme | The scheme of the gRPC endpoint | `grpc` |
| url | The full URL of the gRPC endpoint | |

## Example: container

### Providing

Once an gRPC Route is defined, you can provide it from a [container]({{< ref container >}}) by using the `provides` property:

{{< rad file="snippets/grpc.bicep" embed=true marker="//BACKEND" >}}

### Consuming

To consume an gRPC Route, you can use the `connections` property:

{{< rad file="snippets/grpc.bicep" embed=true marker="//FRONTEND" >}}
