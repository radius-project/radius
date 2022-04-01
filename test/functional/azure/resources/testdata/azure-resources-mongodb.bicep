param magpieimage string = 'radiusdev.azurecr.io/magpiego:latest' string = 'radiusdev.azurecr.io/magpiego:latest' string

resource app 'radius.dev/Application@v1alpha3' = {
  name: 'azure-resources-mongodb'
  
  resource webapp 'Container' = {
    name: 'todoapp'
    properties: {
      connections: {
        mongodb: {
          kind: 'mongo.com/MongoDB'
          source: db.id
        }
      }
      container: {
        image: magpieimage
        readinessProbe:{
          kind:'httpGet'
          containerPort:3000
          path: '/healthz'
        }
      }
    }
  }

  resource db 'mongo.com.MongoDatabase' = {
    name: 'db'
    properties: {
      resource: account::dbinner.id
    }
  }
}

resource account 'Microsoft.DocumentDB/databaseAccounts@2020-04-01' = {
  name: 'account-${guid(resourceGroup().name)}'
  location: resourceGroup().location
  kind: 'MongoDB'
  tags: {
    radiustest: 'azure-resources-mongodb'
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

  resource dbinner 'mongodbDatabases' = {
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
