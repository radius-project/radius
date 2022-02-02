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
