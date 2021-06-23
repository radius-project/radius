resource app 'radius.dev/Applications@v1alpha1' = {
  name: 'dapr-hello'

  resource nodeapplication 'Components' = {
    name: 'nodeapp'
    kind: 'radius.dev/Container@v1alpha1'
    properties: {
      run: {
        container: {
          image: 'radiusteam/tutorial-nodeapp'
        }
      }
      bindings: {
        web: {
          kind: 'http'
          targetPort: 3000
        }
        invoke: {
          kind: 'dapr.io/Invoke'
        }
      }
      traits: [
        {
          kind: 'dapr.io/App@v1alpha1'
          appId: 'nodeapp'
          appPort: 3000
        }
      ]
    }
  }
}
