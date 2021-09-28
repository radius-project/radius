//COSMOS
resource account 'Microsoft.DocumentDB/databaseAccounts@2020-04-01' = {
  name: 'account-${guid(resourceGroup().name)}'
  location: resourceGroup().location
  kind: 'MongoDB'
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
//COSMOS

resource app 'radius.dev/Application@v1alpha3' = {
  name: 'cosmos-container-usermanaged'

  //SAMPLE
  resource db 'azure.com.CosmosDBMongoComponent' = {
    name: 'db'
    properties: {
      resource: account::mongodb.id
    }
  }
  //SAMPLE

  resource webapp 'ContainerComponent@v1alpha3' = {
    name: 'todoapp'
    properties: {
      //HIDE
      container: {
        image: 'rynowak/node-todo:latest'
      }
      //HIDE
      connections: {
        db: {
          kind: 'mongo.com/MongoDB'
          source: db.id
        }
      }
    }
  }
}
