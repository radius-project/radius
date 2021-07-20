//SAMPLE
resource app 'radius.dev/Applications@v1alpha1' = {
  name: 'dapr-tutorial'

  //FRONTEND
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
  //FRONTEND

  resource backend 'Components' = {
    name: 'backend'
    kind: 'radius.dev/Container@v1alpha1'
    properties: {
      //RUN
      run: {
        container: {
          image: 'radius.azurecr.io/daprtutorial-backend'
        }
      }
      //RUN
      //BINDINGS
      bindings: {
        invoke: {
          kind: 'dapr.io/Invoke'
        }
      }
      //BINDINGS
      uses: [
        {
          binding: statestore.properties.bindings.default
        }
      ]
      //TRAITS
      traits: [
        {
          kind: 'dapr.io/App@v1alpha1'
          appId: 'backend'
          appPort: 3000
        }
      ]
      //TRAITS
    }
  }

  //STATESTORE
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
  //STATESTORE
}
//SAMPLE
