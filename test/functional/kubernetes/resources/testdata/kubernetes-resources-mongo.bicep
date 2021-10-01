resource app 'radius.dev/Application@v1alpha3' = {
  name: 'kubernetes-resources-mongo'
  
  resource webapp 'ContainerComponent' = {
    name: 'todoapp'
    properties: {
      container: {
        image: 'radius.azurecr.io/magpie:latest'
      }
      connections: {
        mongo: {
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
