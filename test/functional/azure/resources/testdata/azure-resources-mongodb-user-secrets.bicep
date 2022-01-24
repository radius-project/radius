@description('Admin username for the Mongo database. Default is "admin"')
param username string = 'admin'

@description('Admin password for the Mongo database')
@secure()
param password string = newGuid()

resource app 'radius.dev/Application@v1alpha3' = {
  name: 'azure-resources-mongodb-user-secrets'

  resource webapp 'Container' = {
    name: 'todoapp'
    properties: {
      connections: {
        mongodb: {
          kind: 'mongo.com/MongoDB'
          source: mongo.id
        }
      }
      container: {
        image: 'radius.azurecr.io/magpie:latest'
      }
    }
  }

	// https://hub.docker.com/_/mongo/
  resource mongoContainer 'Container' = {
    name: 'mongo'
    properties: {
      container: {
        image: 'mongo:4.2'
        env: {
          MONGO_INITDB_ROOT_USERNAME: username
          MONGO_INITDB_ROOT_PASSWORD: password
        }
        ports: {
          mongo: {
            containerPort: 27017
            provides: mongoRoute.id
          }
        }
      }
    }
  }

  resource mongoRoute 'HttpRoute' = {
    name: 'mongo-route'
    properties: {
      port: 27017
    }
  }

  resource mongo 'mongo.com.MongoDatabase' = {
    name: 'mongo'
    properties: {
      secrets: {
        connectionString: 'mongodb://${username}:${password}@${mongoRoute.properties.host}:${mongoRoute.properties.port}'
        username: username
        password: password
      }
    }
  }
}
