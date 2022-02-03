---
type: docs
title: "Add eShop infrastructure to application"
linkTitle: "Infrastructure"
slug: "infrastructure"
description: "Learn how to model the eShop infrastructure in Bicep"
weight: 200
---

In this section you will create an eShop Radius Application and add all the resources and services that compose it. 

## Radius application

The primary resource in eShop is the [Radius Application resource]({{< ref application-model >}}):

{{< rad file="snippets/blank.bicep" embed=true >}}

## Infrastructure

The first resources to model are the infrastructure resources:

- SQL databases
- Redis caches
- Message bus: either Azure Service Bus or RabbitMQ
- MongoDB: either Azure Cosmos DB or MongoDB container

### SQL databases

You have the following choices for SQL databases:

{{< tabs "Azure resource" "Radius-managed" "Container" >}}

{{< codetab >}}
Azure SQL databases are compatible with Azure environments only. For Kubernetes environments either use containerized SQL or pass in previously-deployed Azure SQL connection strings as deployment parameters.
<br /><br />
Update your eshop.bicep file with:
<ul>
<li><b>1 Azure SQL server resource</b> - the parent "container" resource which will deploy an [Azure SQL server](https://docs.microsoft.com/en-us/azure/azure-sql/database/logical-servers) in your Azure resource group.</li>
<li><b>4 SQL database resources</b> - the child resources which eShop services will connect to.</li>
<li><b>4 SQL Radius resources</b> - What your containers will connect to and get connection details from.</li>
</ul>

{{% alert title="💡 Concept" color="info" %}}
For Azure SQL you are defining the Azure SQL Bicep resource outside of Radius and then binding it to a Radius connector resource.

Using a connector allows you to swap out the underlying SQL provider by replacing the `resource` parameter. Services connect to the Radius resources, so their definitions and parameters don't change.

You can also connect to already deployed SQL instances with the [existing keyword](https://docs.microsoft.com/en-us/azure/azure-resource-manager/bicep/resource-declaration?#reference-existing-resources).
{{% /alert %}}

{{< rad file="snippets/sql-azure.bicep" embed=true marker="//SQL" >}}
{{< /codetab >}}

{{< codetab >}}
Radius-managed SQL via microsoft.com.SQLDatabase is still in development.

{{% alert title="📋 Feedback" color="info" %}}
Want to see a SQL managed component? Let us know over in the [GitHub discussions](https://github.com/project-radius/radius/discussions/1269).
{{% /alert %}}
{{< /codetab >}}

{{< codetab >}}
Containerized SQL can run on any Radius environment. In Radius you can create a Container to run the SQL server container, and an HttpRoute for other services to connect to.
<br /><br>
Note that this option mimics the built-in behavior of the microsoft.com.SQLDatabase resource, which currently only supports Azure SQL. In the future, we will add support for other SQL providers.
<br /><br>
Update your eshop.bicep file with:
<ul>
<li><b>1 Container with the SQL image</b> - the SQL server that will host the SQL databases.</li>
<li><b>1 HttpRoute</b> - What your containers will connect to in order to communicate with the SQL server.</li>
</ul>

{{% alert title="💡 Concept" color="info" %}}
Here you are manually defining a Container which will provide a SQL database. Other services can communicate with this server through the `sqlRoute` [HttpRoute]({{< ref http-route >}}).
{{% /alert %}}

{{< rad file="snippets/sql-containers.bicep" embed=true marker="//SQL" >}}
{{< /codetab >}}

{{< /tabs >}}

### Redis caches

{{< tabs "Radius-managed" "User-managed Azure" >}}

{{< codetab >}}
The redislabs.com.RedisCache Radius resource will deploy an Azure Redis Cache in Azure environments, and a Redis container in Kubernetes environments.

{{% alert title="💡 Concept" color="info" %}}
Here you are defining two Radius-managed Redis Components, where Radius manages the deployment and deletion of the resources as part of the Application.
{{% /alert %}}

{{< rad file="snippets/redis-managed.bicep" embed=true replace-key-rest="//REST" replace-value-rest="..." >}}
{{< /codetab >}}
{{< codetab >}}
If you prefer to bring your own Redis Caches, you can reference existing Redis Cache resources in Bicep and provide them as a source to the Radius resources.

{{% alert title="💡 Concept" color="info" %}}
Here you are using existing resources where the resources have already been deployed to Azure. Make sure to [specify the correct scope](https://docs.microsoft.com/en-us/azure/azure-resource-manager/bicep/deploy-to-resource-group?tabs=azure-cli#scope-to-different-resource-group), as the default scope is the environment's resource group.
{{% /alert %}}

{{< rad file="snippets/redis-azure.bicep" embed=true replace-key-rest="//REST" replace-value-rest="..."  >}}
{{< /codetab >}}

{{< /tabs >}}

### MongoDB

{{< tabs "Radius-managed" "User-managed Azure" >}}

{{< codetab >}}
The mongo.com.MongoDatabase Radius resource will deploy an Azure CosmosDB with Mongo API in Azure environments, and a Mongo container in Kubernetes environments.

{{< rad file="snippets/mongo-managed.bicep" embed=true replace-key-rest="//REST" replace-value-rest="..."  >}}
{{< /codetab >}}

{{< codetab >}}
If you prefer to bring your own Cosmos DB with Mongo API, you can reference an existing Cosmos resource in Bicep and provide it as a resource to the Radius resource.

{{< rad file="snippets/mongo-azure.bicep" embed=true replace-key-rest="//REST" replace-value-rest="..."  >}}
{{< /codetab >}}

{{< /tabs >}}

### Message Bus

eShop has two different modes: Service Bus and RabbitMQ. A parameter passed into the services determines which mode to use, and which infrastructure to expect.

{{< tabs "Azure Service Bus" "RabbitMQ" >}}

{{< codetab >}}

Azure Service Bus is only compatible with Azure environments. For Kubernetes environments use RabbitMQ (next tab).

{{% alert title="💡 Concept" color="info" %}}
Here you are deploying a platform-specific Service Bus Topic resource</a>, which does not have a Radius connector resource. Other resources in the application can bind directly to this resource.
{{% /alert %}}

{{< rad file="snippets/messagebus-servicebus.bicep" embed=true replace-key-rest="//REST" replace-value-rest="..." replace-key-eshop="//ESHOP"  >}}
{{< /codetab >}}

{{< codetab >}}
RabbitMQ is only compatible with Kubernetes environments. For Azure environments use Azure Service Bus (previous tab).

{{< rad file="snippets/messagebus-rabbitmq.bicep" embed=true replace-key-rest="//REST" replace-value-rest="..." replace-key-eshop="//ESHOP"  >}}
{{< /codetab >}}

{{< /tabs >}}

## Next steps

In the next step, you will learn about the eShop services.

{{< button text="Next: Model eShop services" page="3-services" >}}
