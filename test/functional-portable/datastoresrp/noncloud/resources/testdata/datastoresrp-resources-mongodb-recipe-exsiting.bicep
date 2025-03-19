extension radius

param magpieimage string

param environment string
resource app 'Applications.Core/applications@2023-10-01-preview' = {
  name: 'dsrp-resources-mongodb-recipe-existing'
  location: 'global'
  properties: {
    environment: environment
    extensions: [
      {
        kind: 'kubernetesNamespace'
        namespace: 'dsrp-resources-mongodb-recipe-existing-app'
      }
    ]
  }
}

resource webapp 'Applications.Core/containers@2023-10-01-preview' = {
  name: 'mongodb-existing-app-ctnr'
  location: 'global'
  properties: {
    application: app.id
    connections: {
      mongodb: {
        source: mongodbExisting.id
      }
    }
    container: {
      image: magpieimage
      env: {
        DBCONNECTION: {
          value: mongodbExisting.listSecrets().connectionString
        }
      }
      readinessProbe: {
        kind: 'httpGet'
        containerPort: 3000
        path: '/healthz'
      }
    }
  }
}

resource mongodbExisting 'Applications.Datastores/mongoDatabases@2023-10-01-preview' existing = {
  name: 'mongodb-db-existing'
}
