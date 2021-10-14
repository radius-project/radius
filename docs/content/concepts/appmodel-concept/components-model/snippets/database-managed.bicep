resource app 'radius.dev/Application@v1alpha3' = {
  name: 'myapp'

  resource db 'mongodb.com.MongoDBComponent' = {
    name: 'db'
    properties: {
      managed: true
    }
  }

}
