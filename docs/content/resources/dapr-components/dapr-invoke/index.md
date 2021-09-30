---
type: docs
title: "Dapr service invocation Route"
linkTitle: "Invoke Route"
description: "Learn how to use Dapr service invocation in Radius"
weight: 400
---

## Overview

`dapr.io.InvokeRoute` defines Dapr service invocation communication between two or more [compute Components]({{< ref container >}}).

## Route format

A service invocation Route is defined as a resource within your Application, defined at the same lavel as the Components providing and consuming the service invocation communication.

{{< rad file="snippets/invoke.bicep" embed=true marker="//ROUTE" >}}

## Provided Data

The following data is available for use from the consuming Component:

### Properties

| Property | Description | Example |
|----------|-------------|-------------|
| appId | The appId of the providing Component | `backend` |

## Example: container

### Providing

Once a Dapr service invocation Route is defined, you can provide it from a [container]({{< ref container >}}) by using the `provides` property:

{{< rad file="snippets/invoke.bicep" embed=true marker="//BACKEND" >}}

### Consuming

To consume a Dapr service invocation Route, you can use the `connections` property:

{{< rad file="snippets/invoke.bicep" embed=true marker="//FRONTEND" >}}
