//SQL
param serverName string = uniqueString('sql', resourceGroup().id)
param location string = resourceGroup().location
param administratorLogin string

@secure()
param administratorLoginPassword string

resource sql 'Microsoft.Sql/servers@2019-06-01-preview' = {
  name: serverName
  location: location
  properties: {
    administratorLogin: administratorLogin
    administratorLoginPassword: administratorLoginPassword
  }

  resource identity 'databases' = {
    name: 'identity'
    location: location
    sku: {
      name: 'Standard'
      tier: 'Standard'
    }
  }

  resource catalog 'databases' = {
    name: 'catalog'
    location: location
    sku: {
      name: 'Standard'
      tier: 'Standard'
    }
  }

  resource ordering 'databases' = {
    name: 'ordering'
    location: location
    sku: {
      name: 'Standard'
      tier: 'Standard'
    }
  }

  resource webhooks 'databases' = {
    name: 'webhooks'
    location: location
    sku: {
      name: 'Standard'
      tier: 'Standard'
    }
  }

}
//SQL

//APP
resource eshop 'radius.dev/Application@v1alpha3' = {
  name: 'eshop'

  //RADSQL
  resource sqlIdentity 'microsoft.com.SQLComponent' = {
    name: 'sql-identity'
    properties: {
      resource: sql::identity.id
    }
  }

  resource sqlCatalog 'microsoft.com.SQLComponent' = {
    name: 'sql-catalog'
    properties: {
      resource: sql::identity.id
    }
  }

  resource sqlOrdering 'microsoft.com.SQLComponent' = {
    name: 'sql-ordering'
    properties: {
      resource: sql::identity.id
    }
  }

  resource sqlWebhooks 'microsoft.com.SQLComponent' = {
    name: 'sql-webhooks'
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
}
//APP
