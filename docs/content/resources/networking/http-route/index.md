---
type: docs
title: "HTTP Route"
linkTitle: "HTTP Route"
description: "Learn how to define HTTP communication with an HTTP Route"
weight: 100
---

## Overview

An `HttpRoute` resources defines HTTP communication between two [services]({{< ref services >}}).

A [gatwey]({{< ref gateway >}}) can optionally be added for external users to access the Route.

## Route format

An HTTP Route is defined as a resource within your application, defined at the same level as the services providing and consuming the HTTP communication.

{{< rad file="snippets/http.bicep" embed=true marker="//ROUTE" >}}

The following top-level information is available:

| Key  | Required | Description | Example |
|------|:--------:|-------------|---------|
| name | y | The name of your Route. Used to provide status and visualize the resource. | `'web'`
| properties | n | A set of properties that can be used to customize the Route. | See [Properties](#properties)

### Properties

| Key  | Required | Description | Example |
|------|:--------:|-------------|---------|
| port | n | The port providing communication through the Route. Defaults to 80. | `80`
| gateway | n | Details on providing the Route to external users. | [See Gateway](#gateway)

#### Gateway

You can optionally define a Gateway section for external users to access the Route.

| Key  | Required | Description | Example |
|------|:--------:|-------------|---------|
| hostname | n | The hostname of the Gateway. Wildcards supported | `'example.com'`
| rules | n | The rules to match the request with. | [See Rules](#rules)
| source | n | The gateway which this HttpRoute belongs to. If not defined, Radius will create a gateway implicitly to expose traffic. | `gateway.id`

#### Rules

You can optionally define specific rules for the gateway.

| Key  | Required | Description |
|------|:--------:|-------------|
| path | n | The path to match the request on. |

##### Path

| Key  | Required | Description |
|------|:--------:|-------------|
| value | n | Specifies the path to match the incoming requests. |
| type | n | Specifies the type on matching to match the path on. Supported values: 'prefix', 'exact' |

## Provided Data

The following data is available for use from the consuming service:

### Properties

| Property | Description | Example |
|----------|-------------|-------------|
| host | The hostname of the HTTP endpoint | `example.com` |
| port | The port of the HTTP endpoint | `80` |
| scheme | The scheme of the HTTP endpoint | `http` |
| url | The full URL of the HTTP endpoint | `http://example.com:80` |

## Service compatibility

| Service | Azure | Kubernetes |
|-----------|:-----:|:----------:|
| [`Container`]({{< ref container >}}) | ✅ | ✅ |

## Example

The following example shows two containers, one providing an Http Route and the other consuming it:

### Providing container

Once an HTTP Route is defined, you can provide it from a [container]({{< ref container >}}) by using the `provides` property:

{{< rad file="snippets/http.bicep" embed=true marker="//BACKEND" >}}

## HTTP route

{{< rad file="snippets/http.bicep" embed=true marker="//ROUTE" >}}

### Consuming container

To consume an HTTP Route, you can use the `connections` property:

{{< rad file="snippets/http.bicep" embed=true marker="//FRONTEND" >}}
