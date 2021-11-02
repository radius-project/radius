param serverName string = uniqueString('sql', resourceGroup().id)
param location string = resourceGroup().location
param skuName string = 'Standard'
param skuTier string = 'Standard'
param adminLogin string = 'admin'
@secure()
param adminPassword string
param AZURESERVICEBUSENABLED string = 'True'
//PARAMS
//REST
//REST
param CLUSTER_IP string
param OCHESTRATOR_TYPE string = 'K8S'
param APPLICATION_INSIGHTS_KEY string = ''
param AZURESTORAGEENABLED string = 'False'
param ENABLEDEVSPACES string = 'False'
param TAG string = 'linux-dev'

var CLUSTERDNS = 'http://${CLUSTER_IP}.nip.io'
var PICBASEURL = '${CLUSTERDNS}/webshoppingapigw/c/api/v1/catalog/items/[0]/pic'
//PARAMS

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

  //CATALOG
  resource catalog 'ContainerComponent' = {
    name: 'catalog-api'
    properties: {
      container: {
        image: 'eshop/catalog.api:${TAG}'
        env: {
          UseCustomizationData: 'False'
          PATH_BASE: '/catalog-api'
          ASPNETCORE_ENVIRONMENT: 'Development'
          OrchestratorType: OCHESTRATOR_TYPE
          PORT: '80'
          GRPC_PORT: '81'
          PicBaseUrl: PICBASEURL
          AzureStorageEnabled: AZURESTORAGEENABLED
          ApplicationInsights__InstrumentationKey: APPLICATION_INSIGHTS_KEY
          AzureServiceBusEnabled: AZURESERVICEBUSENABLED
          EnableDevSpaces: ENABLEDEVSPACES
          ConnectionString: 'Server=tcp:${sqlCatalog.properties.server},1433;Initial Catalog=CatalogDb;Persist Security Info=False;User ID=${adminLogin};Password=${adminPassword};MultipleActiveResultSets=False;Encrypt=True;TrustServerCertificate=False;Connection Timeout=30;'
          EventBusConnection: listKeys(servicebus::topic::rootRule.id, servicebus::topic::rootRule.apiVersion).primaryKey
        }
        ports: {
          http: {
            containerPort: 80
            //PROVIDES
            provides: catalogHttp.id
            //PROVIDES
          }
          grpc: {
            containerPort: 81
          }
        }
      }
      connections: {
        sql: {
          kind: 'microsoft.com/SQL'
          source: sqlCatalog.id
        }
        // Connections to non-Radius Bicep resources coming soon
      }
    }
  }
  //CATALOG

  //ROUTE
  resource catalogHttp 'HttpRoute' = {
    name: 'catalog-http'
    properties: {
      port: 5101
    }
  }
  //ROUTE
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
}
