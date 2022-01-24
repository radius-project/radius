---
type: docs
title: "Add cross-platform connectors to your Radius application"
linkTitle: "Connectors"
description: "Learn how to model and deploy portable resources with Radius connectors"
weight: 300
---

## Overview

Connectors provide **abstraction** and **portability** to Radius applications. This allows developement teams to depend on high level resource types and APIs, and let infra teams swap out the underlying resource and configuration.

<img src="connectors.png" alt="Diagram of a connector connecting from a container to either an Azure Redis Cache or a Kubernetes Deployment" width=700px />

### Example

The following examples show how a [container]({{< ref container >}}) can connect to a Redis connector, which in turn binds to an Azure Cache for Redis or a Kubernetes Pod.

{{< tabs Kubernetes Azure >}}

{{< codetab >}}
<h4>Underlying resource</h4>

In this example Redis is provided by a Kubernetes Pod:

{{< rad file="snippets/redis-container.bicep" embed=true marker="//RESOURCE" >}}

<h4>Connector</h4>

A Redis connector can be configured with properties from the Kubernetes Pod:

{{< rad file="snippets/redis-container.bicep" embed=true marker="//CONNECTOR" >}}

{{< /codetab >}}

{{< codetab >}}
<h4>Underlying resource</h4>

In this example Redis is provided by an Azure Cache for Redis:

{{< rad file="snippets/redis-azure.bicep" embed=true marker="//RESOURCE" >}}

<h4>Connector</h4>

A Redis connector can be configured with an Azure resource:

{{< rad file="snippets/redis-azure.bicep" embed=true marker="//CONNECTOR" >}}

{{< /codetab >}}

{{< /tabs >}}

<h4>Container</h4>

A container can connect to the Redis connector without any configuration or knowledge of the underlying resource:

{{< rad file="snippets/redis-azure.bicep" embed=true marker="//CONTAINER" >}}

## Connector categories

Check out the Radius connector library to begin authoring portable applications:
