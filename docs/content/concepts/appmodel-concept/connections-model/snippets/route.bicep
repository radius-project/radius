resource app 'radius.dev/Application@v1alpha3' = {
  name: 'myapp'

  //SAMPLE
  resource fe 'Container' = {
    name: 'frontend'
    properties: {
      //CONTAINER
      container: {
        image: 'radius.azurecr.io/frontend:latest'
      }
      //CONTAINER
      connections: {
        orders:{
          kind: 'Http'
          source: orderRoute.id
        }
      }
    }
  }

  resource orderRoute 'HttpRoute' = {
    name: 'orders'
  }

  resource be 'Container' = {
    name: 'backend'
    properties: {
      container: {
        image: 'radius.azurecr.io/backend:latest'
        ports: {
          orders: {
            containerPort: 80
            provides: orderRoute.id
          }
        }
      }
    }
  }
  //SAMPLE

}
