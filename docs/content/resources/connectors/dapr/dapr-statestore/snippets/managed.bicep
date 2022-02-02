resource app 'radius.dev/Application@v1alpha3' = {
  name: 'myapp'

  //SAMPLE
  resource statestore 'dapr.io.StateStore' = {
    name: 'statestore'
    properties: {
      kind: 'any'
    }
  }
  //SAMPLE
}
