---
type: docs
title: "Separate eShop into modules"
linkTitle: "Separate into modules"
slug: "modules"
description: "Learn how to break the eShop up into modules"
weight: 500
toc_hide: true
hide_summary: true
---

Now that you've modeled and deployed the eShop application as a single file, you can break it up into [Bicep modules](https://docs.microsoft.com/en-us/azure/azure-resource-manager/bicep/modules) to make it easier to manage for large teams.

## Create infrastructure module

Begin by creating a file named `infra.bicep` and add the following:

{{< rad file="snippets/infra.bicep" embed=true >}}

### Outputs

Note that the created infrastructure resources are each listed as an `output`, so the consuming application can use them.

## Create catalog module

Next, create a file named `catalog.bicep` and add the following:

{{< rad file="snippets/catalog.bicep" embed=true >}}

## Create other service modules

Repeat the process of placing each service into it's own file:

- [basket.bicep](snippets/basket.bicep)
- [identity.bicep](snippets/identity.bicep)
- [ordering.bicep](snippets/ordering.bicep)
- [payment.bicep](snippets/payment.bicep)
- [seq.bicep](snippets/seq.bicep)
- [webhooks.bicep](snippets/webhooks.bicep)
- [webmvc.bicep](snippets/webmvc.bicep)
- [webshoppingagg.bicep](snippets/webshoppingagg.bicep)
- [webshoppingapigw.bicep](snippets/webshoppingapigw.bicep)
- [webspa.bicep](snippets/webspa.bicep)

## Update application file to use Bicep

Now that resources are broken up into modules, update your `eshop.bicep`:

{{< rad file="snippets/eshop.bicep" embed=true >}}

## Deploy eshop application
