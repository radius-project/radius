---
type: docs
title: "HTTP Route"
linkTitle: "HTTP Route"
description: "Learn how to define HTTP communication with an HTTP Route"
weight: 100
---

## Overview

`HttpRoute` defines HTTP communication between two [compute Components]({{< ref container >}}), and also provides the ability to specify a gatwey for external users to access the Route.

## Route format

An HTTP Route is defined as a resource within your Application, defined at the same lavel as the Components providing and consuming the HTTP communication.

{{< rad file="snippets/http.bicep" embed=true marker="//ROUTE" >}}

The following top-level information is available:

| Key  | Required | Description | Example |
|------|:--------:|-------------|---------|
| name | y | The name of your Route. Used to provide status and visualize the component. | `'web'`

### Properties

| Key  | Required | Description | Example |
|------|:--------:|-------------|---------|
| port | n | The port providing communication through the Route. Defaults to 80. | `80`
| gateway | n | Details on providing the Route to external users. | [See below](#gateway)

#### Gateway

You can optionally define a Gateway for external users to access the Route.

| Key  | Required | Description | Example |
|------|:--------:|-------------|---------|
| hostname | n | The hostname of the Gateway. Wildcards supported | `'example.com'`

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
