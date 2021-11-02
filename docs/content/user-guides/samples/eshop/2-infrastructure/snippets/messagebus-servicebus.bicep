param AZURESERVICEBUSENABLED string = 'True'

//REST
//REST

// Azure Bicep resources ----------------------------------------------------
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
//ESHOP
resource app 'radius.dev/Application@v1alpha3' = {
  name: AZURESERVICEBUSENABLED
}
//ESHOP
