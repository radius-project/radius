import radius as radius

param environment string

param location string = resourceGroup().location

resource app 'Applications.Core/applications@2022-03-15-privatepreview' = {
  name: 'connectorrp-resources-mongodb'
  location: 'global'
  properties: {
    environment: environment
  }
}

resource db 'Applications.Connector/mongoDatabases@2022-03-15-privatepreview' = {
  name: 'db'
  location: 'global'
  properties: {
    environment: environment
    resource: account::dbinner.id
  }
}

resource account 'Microsoft.DocumentDB/databaseAccounts@2020-04-01' = {
  name: 'account-${guid(resourceGroup().name)}'
  location: location
  kind: 'MongoDB'
  tags: {
    radiustest: 'connectorrp-resources-mongodb'
  }
  properties: {
    consistencyPolicy: {
      defaultConsistencyLevel: 'Session'
    }
    locations: [
      {
        locationName: location
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
