---
type: docs
title: "Add Container App Store Microservice infrastructure to application"
linkTitle: "Infrastructure"
slug: "infrastructure"
description: "Learn how to model the Container App Store Microservice infrastructure in Bicep"
weight: 200
---

In this section you will be creating an Container App Store Microservice Radius Application, and add all the resources and services that compose it. You will be adding [platform-specific resources]({{< ref "components-model#platform-specific" >}}) in your application depending on which type of environment you're looking to deploy to. The resources below are meant to be used as module references for your main bicep file which you will create for your services in the next chapter.

## Radius application

The primary resource in Container App Store Microservice is the [Radius Application resource]({{< ref application-model >}}):

{{< rad file="snippets/app.bicep" embed=true marker="//APP" replace-key-rest="//REST" replace-value-rest="..." >}}

## Infrastructure

The first resources to model are the infrastructure resources:

- MongoDB: Azure Cosmos DB
- Redis caches

### Statestore

{{< tabs "Radius Azure Environment" "Radius Local Dev Environment" >}}

{{< codetab >}}
The statestore Radius resource will deploy an Azure CosmosDB in Azure environments for users wanting to use a cloud provider instead of a local development environment.
{{< rad file="snippets/infra.bicep" embed=true replace-key-rest="//REST" replace-value-rest="..."  >}}
{{< /codetab >}}

{{< codetab >}}
The statestore Radius resource will deploy a Redis cache in Radius local environments.
{{< rad file="snippets/infra.dev.bicep" embed=true replace-key-rest="//REST" replace-value-rest="..."  >}}
{{< /codetab >}}

{{< /tabs >}}

## Next steps

In the next step, you will learn about the Container App Store Microservice services.

{{< button text="Next: Model Container App Store Microservice services" page="3-services" >}}
