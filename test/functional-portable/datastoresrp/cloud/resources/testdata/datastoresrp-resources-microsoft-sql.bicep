extension radius

@description('Specifies the location for resources.')
param location string = 'East US'

@description('Specifies the image for the container resource.')
param magpieImage string

@description('Specifies the port for the container resource.')
param magpiePort int = 3000

@description('Specifies the environment for resources.')
param environment string = 'test'

@description('Specifies the SQL username.')
param adminUsername string

@description('Specifies the SQL password.')
@secure()
param adminPassword string

param mssqlresourceid string

param database string

param server string

resource app 'Applications.Core/applications@2023-10-01-preview' = {
  name: 'dsrp-resources-microsoft-sql'
  location: location
  properties: {
    environment: environment
  }
}

resource sqlapp 'Applications.Core/containers@2023-10-01-preview' = {
  name: 'mssql-app-ctnr'
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
        CONNECTION_SQL_CONNECTIONSTRING: {
          value: db.listSecrets().connectionString
        }
      }
      readinessProbe: {
        kind: 'httpGet'
        containerPort: magpiePort
        path: '/healthz'
      }
    }
  }
}

resource db 'Applications.Datastores/sqlDatabases@2023-10-01-preview' = {
  name: 'mssql-db'
  location: location
  properties: {
    application: app.id
    environment: environment
    resourceProvisioning: 'manual'
    resources: [
      {
        id: mssqlresourceid
      }
    ]
    database: database
    server: server
    port: 1433
    username: adminUsername
    secrets: {
      password: adminPassword
    }
  }
}
