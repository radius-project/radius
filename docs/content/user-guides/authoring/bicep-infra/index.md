---
type: docs
title: "Model your application infrastructure in Bicep"
linkTitle: "Model infrastucture"
description: "Learn how to model your infrastucture in the Bicep language."
weight: 200
---

Begin by modeling your infrastructure in a Bicep file. This can be done by declaring and deploying new resources, or by referencing existing resources that have already been deployed.

## Model and deploy with Bicep

The following example shows an Azure CosmosDB account and MondoDB database that will be deployed with Bicep. This is useful if you want to leverage Bicep and Azure to manage the lifecycle of your resource:

{{< rad file="snippets/new.bicep" embed=true >}}

You can now use the `cosmos::db` resource in your Radius application.

## Reference an existing resource

Alternately, you can [reference an existing resource](https://docs.microsoft.com/en-us/azure/azure-resource-manager/bicep/resource-declaration?tabs=azure-powershell#reference-existing-resources) that is deployed and managed by another process.

{{% alert title="Incremental adoption" color="info" %}}
The `existing` keyword lets you add Radius to exsting resources and infrastructure. This can be useful if you want to reuse existing infrastructure that you've already deployed through another process.
{{% /alert %}}

Here's an example of a CosmosDB account and MongoDB resource:

{{< rad file="snippets/existing.bicep" embed=true >}}

You can now use `cosmos::db` in your Radius application, just like if you freshly deployed the resources.

## Available resources

- Visit the [Azure templates](https://docs.microsoft.com/azure/templates/) docs page to learn what resources are available in Bicep.
- Visit the [Radius resource library]({{< ref resources >}}) to learn what Radius resources are available in Bicep.

## Next step

Now that you have modeled your infrasturcture in Bicep, you can add your Radius application and services:

{{< button page="bicep-app" text="Model your application and services" >}}
