resource app 'radius.dev/Application@v1alpha3' = {
  name: 'container-app-store'

  //RESOURCES
  resource mongo 'mongo.com.MongoDatabase' = {
    name: 'mongo'
    properties: {
      managed: true
    }
  }

  resource ordersStateStore 'dapr.io.StateStore' = {
    name: 'orders'
    properties: {
      kind: 'any'
      managed: true
    }
  }
  //RESOURCES
}
