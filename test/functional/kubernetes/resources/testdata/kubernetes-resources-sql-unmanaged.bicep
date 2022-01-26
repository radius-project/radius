var serverName = 'test'
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
          source: db.id
        }
      }
      container: {
        image: 'radius.azurecr.io/magpie:latest'
        env: {
          CONNECTION_SQL_CONNECTIONSTRING: 'Data Source=tcp:${db.properties.server},1433;Initial Catalog=${db.properties.database};User Id=${adminUsername}@${db.properties.server};Password=${adminPassword};Encrypt=true'
        }
      }
    }
  }

  resource db 'microsoft.com.SQLDatabase' = {
    name: 'db'
    properties: {
      server: sqlContainer.Name
      database: sqlContainer.Name
    }
  }

  resource sqlContainer 'Container' = {
    name: 'container-${serverName}'
    properties: {
      container: {
        image: 'mcr.microsoft.com/mssql/server:2019-latest'
        env: {
          ACCEPT_EULA: 'Y'
          MSSQL_PID: 'Developer'
          MSSQL_SA_PASSWORD: adminPassword
        }
        ports: {
          sql: {
            containerPort: 1433
          }
        }
      }
    }
  }
}
