import radius as radius

@description('Admin username for the Mongo database. Default is "admin"')
param username string = 'admin'

@description('Admin password for the Mongo database')
@secure()
param password string = newGuid()

param environment string = 'West US'

param magpieimage string = 'radiusdev.azurecr.io/magpiego:latest'

resource app 'Applications.Core/applications@2022-03-15-privatepreview' = {
  name: 'corerp-resources-sqldb-app'
  location: 'global'
  properties: {
    environment: environment
  }
}

resource webapp 'Applications.Core/containers@2022-03-15-privatepreview' = {
  name: 'todoapp'
  location: 'global'
  properties: {
    application: app.id
    connections: {
      sql: {
        source: db.id
      }
    }
    container: {
      image: magpieimage
      env: {
        CONNECTION_SQL_CONNECTIONSTRING: 'Data Source=tcp:${db.properties.server},${sqlRoute.properties.port};Initial Catalog=${db.properties.database};User Id=${username};Password=${password};Encrypt=True;TrustServerCertificate=True'
      }
      readinessProbe: {
        kind: 'httpGet'
        containerPort: 3000
        path: '/healthz'
      }
    }
  }
}

resource db 'Applications.Connector/sqlDatabases@2022-03-15-privatepreview' = {
  name: 'db'
  location: 'global'
  properties: {
    environment: environment
    server: sqlRoute.properties.hostname
    database: 'master'
  }
}

resource sqlRoute 'Applications.Core/httpRoutes@2022-03-15-privatepreview' = {
  name: 'sql-route'
  location: 'global'
  properties: {
    application: app.id
    port: 27017
  }
}

resource sqlContainer 'Applications.Core/containers@2022-03-15-privatepreview' = {
  name: 'sql-container'
  location: 'global'
  properties: {
    application: app.id
    container: {
      image: 'mcr.microsoft.com/mssql/server:2019-latest'
      env: {
        // DBCONNECTION: mongo.connectionString()
        ACCEPT_EULA: 'Y'
        MSSQL_PID: 'Developer'
        MSSQL_SA_PASSWORD: password
      }
      ports: {
        sql: {
          containerPort: 1433
          provides: sqlRoute.id
        }
      }
    }
    connections: {}
  }
}
