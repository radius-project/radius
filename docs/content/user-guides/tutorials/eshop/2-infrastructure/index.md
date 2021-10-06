---
type: docs
title: "Add eShop infrastructure to application"
linkTitle: "Add infrastructure"
slug: "infrastructure"
description: "Learn how to model the eShop infrastructure in Bicep"
weight: 200
---

## Add SQL server and databases

To begin, create a file named `eshop.bicep` and add the Bicep template for a SQL server and four SQL databases. These will be [user-managed]({{< ref "components-model#resource-lifecycle" >}}) resources used by your applications.

{{< rad file="snippets/infra.bicep" embed=true marker="//SQL" >}}

## Create eShop Application

Now add an [Application resource]({{< ref application-model >}}):

{{< rad file="snippets/infra.bicep" embed=true marker="//APP" replace-key-sql="//RADSQL" replace-value-sql="" replace-key-redis="//REDIS" replace-value-redis="" replace-key-mongo="//MONGO" replace-value-mongo="" >}}

### Model SQL as Radius Component

To make your Radius Application portable across environments add a `microsoft.com.SQLComponent` resource. This will let you swap out the underlying resource from `Microsoft.Sql/servers/databases` to other SQL providers.

Add the following resources to your `eshop.bicep` file, nested within the `eshop` reosurce:

{{< rad file="snippets/infra.bicep" embed=true marker="//RADSQL" >}}

### Add Redis cache and Mongo DB

In addition to linking to Azure Bicep resources, Radius applications can also employ [Radius-managed resources]({{< ref "components-model#resource-lifecycle" >}}). This lets Radius manage the lifecycle and deployment of the underlying resource.

Add the following resources to your `eshop.bicep` file, nested within the `eshop` reosurce:

{{< rad file="snippets/infra.bicep" embed=true marker="//REDIS" >}}

{{< rad file="snippets/infra.bicep" embed=true marker="//MONGO" >}}

## Next steps

Now that you have modeled your eShop infrastructure, you can model your services in the same file.

{{< button text="Next: Model eShop services" page="3-services" >}}
