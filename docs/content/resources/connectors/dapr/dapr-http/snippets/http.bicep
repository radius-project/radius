resource app 'radius.dev/Application@v1alpha3' = {
  name: 'myapp'

  //BACKEND
  resource backend 'Container' = {
    name: 'backend'
    properties: {
      container: {
        image: 'registry/container:tag'
      }
      traits: [
        {
          kind: 'dapr.io/Sidecar@v1alpha1'
          appId: 'backend'
          appPort: 80
          provides: backendDapr.id
        }
      ]
    }
  }
  //BACKEND

  //ROUTE
  resource backendDapr 'dapr.io.InvokeHttpRoute' = {
    name: 'dapr-backend'
    properties: {
      appId: 'backend'
    }
  }
  //ROUTE

  //FRONTEND
  resource frontend 'Container' = {
    name: 'frontend'
    properties: {
      container: {
        image: 'registry/container:tag'
        env: {
          BACKEND_ID: backendDapr.properties.appId
        }
      }
      connections: {
        orders: {
          kind: 'dapr.io/InvokeHttp'
          source: backendDapr.id
        }
      }
    }
  }
  //FRONTEND

}
