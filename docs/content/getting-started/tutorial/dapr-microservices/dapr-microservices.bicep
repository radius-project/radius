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
      uses: [
        {
          binding: statestore.properties.bindings.default
        }
      ]
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

  resource pythonapplication 'Components' = {
    name: 'pythonapp'
    kind: 'radius.dev/Container@v1alpha1'
    properties: {
      run: {
        container: {
          image: 'radiusteam/tutorial-pythonapp'
        }
      }
      uses: [
        {
          binding: nodeapplication.properties.bindings.invoke
        }
      ]
      traits: [
        {
          kind: 'dapr.io/App@v1alpha1'
          appId: 'pythonapp'
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
