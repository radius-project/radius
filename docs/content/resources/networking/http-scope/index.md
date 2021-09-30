---
type: docs
title: "HTTP Route"
linkTitle: "HTTP Route"
description: "Learn how to define HTTP communication with an HTTP Route"
weight: 100
---

## Overview

`HttpRoute` defines HTTP communication between two [compute Components]({{< ref container >}}).

## Route format

An HTTP Route is defined as a resource within your Application, defined at the same lavel as the Components providing and consuming the HTTP communication.

{{< rad file="snippets/http.bicep" embed=true marker="//ROUTE" >}}

## Provided Data

The following data is available for use from the consuming Component:

### Properties

| Property | Description | Example |
|----------|-------------|-------------|
| host | The hostname of the HTTP endpoint | `example.com` |
| port | The port of the HTTP endpoint | `80` |
| scheme | The scheme of the HTTP endpoint | `http` |
| url | The full URL of the HTTP endpoint | `http://example.com:80` |

## Component compatibility

| Component | Azure | Kubernetes |
|-----------|:-----:|:----------:|
| [`ContainerComponent`]({{< ref container >}}) | ✅ | ✅ |

## Example: container

### Providing

Once an HTTP Route is defined, you can provide it from a [container]({{< ref container >}}) by using the `provides` property:

{{< rad file="snippets/http.bicep" embed=true marker="//BACKEND" >}}

### Consuming

To consume an HTTP Route, you can use the `connections` property:

{{< rad file="snippets/http.bicep" embed=true marker="//FRONTEND" >}}
