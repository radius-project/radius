param app object
param sqlWebhooks object
param servicebus object
param identityHttp object

param ESHOP_EXTERNAL_DNS_NAME_OR_IP string
param OCHESTRATOR_TYPE string

// Based on https://github.com/dotnet-architecture/eShopOnContainers/tree/dev/deploy/k8s/helm/webhooks-api
resource webhooks 'radius.dev/Application/ContainerComponent@v1alpha3' = {
  name: '${app.name}/webhooks-api'
  properties: {
    container: {
      image: 'eshop/webhooks.api:latest'
      env: {
        'ASPNETCORE_ENVIRONMENT': 'Development'
        'ASPNETCORE_URLS': 'http://0.0.0.0:80'
        'OrchestratorType': OCHESTRATOR_TYPE
        'AzureServiceBusEnabled': 'True'
        'ConnectionString': sqlWebhooks.connectionString()
        'EventBusConnection': servicebus.queueConnectionString()
        'identityUrl': identityHttp.properties.url
        'IdentityUrlExternal': identityHttp.properties.url
      }
      ports: {
        http: {
          containerPort: 80
          provides: webhooksHttp.id
        }
      }
    }
    traits: []
    connections: {
      sql: {
        kind: 'microsoft.com/SQL'
        source: sqlWebhooks.id
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

resource webhooksHttp 'radius.dev/Application/HttpRoute@v1alpha3' = {
  name: '${app.name}/webhooks-http'
  properties: {
    port: 5113
  }
}

// Based on https://github.com/dotnet-architecture/eShopOnContainers/tree/dev/deploy/k8s/helm/webhooks-web
resource webhooksclient 'radius.dev/Application/ContainerComponent@v1alpha3' = {
  name: '${app.name}/webhooks-client'
  properties: {
    container: {
      image: 'eshop/webhooks.client:latest'
      env: {
        'ASPNETCORE_URLS': 'http://0.0.0.0:80'
        'Token': '6168DB8D-DC58-4094-AF24-483278923590' // Webhooks are registered with this token (any value is valid) but the client won't check it
        'CallBackUrl': 'http://${ESHOP_EXTERNAL_DNS_NAME_OR_IP}:5114'
        'SelfUrl': 'http://webhooks-client/'
        'WebhooksUrl': webhooksHttp.properties.url
        'IdentityUrl': identityHttp.properties.url
      }
      ports: {
        http: {
          containerPort: 80
          provides: webhooksclientHttp.id
        }
      }
    }
    traits: []
    connections: {
      webhooks: {
        kind: 'Http'
        source: webhooksHttp.id
      }
      identity: {
        kind: 'Http'
        source: identityHttp.id
      }
    }
  }
}

resource webhooksclientHttp 'radius.dev/Application/HttpRoute@v1alpha3' = {
  name: '${app.name}/webhooksclient-http'
  properties: {
    port: 5114
  }
}

output webhooks object = webhooks
output webhooksHttp object = webhooksHttp
output webhooksclient object = webhooksclient
output webhooksclientHttp object = webhooksclientHttp
