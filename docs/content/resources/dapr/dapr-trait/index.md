---
type: docs
title: "Dapr Sidecar Trait"
linkTitle: "Dapr Trait"
description: "Learn how to add a Dapr sidecar with a Dapr trait"
weight: 100
slug: "trait"
---

## Overview

The `dapr.io/Sidecar` trait adds and configures a Dapr sidecar to your application.

## Trait format

In this example, a [container Component]({{< ref container >}}) adds a Dapr trait to add a Dapr sidecar:

{{< rad file="snippets/dapr.bicep" embed=true marker="//SAMPLE" replace-key-run="//CONTAINER" replace-value-run="container: {...}" >}}

### Properties

| Property | Required | Description | Example |
|----------|:--------:|-------------|---------|
| appId | n | The appId of the Dapr sidecar. Will use the value of an attached [Route]({{< ref dapr-http >}}) if present. | `backend` |
| appPort | n | The port your Component exposes to Dapr | `3500`
| provides | n | The [Dapr Route]({{< ref dapr-http >}}) provided by the Trait | `daprHttp.id`

## Component compatibility

| Component | Azure | Kubernetes |
|-----------|:-----:|:----------:|
| [`ContainerComponent`]({{< ref container >}}) | ✅ | ✅ |
