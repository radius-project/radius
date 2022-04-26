var adminUsername = 'sa'
var adminPassword = 'p@ssw0rd'

param magpieimage string = 'radiusdev.azurecr.io/magpiego:latest' 

resource app 'radius.dev/Application@v1alpha3' = {
  name: 'kubernetes-resources-sql'

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
        image: magpieimage
        env: {
          CONNECTION_SQL_CONNECTIONSTRING: 'Data Source=tcp:${db.properties.server},${sqlRoute.properties.port};Initial Catalog=${db.properties.database};User Id=${adminUsername};Password=${adminPassword};Encrypt=True;TrustServerCertificate=True'
        }
        readinessProbe:{
          kind:'httpGet'
          containerPort:3000
          path: '/healthz'
        }
      }
    }
  }

  resource db 'microsoft.com.SQLDatabase' = {
    name: 'db'
    properties: {
      server: sqlRoute.properties.host
      database: 'master'
    }
  }

  resource sqlRoute 'HttpRoute' = {
    name: 'sql-route'
  }

  resource sqlContainer 'Container' = {
    name: 'container-test'
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
            provides: sqlRoute.id
          }
        }
      }
    }
  }
}
