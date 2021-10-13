@secure()
param adminLoginPassword string

resource eshop 'radius.dev/Application@v1alpha3' = {
  name: 'eshop'

  //RADSQL
  resource sqlIdentity 'ContainerComponent' = {
    name: 'IdentityDb'
    properties: {
      container: {
        image: 'mcr.microsoft.com/mssql/server:2019-latest'
        env: {
          ACCEPT_EULA: 'Y'
          MSSQL_PID: 'Developer'
          MSSQL_SA_PASSWORD: adminLoginPassword
        }
        ports: {
          sql: {
            containerPort: 1433
          }
        }
      }
    }
  }

  resource sqlRoute 'HttpRoute' = {
    name: 'sql-route'
  }
}
