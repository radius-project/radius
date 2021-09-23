resource app 'radius.dev/Applications@v1alpha1' = {
  name: 'webapp'
  //SAMPLE
  resource todoapplication 'Components' = {
    name: 'todoapp'
    kind: 'radius.dev/Container@v1alpha1'
    properties: {
      //HIDE
      run: {
        container: {
          image: 'radiusteam/tutorial-todoapp'
        }
      }
      //HIDE
      bindings: {
        web: {
          kind: 'http'
          targetPort: 3000
        }
      }
    }
  }
  //SAMPLE

  //COSMOS
  resource db 'Components' = {
    name: 'db'
    kind: 'azure.com/CosmosDBMongo@v1alpha1'
    properties: {
      config: {
        managed: true
      }
    }
  }
  //COSMOS
}
