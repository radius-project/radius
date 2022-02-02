resource app 'radius.dev/Application@v1alpha3' = {
  name: 'store'
}

module statestore 'statestore-redis.bicep' = {
  name: 'statestore'
  params: {
    application: app.name
    stateStoreName: 'orders'
  }
}
