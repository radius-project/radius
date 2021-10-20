param app object
param identityHttp object
param webshoppingapigwHttp object
param webshoppingaggHttp object
param orderingsignalrhubHttp object

param APPLICATION_INSIGHTS_KEY string
param OCHESTRATOR_TYPE string

// Based on https://github.com/dotnet-architecture/eShopOnContainers/tree/dev/deploy/k8s/helm/webspa
resource webspa 'radius.dev/Application/ContainerComponent@v1alpha3' = {
  name: '${app.name}/web-spa'
  properties: {
    container: {
      image: 'eshop/webspa:latest'
      env: {
        'ASPNETCORE_ENVIRONMENT': 'Production'
        'ASPNETCORE_URLS': 'http://0.0.0.0:80'
        'UseCustomizationData': 'False'
        'ApplicationInsights__InstrumentationKey': APPLICATION_INSIGHTS_KEY
        'OrchestratorType': OCHESTRATOR_TYPE
        'IsClusterEnv': 'True'
        'IdentityUrl': identityHttp.properties.url
        'IdentityUrlHC': '${identityHttp.properties.url}/hc'
        'PurchaseUrl': webshoppingapigwHttp.properties.url
        'SignalrHubUrl': orderingsignalrhubHttp.properties.url
      }
      ports: {
        http: {
          containerPort: 80
          provides: webspaHttp.id
        }
      }
    }
    traits: []
    connections: {
      webshoppingagg: {
        kind: 'Http'
        source: webshoppingaggHttp.id
      }
      identity: {
        kind: 'Http'
        source: identityHttp.id
      }
      webshoppingapigw: {
        kind: 'Http'
        source: webshoppingapigwHttp.id
      }
      orderingsignalrhub: {
        kind: 'Http'
        source: orderingsignalrhubHttp.id
      }
    }
  }
}

resource webspaHttp 'radius.dev/Application/HttpRoute@v1alpha3' = {
  name: '${app.name}/webspa-http'
  properties: {
    port: 5104
  }
}

output webspa object = webspa
output webspaHttp object = webspaHttp
