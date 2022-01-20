---
type: docs
title: "Dapr HTTP Route"
linkTitle: "HTTP Route"
description: "Learn how to use Dapr's HTTP API in Radius"
weight: 150
slug: "http"
---

## Overview

`dapr.io.InvokeHttpRoute` defines Dapr communication through the HTTP API between two or more [compute Components]({{< ref container >}}).

## Route format

A Dapr HTTP Route is defined as a resource within your Application, defined at the same lavel as the Components providing and consuming the Dapr HTTP API communication.

{{< rad file="snippets/http.bicep" embed=true marker="//ROUTE" >}}

## Provided Data

The following data is available for use from the consuming Component:

### Properties

| Property | Description | Example |
|----------|-------------|-------------|
| appId    | The appId of the providing Component | `backend` |

## Example: container

### Providing

Once a Dapr service invocation Route is defined, you can provide it from a [container]({{< ref container >}}) by using the `provides` property:

{{< rad file="snippets/http.bicep" embed=true marker="//BACKEND" >}}
{{< rad file="snippets/http.bicep" embed=true marker="//ROUTE" >}}

### Consuming

To consume a Dapr service invocation Route, you can use the `connections` property:

{{< rad file="snippets/http.bicep" embed=true marker="//FRONTEND" >}}
