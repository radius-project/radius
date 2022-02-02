---
type: docs
title: "Model Container App Store Microservice services in Radius"
linkTitle: "Services"
slug: "model-services"
description: "Learn how to model the Container App Store Microservice services in Radius"
weight: 300
---

## Add parameters

The following parameters are added to the Container App Store Microservice file.

{{< rad file="snippets/params.bicep" embed=true >}}

## Services

Taking a closer look at the services, they are modeled as [Containers]({{< ref container >}}) resources.

### Store API (node-app)

{{< rad file="snippets/node-app.bicep" embed=true >}}

### Order Service (python-app)

{{< rad file="snippets/python-app.bicep" embed=true >}}

### Inventory Service (go-app)

{{< rad file="snippets/go-app.bicep" embed=true >}}

### Statestore

{{< rad file="snippets/statestore.bicep" embed=true >}}

## HTTP Route

Other services will communicate with  each other through Dapr via HTTP.

An [HttpRouteComponent]({{< ref http-route >}}) resource allows other resources to connect to each other resource:

{{< rad file="snippets/routes.bicep" embed=true >}}

## Next steps

Now that we have looked at the Container App Store Microservice infrastructure, and how we can model its services, let's now deploy it to a Radius environment.

{{< button text="Next: Deploy Container App Store Microservice application" page="4-deploy" >}}
