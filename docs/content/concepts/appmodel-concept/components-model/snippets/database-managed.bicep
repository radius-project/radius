resource app 'radius.dev/Applications@v1alpha1' = {
  name: 'cosmos-container-managed'
  
  //SAMPLE
  resource db 'Components' = {
    name: 'db'
    kind: 'azure.com/CosmosDBMongo@v1alpha1'
    properties: {
      config: {
        managed: true
      }
    }
  }
  //SAMPLE
}
