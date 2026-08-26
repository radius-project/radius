extension radius

@description('Specifies the location for resources.')
param location string = 'global'

@description('Specifies the environment for resources.')
param environment string

@secure()
@description('Administrator password for the PostgreSQL database.')
param password string

resource app 'Radius.Core/applications@2025-08-01-preview' = {
  name: 'corerp-resources-postgresqldb'
  location: location
  properties: {
    environment: environment
  }
}

resource postgresql 'Radius.Data/postgreSqlDatabases@2025-08-01-preview' = {
  name: 'postgresqldb-db'
  location: location
  properties: {
    environment: environment
    application: app.id
    database: 'appdb'
    username: 'admin'
    password: password
  }
}
