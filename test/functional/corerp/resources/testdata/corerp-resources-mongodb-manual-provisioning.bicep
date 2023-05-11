import radius as radius

@description('Admin username for the Mongo database. Default is "admin"')
param username string = 'admin'

@description('Admin password for the Mongo database')
@secure()
param password string = newGuid()

param environment string

param magpieimage string

resource app 'Applications.Core/applications@2022-03-15-privatepreview' = {
  name: 'corerp-resources-mongodb-manual-provisioning'
  location: 'global'
  properties: {
    environment: environment
  }
}

resource webapp 'Applications.Core/containers@2022-03-15-privatepreview' = {
  name: 'mdb-us-app-ctnr'
  location: 'global'
  properties: {
    application: app.id
    connections: {
      mongodb: {
        source: mongo.id
      }
    }
    container: {
      image: magpieimage
    }
  }
}


// https://hub.docker.com/_/mongo/
resource mongoContainer 'Applications.Core/containers@2022-03-15-privatepreview' = {
  name: 'mdb-us-ctnr'
  location: 'global'
  properties: {
    application: app.id
    container: {
      image: 'mongo:4.2'
      env: {
        DBCONNECTION: mongo.connectionString()
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
    connections: {}
  }
}

resource mongoRoute 'Applications.Core/httproutes@2022-03-15-privatepreview' = {
  name: 'mdb-us-rte'
  location: 'global'
  properties: {
    application: app.id
    port: 27017
  }
}

resource mongo 'Applications.Link/mongoDatabases@2022-03-15-privatepreview' = {
  name: 'mdb-us-db'
  location: 'global'
  properties: {
    application: app.id
    environment: environment
    host: mongoRoute.properties.hostname
    port: mongoRoute.properties.port
    secrets: {
      connectionString: 'mongodb://${username}:${password}@${mongoRoute.properties.hostname}:${mongoRoute.properties.port}'
      username: username
      password: password
    }
  }
}
