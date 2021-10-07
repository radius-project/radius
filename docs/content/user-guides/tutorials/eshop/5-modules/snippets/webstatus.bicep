param app object
param webmvcHttp object
param webspaHttp object
param webshoppingaggHttp object
param orderingHttp object
param basketHttp object
param catalogHttp object
param identityHttp object
param paymentHttp object
param orderingsignalrhubHttp object
param orderbgtasksHttp object

param APPLICATION_INSIGHTS_KEY string
param OCHESTRATOR_TYPE string

// Based on https://github.com/dotnet-architecture/eShopOnContainers/tree/dev/deploy/k8s/helm/webstatus
resource webstatus 'radius.dev/Application/ContainerComponent@v1alpha3' = {
  name: '${app.name}/webstatus'
  properties: {
    container: {
      image: 'eshop/webstatus:latest'
      env: {
        'ASPNETCORE_ENVIRONMENT': 'Development'
        'ASPNETCORE_URLS': 'http://0.0.0.0:80'
        'HealthChecksUI__HealthChecks__0__Name': 'WebMVC HTTP Check'
        'HealthChecksUI__HealthChecks__0__Uri': '${webmvcHttp.properties.url}/hc'
        'HealthChecksUI__HealthChecks__1__Name': 'WebSPA HTTP Check'
        'HealthChecksUI__HealthChecks__1__Uri': '${webspaHttp.properties.url}/hc'
        'HealthChecksUI__HealthChecks__2__Name': 'Web Shopping Aggregator GW HTTP Check'
        'HealthChecksUI__HealthChecks__2__Uri': '${webshoppingaggHttp.properties.url}/hc'
        'HealthChecksUI__HealthChecks__4__Name': 'Ordering HTTP Check'
        'HealthChecksUI__HealthChecks__4__Uri': '${orderingHttp.properties.url}/hc'
        'HealthChecksUI__HealthChecks__5__Name': 'Basket HTTP Check'
        'HealthChecksUI__HealthChecks__5__Uri': '${basketHttp.properties.url}/hc'
        'HealthChecksUI__HealthChecks__6__Name': 'Catalog HTTP Check'
        'HealthChecksUI__HealthChecks__6__Uri': '${catalogHttp.properties.url}/hc'
        'HealthChecksUI__HealthChecks__7__Name': 'Identity HTTP Check'
        'HealthChecksUI__HealthChecks__7__Uri': '${identityHttp.properties.url}/hc'
        'HealthChecksUI__HealthChecks__8__Name': 'Payments HTTP Check'
        'HealthChecksUI__HealthChecks__8__Uri': '${paymentHttp.properties.url}/hc'
        'HealthChecksUI__HealthChecks__9__Name': 'Ordering SignalRHub HTTP Check'
        'HealthChecksUI__HealthChecks__9__Uri': '${orderingsignalrhubHttp.properties.url}/hc'
        'HealthChecksUI__HealthChecks__10__Name': 'Ordering HTTP Background Check'
        'HealthChecksUI__HealthChecks__10__Uri': '${orderbgtasksHttp.properties.url}/hc'
        'ApplicationInsights__InstrumentationKey': APPLICATION_INSIGHTS_KEY
        'OrchestratorType': OCHESTRATOR_TYPE
      }
      ports: {
        http: {
          containerPort: 80
          provides: webstatusHttp.id
        }
      }
    }
    traits: []
    connections: {}
  }
}

resource webstatusHttp 'radius.dev/Application/HttpRoute@v1alpha3' = {
  name: '${app.name}/webstatus-http'
  properties: {
    port: 8107
  }
}

output webstatus object = webstatus
output webstatusHttp object = webstatusHttp
