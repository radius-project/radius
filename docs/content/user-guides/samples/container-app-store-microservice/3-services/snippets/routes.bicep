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
