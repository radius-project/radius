resource app 'radius.dev/Application@v1alpha3' = {
  name: 'kubernetes-resources-mongo'
  
  resource webapp 'ContainerComponent' = {
    name: 'todomongo'
    properties: {
      container: {
        image: 'radius.azurecr.io/magpie:latest'
      }
      connections: {
        mongodb: {
          kind: 'mongo.com/MongoDB'
          source: mongodb.id
        }
      }
    }
  }

  resource mongodb 'mongodb.com.MongoDBComponent' = {
    name: 'mongodb'
    properties: {
        managed: true
    }
  }
}
