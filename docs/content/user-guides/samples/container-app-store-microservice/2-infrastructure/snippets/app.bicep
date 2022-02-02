//APP
resource app 'radius.dev/Application@v1alpha3' existing = {
  name: 'store'
//APP

//COSMO
  resource statestore 'Container' = {
    name: 'statestore'
    params: {
      application: app.name
      stateStoreName: 'orders'
    }
  }
//COSMO
//REDIS
resource statestoreredis 'Container' = {
  name: 'statestoreredis'
  params: {
    application: app.name
    stateStoreName: 'orders'
  }
}
//REDIS
}
