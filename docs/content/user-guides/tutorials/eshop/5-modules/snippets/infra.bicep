param serverName string = uniqueString('sql', resourceGroup().id)
param location string = resourceGroup().location
param skuName string = 'Standard'
param skuTier string = 'Standard'
param adminLogin string
@secure()
param adminLoginPassword string

param app object

resource sql 'Microsoft.Sql/servers@2019-06-01-preview' = {
  name: serverName
  location: location
  properties: {
    administratorLogin: adminLogin
    administratorLoginPassword: adminLoginPassword
  }

  resource identity 'databases' = {
    name: 'identity'
    location: location
    sku: {
      name: skuName
      tier: skuTier
    }
  }

  resource catalog 'databases' = {
    name: 'catalog'
    location: location
    sku: {
      name: skuName
      tier: skuTier
    }
  }

  resource ordering 'databases' = {
    name: 'ordering'
    location: location
    sku: {
      name: skuName
      tier: skuTier
    }
  }

  resource webhooks 'databases' = {
    name: 'webhooks'
    location: location
    sku: {
      name: skuName
      tier: skuTier
    }
  }
}

resource sqlIdentity 'radius.dev/Application/microsoft.com.SQLComponent@v1alpha3' = {
  name: '${app.name}/sql-identity'
  properties: {
    resource: sql::identity.id
  }
}

resource sqlCatalog 'radius.dev/Application/microsoft.com.SQLComponent@v1alpha3' = {
  name: '${app.name}/sql-catalog'
  properties: {
    resource: sql::identity.id
  }
}

resource sqlOrdering 'radius.dev/Application/microsoft.com.SQLComponent@v1alpha3' = {
  name: '${app.name}/sql-ordering'
  properties: {
    resource: sql::identity.id
  }
}

resource sqlWebhooks 'radius.dev/Application/microsoft.com.SQLComponent@v1alpha3' = {
  name: '${app.name}/sql-webhooks'
  properties: {
    resource: sql::identity.id
  }
}

resource redis 'radius.dev/Application/redislabs.com.RedisComponent@v1alpha3' = {
  name: '${app.name}/redis'
  properties: {
    managed: true
  }
}

resource mongo 'radius.dev/Application/mongodb.com.MongoDBComponent@v1alpha3' = {
  name: '${app.name}/mongo'
  properties: {
    managed: true
  }
}

resource servicebus 'radius.dev/Application/azure.com.ServiceBusQueueComponent@v1alpha3' = {
  name: '${app.name}/servicebus'
  properties: {
    managed: true
    queue: 'orders'
  }
}

output sql object = sql
output sqlIdentity object = sqlIdentity
output sqlCatalog object = sqlCatalog
output slqOrdering object = sqlOrdering
output sqlWebhooks object = sqlWebhooks
output redis object = redis
output mongo object = mongo
output serviceBus object = servicebus
