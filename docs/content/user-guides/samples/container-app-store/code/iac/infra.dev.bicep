resource app 'radius.dev/Application@v1alpha3' = {
  name: 'container-app-store'

  resource statestore 'dapr.io.StateStore' = {
    name: 'orders'
    properties: {
      kind: 'state.redis'
      managed: true
    }
  }

}
