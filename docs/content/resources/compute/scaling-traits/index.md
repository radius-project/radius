---
type: docs
title: "Scaling Trait"
linkTitle: "Scaling Trait"
description: "Learn how to add a scaling Trait to your compute Components"
weight: 100
---

## Overview

The `radius.dev/ManualScaling` trait configures the number of replicas of a compute instance (such as a container) to run.

## Trait format

In this example, a [container Component]({{< ref container >}}) adds a manual scaling trait to set the number of container replicas.

{{< rad file="snippets/manual.bicep" embed=true marker="//SAMPLE" replace-key-run="//CONTAINER" replace-value-run="container: {...}" >}}

### Properties

| Property | Required | Description | Example |
|----------|:--------:|-------------|---------|
| replicas | Y | The number of replicas to run | `5` |

## Component compatibility

| Component | Azure | Kubernetes |
|-----------|:-----:|:----------:|
| [`ContainerComponent`]({{< ref container >}}) | ✅ | ✅ |
