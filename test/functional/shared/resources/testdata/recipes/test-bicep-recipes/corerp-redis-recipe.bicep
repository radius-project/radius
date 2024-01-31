param redisName string

resource redis 'Microsoft.Cache/redis@2022-06-01' = {
  name: 'rds-${uniqueString(resourceGroup().id, redisName, deployment().name)}'
  location: 'global'
  properties: {
    enableNonSslPort: false
    minimumTlsVersion: '1.2'
    sku: {
      family: 'C'
      capacity: 1
      name: 'Basic'
    }
  }
}

output result object = {
  values: {
    host: redis.properties.hostName
    port: redis.properties.port
  }
  secrets: {
    connectionString: 'redis://${redis.properties.hostName}:${redis.properties.port}'
    #disable-next-line outputs-should-not-contain-secrets
    password: redis.listKeys().primaryKey
  }
}
