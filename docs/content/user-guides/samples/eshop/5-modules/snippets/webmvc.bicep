param app object
param webshoppingaggHttp object
param identityHttp object
param webshoppingapigwHttp object
param orderingsignalrhubHttp object

param APPLICATION_INSIGHTS_KEY string
param OCHESTRATOR_TYPE string


// Based on https://github.com/dotnet-architecture/eShopOnContainers/tree/dev/deploy/k8s/helm/webmvc
resource webmvc 'radius.dev/Application/ContainerComponent@v1alpha3' = {
  name: '${app.name}/webmvc'
  properties: {
    container: {
      image: 'eshop/webmvc:latest'
      env: {
        'ASPNETCORE_ENVIRONMENT': 'Development'
        'ASPNETCORE_URLS': 'http://0.0.0.0:80'
        'UseCustomizationData': 'False'
        'ApplicationInsights__InstrumentationKey': APPLICATION_INSIGHTS_KEY
        'UseLoadTest': 'False'
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
          provides: webmvcHttp.id
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

resource webmvcHttp 'radius.dev/Application/HttpRoute@v1alpha3' = {
  name: '${app.name}/webmvc-http'
  properties: {
    port: 5100
  }
}

output webmvc object = webmvc
output webmvcHttp object = webmvcHttp
