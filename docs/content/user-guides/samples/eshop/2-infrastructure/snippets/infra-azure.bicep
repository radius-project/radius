param serverName string = uniqueString('sql', resourceGroup().id)
param location string = resourceGroup().location
param skuName string = 'Standard'
param skuTier string = 'Standard'
param adminLogin string = 'admin'
@secure()
param adminPassword string

param AZURESERVICEBUSENABLED string = 'True'

resource sql 'Microsoft.Sql/servers@2019-06-01-preview' = {
  name: serverName
  location: location
  properties: {
    administratorLogin: adminLogin
    administratorLoginPassword: adminPassword
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


resource redisCache 'Microsoft.Cache/redis@2020-06-01' existing = {
  name: 'eshop'
}

resource cosmos 'Microsoft.DocumentDB/databaseAccounts@2021-06-15' existing = {
  name: 'eshop'

  resource db 'mongodbDatabases' existing = {
    name: 'db'
  }
}

resource servicebus 'Microsoft.ServiceBus/namespaces@2021-06-01-preview' = {
  name: 'eshop${uniqueString(resourceGroup().id)}'
  location: resourceGroup().location
  sku: {
    name: 'Standard'
    tier: 'Standard'
  }

  resource topic 'topics' = {
    name: 'eshop_event_bus'
    properties: {
      defaultMessageTimeToLive: 'P14D'
      maxSizeInMegabytes: 1024
      requiresDuplicateDetection: false
      enableBatchedOperations: true
      supportOrdering: false
      enablePartitioning: true
      enableExpress: false
    }

    resource rootRule 'authorizationRules' = {
      name: 'Root'
      properties: {
        rights: [
          'Manage'
          'Send'
          'Listen'
        ]
      }
    }

    resource basket 'subscriptions' = {
      name: 'Basket'
      properties: {
        requiresSession: false
        defaultMessageTimeToLive: 'P14D'
        deadLetteringOnMessageExpiration: true
        deadLetteringOnFilterEvaluationExceptions: true
        maxDeliveryCount: 10
        enableBatchedOperations: true
      }
    }

    resource catalog 'subscriptions' = {
      name: 'Catalog'
      properties: {
        requiresSession: false
        defaultMessageTimeToLive: 'P14D'
        deadLetteringOnMessageExpiration: true
        deadLetteringOnFilterEvaluationExceptions: true
        maxDeliveryCount: 10
        enableBatchedOperations: true
      }
    }

    resource ordering 'subscriptions' = {
      name: 'Ordering'
      properties: {
        requiresSession: false
        defaultMessageTimeToLive: 'P14D'
        deadLetteringOnMessageExpiration: true
        deadLetteringOnFilterEvaluationExceptions: true
        maxDeliveryCount: 10
        enableBatchedOperations: true
      }
    }

    resource graceperiod 'subscriptions' = {
      name: 'GracePeriod'
      properties: {
        requiresSession: false
        defaultMessageTimeToLive: 'P14D'
        deadLetteringOnMessageExpiration: true
        deadLetteringOnFilterEvaluationExceptions: true
        maxDeliveryCount: 10
        enableBatchedOperations: true
      }
    }

    resource payment 'subscriptions' = {
      name: 'Payment'
      properties: {
        requiresSession: false
        defaultMessageTimeToLive: 'P14D'
        deadLetteringOnMessageExpiration: true
        deadLetteringOnFilterEvaluationExceptions: true
        maxDeliveryCount: 10
        enableBatchedOperations: true
      }
    }

    resource backgroundTasks 'subscriptions' = {
      name: 'backgroundtasks'
      properties: {
        requiresSession: false
        defaultMessageTimeToLive: 'P14D'
        deadLetteringOnMessageExpiration: true
        deadLetteringOnFilterEvaluationExceptions: true
        maxDeliveryCount: 10
        enableBatchedOperations: true
      }
    }

    resource OrderingSignalrHub 'subscriptions' = {
      name: 'Ordering.signalrhub'
      properties: {
        requiresSession: false
        defaultMessageTimeToLive: 'P14D'
        deadLetteringOnMessageExpiration: true
        deadLetteringOnFilterEvaluationExceptions: true
        maxDeliveryCount: 10
        enableBatchedOperations: true
      }
    }

    resource webhooks 'subscriptions' = {
      name: 'Webhooks'
      properties: {
        requiresSession: false
        defaultMessageTimeToLive: 'P14D'
        deadLetteringOnMessageExpiration: true
        deadLetteringOnFilterEvaluationExceptions: true
        maxDeliveryCount: 10
        enableBatchedOperations: true
      }
    }

  }

}

resource eshop 'radius.dev/Application@v1alpha3' = {
  name: 'eshop'

  resource sqlIdentity 'microsoft.com.SQLComponent' = {
    name: 'IdentityDb'
    properties: {
      resource: sql::identity.id
    }
  }

  resource sqlCatalog 'microsoft.com.SQLComponent' = {
    name: 'CatalogDb'
    properties: {
      resource: sql::catalog.id
    }
  }

  resource sqlOrdering 'microsoft.com.SQLComponent' = {
    name: 'OrderingDb'
    properties: {
      resource: sql::ordering.id
    }
  }

  resource sqlWebhooks 'microsoft.com.SQLComponent' = {
    name: 'WebhooksDb'
    properties: {
      resource: sql::webhooks.id
    }
  }

  resource redis 'redislabs.com.RedisComponent' = {
    name: 'redis'
    properties: {
      resource: redisCache.id
    }
  }

  resource mongo 'mongodb.com.MongoDBComponent' = {
    name: 'mongo'
    properties: {
      managed: true
    }
  }

}

