param app object
param redis object
param servicebus object
param identityHttp object

param APPLICATION_INSIGHTS_KEY string
param OCHESTRATOR_TYPE string

// Based on https://github.com/dotnet-architecture/eShopOnContainers/tree/dev/deploy/k8s/helm/basket-api
resource basket 'radius.dev/Application/ContainerComponent@v1alpha3' = {
  name: '${app.name}/basket-api'
  properties: {
    container: {
      image: 'eshop/basket.api:latest'
      env: {
        'ASPNETCORE_ENVIRONMENT': 'Development'
        'ASPNETCORE_URLS': 'http://0.0.0.0:80'
        'ApplicationInsights__InstrumentationKey': APPLICATION_INSIGHTS_KEY
        'UseLoadTest': 'False'
        'PATH_BASE': '/basket-api'
        'OrchestratorType': OCHESTRATOR_TYPE
        'PORT': '80'
        'GRPC_PORT': '81'
        'AzureServiceBusEnabled': 'True'
        'ConnectionString': redis.connectionString()
        'EventBusConnection': servicebus.queueConnectionString()
        'identityUrl': identityHttp.properties.url
        'IdentityUrlExternal': identityHttp.properties.url
      }
      ports: {
        http: {
          containerPort: 80
          provides: basketHttp.id
        }
        grpc: {
          containerPort: 81
          provides: basketGrpc.id
        }
      }
    }
    traits: []
    connections: {
      redis: {
        kind: 'redislabs.com/Redis'
        source: redis.id
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

resource basketHttp 'radius.dev/Application/HttpRoute@v1alpha3' = {
  name: '${app.name}/basket-http'
  properties: {
    port: 5103
  }
}

resource basketGrpc 'radius.dev/Application/HttpRoute@v1alpha3' = {
  name: '${app.name}/basket-grpc'
  properties: {
    port: 9103
  }
}


output basket object = basket
output basketHttp object = basketHttp
output basketGrpc object = basketGrpc
