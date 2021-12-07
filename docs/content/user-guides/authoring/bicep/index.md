---
type: docs
title: "Bicep language overview"
linkTitle: "Bicep language"
description: "Learn about the Bicep language and how Radius integrates with it."
weight: 99
---

Project Radius uses the [Bicep language](https://docs.microsoft.com/EN-US/azure/azure-resource-manager/bicep/) to describe your application and its resources.

## Bicep

The Bicep language makes it easy to model your infrastructure in a declarative way. This means you declare your resources and resource properties in a Bicep file, without writing a sequence of programming commands to create resources.

<iframe width="560" height="315" src="https://www.youtube.com/embed/kKIa8I6qF7I" title="YouTube video player" frameborder="0" allow="accelerometer; autoplay; clipboard-write; encrypted-media; gyroscope; picture-in-picture" allowfullscreen></iframe>

### Declarative vs. Imperative

For example, compare creating a database *declaratively* with Bicep, vs. *imperatively* with the az CLI:

{{< tabs "Bicep (Declarative)" "az CLI (Imperative)" >}}

{{% codetab %}}
The following template defines a CosmosDB account and MongoDB database. Deploying it to Azure will create the resources, and re-deploying it will update the resources to match the latest definition.

```sh
resource cosmosAccount 'Microsoft.DocumentDB/databaseAccounts@2021-04-15' = {
  name: 'myaccount'
  location: 'westus2'
  properties: {...}
  
  resource db 'mongodbDatabases' = {
    name: 'mydb'
    properties: {...}
  }

}
```

{{% /codetab %}}

{{% codetab %}}
The following commands will create CosmosDB account and MongoDB resources on each respective command entry. Re-running commands will throw an error as the resources have already been created.

```bash
$ az cosmosdb create \
    -n $accountName \
    -g $resourceGroupName \
    --kind MongoDB \
    --server-version $serverVersion
    ...
$ az cosmosdb create \
    -n $accountName \
    -g $resourceGroupName \
    --kind MongoDB \
    --server-version $serverVersion
```

{{% /codetab %}}

{{< /tabs >}}

## Install Bicep

Visit the [Radius getting started guide]({{< ref getting-started >}}) to install the Radius CLI, Bicep CLI and compiler, and the Bicep extension for VS Code.

## Radius resources

Project Radius resource types are available in Bicep, allowing you to model and connect Radius resources to Azure and Kubernetes resources.

In the below example, a Radius resource of type `radius.dev/Application@v1alpha3` is defined:" 

{{< rad file="snippets/app.bicep" embed=true replace-key-resources="//RESOURCES" replace-value-resources="..." >}}

## Next step

Next, we'll use the Bicep language to model your application's infrastructure:

{{< button page="bicep-infra" text="Model your infrastructure in Bicep" >}}
