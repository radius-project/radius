resource app 'radius.dev/Applications@v1alpha1' = {
  name: 'webapp'

  resource todoapplication 'Components' = {
    name: 'todoapp'
    kind: 'radius.dev/Container@v1alpha1'
    properties: {
      run: {
        container: {
          image: 'radius.azurecr.io/webapptutorial-todoapp'
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
