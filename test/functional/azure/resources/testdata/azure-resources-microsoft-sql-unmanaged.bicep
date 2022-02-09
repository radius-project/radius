var adminUsername = 'cooluser'
var adminPassword = 'p@ssw0rd'

resource app 'radius.dev/Application@v1alpha3' = {
  name: 'azure-resources-microsoft-sql-unmanaged'

  resource webapp 'Container' = {
    name: 'todoapp'
    properties: {
      connections: {
        sql: {
          kind: 'microsoft.com/SQL'
          source: sqlDatabase.id
        }
      }
      container: {
        image: 'radius.azurecr.io/magpie:latest'
        env: {
          CONNECTION_SQL_CONNECTIONSTRING: 'Data Source=tcp:${sqlDatabase.properties.server},1433;Initial Catalog=${sqlDatabase.properties.database};User Id=${adminUsername}@${sqlDatabase.properties.server};Password=${adminPassword};Encrypt=true'
        }
        readinessProbe:{
          kind:'httpGet'
          containerPort:3000
          path: '/healthz'
        }
      }
    }
  }

  resource sqlDatabase 'microsoft.com.SQLDatabase' existing = {
    name: 'cool-database'
  }
}

module db 'br:radius.azurecr.io/starters/sql-azure:latest' = {
  name: 'db-module'
  params: {
    adminLogin: adminUsername
    adminPassword: adminPassword
    radiusApplication: app
    serverName: 'cool-database'
  }
}
