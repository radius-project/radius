import radius as radius
@description('Specifies the location for resources.')
param location string = 'global'

@description('Specifies the image for the container resource.')
param magpieImage string

@description('Specifies the port for the container resource.')
param magpiePort int = 3000

@description('Specifies the port for the container resource.')
param sqlPort int = 1433

@description('Specifies the SQL username.')
param username string = 'sa'

@description('Specifies the SQL password.')
@secure()
param password string = newGuid()

param registry string

param version string

resource env 'Applications.Core/environments@2022-03-15-privatepreview' = {
  name: 'corerp-resources-environment-sql-recipe-env'
  location: 'global'
  properties: {
    compute: {
      kind: 'kubernetes'
      resourceId: 'self'
      namespace: 'corerp-resources-environment-sql-recipe-env'
    }
    recipes: {
      'Applications.Link/sqlDatabases': {
        default: {
          templateKind: 'bicep'
          templatePath: '${registry}/test/functional/corerp/recipes/sqldb-recipe:${version}'
          parameters: {
            username: username
            password: password
          }
        }
      }
    }
  }
}

resource app 'Applications.Core/applications@2022-03-15-privatepreview' = {
  name: 'corerp-resources-sqldb-recipe'
  location: location
  properties: {
    environment: env.id
    extensions: [
      {
        kind: 'kubernetesNamespace'
        namespace: 'corerp-resources-sqldb-recipe-app'
      }
    ]
  }
}

resource webapp 'Applications.Core/containers@2022-03-15-privatepreview' = {
  name: 'sql-recipe-app-ctnr'
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
        CONNECTION_SQL_CONNECTIONSTRING: 'Data Source=tcp:${db.properties.server},${sqlPort};Initial Catalog=${db.properties.database};User Id=${username};Password=${password};Encrypt=True;TrustServerCertificate=True'
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
  name: 'sql-db-recipe'
  location: location
  properties: {
    application: app.id
    environment: env.id
  }
}
