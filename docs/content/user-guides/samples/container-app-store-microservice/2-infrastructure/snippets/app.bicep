//APP
resource app 'radius.dev/Application@v1alpha3' existing = {
  name: 'store'
//APP

//COSMO
  resource statestore 'Microsoft.DocumentDB/databaseAccounts@2021-10-15' = {
    name: 'statestore'
    params: {
      application: app.name
      stateStoreName: 'orders'
    }
  }
//COSMO
//REDIS
resource statestoreredis 'radius.dev/Application/dapr.io.StateStore@v1alpha3' = {
  name: 'statestoreredis'
  params: {
    application: app.name
    stateStoreName: 'orders'
  }
}
//REDIS
}
