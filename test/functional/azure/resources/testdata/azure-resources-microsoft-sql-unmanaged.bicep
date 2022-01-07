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
      resource: server::db.id
    }
  }
}

resource server 'Microsoft.Sql/servers@2021-02-01-preview' = {
  name: 'sql-${uniqueString(resourceGroup().id)}'
  location: resourceGroup().location
  tags: {
    radiustest: 'azure-resources-microsoft-sql-unmanaged'
  }
  properties: {
    administratorLogin: adminUsername
    administratorLoginPassword: adminPassword
  }

  resource db 'databases' = {
    name: 'cool-database'
    location: resourceGroup().location
  }

  resource firewall 'firewallRules' = {
    name: 'allow'
    properties: {
      startIpAddress: '0.0.0.0'
      endIpAddress: '0.0.0.0'
    }
  }
}
