//REST
//REST

resource redisCache 'Microsoft.Cache/redis@2020-06-01' existing = {
  name: 'eshop'
}

resource eshop 'radius.dev/Application@v1alpha3' = {
  name: 'eshop'

  //REST
  //REST

  resource redis 'redislabs.com.RedisComponent' = {
    name: 'redis'
    properties: {
      resource: redisCache.id
    }
  }

}
