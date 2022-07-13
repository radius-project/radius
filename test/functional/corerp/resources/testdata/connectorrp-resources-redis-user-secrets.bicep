import radius as radius

param environment string

resource app 'Applications.Core/applications@2022-03-15-privatepreview'  = {
  name: 'connectorrp-resources-redis-user-secrets'
  location: 'global'
  properties:{
    environment: environment
  }
}

resource redis 'Applications.Connector/redisCaches@2022-03-15-privatepreview' = {
  name: 'redis'
  location: 'global'

  properties: {
    environment: environment
    host: 'testHostname'
    port: 1234
    secrets: {
      connectionString: 'testConnectionString'
      password: ''
    }
  }
}
