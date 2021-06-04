---
type: docs
title: "Azure CosmosDB Mongo"
linkTitle: "Azure CosmosDB Mongo"
description: "Sample application running MongoDB through Azure CosmosDB API"
---

This application showcases how Radius can use an Azure CosmosDB API for MongoDB in two different scenarios.

## Using a Radius-managed CosmosDB

This example sets the property `managed: true` for the CosmosDB component. When `managed` is set to true, Radius will manage the lifecycle of the underlying database account and database.

```sh
resource app 'radius.dev/Applications@v1alpha1' = {
  name: 'cosmos-container-managed'
  
  resource webapp 'Components' = {
    name: 'todoapp'
    kind: 'radius.dev/Container@v1alpha1'
    properties: {
      run: {
        container: {
          image: 'rynowak/node-todo:latest'
        }
      }
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

  resource db 'Components' = {
    name: 'db'
    kind: 'azure.com/CosmosDBMongo@v1alpha1'
    properties: {
      config: {
        managed: true
      }
    }
  }
}
```

{{< rad file="managed.bicep">}}

## Using a user-managed CosmosDB

This example sets the `resource` property to a CosmosDB Mongo database. Setting `managed: false` or using the default value allows you to explicitly specify a link to an Azure resource that you manage. When you supply your own `resource` value, Radius will not change or delete the resource you provide. 

In this example the CosmosDB resources are configured as part of the same `.bicep` template.

```sh
resource app 'radius.dev/Applications@v1alpha1' = {
  name: 'cosmos-container-unmanaged'
  
  resource webapp 'Components' = {
    name: 'todoapp'
    kind: 'radius.dev/Container@v1alpha1'
    properties: {
      run: {
        container: {
          image: 'rynowak/node-todo:latest'
        }
      }
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

  resource db 'Components' = {
    name: 'db'
    kind: 'azure.com/CosmosDBMongo@v1alpha1'
    properties: {
      config: {
        resource: account::db.id
      }
    }
  }
}

resource account 'Microsoft.DocumentDB/databaseAccounts@2020-04-01' = {
  name: 'account-${guid(resourceGroup().name)}'
  location: resourceGroup().location
  kind: 'MongoDB'
  tags: {
    radiustest: 'true'
  }
  properties: {
    consistencyPolicy: {
      defaultConsistencyLevel: 'Session'
    }
    locations: [
      {
        locationName: resourceGroup().location
        failoverPriority: 0
        isZoneRedundant: false
      }
    ]
    databaseAccountOfferType: 'Standard'
  }

  resource db 'mongodbDatabases' = {
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
```

{{< rad file="unmanaged.bicep">}}