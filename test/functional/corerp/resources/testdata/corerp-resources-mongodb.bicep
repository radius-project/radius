import radius as radius

param magpieimage string

param environment string

param location string = resourceGroup().location

resource app 'Applications.Core/applications@2022-03-15-privatepreview' = {
  name: 'corerp-resources-mongodb'
  location: 'global'
  properties: {
    environment: environment
  }
}

resource webapp 'Applications.Core/containers@2022-03-15-privatepreview' = {
  name: 'mdb-app-ctnr'
  location: 'global'
  properties: {
    application: app.id
    connections: {
      mongodb: {
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

resource db 'Applications.Link/mongoDatabases@2022-03-15-privatepreview' = {
  name: 'mdb-db'
  location: 'global'
  properties: {
    application: app.id
    environment: environment
    mode: 'resource'
    resource: account::dbinner.id
  }
}

resource account 'Microsoft.DocumentDB/databaseAccounts@2020-04-01' = {
  name: 'account-${guid(resourceGroup().name)}'
  location: location
  kind: 'MongoDB'
  tags: {
    radiustest: 'corerp-resources-mongodb'
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
    name: 'mdb-mydb'
    properties: {
      resource: {
        id: 'mdb-mydb'
      }
      options: { 
        throughput: 400
      }
    }
  }
}
