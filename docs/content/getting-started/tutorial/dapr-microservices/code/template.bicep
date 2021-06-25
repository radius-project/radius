resource app 'radius.dev/Applications@v1alpha1' = {
  name: 'dapr-hello'

  resource backend 'Components' = {
    name: 'backend'
    kind: 'radius.dev/Container@v1alpha1'
    properties: {
      run: {
        container: {
          image: 'radiusteam/dapr-hello-nodeapp:latest-linux-amd64'
        }
      }
      uses: [
        {
          binding: statestore.properties.bindings.default
        }
      ]
      bindings: {
        invoke: {
          kind: 'dapr.io/Invoke'
        }
      }
      traits: [
        {
          kind: 'dapr.io/App@v1alpha1'
          appId: 'backend'
          appPort: 3000
        }
      ]
    }
  }

  resource frontend 'Components' = {
    name: 'frontend'
    kind: 'radius.dev/Container@v1alpha1'
    properties: {
      run: {
        container: {
          image: 'radiusteam/dapr-hello-ui:latest-linux-amd64'
        }
      }
      uses: [
        {
          binding: backend.properties.bindings.invoke
        }
      ]
      bindings: {
        web: {
          kind: 'http'
          targetPort: 80
        }
      }
      traits: [
        {
          kind: 'dapr.io/App@v1alpha1'
          appId: 'frontend'
        }
      ]
    }
  }

  resource statestore 'Components' = {
    name: 'statestore'
    kind: 'dapr.io/StateStore@v1alpha1'
    properties: {
      config: {
        kind: 'any'
        managed: true
      }
    }
  }
}
