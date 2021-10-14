---
type: docs
title: "Add eShop infrastructure to application"
linkTitle: "Add infrastructure"
slug: "infrastructure"
description: "Learn how to model the eShop infrastructure in Bicep"
weight: 200
---

In this section you will be creating an eShop Radius Application, and add all the resources and services that compose it. You will be adding a mixture of [user-managed Radius resources]({{< ref "components-model#user-managed" >}}), [Radius-managed resources]({{< ref "components-model#radius-managed" >}}), and [platform-specific resources]({{< ref "components-model#platform-specific" >}}) in your application to learn all the ways you can create an application.

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

{{< tabs "Azure resource" "Radius-managed" "Container" >}}

{{< codetab >}}
Azure SQL databases are compatible with Azure environments only. For Kubernetes environments either use containerized SQL or pass in previously-deployed Azure SQL connection strings as deployment parameters.
<br /><br />
Update your eshop.bicep file with:
<ul>
<li><b>1 Azure SQL server resource</b> - the parent "container" resource which will deploy an Azure SQL server in your Azure resource group.</li>
<li><b>4 SQL database resources</b> - the child resources which eShop services will connect to.</li>
<li><b>4 SQL Radius resources</b> - What your containers will connect to and get connection details from.</li>
</ul>

{{% alert title="💡 Concept" color="info" %}}
For Azure SQL you are using a <a href="{{< ref "components-model#user-managed" >}}">user-managed Radius Component</a>, where you are defining the Azure SQL Bicep resource outside of Radius, and then binding it to a Radius non-runnable Component.

This allows you to swap out the underlying SQL provider by replacing the `resource` parameter. Services connect to the Radius resources, so their definitions and parameters don't change.

You can also connect to already deployed SQL instances with the [existing keyword](https://docs.microsoft.com/en-us/azure/azure-resource-manager/bicep/resource-declaration?#reference-existing-resources).
{{% /alert %}}

{{< rad file="snippets/sql-azure.bicep" embed=true marker="//SQL" >}}
{{< /codetab >}}

{{< codetab >}}
Radius-managed SQL via `microsoft.com.SQLComponent is still in development.

{{% alert title="📋 Feedback" color="info" %}}
Want to see a SQL managed component? Let us know over in the [GitHub discussions](https://github.com/Azure/radius/discussions/1269).
{{% /alert %}}
{{< /codetab >}}

{{< codetab >}}
Containerized SQL can run on any Radius environment. In Radius you can create a ContainerComponent to run the SQL server container, and an HttpRoute for other services to connect to.
<br /><br>
Note that this option mimics the built-in behavior of the microsoft.com.SQLComponent resource, which currently only supports Azure SQL. In the future, we will add support for other SQL providers.
<br /><br>
Update your eshop.bicep file with:
<ul>
<li><b>1 ContainerComponent with the SQL image</b> - the SQL server that will host the SQL databases.</li>
<li><b>1 HttpRoute</b> - What your containers will connect to in order to communicate with the SQL server.</li>
</ul>

{{% alert title="💡 Concept" color="info" %}}
Here you are manually defining a ContainerComponent which will provide a SQL database. Other services can communicate with this server through the `sqlRoute` [HttpRoute]({{< ref http-route >}}).
{{% /alert %}}

{{< rad file="snippets/sql-containers.bicep" embed=true marker="//SQL" >}}
{{< /codetab >}}

{{< /tabs >}}

### Redis cache

{{< tabs "Radius-managed" "User-managed Azure" >}}

{{< codetab >}}
The redislabs.com.RedisComponent Radius resource will deploy an Azure Redis Cache in Azure environments, and a Redis container in Kubernetes environments. Add the following resource to your eShop application:

{{% alert title="💡 Concept" color="info" %}}
Here you are defining a <a href="{{< ref "components-model#radius-managed" >}}">Radius-managed Redis Component</a>, where Radius manages the deployment and deletion of the resource as part of the Application.
{{% /alert %}}

{{< rad file="snippets/redis-managed.bicep" embed=true replace-key-rest="//REST" replace-value-rest="..." >}}
{{< /codetab >}}
{{< codetab >}}
If you prefer to bring your own Redis Cache, you can reference an existing Redis Cache resource in Bicep and provide it as a resource to the Radius resource. Add the following resources to your eShop file and application:

{{% alert title="💡 Concept" color="info" %}}
Here you are using a <a href="{{< ref "components-model#user-managed" >}}">user-managed Radius Component</a>, but there the resource has already been deployed to Azure. Make sure to [specify the correct scope](https://docs.microsoft.com/en-us/azure/azure-resource-manager/bicep/deploy-to-resource-group?tabs=azure-cli#scope-to-different-resource-group), as the default scope is the environment's resource group.
{{% /alert %}}

{{< rad file="snippets/redis-azure.bicep" embed=true replace-key-rest="//REST" replace-value-rest="..."  >}}
{{< /codetab >}}

{{< /tabs >}}

### MongoDB

{{< tabs "Radius-managed" "User-managed Azure" >}}

{{< codetab >}}
The mongodb.com.MongoDBComponent Radius resource will deploy an Azure CosmosDB with Mongo API in Azure environments, and a Mongo container in Kubernetes environments. Add the following resource to your eShop application:

{{< rad file="snippets/mongo-managed.bicep" embed=true replace-key-rest="//REST" replace-value-rest="..."  >}}
{{< /codetab >}}

{{< codetab >}}
If you prefer to bring your own Cosmos DB with Mongo API, you can reference an existing Cosmos resource in Bicep and provide it as a resource to the Radius resource. Add the following resources to your eShop file and application:

{{< rad file="snippets/mongo-azure.bicep" embed=true replace-key-rest="//REST" replace-value-rest="..."  >}}
{{< /codetab >}}

{{< /tabs >}}

### Message Bus

eShop has two different modes: Service Bus and RabbitMQ. A parameter passed into the services determines which mode to use, and which infrastructure to expect.

{{< tabs "Azure Service Bus" "RabbitMQ" >}}

{{< codetab >}}

{{% alert title="💡 Concept" color="info" %}}
Here you are deploying a <a href="{{< ref "components-model#platform-specific-resources" >}}">platform-specific Service Bus Topic resource</a>, which does not have a portable, Radius component. Other Components can bind directly to this resource.
{{% /alert %}}


{{< rad file="snippets/messagebus-servicebus.bicep" embed=true replace-key-rest="//REST" replace-value-rest="..."  >}}
{{< /codetab >}}

{{< codetab >}}

{{< rad file="snippets/messagebus-rabbitmq.bicep" embed=true replace-key-rest="//REST" replace-value-rest="..."  >}}
{{< /codetab >}}

{{< /tabs >}}

## Next steps

You now have a Bicep file which contains all the infrastructure for your eShop application. The following tempaltes include the default choices for each environment. Feel free to customize them if you want to try an alternate resource option described above.

{{< tabs "Azure environment" "Kubernetes environment" >}}

{{< codetab >}}
{{< rad file="snippets/infra-azure.bicep" download=true >}}
{{< /codetab >}}
{{< codetab >}}
{{< rad file="snippets/infra-kubernetes.bicep" download=true >}}
{{< /codetab >}}

{{< /tabs >}}

In the next step, you will add your eShop services and relationships to the Bicep file.

{{< button text="Next: Model eShop services" page="3-services" >}}
