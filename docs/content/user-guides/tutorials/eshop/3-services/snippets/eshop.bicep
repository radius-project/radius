//PARAMS
param ESHOP_EXTERNAL_DNS_NAME_OR_IP string = 'localhost'
param OCHESTRATOR_TYPE string = 'K8S'
param APPLICATION_INSIGHTS_KEY string = ''
//PARAMS
//SQL
param serverName string = uniqueString('sql', resourceGroup().id)
param location string = resourceGroup().location
param skuName string = 'Standard'
param skuTier string = 'Standard'
param adminLogin string
@secure()
param adminLoginPassword string

resource sql 'Microsoft.Sql/servers@2019-06-01-preview' = {
  name: serverName
  location: location
  properties: {
    administratorLogin: adminLogin
    administratorLoginPassword: adminLoginPassword
  }

  resource identity 'databases' = {
    name: 'identity'
    location: location
    sku: {
      name: skuName
      tier: skuTier
    }
  }

  resource catalog 'databases' = {
    name: 'catalog'
    location: location
    sku: {
      name: skuName
      tier: skuTier
    }
  }

  resource ordering 'databases' = {
    name: 'ordering'
    location: location
    sku: {
      name: skuName
      tier: skuTier
    }
  }

  resource webhooks 'databases' = {
    name: 'webhooks'
    location: location
    sku: {
      name: skuName
      tier: skuTier
    }
  }
}
//SQL

//APP
resource eshop 'radius.dev/Application@v1alpha3' = {
  name: 'eshop'

  // Based on https://github.com/dotnet-architecture/eShopOnContainers/tree/dev/deploy/k8s/helm/catalog-api
  //CATALOG
  resource catalog 'ContainerComponent' = {
    name: 'catalog-api'
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
  resource catalogHttp 'HttpRoute' = {
    name: 'catalog-http'
    properties: {
      port: 5101
      gateway: {
        hostname: ESHOP_EXTERNAL_DNS_NAME_OR_IP
      }
    }
  }
  //ROUTE

  resource catalogGrpc 'HttpRoute' = {
    name: 'catalog-grpc'
    properties: {
      port: 9101
    }
  }

  // Based on https://github.com/dotnet-architecture/eShopOnContainers/tree/dev/deploy/k8s/helm/identity-api
  resource identity 'ContainerComponent' = {
    name: 'identity-api'
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

  resource identityHttp 'HttpRoute' = {
    name: 'identity-http'
    properties: {
      port: 5105
    }
  }

  // Based on https://github.com/dotnet-architecture/eShopOnContainers/tree/dev/deploy/k8s/helm/ordering-api
  resource ordering 'ContainerComponent' = {
    name: 'ordering-api'
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

  resource orderingHttp 'HttpRoute' = {
    name: 'ordering-http'
    properties: {
      port: 5102
    }
  }

  resource orderingGrpc 'HttpRoute' = {
    name: 'ordering-grpc'
    properties: {
      port: 9102
    }
  }

  // Based on https://github.com/dotnet-architecture/eShopOnContainers/tree/dev/deploy/k8s/helm/basket-api
  resource basket 'ContainerComponent' = {
    name: 'basket-api'
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

  resource basketHttp 'HttpRoute' = {
    name: 'basket-http'
    properties: {
      port: 5103
    }
  }

  resource basketGrpc 'HttpRoute' = {
    name: 'basket-grpc'
    properties: {
      port: 9103
    }
  }

  // Based on https://github.com/dotnet-architecture/eShopOnContainers/tree/dev/deploy/k8s/helm/webhooks-api
  resource webhooks 'ContainerComponent' = {
    name: 'webhooks-api'
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

  resource webhooksHttp 'HttpRoute' = {
    name: 'webhooks-http'
    properties: {
      port: 5113
    }
  }

  // Based on https://github.com/dotnet-architecture/eShopOnContainers/tree/dev/deploy/k8s/helm/payment-api
  resource payment 'ContainerComponent' = {
    name: 'payment-api'
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

  resource paymentHttp 'HttpRoute' = {
    name: 'payment-http'
    properties: {
      port: 5108
    }
  }

  // Based on https://github.com/dotnet-architecture/eShopOnContainers/tree/dev/deploy/k8s/helm/ordering-backgroundtasks
  resource orderbgtasks 'ContainerComponent' = {
    name: 'ordering-backgroundtasks'
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

  resource orderbgtasksHttp 'HttpRoute' = {
    name: 'orderbgtasks-http'
    properties: {
      port: 5111
    }
  }

  // Other ---------------------------------------------

  // Based on https://github.com/dotnet-architecture/eShopOnContainers/tree/dev/deploy/k8s/helm/webshoppingagg
  resource webshoppingagg 'ContainerComponent' = {
    name: 'webshoppingagg'
    properties: {
      container: {
        image: 'eshop/webshoppingagg:latest'
        env: {
          'ASPNETCORE_ENVIRONMENT': 'Development'
          'urls__basket': 'http://basket-api'
          'urls__catalog': 'http://catalog-api'
          'urls__orders': 'http://ordering-api'
          'urls__identity': 'http://identity-api'
          'urls__grpcBasket': 'http://basket-api:81'
          'urls__grpcCatalog': 'http://catalog-api:81'
          'urls__grpcOrdering': 'http://ordering-api:81'
          'CatalogUrlHC': 'http://catalog-api/hc'
          'OrderingUrlHC': 'http://ordering-api/hc'
          'IdentityUrlHC': 'http://identity-api/hc'
          'BasketUrlHC': 'http://basket-api/hc'
          'PaymentUrlHC': 'http://payment-api/hc'
          'IdentityUrlExternal': 'http://${ESHOP_EXTERNAL_DNS_NAME_OR_IP}:5105'
        }
        ports: {
          http: {
            containerPort: 80
            provides: webshoppingaggHttp.id
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

  resource webshoppingaggHttp 'HttpRoute' = {
    name: 'webshoppingagg-http'
    properties: {
      port: 5121
    }
  }

  // Based on https://github.com/dotnet-architecture/eShopOnContainers/tree/dev/deploy/k8s/helm/apigwws
  resource webshoppingapigw 'ContainerComponent' = {
    name: 'webshoppingapigw'
    properties: {
      container: {
        image: 'envoyproxy/envoy:v1.11.1'
        env: {}
        ports: {
          http: {
            containerPort: 80
            provides: webshoppingapigwHttp.id
          }
          http2: {
            containerPort: 8001
            provides: webshoppingapigwHttp2.id
          }
        }
      }
      traits: []
      connections: {}
    }
  }

  resource webshoppingapigwHttp 'HttpRoute' = {
    name: 'webshoppingapigw-http'
    properties: {
      port: 5202
    }
  }

  resource webshoppingapigwHttp2 'HttpRoute' = {
    name: 'webshoppingapigw-http-2'
    properties: {
      port: 15202
    }
  }

  // Based on https://github.com/dotnet-architecture/eShopOnContainers/tree/dev/deploy/k8s/helm/ordering-signalrhub
  resource orderingsignalrhub 'ContainerComponent' = {
    name: 'ordering-signalrhub'
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

  resource orderingsignalrhubHttp 'HttpRoute' = {
    name: 'orderingsignalrhub-http'
    properties: {
      port: 5112
    }
  }

  // Based on https://github.com/dotnet-architecture/eShopOnContainers/tree/dev/deploy/k8s/helm/webhooks-web
  resource webhooksclient 'ContainerComponent' = {
    name: 'webhooks-client'
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

  resource webhooksclientHttp 'HttpRoute' = {
    name: 'webhooksclient-http'
    properties: {
      port: 5114
    }
  }

  // Sites ----------------------------------------------

  // Based on https://github.com/dotnet-architecture/eShopOnContainers/tree/dev/deploy/k8s/helm/webstatus
  resource webstatus 'ContainerComponent' = {
    name: 'webstatus'
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

  resource webstatusHttp 'HttpRoute' = {
    name: 'webstatus-http'
    properties: {
      port: 8107
    }
  }

  // Based on https://github.com/dotnet-architecture/eShopOnContainers/tree/dev/deploy/k8s/helm/webspa
  resource webspa 'ContainerComponent' = {
    name: 'web-spa'
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

  resource webspaHttp 'HttpRoute' = {
    name: 'webspa-http'
    properties: {
      port: 5104
    }
  }

  // Based on https://github.com/dotnet-architecture/eShopOnContainers/tree/dev/deploy/k8s/helm/webmvc
  resource webmvc 'ContainerComponent' = {
    name: 'webmvc'
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
          source: identity.id
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

  resource webmvcHttp 'HttpRoute' = {
    name: 'webmvc-http'
    properties: {
      port: 5100
    }
  }

  // Logging --------------------------------------------

  resource seq 'ContainerComponent' = {
    name: 'seq'
    properties: {
      container: {
        image: 'datalust/seq:latest'
        env: {
          'ACCEPT_EULA': 'Y'
        }
        ports: {
          web: {
            containerPort: 80
            provides: seqHttp.id
          }
        }
      }
      traits: []
      connections: {}
    }
  }

  resource seqHttp 'HttpRoute' = {
    name: 'seq-http'
    properties: {
      port: 5340
    }
  }

  // Infrastructure --------------------------------------------

  //RADSQL
  resource sqlIdentity 'microsoft.com.SQLComponent' = {
    name: 'sql-identity'
    properties: {
      resource: sql::identity.id
    }
  }

  resource sqlCatalog 'microsoft.com.SQLComponent' = {
    name: 'sql-catalog'
    properties: {
      resource: sql::identity.id
    }
  }

  resource sqlOrdering 'microsoft.com.SQLComponent' = {
    name: 'sql-ordering'
    properties: {
      resource: sql::identity.id
    }
  }

  resource sqlWebhooks 'microsoft.com.SQLComponent' = {
    name: 'sql-webhooks'
    properties: {
      resource: sql::identity.id
    }
  }
  //RADSQL

  //REDIS
  resource redis 'redislabs.com.RedisComponent' = {
    name: 'redis'
    properties: {
      managed: true
    }
  }
  //REDIS

  //MONGO
  resource mongo 'mongodb.com.MongoDBComponent' = {
    name: 'mongo'
    properties: {
      managed: true
    }
  }
  //MONGO

  //SERVICEBUS
  resource servicebus 'azure.com.ServiceBusQueueComponent' = {
    name: 'servicebus'
    properties: {
      managed: true
      queue: 'orders'
    }
  }
  //SERVICEBUS
}
//APP
