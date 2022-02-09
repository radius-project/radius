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
          source: sqlDB.id
        }
      }
      container: {
        image: 'radius.azurecr.io/magpie:latest'
        env: {
          CONNECTION_SQL_CONNECTIONSTRING: 'Data Source=tcp:${sqlDB.properties.server},1433;Initial Catalog=${sqlDB.properties.database};User Id=${adminUsername}@${sqlDB.properties.server};Password=${adminPassword};Encrypt=true'
        }
      }
    }
  }
  resource sqlDB 'microsoft.com.SQLDatabase' existing = {
    name: 'cool-database'
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
