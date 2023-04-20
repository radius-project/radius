import radius as radius

param magpieimage string

param environment string

param mongodbresourceid string

resource app 'Applications.Core/applications@2023-04-15-preview' = {
  name: 'corerp-resources-mongodb'
  location: 'global'
  properties: {
    environment: environment
  }
}

resource webapp 'Applications.Core/containers@2023-04-15-preview' = {
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

resource db 'Applications.Link/mongoDatabases@2023-04-15-preview' = {
  name: 'mdb-db'
  location: 'global'
  properties: {
    application: app.id
    environment: environment
    mode: 'resource'
    resource: mongodbresourceid
  }
}

