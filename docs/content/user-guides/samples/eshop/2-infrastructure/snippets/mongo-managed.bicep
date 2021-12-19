//REST
//REST

resource eshop 'radius.dev/Application@v1alpha3' = {
  name: 'eshop'

  //REST
  //REST

  resource mongo 'mongo.com.MongoDatabase' = {
    name: 'mongo'
    properties: {
      managed: true
    }
  }
}
