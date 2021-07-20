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
      traits: [
        {
          kind: 'dapr.io/App@v1alpha1'
          appId: 'backend'
          appPort: 3000
        }
      ]
    }
  }
}
//SAMPLE
