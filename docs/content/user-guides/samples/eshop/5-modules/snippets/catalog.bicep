param app object
param sqlCatalog object
param servicebus object

param OCHESTRATOR_TYPE string
param APPLICATION_INSIGHTS_KEY string
param ESHOP_EXTERNAL_DNS_NAME_OR_IP string

resource catalog 'radius.dev/Application/ContainerComponent@v1alpha3' = {
  name: '${app.name}/catalog-api'
  properties: {
    container: {
      image: 'eshop/catalog.api:latest'
      env: {
        'UseCustomizationData': 'False'
        'PATH_BASE': '/catalog-api'
        'ASPNETCORE_ENVIRONMENT': 'Development'
        'OrchestratorType': OCHESTRATOR_TYPE
        'PORT': '80'
        'GRPC_PORT': '81'
        'PicBaseUrl': ''
        'AzureStorageEnabled': 'False'
        'ApplicationInsights__InstrumentationKey': APPLICATION_INSIGHTS_KEY
        'AzureServiceBusEnabled': 'True'
        'ConnectionString': sqlCatalog.connectionString()
        'EventBusConnection': servicebus.queueConnectionString()
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
      servicebus: {
        kind: 'azure.com/ServiceBusQueue'
        source: servicebus.id
      }
    }
  }
}
//CATALOG

//ROUTE
resource catalogHttp 'radius.dev/Application/HttpRoute@v1alpha3' = {
  name: '${app.name}/catalog-http'
  properties: {
    port: 5101
    gateway: {
      hostname: ESHOP_EXTERNAL_DNS_NAME_OR_IP
    }
  }
}
//ROUTE

resource catalogGrpc 'radius.dev/Application/HttpRoute@v1alpha3' = {
  name: '${app.name}/catalog-grpc'
  properties: {
    port: 9101
  }
}

output catalog object = catalog
output catalogHttp object = catalogHttp
output catalogGrpc object = catalogGrpc
