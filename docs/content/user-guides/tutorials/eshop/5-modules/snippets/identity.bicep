param app object
param sqlIdentity object
param webmvcHttp object
param webspaHttp object
param basketHttp object
param orderingHttp object
param webshoppingaggHttp object
param webhooksHttp object
param webhooksclientHttp object

param OCHESTRATOR_TYPE string
param APPLICATION_INSIGHTS_KEY string


// Based on https://github.com/dotnet-architecture/eShopOnContainers/tree/dev/deploy/k8s/helm/identity-api
resource identity 'radius.dev/Application/ContainerComponent@v1alpha3' = {
  name: '${app.name}/identity-api'
  properties: {
    container: {
      image: 'eshop/identity.api:latest'
      env: {
        'PATH_BASE': '/identity-api'
        'ASPNETCORE_ENVIRONMENT': 'Development'
        'ASPNETCORE_URLS': 'http://0.0.0.0:80'
        'OrchestratorType': OCHESTRATOR_TYPE
        'IsClusterEnv': 'True'
        'DPConnectionString': ''
        'ApplicationInsights__InstrumentationKey': APPLICATION_INSIGHTS_KEY
        'XamarinCallback': ''
        'EnableDevspaces': 'False'
        'ConnectionString': sqlIdentity.connectionString()
        'MvcClient': webmvcHttp.properties.url
        'SpaClient': webspaHttp.properties.url
        'BasketApiClient': basketHttp.properties.url
        'OrderingApiClient': orderingHttp.properties.url
        'WebShoppingAggClient': webshoppingaggHttp.properties.url
        'WebhooksApiClient': webhooksHttp.properties.url
        'WebhooksWebClient': webhooksclientHttp.properties.url
      }
      ports: {
        http: {
          containerPort: 80
          provides: identityHttp.id
        }
      }
    }
    traits: []
    connections: {
      sql: {
        kind: 'microsoft.com/SQL'
        source: sqlIdentity.id
      }
      webmvc: {
        kind: 'Http'
        source: webmvcHttp.id
      }
      webspa: {
        kind: 'Http'
        source: webspaHttp.id
      }
      basket: {
        kind: 'Http'
        source: basketHttp.id
      }
      ordering: {
        kind: 'Http'
        source: orderingHttp.id
      }
      webshoppingagg: {
        kind: 'Http'
        source: webshoppingaggHttp.id
      }
      webhooks: {
        kind: 'Http'
        source: webhooksHttp.id
      }
      webhoolsclient: {
        kind: 'Http'
        source: webhooksclientHttp.id
      }
    }
  }
}

resource identityHttp 'radius.dev/Application/HttpRoute@v1alpha3' = {
  name: '${app.name}/identity-http'
  properties: {
    port: 5105
  }
}

output identity object = identity
output identityHttp object = identityHttp
