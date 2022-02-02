resource app 'radius.dev/Application@v1alpha3' = {
  name: 'dapr-tutorial'

  //SAMPLE
  resource backend 'Container' = {
    name: 'backend'
    properties: {
      //RUN
      container: {
        image: 'radius.azurecr.io/daprtutorial-backend'
        ports: {
          orders: {
            containerPort: 3000
            provides: daprBackend.id
          }
        }
      }
      //RUN
      connections: {
        orders: {
          kind: 'dapr.io/StateStore'
          source: statestore.id
        }
      }
      //TRAITS
      traits: [
        {
          kind: 'dapr.io/Sidecar@v1alpha1'
          appPort: 3000
          appId: 'backend'
          provides: daprBackend.id
        }
      ]
      //TRAITS
    }
  }

  //STATESTORE
  resource statestore 'dapr.io.StateStore' = {
    name: 'statestore'
    properties: {
      kind: 'any'
    }
  }
  //STATESTORE
  //SAMPLE

  resource daprBackend 'dapr.io.InvokeHttpRoute' = {
    name: 'dapr-backend'
    properties: {
      appId: 'backend'
    }
  }

}
