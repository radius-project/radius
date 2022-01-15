resource app 'radius.dev/Application@v1alpha3' = {
  name: 'todo'
  
  resource container 'Container' = {
    name: 'container'
    properties: {
      container: {
        image: 'radius.azurecr.io/todoapp:latest'
        env: {
          DB_CONNECTION: infra.outputs.db.connectionString()
        }
      }
    }
  }
}

module infra 'infra.bicep' = {
  name: 'infra'
  params: {
    application: app.name
    computedValue: format('{0}', app.name) // test that we evaluate parameters
  }
}
