resource app 'radius.dev/Application@v1alpha3' = {
  name: 'mongo-container-managed'

  //SAMPLE
  resource db 'mongodb.com.MongoDBComponent' = {
    name: 'db'
    properties: {
      managed: true
    }
  }
  //SAMPLE
}
