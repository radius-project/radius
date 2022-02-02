//APP
resource app 'radius.dev/Application@v1alpha3' existing = {
  name: 'store'
//APP

//COSMO
  resource statestore 'statestore-cosmos' = {
    name: 'statestore'
    params: {
      application: app.name
      stateStoreName: 'orders'
    }
  }
//COSMO
//REDIS
resource statestoreredis 'statestore-redis' = {
  name: 'statestoreredis'
  params: {
    application: app.name
    stateStoreName: 'orders'
  }
}
//REDIS
}
