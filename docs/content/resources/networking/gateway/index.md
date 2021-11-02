---
type: docs
title: "Gateway"
linkTitle: "Gateway"
description: "Learn how to route requests to different resources."
weight: 100
---

## Overview

`Gateway` defines how requests are routed to different resources, and also provides the ability to expose traffic to the internet. Conceptually, gateways allow you to have a single point of entry for  traffic in your application, whether it be internal or external traffic.

`Gateway` in radius are split into two main pieces; the `Gateway` resource itself, which defines which port and protocol to listen on, and Route(s) which define the rules for routing traffic to different resources.

## Gateway format

A Gateway is defined as a resource within your Application, defined at the same level as the Components providing and consuming the HTTP communication.

{{< rad file="snippets/gateway.bicep" embed=true marker="//GATEWAY" >}}

The following top-level information is available:

| Key  | Required | Description | Example |
|------|:--------:|-------------|---------|
| name | y | The name of your Gateway. Used to provide status and visualize the component. | `'gateway'`

### Properties

| Key  | Required | Description | Example |
|------|:--------:|-------------|---------|
| listeners | y | The bindings that your gateway should listen to. |  [See below](#listeners)

#### Listeners

You can define multiple listeners, each with a different port and protocol.

| Key  | Required | Description | Example |
|------|:--------:|-------------|---------|
| port | y | The port to listen on. | `80`
| protocol | y | The protocol to use for traffic on this binding. | `'HTTP'`

