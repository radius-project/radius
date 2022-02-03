//SAMPLE
resource app 'radius.dev/Application@v1alpha3' = {
  name: 'dapr-tutorial'

  resource backend 'Container' = {
    name: 'backend'
    properties: {
      container: {
        image: 'radius.azurecr.io/daprtutorial-backend'
      }
      traits: [
        {
          kind: 'dapr.io/Sidecar@v1alpha1'
          appId: 'backend'
          appPort: 3000
          provides: daprBackend.id
        }
      ]
    }
  }

  resource daprBackend 'dapr.io.InvokeHttpRoute' = {
    name: 'dapr-backend'
    properties: {
      appId: 'backend'
    }
  }
}
//SAMPLE
