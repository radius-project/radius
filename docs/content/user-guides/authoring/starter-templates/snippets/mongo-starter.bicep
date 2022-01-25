resource app 'radius.dev/Application@v1alpha3' = {
  name: 'myapp'

  resource container 'Container' = {
    name: 'mycontainer'
    properties: {
      container: {
        image: 'myregistry/myimage'
        env: {
          MONGO_CS: mongoDB.outputs.mongoDB.connectionString()
        }
      }
      connections: {
        inventory: {
          kind: 'mongo.com/MongoDB'
          source: mongoDB.outputs.mongoDB.id
        }
      }
    }
  }

}

module mongoDB 'br:radius.azurecr.io/starters/mongo:latest' = {
  name: 'mongoDb'
  params: {
    radiusApplication: app
  }
}
