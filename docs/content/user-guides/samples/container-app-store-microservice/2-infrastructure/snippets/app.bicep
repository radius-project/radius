//APP
resource app 'radius.dev/Application@v1alpha3' existing = {
  name: 'store'
//APP

//COSMO
  resource statestore 'mongodb.com.MongoDBComponent' = {
    name: 'statestore'
    properties: {
      managed: true
    }
  }
//COSMO
//REDIS
resource statestoreredis 'radius.dev/Application/dapr.io.StateStore@v1alpha3' = {
  name: 'statestoreredis'
    properties: {
      managed: true
    }
}
//REDIS
}
