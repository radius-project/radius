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

resource myapp 'radius.dev/Application@v1alpha3' = {
  name: 'my-application'

  resource mongo 'mongodb.com.MongoDBComponent' = {
    name: 'mongo'
    properties: {
      resource: cosmos::db.id
    }
  }

  resource frontendHttp 'HttpRoute' = {
    name: 'frontend-http'
    properties: {
      port: 80
      gateway: {
        hostname: '*'
      }
    }
  }

  resource frontend 'ContainerComponent' = {
    name: 'frontend'
    properties: {
      container: {
        image: 'nginx:latest'
      }
      connections: {
        backend: {
          kind: 'Http'
          source: backendHttp.id
        }
      }
    }
  }

  resource backendHttp 'HttpRoute' = {
    name: 'backend-http'
    properties: {
      port: 80
    }
  }

  resource backend 'ContainerComponent' = {
    name: 'backend'
    properties: {
      container: {
        image: 'nginx:latest'
        ports: {
          web: {
            containerPort: 80
            provides: backendHttp.id
          }
        }
      }
      connections: {
        mongo: {
          kind: 'mongo.com/MongoDB'
          source: mongo.id
        }
      }
    }
  }

}
