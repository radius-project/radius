param app object
param location string = 'westus2'

resource cosmos 'Microsoft.DocumentDB/databaseAccounts@2021-04-15' = {
  name: 'myaccount'
  location: location
  properties: {
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

  resource db 'mongodbDatabases' = {
    name: 'mydb'
    properties: {
      resource: {
        id: 'mydatabase'
      }
      options: {
        throughput: 400
      }
    }
  }
}

resource myapp 'radius.dev/Application@v1alpha3' existing = {
  name: app.name

  resource mongo 'mongo.com.MongoDatabase' = {
    name: 'mongo'
    properties: {
      resource: cosmos::db.id
    }
  }

}

output mongo object = myapp::mongo
