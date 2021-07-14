resource app 'radius.dev/Applications@v1alpha1' = {
  name: 'dapr-tutorial'

  resource backend 'Components' = {
    name: 'backend'
    kind: 'radius.dev/Container@v1alpha1'
    properties: {
      run: {
        container: {
          image: 'radius.azurecr.io/daprtutorial-backend'
        }
      }
      bindings: {
        web: {
          kind: 'http'
          targetPort: 3000
        }
      }
    }
  }
}
