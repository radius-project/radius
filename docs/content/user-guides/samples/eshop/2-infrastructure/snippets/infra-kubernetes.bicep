@secure()
param adminPassword string

param AZURESERVICEBUSENABLED string = 'False'

resource eshop 'radius.dev/Application@v1alpha3' = {
  name: 'eshop'

  resource sqlIdentity 'ContainerComponent' = {
    name: 'IdentityDb'
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

  resource sqlRoute 'HttpRoute' = {
    name: 'sql-route'
    properties: {
      port: 1433
    }
  }

  resource redisKeystore 'redislabs.com.RedisComponent' = {
    name: 'redis-keystore'
    properties: {
      managed: true
    }
  }

  resource redisBasket 'redislabs.com.RedisComponent' = {
    name: 'redis-basket'
    properties: {
      managed: true
    }
  }

  resource mongo 'mongodb.com.MongoDBComponent' = {
    name: 'mongo'
    properties: {
      managed: true
    }
  }

  resource rabbitmq 'rabbitmq.com.MessageQueueComponent' = {
    name: 'rabbitmq'
    properties: {
      managed: true
      queue: 'eshop_event_bus'
    }
  }

}
