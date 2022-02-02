module statestore 'statestore-redis.bicep' = {
  name: 'statestore'
  params: {
    application: app.name
    stateStoreName: 'orders'
  }
