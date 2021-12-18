//REST
//REST

resource redisCacheKeystore 'Microsoft.Cache/redis@2020-06-01' existing = {
  name: 'eshop-keystore'
}

resource redisCacheBasket 'Microsoft.Cache/redis@2020-06-01' existing = {
  name: 'eshop-basket'
}

resource eshop 'radius.dev/Application@v1alpha3' = {
  name: 'eshop'

  //REST
  //REST

  resource redisKeystore 'redislabs.com.RedisCache' = {
    name: 'redis-keystore'
    properties: {
      resource: redisCacheKeystore.id
    }
  }

  resource redisBasket 'redislabs.com.RedisCache' = {
    name: 'redis-basket'
    properties: {
      resource: redisCacheBasket.id
    }
  }

}
