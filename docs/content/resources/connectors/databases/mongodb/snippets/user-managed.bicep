//BICEP
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
//BICEP

resource app 'radius.dev/Application@v1alpha3' = {
  name: 'cosmos-container-usermanaged'
  
  //SAMPLE
  resource db 'mongo.com.MongoDatabase' = {
    name: 'db'
    properties: {
      resource: account::mongodb.id
    }
  }
  //SAMPLE

  resource webapp 'Container' = {
    name: 'todoapp'
    properties: {
      //HIDE
      container: {
        image: 'rynowak/node-todo:latest'
        env: {
          DBCONNECTION: db.id
        }
      }
      //HIDE
      connections: {
        mongo: {
          kind: 'mongo.com/MongoDB'
          source: db.id
          
        }
      }
    }
  }
}
