resource app 'radius.dev/Applications@v1alpha1' = {
  name: 'dapr-tutorial'

  resource frontend 'Components' = {
    name: 'frontend'
    kind: 'radius.dev/Container@v1alpha1'
    properties: {
      run: {
        container: {
          image: 'radius.azurecr.io/daprtutorial-frontend'
        }
      }
      uses: [
        {
          binding: backend.properties.bindings.invoke
        }
      ]
      traits: [
        {
          kind: 'dapr.io/App@v1alpha1'
          appId: 'frontend'
        }
      ]
    }
  }

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
        invoke: {
          kind: 'dapr.io/Invoke'
        }
      }
      uses: [
        {
          binding: statestore.properties.bindings.default
        }
      ]
      traits: [
        {
          kind: 'dapr.io/App@v1alpha1'
          appId: 'backend'
          appPort: 3000
        }
      ]
    }
  }

  resource statestore 'Components' = {
    name: 'statestore'
    kind: 'dapr.io/StateStore@v1alpha1'
    properties: {
      config: {
        kind: 'state.azure.tablestorage'
        managed: true
      }
    }
  }
}
