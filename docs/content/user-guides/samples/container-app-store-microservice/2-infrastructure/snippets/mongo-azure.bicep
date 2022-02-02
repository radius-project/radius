module statestore 'statestore-cosmos.bicep' = {
  name: 'statestore'
  params: {
    application: app.name
    stateStoreName: 'orders'
  }
}
