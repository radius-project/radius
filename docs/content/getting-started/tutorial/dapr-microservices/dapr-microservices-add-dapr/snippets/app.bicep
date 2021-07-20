//SAMPLE
resource app 'radius.dev/Applications@v1alpha1' = {
  name: 'dapr-tutorial'

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
