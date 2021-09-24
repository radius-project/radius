---
type: docs
title: "Add Radius into your existing application"
linkTitle: "Add to existing apps"
description: "Learn how to adopt Radius into your existing applications"
weight: 300
---

Have an existing application and want to begin using Radius? It's easy to incrementally adopt Radius and start taking advtage of its features.

## Model your infrastructure in Bicep

[Azure Bicep](https://docs.microsoft.com/EN-US/azure/azure-resource-manager/bicep/) makes it easy to model your infrastructure in a declarative way. This means you declare your resources and resource properties in a Bicep file, without writing a sequence of programming commands to create resources.

### Install Bicep

Visit the [Radius getting started guide]({{< ref getting-started >}}) to install the Radius CLI, Bicep CLI and compiler, and the Bicep extension for VS Code.

### Model with Bicep

To begin adopting Radius, begin by modeling your infrastructure in a Bicep file. This can be done by declaring the resource and deploying it via the Bicep file, or by using the `existing` keyword to reference an existing resource that is deployed and managed by another service.

{{< tabs "Model and deploy with Bicep" "Reference an existing resource" >}}

{{% codetab %}}
The following example shows an Azure CosmosDB account and MondoDB database that will be deployed with Bicep. This is useful if you want to leverage Bicep and Azure to manage the lifecycle of your resource:

```bicep
param locationParam string = 'westus2'
resource cosmosAccount 'Microsoft.DocumentDB/databaseAccounts@2021-04-15' = {
  name: 'myaccount'
  location: locationParam
  properties: {
    enableFreeTier: true
    databaseAccountOfferType: 'Standard'
    consistencyPolicy: {
      defaultConsistencyLevel: 'Session'
    }
    locations: [
      {
        locationName: location
      }
    ]
  }
}

resource cosmosMongo 'Microsoft.DocumentDB/databaseAccounts/mongodbDatabases@2021-06-15' = {
  name: '${cosmosAccount.name}/mydatabase'
  location: locationParam
  properties: {
    options: {
      throughput: 400
    }
    resource: {
      id: 'mydatabase'
    }
  }
}
```

{{% /codetab %}}

{{% codetab %}}
Alternately, you can [reference an existing resource](https://docs.microsoft.com/en-us/azure/azure-resource-manager/bicep/resource-declaration?tabs=azure-powershell#reference-existing-resources) that is deployed and managed by another service. Here's an example of a CosmosDB resource that is deployed by another service:

```bicep
resource cosmosMongo 'Microsoft.DocumentDB/databaseAccounts/mongodbDatabases@2021-06-15' existing = {
    name: 'myaccount/mydatabase'
}
```
{{% /codetab %}}

{{< /tabs >}}

You can now use the `cosmosMongo` resource in your Radius application.

## Model your services in Radius

Now that your infrastructure is modeled in Bicep, you can model your services with Radius resources. In these examples we will be modeling a `website` service that will connect to the MongoDB database from above.

### Connect to a resource directly

Radius allows you to directly reference a Bicep resource from a Radius application. This is useful if you deploy to a single platform and don't need portability or abstraction.

```bicep
resource myapp 'radius.dev/Application@v1alpha3' = {
  name: 'myapp'
  resource website 'ContainerComponent@v1alpha3' = {
    name: 'website'
    properties: {
      container: {
        image: 'latest'
      }
      connections: {
        datastore: {
          resource: cosmosMongo.id
        }
      }
    }
  }
}
```

Once deployed, you can now access the connection string of the CosmosDb and use it in your Radius application.

### Connect using an abstraction

If you application is deployed to multiple platforms, you can use an abstraction to connect to the resource. This allows you to swap out the underlying resource implementation between platforms without changing your Radius application.

```bicep
resource myapp 'radius.dev/Application@v1alpha3' = {
  name: 'myapp'

  resource mongo 'mongo.com.mongoDB@v1alpha3' = {
    name: 'mongodb'
    properties: {
      config: {
        resource: cosmosMongo.id
      }
    }
  }

  resource website 'ContainerComponent@v1alpha3' = {
    name: 'website'
    properties: {
      container: {
        image: 'nginx:latest'
      }
      connections: {
        datastore: {
          resource: mongo.id
        }
      }
    }
  }
}
```

This application can now be deployed to Microsoft Azure, or the `cosmosMongo` resource can be swapped for a different resource implementation such as `kubernetesMongo` and be deployed to Kubernetes.

For a full list of abstraction components visit the [Components docs]({{< ref components-model >}}).
