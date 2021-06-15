---
type: docs
title: "Azure CosmosDB Mongo"
linkTitle: "Azure CosmosDB Mongo"
description: "Sample application running MongoDB through Azure CosmosDB API"
---

The `azure.com/CosmosDBMongo` component defined an [Azure CosmosDB](https://azure.microsoft.com/en-us/services/cosmos-db/) configured with a MongoDB API.

## Resource lifecycle

An `azure.com/CosmosDBMongo` Component can be deployed and managed by either by a user (unmanaged) or by Radius (managed).

## Using a Radius-managed CosmosDB

When `properties.config.managed` is set to `true`, Radius will manage the lifecycle of the underlying database account and database:

```sh
resource app 'radius.dev/Applications@v1alpha1' = {
  name: 'cosmos-container-managed'

  resource db 'Components' = {
    name: 'db'
    kind: 'azure.com/CosmosDBMongo@v1alpha1'
    properties: {
      config: {
        managed: true
      }
    }
  }

  resource webapp 'Components' = {
    name: 'todoapp'
    kind: 'radius.dev/Container@v1alpha1'
    properties: {
      run: {...}
      uses: [
        {
          binding: db.properties.bindings.mongo
          env: {
            DBCONNECTION: db.properties.bindings.mongo.connectionString
          }
        }
      ]
    }
  }
}
```

{{< rad file="managed.bicep">}}

## Using a user-managed CosmosDB

When `properties.config.managed` is set to `false` or ommitted, you can explicitly specify an existing Azure `resource` which you manage. This allows you to connect your Radius Components to existing databases. When you delete your application Radius will not change or delete your existing database.

In this example `Microsoft.DocumentDB/databaseAccounts` and `mongodbDatabases` resources are defined in Bicep, and then referenced in a Radius application. Note you can also use Bicep's [existing functionality](https://docs.microsoft.com/en-us/azure/azure-resource-manager/bicep/resource-declaration?tabs=azure-powershell#reference-existing-resources) to reference a resource that has previously been deployed.

```sh
resource account 'Microsoft.DocumentDB/databaseAccounts@2020-04-01' = {
  name: 'account-${guid(resourceGroup().name)}'
  location: resourceGroup().location
  kind: 'MongoDB'
  properties: {...}

  resource mongodb 'mongodbDatabases' = {
    name: 'mydb'
    properties: {
      resource: {
        id: 'mydb'
      }
      options: { 
        throughput: 400
      }
    }
  }
}

resource app 'radius.dev/Applications@v1alpha1' = {
  name: 'cosmos-container-usermanaged'

  resource db 'Components' = {
    name: 'db'
    kind: 'azure.com/CosmosDBMongo@v1alpha1'
    properties: {
      config: {
        resource: account::mongodb.id
      }
    }
  }

  resource webapp 'Components' = {
    name: 'todoapp'
    kind: 'radius.dev/Container@v1alpha1'
    properties: {
      run: {...}
      uses: [
        {
          binding: db.properties.bindings.mongo
          env: {
            DBCONNECTION: db.properties.bindings.mongo.connectionString
          }
        }
      ]
    }
  }

}

```

{{< rad file="usermanaged.bicep">}}
