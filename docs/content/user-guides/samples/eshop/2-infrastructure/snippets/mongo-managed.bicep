//REST
//REST

resource eshop 'radius.dev/Application@v1alpha3' = {
  name: 'eshop'

  //REST
  //REST

  resource mongo 'mongodb.com.MongoDBComponent' = {
    name: 'mongo'
    properties: {
      managed: true
    }
  }
}
