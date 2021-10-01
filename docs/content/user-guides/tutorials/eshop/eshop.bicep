param ESHOP_EXTERNAL_DNS_NAME_OR_IP string = 'localhost'
param OCHESTRATOR_TYPE string = 'K8S'
param APPLICATION_INSIGHTS_KEY string = ''

resource eshop 'radius.dev/Application@v1alpha3' = {
  name: 'eShop'

  // APIs -----------------------------------------------

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
          'ConnectionString': sqldbi.connectionString()
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
            //Port 5105
          }
        }
      }
      traits: [
        {
          kind: 'radius.dev/InboundRoute@v1alpha1'
          binding: 'http'
        }
      ]
      connections: {
        sql: {
          kind: 'microsoft.com/SQL'
          source: sqldbi.id
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
  }

  // Based on https://github.com/dotnet-architecture/eShopOnContainers/tree/dev/deploy/k8s/helm/catalog-api
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
          'ConnectionString': sqldbc.connectionString()
          'EventBusConnection': servicebus.connectionString()
        }
        ports: {
          http: {
            containerPort: 80
            provides: catalogHttp.id
            // Port 5101
          }
          grpc: {
            containerPort: 81
            // Port 9101
          }
        }
      }
      traits: [
        {
          kind: 'radius.dev/InboundRoute@v1alpha1'
          binding: 'http'
        }
      ]
      connections: {
        sql: {
          kind: 'microsoft.com/SQL'
          source: sqldbc.id
        }
        servicebus: {
          kind: 'azure.com/ServiceBusQueue'
          source: servicebus.id
        }
      }
    }
  }

  resource catalogHttp 'HttpRoute' = {
    name: 'catalog-http'
  }

  resource catalogGrpc 'GrpcRoute' = {
    name: 'catalog-grpc'
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
          'ApplicationInsights__InstrumentationKey': '${APPLICATION_INSIGHTS_KEY}'
          'OrchestratorType': '${OCHESTRATOR_TYPE}'
          'UseLoadTest': 'False'
          'Serilog__MinimumLevel__Override__Microsoft.eShopOnContainers.BuildingBlocks.EventBusRabbitMQ': 'Verbose'
          'Serilog__MinimumLevel__Override__ordering-api': 'Verbose'
          'PATH_BASE': '/ordering-api'
          'GRPC_PORT': '81'
          'PORT': '80'
          'ConnectionString': sqldbo.connectionString()
          'EventBusConnection': servicebus.connectionString()
          'identityUrl': identityHttp.properties.url
          'IdentityUrlExternal': identityHttp.properties.url
        }
        ports: {
          http: {
            containerPort: 80
            provides: orderingHttp.id
            //Port 5102
          }
          grpc: {
            containerPort: 81
            provides: orderingGrpc.id
            //Port 9102
          }
        }
      }
      traits: []
      connections: {
        sql: {
          kind: 'microsoft.com/SQL'
          source: sqldbo.id
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
  }

  resource orderingGrpc 'GrpcRoute' = {
    name: 'ordering-grpc'
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
          'EventBusConnection': servicebus.connectionString()
          'identityUrl': identityHttp.properties.url
          'IdentityUrlExternal': identityHttp.properties.url
        }
        ports: {
          http: {
            containerPort: 80
            provides: basketHttp.id
            //Port 5103
          }
          grpc: {
            containerPort: 81
            provides: basketGrpc.id
            //Port 9103
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
  }

  resource basketGrpc 'GrpcRoute' = {
    name: 'basket-grpc'
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
          'ConnectionString': sqldbw.connectionString()
          'EventBusConnection': servicebus.connectionString()
          'identityUrl': identityHttp.properties.url
          'IdentityUrlExternal': identityHttp.properties.url
        }
        ports: {
          http: {
            containerPort: 80
            provides: webhooksHttp.id
            //Port 5113
          }
        }
      }
      traits: []
      connections: {
        sql: {
          kind: 'microsoft.com/SQL'
          source: sqldbw.id
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
          'EventBusConnection': servicebus.connectionString()
        }
        ports: {
          http: {
            containerPort: 80
            provides: paymentHttp.id
            //Port 5108
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
          'ConnectionString': sqldbo.connectionString()
          'EventBusConnection': servicebus.connectionString()
        }
        ports: {
          http: {
            containerPort: 80
            provides: orderbgtasksHttp.id
            //Port 5111
          }
        }
      }
      traits: []
      connections: {
        sql: {
          kind: 'microsoft.com/SQL'
          source: sqldbo.id
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
            //Port 5121
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
            //Port 5202
          }
          http2: {
            containerPort: 8001
            provides: webshoppingapigwHttp2.id
            //Port 15202
          }
        }
      }
      traits: []
      connections: {}
    }
  }

  resource webshoppingapigwHttp 'HttpRoute' = {
    name: 'webshoppingapigw-http'
  }

  resource webshoppingapigwHttp2 'HttpRoute' = {
    name: 'webshoppingapigw-http-2'
  }

  // Based on https://github.com/dotnet-architecture/eShopOnContainers/tree/dev/deploy/k8s/helm/ordering-signalrhub
  resource orderingsignalrhub 'ContainerComponent' = {
    name: 'ordering-signalrhub'
    kind: 'radius.dev/Container@v1alpha1'
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
          'EventBusConnection': servicebus.connectionString()
          'identityUrl': identityHttp.properties.url
          'IdentityUrlExternal': identityHttp.properties.url
        }
        ports: {
          http: {
            containerPort: 80
            provides: orderingsignalrhubHttp.id
            //Port 5112
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
            //Port 5114
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
          'HealthChecksUI__HealthChecks__0__Uri': 'http://webmvc/hc'
          'HealthChecksUI__HealthChecks__1__Name': 'WebSPA HTTP Check'
          'HealthChecksUI__HealthChecks__1__Uri': 'http://webspa/hc'
          'HealthChecksUI__HealthChecks__2__Name': 'Web Shopping Aggregator GW HTTP Check'
          'HealthChecksUI__HealthChecks__2__Uri': 'http://webshoppingagg/hc'
          'HealthChecksUI__HealthChecks__3__Name': 'Mobile Shopping Aggregator HTTP Check'
          'HealthChecksUI__HealthChecks__3__Uri': 'http://mobileshoppingagg/hc'
          'HealthChecksUI__HealthChecks__4__Name': 'Ordering HTTP Check'
          'HealthChecksUI__HealthChecks__4__Uri': 'http://ordering-api/hc'
          'HealthChecksUI__HealthChecks__5__Name': 'Basket HTTP Check'
          'HealthChecksUI__HealthChecks__5__Uri': 'http://basket-api/hc'
          'HealthChecksUI__HealthChecks__6__Name': 'Catalog HTTP Check'
          'HealthChecksUI__HealthChecks__6__Uri': 'http://catalog-api/hc'
          'HealthChecksUI__HealthChecks__7__Name': 'Identity HTTP Check'
          'HealthChecksUI__HealthChecks__7__Uri': 'http://identity-api/hc'
          'HealthChecksUI__HealthChecks__8__Name': 'Payments HTTP Check'
          'HealthChecksUI__HealthChecks__8__Uri': 'http://payment-api/hc'
          'HealthChecksUI__HealthChecks__9__Name': 'Ordering SignalRHub HTTP Check'
          'HealthChecksUI__HealthChecks__9__Uri': 'http://ordering-signalrhub/hc'
          'HealthChecksUI__HealthChecks__10__Name': 'Ordering HTTP Background Check'
          'HealthChecksUI__HealthChecks__10__Uri': 'http://ordering-backgroundtasks/hc'
          'ApplicationInsights__InstrumentationKey': APPLICATION_INSIGHTS_KEY
          'OrchestratorType': OCHESTRATOR_TYPE
        }
        ports: {
          http: {
            containerPort: 80
            provides: webstatusHttp.id
            //Port 8107
          }
        }
      }
      traits: []
      connections: {}
    }
  }

  resource webstatusHttp 'HttpRoute' = {
    name: 'webstatus-http'
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
          'IdentityUrlHC': 'http://${identityHttp.properties.host}/hc'
          'PurchaseUrl': webshoppingapigwHttp.properties.url
          'SignalrHubUrl': orderingsignalrhubHttp.properties.url
        }
        ports: {
          http: {
            containerPort: 80
            provides: webspaHttp.id
            //Port 5104
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
          'IdentityUrlHC': 'http://${identityHttp.properties.host}/hc'
          'PurchaseUrl': webshoppingapigwHttp.properties.url
          'SignalrHubUrl': orderingsignalrhubHttp.properties.url
        }
        ports: {
          http: {
            containerPort: 80
            provides: webmvcHttp.id
            //Port 5100
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
            provides:seqHttp.id
            //Ports 5340
          }
        }
      }
      traits: []
      connections: {}
    }
  }

  resource seqHttp 'HttpRoute' = {
    name: 'seq-http'
  }

  // Resources ------------------------------------------

  resource servicebus 'azure.com.ServiceBusQueueComponent' = {
    name: 'servicebus'
    properties: {
      managed: true
      queue: 'orders'
    }
  }

  resource sqldbi 'azure.com.CosmosDBSQLComponent' = {
    name: 'sqldb-identity'
    properties: {
      managed: true
    }
  }

  resource sqldbc 'azure.com.CosmosDBSQLComponent' = {
    name: 'sqldb-catalog'
    properties: {
      managed: true
    }
  }

  resource sqldbo 'azure.com.CosmosDBMongoComponent' = {
    name: 'sqldb-ordering'
    properties: {
      managed: true
    }
  }

  resource sqldbw 'azure.com.CosmosDBSQLComponent' = {
    name: 'sqldb-webhooks'
    properties: {
      managed: true
    }
  }

  resource redis 'redislabs.com.RedisComponent' = {
    name: 'redis'
    properties: {
      managed: true
    }
  }

  resource mongodb 'mongodb.com.MongoComponent' = {
    name: 'mongodb'
    properties: {
      managed: true
    }
  }
}
