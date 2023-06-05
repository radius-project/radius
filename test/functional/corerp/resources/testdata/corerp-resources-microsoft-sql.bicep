import radius as radius

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

resource app 'Applications.Core/applications@2022-03-15-privatepreview' = {
  name: 'corerp-resources-microsoft-sql'
  location: location
  properties: {
    environment: environment
  }
}

resource sqlapp 'Applications.Core/containers@2022-03-15-privatepreview' = {
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
        CONNECTION_SQL_CONNECTIONSTRING: 'Data Source=tcp:${db.properties.server},1433;Initial Catalog=${db.properties.database};User Id=${adminUsername}@${db.properties.server};Password=${adminPassword};Encrypt=true'
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
  name: 'mssql-db'
  location: location
  properties: {
    application: app.id
    environment: environment
    resourceProvisioning: 'manual'
    resources:[
      {
        id:mssqlresourceid
      }
    ]
    database: database
    server: server
  }
}
