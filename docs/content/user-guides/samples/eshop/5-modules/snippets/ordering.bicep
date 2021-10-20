param app object
param sqlOrdering object
param servicebus object
param identityHttp object
param catalogHttp object
param basketHttp object

param OCHESTRATOR_TYPE string
param APPLICATION_INSIGHTS_KEY string

// Based on https://github.com/dotnet-architecture/eShopOnContainers/tree/dev/deploy/k8s/helm/ordering-api
resource ordering 'radius.dev/Application/ContainerComponent@v1alpha3' = {
  name: '${app.name}/ordering-api'
  properties: {
    container: {
      image: 'eshop/ordering.api:latest'
      env: {
        'ASPNETCORE_ENVIRONMENT': 'Development'
        'ASPNETCORE_URLS': 'http://0.0.0.0:80'
        'UseCustomizationData': 'False'
        'AzureServiceBusEnabled': 'True'
        'CheckUpdateTime': '30000'
        'ApplicationInsights__InstrumentationKey': APPLICATION_INSIGHTS_KEY
        'OrchestratorType': OCHESTRATOR_TYPE
        'UseLoadTest': 'False'
        'Serilog__MinimumLevel__Override__Microsoft.eShopOnContainers.BuildingBlocks.EventBusRabbitMQ': 'Verbose'
        'Serilog__MinimumLevel__Override__ordering-api': 'Verbose'
        'PATH_BASE': '/ordering-api'
        'GRPC_PORT': '81'
        'PORT': '80'
        'ConnectionString': sqlOrdering.connectionString()
        'EventBusConnection': servicebus.queueConnectionString()
        'identityUrl': identityHttp.properties.url
        'IdentityUrlExternal': identityHttp.properties.url
      }
      ports: {
        http: {
          containerPort: 80
          provides: orderingHttp.id
        }
        grpc: {
          containerPort: 81
          provides: orderingGrpc.id
        }
      }
    }
    traits: []
    connections: {
      sql: {
        kind: 'microsoft.com/SQL'
        source: sqlOrdering.id
      }
      servicebus: {
        kind: 'azure.com/ServiceBusQueue'
        source: servicebus.id
      }
      identity: {
        kind: 'Http'
        source: identityHttp.id
      }
    }
  }
}

resource orderingHttp 'radius.dev/Application/HttpRoute@v1alpha3' = {
  name: '${app.name}/ordering-http'
  properties: {
    port: 5102
  }
}

resource orderingGrpc 'radius.dev/Application/HttpRoute@v1alpha3' = {
  name: '${app.name}/ordering-grpc'
  properties: {
    port: 9102
  }
}

// Based on https://github.com/dotnet-architecture/eShopOnContainers/tree/dev/deploy/k8s/helm/ordering-backgroundtasks
resource orderbgtasks 'radius.dev/Application/ContainerComponent@v1alpha3' = {
  name: '${app.name}/ordering-backgroundtasks'
  properties: {
    container: {
      image: 'eshop/ordering.backgroundtasks:latest'
      env: {
        'ASPNETCORE_ENVIRONMENT': 'Development'
        'ASPNETCORE_URLS': 'http://0.0.0.0:80'
        'UseCustomizationData': 'False'
        'CheckUpdateTime': '30000'
        'GracePeriodTime': '1'
        'ApplicationInsights__InstrumentationKey': APPLICATION_INSIGHTS_KEY
        'UseLoadTest': 'False'
        'Serilog__MinimumLevel__Override__Microsoft.eShopOnContainers.BuildingBlocks.EventBusRabbitMQ': 'Verbose'
        'OrchestratorType': OCHESTRATOR_TYPE
        'AzureServiceBusEnabled': 'True'
        'ConnectionString': sqlOrdering.connectionString()
        'EventBusConnection': servicebus.queueConnectionString()
      }
      ports: {
        http: {
          containerPort: 80
          provides: orderbgtasksHttp.id
        }
      }
    }
    traits: []
    connections: {
      sql: {
        kind: 'microsoft.com/SQL'
        source: sqlOrdering.id
      }
      servicebus: {
        kind: 'azure.com/ServiceBusQueue'
        source: servicebus.id
      }
    }
  }
}

resource orderbgtasksHttp 'radius.dev/Application/HttpRoute@v1alpha3' = {
  name: '${app.name}/orderbgtasks-http'
  properties: {
    port: 5111
  }
}

// Based on https://github.com/dotnet-architecture/eShopOnContainers/tree/dev/deploy/k8s/helm/ordering-signalrhub
resource orderingsignalrhub 'radius.dev/Application/ContainerComponent@v1alpha3' = {
  name: '${app.name}/ordering-signalrhub'
  properties: {
    container: {
      image: 'eshop/ordering.signalrhub:latest'
      env: {
        'ASPNETCORE_ENVIRONMENT': 'Development'
        'ASPNETCORE_URLS': 'http://0.0.0.0:80'
        'ApplicationInsights__InstrumentationKey': APPLICATION_INSIGHTS_KEY
        'OrchestratorType': OCHESTRATOR_TYPE
        'IsClusterEnv': 'True'
        'AzureServiceBusEnabled': 'True'
        'EventBusConnection': servicebus.queueConnectionString()
        'identityUrl': identityHttp.properties.url
        'IdentityUrlExternal': identityHttp.properties.url
      }
      ports: {
        http: {
          containerPort: 80
          provides: orderingsignalrhubHttp.id
        }
      }
    }
    traits: []
    connections: {
      servicebus: {
        kind: 'azure.com/ServiceBusQueue'
        source: servicebus.id
      }
      identity: {
        kind: 'Http'
        source: identityHttp.id
      }
      ordering: {
        kind: 'Http'
        source: orderingHttp.id
      }
      catalog: {
        kind: 'Http'
        source: catalogHttp.id
      }
      basket: {
        kind: 'Http'
        source: basketHttp.id
      }
    }
  }
}

resource orderingsignalrhubHttp 'radius.dev/Application/HttpRoute@v1alpha3' = {
  name: '${app.name}/orderingsignalrhub-http'
  properties: {
    port: 5112
  }
}


output ordering object = ordering
output orderingHttp object = orderingHttp
output orderingGrpc object = orderingGrpc
output orderbgtasks object = orderbgtasks
output orderbgtasksHttp object = orderbgtasksHttp
