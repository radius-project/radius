//PARAMS
//REST
//REST
param go_service_build object
param node_service_build object
param python_service_build object
//PARAMS

//APP
resource app 'radius.dev/Application@v1alpha3' existing = {
  name: 'store'

//APP
//GOAPP
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
//GOAPP

//ROUTE
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
  resource python_app_route 'dapr.io.InvokeHttpRoute' = {
    name: 'python-app'
    properties: {
      appId: 'python-app'
    }
  }
//ROUTE

//NODEAPP
  resource node_app 'Container' = {
    name: 'node-app'
    properties: {
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
      traits: [
        {
          kind: 'dapr.io/Sidecar@v1alpha1'
          appId: 'node-app'
        }
      ]
    }
  }
//NODEAPP

//PYTHONAPP
  resource python_app 'Container' = {
    name: 'python-app'
    properties: {
      connections: {
        kind: {
          kind: 'dapr.io/StateStore'
          source: statestore.id
        }
      }
      container: {
        image: python_service_build.image
        ports: {
          web: {
            containerPort: 5000
          }
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
//PYTHONAPP


//STATESTORE
  resource statestore 'dapr.io.StateStore' existing = {
    name: 'orders'
  }
//STATESTORE
}
