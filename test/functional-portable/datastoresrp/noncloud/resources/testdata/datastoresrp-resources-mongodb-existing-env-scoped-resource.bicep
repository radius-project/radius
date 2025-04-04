extension radius

param magpieimage string

param environment string
resource app 'Applications.Core/applications@2023-10-01-preview' = {
  name: 'mongodb-recipe-existing'
  location: 'global'
  properties: {
    environment: environment
    extensions: [
      {
        kind: 'kubernetesNamespace'
        namespace: 'mongodb-recipe-existing-app'
      }
    ]
  }
}

resource webapp 'Applications.Core/containers@2023-10-01-preview' = {
  name: 'mongo-ctnr-exst'
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
  name: 'existing-mongodb'
}
