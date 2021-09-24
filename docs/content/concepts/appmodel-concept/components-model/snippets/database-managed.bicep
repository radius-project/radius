resource app 'radius.dev/Application@v1alpha3' = {
  name: 'cosmos-container-managed'

  //SAMPLE
  resource db 'azure.com.CosmosDBMongoComponent' = {
    name: 'db'
    properties: {
      managed: true
    }
  }
  //SAMPLE
}
