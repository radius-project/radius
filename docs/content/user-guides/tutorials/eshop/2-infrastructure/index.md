---
type: docs
title: "Add eShop infrastructure to application"
linkTitle: "Add infrastructure"
slug: "infrastructure"
description: "Learn how to model the eShop infrastructure in Bicep"
weight: 200
---

## Create Radius application

Create a file named `eshop.radius` and add a [Radius Application resource]({{< ref application-model >}}):

{{< rad file="snippets/blank.bicep" embed=true >}}

## Add infrastructure

Now you'll add the required infrastucture:

- SQL databases
- Redis cache
- Message bus, either Azure Service Bus or RabbitMQ
- MongoDB, either Azure Cosmos DB or MongoDB container

### SQL databases

You have the following choices for SQL databases:

{{< tabs "Azure resources" "Containers" "Radius-managed" >}}

{{< codetab >}}
To deploy an Azure SQL server and databases create a Microsoft.Sql/servers resource and four Microsoft.Sql/servers/databases resources at the same level as the eshop resource you previously created, not within it.

{{< rad file="snippets/sql-azure.bicep" embed=true marker="//SQL" >}}
{{< /codetab >}}

{{< codetab >}}
Container-based SQL is modeled as a Radius ContainerComponent. Follow the steps in the next steps.
{{< /codetab >}}

{{< codetab >}}
Radius-managed SQL via `microsoft.com.SQLComponent is still in development. Please check back later.
{{< /codetab >}}

{{< /tabs >}}

### SQL Radius Components

{{< tabs "Azure SQL" "Containers" >}}

{{< codetab >}}
To model your SQL databases in Radius, you will use a `microsoft.com.SQLComponent` resource. This will let you swap out the underlying resource from `Microsoft.Sql/servers/databases` to other SQL providers in the future.

Add the following resources to your `eshop.bicep` file, nested within the `eshop` resource:

{{< rad file="snippets/infra-azure.bicep" embed=true marker="//RADSQL" >}}
{{< /codetab >}}

{{< codetab >}}
Kubernetes environments will use container-based SQL in a Radius ContainerComponent. Follow the steps in the next steps.

{{% alert title="⚠️ Kubernetes resources" color="warning" %}}
Currently Bicep does not support Kubernetes resources, so SQL must be defined as a Radius ContainerComponent. Future releases of Bicep will support Service/Deployments outside of Radius. Stay tuned for more information.
{{% /alert %}}
{{< /codetab >}}

{{< /tabs >}}

### Redis cache, Service Bus, and Mongo DB

In addition to linking to Azure Bicep resources, Radius applications can also employ [Radius-managed resources]({{< ref "components-model#resource-lifecycle" >}}). This lets Radius manage the lifecycle and deployment of the underlying resource.

Add the following resources to your `eshop.bicep` file, nested within the `eshop` reosurce:

{{< rad file="snippets/infra-azure.bicep" embed=true marker="//REDIS" >}}

{{< rad file="snippets/infra-azure.bicep" embed=true marker="//SERVICEBUS" >}}

{{< rad file="snippets/infra-azure.bicep" embed=true marker="//MONGO" >}}

## Next steps

You now have a Bicep file which contains all the infrastructure for your eShop application. Make sure your Bicep file matches the following template:

{{< rad file="snippets/infra-azure.bicep" download=true >}}

In the next step, you will add your eShop services and relationships to the Bicep file.

{{< button text="Next: Model eShop services" page="3-services" >}}
