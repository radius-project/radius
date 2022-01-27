resource app 'radius.dev/Application@v1alpha3' = {
  name: 'myapp'

  resource container 'Container' = {
    name: 'mycontainer'
    properties: {
      container: {
        image: 'myregistry/myimage'
        env: {
          SQL_SERVER: sqlDb.outputs.sqlDB.properties.server
        }
      }
    }
  }
}

module sqlDb 'br:radius.azurecr.io/starters/sql-azure:latest' = {
  name: 'sqlDb'
  params: {
    radiusApplication: app
    databaseName: 'inventory'
    adminLogin: 'admin'
    adminPassword: '***'
  }
}
