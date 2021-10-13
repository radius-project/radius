//APP
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

  resource sqlCatalog 'ContainerComponent' = {
    name: 'CatalogDb'
    properties: {
      r
    }
  }

  resource sqlOrdering 'ContainerComponent' = {
    name: 'OrderingDb'
    properties: {
      resource: sql::identity.id
    }
  }

  resource sqlWebhooks 'ContainerComponent' = {
    name: 'WebhooksDb'
    properties: {
      resource: sql::identity.id
    }
  }
  //RADSQL

  //REDIS
  resource redis 'redislabs.com.RedisComponent' = {
    name: 'redis'
    properties: {
      managed: true
    }
  }
  //REDIS

  //MONGO
  resource mongo 'mongodb.com.MongoDBComponent' = {
    name: 'mongo'
    properties: {
      managed: true
    }
  }
  //MONGO

  //RABBITMQ
  resource rabbitmq 'rabbitmq.com.MessageQueueComponent' = {
    name: 'rabbitmq'
    properties: {
      managed: true
      queue: 'eshop_event_bus'
    }
  }
  //RABBITMQ
}
//APP
