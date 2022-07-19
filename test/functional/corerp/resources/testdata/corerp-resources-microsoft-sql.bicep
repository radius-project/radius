import radius as radius

@description('Specifies the location for resources.')
param location string = resourceGroup().location

@description('Specifies the image for the container resource.')
param magpieImage string = 'radiusdev.azurecr.io/magpiego:latest'

@description('Specifies the port for the container resource.')
param magpiePort int = 3000

@description('Specifies the environment for resources.')
param environment string = 'test'

@description('Specifies the port for the container resource.')
param sqlPort int = 1433

@description('Specifies the SQL username.')
param adminUsername string = 'cooluser'

@description('Specifies the SQL password.')
param adminPassword string = 'p@ssw0rd'

resource app 'Applications.Core/applications@2022-03-15-privatepreview' = {
  name: 'corerp-resources-microsoft-sql'
  location: location
  properties: {
    environment: environment
  }
}

resource sqlapp 'Applications.Core/containers@2022-03-15-privatepreview' = {
  name: 'app'
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

resource db 'Applications.Connector/sqlDatabases@2022-03-15-privatepreview' = {
  name: 'db'
  location: location
  properties: {
    application: app.id
    environment: environment
    server: sqlRoute.properties.hostname
    database: 'master'
  }
}

resource sqlRoute 'Applications.Core/httpRoutes@2022-03-15-privatepreview' = {
  name: 'route'
  location: location
  properties: {
    application: app.id
    port: sqlPort
  }
}

resource server 'Microsoft.Sql/servers@2021-02-01-preview' = {
  name: 'sql-${uniqueString(resourceGroup().id)}'
  location: location
  tags: {
    radiustest: 'corerp-resources-microsoft-sql'
  }
  properties: {
    administratorLogin: adminUsername
    administratorLoginPassword: adminPassword
  }

  resource dbinner 'databases' = {
    name: 'cool-database'
    location: location
  }

  resource firewall 'firewallRules' = {
    name: 'allow'
    properties: {
      startIpAddress: '0.0.0.0'
      endIpAddress: '0.0.0.0'
    }
  }
}
