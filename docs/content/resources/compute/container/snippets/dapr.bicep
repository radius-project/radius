resource app 'radius.dev/Applications@v1alpha1' = {
  name: 'dapr'

  //SAMPLE
  resource frontend 'Components' = {
    name: 'frontend'
    kind: 'radius.dev/Container@v1alpha1'
    properties: {
      //HIDE
      run: {
        container: {
          image: 'rynowak/frontend:0.5.0-dev'
        }
      }
      //HIDE
      traits: [
        {
          kind: 'dapr.io/App@v1alpha1'
          appId: 'frontend'
          appPort: 3000
        }
      ]
    }
  }
  //SAMPLE
}
