---
type: docs
title: "Dapr sidecar Trait"
linkTitle: "Dapr Trait"
description: "Learn how to add a Dapr sidecar with a Dapr trait"
weight: 100
slug: "trait"
---

## Overview

The `dapr.io/App` trait adds and configures a Dapr sidecar to your application.

## Trait format

In this example, a [container Component]({{< ref container >}}) adds a Dapr trait to add a Dapr sidecar:

{{< rad file="snippets/dapr.bicep" embed=true marker="//SAMPLE" replace-key-run="//CONTAINER" replace-value-run="container: {...}" >}}

### Properties

| Property | Required | Description | Example |
|----------|:--------:|-------------|---------|
| appId | n | The appId of the Dapr sidecar | `backend` |
| appPort | n | The port your Component exposes to Dapr | `3500`

## Component compatibility

| Component | Azure | Kubernetes |
|-----------|:-----:|:----------:|
| [`ContainerComponent`]({{< ref container >}}) | ✅ | ✅ |
