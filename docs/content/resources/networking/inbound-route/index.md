---
type: docs
title: "Inbound route Trait"
linkTitle: "Inbound route Trait"
description: "Learn how to add a public inbound route to your Component"
weight: 300
---

## Overview

The `radius.dev/InboundRoute` Trait adds a public route to your compute Component, in the form of an ingress controller.

## Trait format

In this example, a [container Component]({{< ref container >}}) adds an inbound route Trait to expose a port to the public:

{{< rad file="snippets/inbound-route.bicep" embed=true marker="//SAMPLE" >}}

### Properties

| Property | Required | Description | Example |
|----------|:--------:|-------------|---------|
| binding | Y | The port name to bind to | `'web'` |

## Component compatibility

| Component | Azure | Kubernetes |
|-----------|:-----:|:----------:|
| [`ContainerComponent`]({{< ref container >}}) | ✅ | ✅ |
