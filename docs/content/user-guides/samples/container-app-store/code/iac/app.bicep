param go_service_build object
param node_service_build object
param python_service_build object

resource app 'radius.dev/Application@v1alpha3' existing = {
  name: 'store'

  resource go_app 'Container' = {
    name: 'go-app'
    properties: {
      container: {
        image: go_service_build.image
        ports: {
          web: {
            containerPort: 8050
          }
        }
      }
      traits: [
        {
          kind: 'dapr.io/Sidecar@v1alpha1'
          appId: 'go-app'
          appPort: 8050
          provides: go_app_route.id
        }
      ]
    }
  }

  resource go_app_route 'dapr.io.InvokeHttpRoute' = {
    name: 'go-app'
    properties: {
      appId: 'go-app'
    }
  }

  resource node_app_route 'HttpRoute' = {
    name: 'node-app'
    properties: {
      gateway: {
        hostname: '*'
      }
    }
  }

  resource node_app 'Container' = {
    name: 'node-app'
    properties: {
      container: {
        image: node_service_build.image
        env: {
          'ORDER_SERVICE_NAME': python_app_route.properties.appId
          'INVENTORY_SERVICE_NAME': go_app_route.properties.appId
        }
        ports: {
          web: {
            containerPort: 3000
            provides: node_app_route.id
          }
        }
      }
      connections: {
        inventory: {
          kind: 'dapr.io/InvokeHttp'
          source: go_app_route.id
        }
        orders: {
          kind: 'dapr.io/InvokeHttp'
          source: python_app_route.id
        }
      }
      traits: [
        {
          kind: 'dapr.io/Sidecar@v1alpha1'
          appId: 'node-app'
        }
      ]
    }
  }

  resource python_app 'Container' = {
    name: 'python-app'
    properties: {
      container: {
        image: python_service_build.image
        ports: {
          web: {
            containerPort: 5000
          }
        }
      }
      connections: {
        kind: {
          kind: 'dapr.io/StateStore'
          source: statestore.id
        }
      }
      traits: [
        {
          kind: 'dapr.io/Sidecar@v1alpha1'
          appId: 'python-app'
          appPort: 5000
          provides: python_app_route.id
        }
      ]
    }
  }

  resource python_app_route 'dapr.io.InvokeHttpRoute' = {
    name: 'python-app'
    properties: {
      appId: 'python-app'
    }
  }

  resource statestore 'dapr.io.StateStore' existing = {
    name: 'orders'
  }
}
