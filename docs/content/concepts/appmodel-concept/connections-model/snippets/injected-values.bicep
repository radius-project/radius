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
  //SAMPLE

}
