//REST
//REST

resource eshop 'radius.dev/Application@v1alpha3' = {
  name: 'eshop'

  //REST
  //REST

  resource redisKeystore 'redislabs.com.RedisCache' = {
    name: 'redis-keystore'
    properties: {
      managed: true
    }
  }

  resource redisBasket 'redislabs.com.RedisCache' = {
    name: 'redis-basket'
    properties: {
      managed: true
    }
  }
  
}
