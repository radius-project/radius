import radius as radius

param environment string

resource app 'Applications.Core/applications@2022-03-15-privatepreview' = {
  name: 'connectorrp-resources-mongodb-user-secrets'
  location: 'global'
  properties: {
    environment: environment
  }
}

resource mongo 'Applications.Connector/mongoDatabases@2022-03-15-privatepreview' = {
  name: 'mongo'
  location: 'global'
  properties: {
    environment: environment
    secrets: {
      connectionString: 'testConnectionString'
      username: 'testUser'
      password: 'testPassword'
    }
  }
}
