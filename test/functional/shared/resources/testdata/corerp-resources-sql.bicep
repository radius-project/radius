import radius as radius

@description('Specifies the location for resources.')
param location string = 'global'

@description('Specifies the image for the container resource.')
param magpieImage string

@description('Specifies the port for the container resource.')
param magpiePort int = 3000

@description('Specifies the environment for resources.')
param environment string = 'test'

@description('Specifies the image for the sql container resource.')
param sqlImage string = 'mcr.microsoft.com/mssql/server:2019-latest'

@description('Specifies the port for the container resource.')
param sqlPort int = 1433

@description('Specifies the SQL username.')
param username string = 'sa'

@description('Specifies the SQL password.')
@secure()
param password string = newGuid()

resource app 'Applications.Core/applications@2022-03-15-privatepreview' = {
  name: 'corerp-resources-sql'
  location: location
  properties: {
    environment: environment
  }
}

resource webapp 'Applications.Core/containers@2022-03-15-privatepreview' = {
  name: 'sql-app-ctnr-old'
  location: location
  properties: {
    application: app.id
    connections: {
      sql: {
        source: db.id
      }
    }
    container: {
      image: magpieImage
      env: {
        CONNECTION_SQL_CONNECTIONSTRING: db.connectionString()
      }
      readinessProbe: {
        kind: 'httpGet'
        containerPort: magpiePort
        path: '/healthz'
      }
    }
  }
}

resource db 'Applications.Link/sqlDatabases@2022-03-15-privatepreview' = {
  name: 'sql-db-old'
  location: location
  properties: {
    application: app.id
    environment: environment
    server: sqlRoute.properties.hostname
    database: 'master'
    resourceProvisioning: 'manual'
    port: sqlRoute.properties.port
    username: username
    secrets:{
      password: password
    }
  }
}

resource sqlRoute 'Applications.Core/httpRoutes@2022-03-15-privatepreview' = {
  name: 'sql-rte-old'
  location: location
  properties: {
    application: app.id
    port: sqlPort
  }
}

resource sqlContainer 'Applications.Core/containers@2022-03-15-privatepreview' = {
  name: 'sql-ctnr-old'
  location: location
  properties: {
    application: app.id
    container: {
      image: sqlImage
      env: {
        ACCEPT_EULA: 'Y'
        MSSQL_PID: 'Developer'
        MSSQL_SA_PASSWORD: password
      }
      ports: {
        sql: {
          containerPort: sqlPort
          provides: sqlRoute.id
        }
      }
    }
  }
}
