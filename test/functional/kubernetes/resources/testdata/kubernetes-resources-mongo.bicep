param magpieimage string = 'radiusdev.azurecr.io/magpiego:latest'
param port int = 27017
param username string = 'mongoadmin'
param password string = 'secret'

resource app 'radius.dev/Application@v1alpha3' = {
  name: 'kubernetes-resources-mongo'

  resource webapp 'Container' = {
    name: 'webapp'
    properties: {
      container: {
        image: magpieimage
        readinessProbe:{
          kind: 'httpGet'
          containerPort: 3000
          path: '/healthz'
        }
      }
      connections: {
        mongodb: {
          kind: 'mongo.com/MongoDB'
          source: mongo.id
        }
      }
    }
  }

  resource mongoContainer 'Container' = {
    name: 'mongo-container'
    properties: {
      container: {
        image: 'mongo:5.0'
        ports: {
          mongodb: {
            containerPort: port
            provides: mongoRoute.id
          }
        }
        env: {
          MONGO_INITDB_ROOT_USERNAME: username
          MONGO_INITDB_ROOT_PASSWORD: password
        }
      }
    }
  }

  resource mongoRoute 'HttpRoute' = {
    name: 'mongo-route'
    properties: {
      port: port
    }
  }

  resource mongo 'mongo.com.MongoDatabase' = {
    name: 'mongo'
    properties: {
      secrets: {
        connectionString: 'mongodb://${username}:${password}@${mongoRoute.properties.host}:${mongoRoute.properties.port}'
      }
    }
  }
}
