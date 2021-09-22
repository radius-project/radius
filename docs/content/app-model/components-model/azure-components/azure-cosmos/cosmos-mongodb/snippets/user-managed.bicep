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

resource app 'radius.dev/Applications@v1alpha1' = {
  name: 'cosmos-container-usermanaged'
  
  //SAMPLE
  resource db 'Components' = {
    name: 'db'
    kind: 'azure.com/CosmosDBMongo@v1alpha1'
    properties: {
      config: {
        resource: account::mongodb.id
      }
    }
  }
  //SAMPLE

  resource webapp 'Components' = {
    name: 'todoapp'
    kind: 'radius.dev/Container@v1alpha1'
    properties: {
      //HIDE
      run: {
        container: {
          image: 'rynowak/node-todo:latest'
        }
      }
      //HIDE
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
