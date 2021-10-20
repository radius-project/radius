param app object
param servicebus object

param APPLICATION_INSIGHTS_KEY string
param OCHESTRATOR_TYPE string

// Based on https://github.com/dotnet-architecture/eShopOnContainers/tree/dev/deploy/k8s/helm/payment-api
resource payment 'radius.dev/Application/ContainerComponent@v1alpha3' = {
  name: '${app.name}/payment-api'
  properties: {
    container: {
      image: 'eshop/payment.api:latest'
      env: {
        'ASPNETCORE_ENVIRONMENT': 'Development'
        'ASPNETCORE_URLS': 'http://0.0.0.0:80'
        'ApplicationInsights__InstrumentationKey': APPLICATION_INSIGHTS_KEY
        'Serilog__MinimumLevel__Override__payment-api.IntegrationEvents.EventHandling': 'Verbose'
        'Serilog__MinimumLevel__Override__Microsoft.eShopOnContainers.BuildingBlocks.EventBusRabbitMQ': 'Verbose'
        'OrchestratorType': OCHESTRATOR_TYPE
        'AzureServiceBusEnabled': 'True'
        'EventBusConnection': servicebus.queueConnectionString()
      }
      ports: {
        http: {
          containerPort: 80
          provides: paymentHttp.id
        }
      }
    }
    traits: []
    connections: {
      servicebus: {
        kind: 'azure.com/ServiceBusQueue'
        source: servicebus.id
      }
    }
  }
}

resource paymentHttp 'radius.dev/Application/HttpRoute@v1alpha3' = {
  name: '${app.name}/payment-http'
  properties: {
    port: 5108
  }
}

output payment object = payment
output paymentHttp object = paymentHttp
