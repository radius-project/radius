//SAMPLE
resource app 'radius.dev/Application@v1alpha3' = {
  name: 'dapr-tutorial'

  //FRONTEND
  resource frontendRoute 'HttpRoute' = {
    name: 'frontend-route'
    properties: {
      gateway: {
        hostname: '*'
      }
    }
  }
  
  resource frontend 'Container' = {
    name: 'frontend'
    properties: {
      container: {
        image: 'radius.azurecr.io/daprtutorial-frontend'
        ports:{
          ui: {
            containerPort: 80
            provides: frontendRoute.id
          }
        }
      }
      connections: {
        backend: {
          kind: 'dapr.io/InvokeHttp'
          source: daprBackend.id
        }
      }
      traits: [
        {
          kind: 'dapr.io/Sidecar@v1alpha1'
          appId: 'frontend'
        }
      ]
    }
  }
  //FRONTEND

  resource backend 'Container' = {
    name: 'backend'
    properties: {
      container: {
        image: 'radius.azurecr.io/daprtutorial-backend'
      }
      connections: {
        orders: {
          kind: 'dapr.io/StateStore'
          source: statestore.id
        }
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
    name: 'backend-dapr'
    properties: {
      appId: 'backend'
    }
  }

  resource statestore 'dapr.io.StateStore' = {
    name: 'statestore'
    properties: {
      kind: 'any'
    }
  }
}
