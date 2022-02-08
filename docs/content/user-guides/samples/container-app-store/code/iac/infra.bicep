resource app 'radius.dev/Application@v1alpha3' = {
  name: 'store'

}

module statestore 'br:radius.azurecr.io/starters/dapr-statestore-azure-tablestorage:latest' = {
  name: 'orders'
  params: {
    radiusApplication: app
    stateStoreName: 'orders'
  }
}
