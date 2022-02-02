resource app 'radius.dev/Application@v1alpha3' = {
  name: 'myapp'

  resource db 'mongo.com.MongoDatabase' = {
    name: 'db'
    properties: {
      managed: true
    }
  }

}
