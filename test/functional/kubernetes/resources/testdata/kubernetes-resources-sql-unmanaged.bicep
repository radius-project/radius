var adminUsername = 'cooluser'
var adminPassword = 'p@ssw0rd'

resource app 'radius.dev/Application@v1alpha3' = {
  name: 'kubernetes-resources-sql-unmanaged'

  resource webapp 'Container' = {
    name: 'todoapp'
    properties: {
      connections: {
        sql: {
          kind: 'microsoft.com/SQL'
          source: db.outputs.sqlDB.id
        }
      }
      container: {
        image: 'radius.azurecr.io/magpie:latest'
        env: {
          CONNECTION_SQL_CONNECTIONSTRING: 'Data Source=tcp:${db.outputs.sqlDB.properties.server},1433;Initial Catalog=${db.outputs.sqlDB.properties.database};User Id=${adminUsername}@${db.outputs.sqlDB.properties.server};Password=${adminPassword};Encrypt=true'
        }
      }
    }
  }
}

module db 'br:radius.azurecr.io/starters/sql:latest' = {
  name: 'db-module'
  params: {
    adminPassword: adminPassword
    radiusApplication: app
    serverName: 'cool-database'
  }
}
