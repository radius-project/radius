param ESHOP_EXTERNAL_DNS_NAME_OR_IP string = '*'
param CLUSTER_IP string
param OCHESTRATOR_TYPE string = 'K8S'
param APPLICATION_INSIGHTS_KEY string = ''
param AZURESTORAGEENABLED string = 'False'
param AZURESERVICEBUSENABLED string = 'False'
param ENABLEDEVSPACES string = 'False'
param TAG string = 'linux-dev'

var CLUSTERDNS = 'http://${CLUSTER_IP}.nip.io'
var PICBASEURL = '${CLUSTERDNS}/webshoppingapigw/c/api/v1/catalog/items/[0]/pic'

param adminLogin string = 'SA'
@secure()
param adminPassword string

resource eshop 'radius.dev/Application@v1alpha3' = {
  name: 'eshop'

  // Based on https://github.com/dotnet-architecture/eShopOnContainers/tree/dev/deploy/k8s/helm/catalog-api
  resource catalog 'Container' = {
    name: 'catalog-api'
    properties: {
      container: {
        image: 'eshop/catalog.api:${TAG}'
        env: {
          UseCustomizationData: 'False'
          PATH_BASE: '/catalog-api'
          ASPNETCORE_ENVIRONMENT: 'Development'
          OrchestratorType: OCHESTRATOR_TYPE
          PORT: '80'
          GRPC_PORT: '81'
          PicBaseUrl: PICBASEURL
          AzureStorageEnabled: AZURESTORAGEENABLED
          ApplicationInsights__InstrumentationKey: APPLICATION_INSIGHTS_KEY
          AzureServiceBusEnabled: AZURESERVICEBUSENABLED
          ConnectionString: 'Server=${sqlRoute.properties.host};Initial Catalog=CatalogDb;User Id=${adminLogin};Password=${adminPassword};'
          EventBusConnection: 'eshop-rabbitmq'
        }
        ports: {
          http: {
            containerPort: 80
            provides: catalogHttp.id
          }
          grpc: {
            containerPort: 81
            provides: catalogGrpc.id
          }
        }
      }
      connections: {
        sql: {
          kind: 'Http'
          source: sqlRoute.id
        }
        servicebus: {
          kind: 'rabbitmq.com/MessageQueue'
          source: rabbitmq.id
        }
      }
    }
  }

  resource catalogHttp 'HttpRoute' = {
    name: 'catalog-http'
    properties: {
      port: 5101
    }
  }

  resource catalogGrpc 'HttpRoute' = {
    name: 'catalog-grpc'
    properties: {
      port: 9101
    }
  }

  // Based on https://github.com/dotnet-architecture/eShopOnContainers/tree/dev/deploy/k8s/helm/identity-api
  resource identity 'Container' = {
    name: 'identity-api'
    properties: {
      container: {
        image: 'eshop/identity.api:${TAG}'
        env: {
          PATH_BASE: '/identity-api'
          ASPNETCORE_ENVIRONMENT: 'Development'
          ASPNETCORE_URLS: 'http://0.0.0.0:80'
          OrchestratorType: 'K8S'
          IsClusterEnv: 'True' 
          DPConnectionString: '${redisKeystore.properties.host}'
          ApplicationInsights__InstrumentationKey: APPLICATION_INSIGHTS_KEY
          XamarinCallback: ''
          EnableDevspaces: ENABLEDEVSPACES
          ConnectionString: 'Server=${sqlRoute.properties.host};Initial Catalog=IdentityDb;User Id=${adminLogin};Password=${adminPassword};'
          MvcClient: '${CLUSTERDNS}${webmvcHttp.properties.gateway.rules.webmvc.path.value}'
          SpaClient: CLUSTERDNS
          BasketApiClient: '${CLUSTERDNS}${basketHttp.properties.gateway.rules.basket.path.value}'
          OrderingApiClient: '${CLUSTERDNS}${orderingHttp.properties.gateway.rules.ordering.path.value}'
          WebShoppingAggClient: '${CLUSTERDNS}${webshoppingaggHttp.properties.gateway.rules.webshoppingagg.path.value}'
          WebhooksApiClient: '${CLUSTERDNS}${webhooksHttp.properties.gateway.rules.webhooks.path.value}'
          WebhooksWebClient: '${CLUSTERDNS}${webhooksclientHttp.properties.gateway.rules.webhooks.path.value}'
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
        redis: {
          kind: 'redislabs.com/Redis'
          source: redisKeystore.id
        }
        sql: {
          kind: 'Http'
          source: sqlRoute.id
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
      gateway: {
        source: gateway.id
        hostname: ESHOP_EXTERNAL_DNS_NAME_OR_IP
        rules: {
          identity: {
            path: {
              value: '/identity-api'
            }
          }
        }
      }
    }
  }

  // Based on https://github.com/dotnet-architecture/eShopOnContainers/tree/dev/deploy/k8s/helm/ordering-api
  resource ordering 'Container' = {
    name: 'ordering-api'
    properties: {
      container: {
        image: 'eshop/ordering.api:${TAG}'
        env: {
          ASPNETCORE_ENVIRONMENT: 'Development'
          ASPNETCORE_URLS: 'http://0.0.0.0:80'
          UseCustomizationData: 'False'
          AzureServiceBusEnabled: AZURESERVICEBUSENABLED
          CheckUpdateTime: '30000'
          ApplicationInsights__InstrumentationKey: APPLICATION_INSIGHTS_KEY
          OrchestratorType: OCHESTRATOR_TYPE
          UseLoadTest: 'False'
          'Serilog__MinimumLevel__Override__Microsoft.eShopOnContainers.BuildingBlocks.EventBusRabbitMQ': 'Verbose'
          'Serilog__MinimumLevel__Override__ordering-api': 'Verbose'
          PATH_BASE: '/ordering-api'
          GRPC_PORT: '81'
          PORT: '80'
          ConnectionString: 'Server=${sqlRoute.properties.host};Initial Catalog=OrderingDb;User Id=${adminLogin};Password=${adminPassword};'
          EventBusConnection: 'eshop-rabbitmq'
          identityUrl: identityHttp.properties.url
          IdentityUrlExternal: '${CLUSTERDNS}${identityHttp.properties.gateway.rules.identity.path.value}'
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
          kind: 'Http'
          source: sqlRoute.id
        }
        servicebus: {
          kind: 'rabbitmq.com/MessageQueue'
          source: rabbitmq.id
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
      gateway: {
        source: gateway.id
        hostname: ESHOP_EXTERNAL_DNS_NAME_OR_IP
        rules: {
          ordering: {
            path: {
              value: '/ordering-api'
            }
          }
        }
      }
    }
  }

  resource orderingGrpc 'HttpRoute' = {
    name: 'ordering-grpc'
    properties: {
      port: 9102
    }
  }

  // Based on https://github.com/dotnet-architecture/eShopOnContainers/tree/dev/deploy/k8s/helm/basket-api
  resource basket 'Container' = {
    name: 'basket-api'
    properties: {
      container: {
        image: 'eshop/basket.api:${TAG}'
        env: {
          ASPNETCORE_ENVIRONMENT: 'Development'
          ASPNETCORE_URLS: 'http://0.0.0.0:80'
          ApplicationInsights__InstrumentationKey: APPLICATION_INSIGHTS_KEY
          UseLoadTest: 'False'
          PATH_BASE: '/basket-api'
          OrchestratorType: 'K8S'
          PORT: '80'
          GRPC_PORT: '81'
          AzureServiceBusEnabled: AZURESERVICEBUSENABLED
          ConnectionString: '${redisBasket.properties.host}:${redisBasket.properties.port}'
          EventBusConnection: 'eshop-rabbitmq'
          identityUrl: identityHttp.properties.url
          IdentityUrlExternal: '${CLUSTERDNS}${identityHttp.properties.gateway.rules.identity.path.value}'
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
          source: redisBasket.id
        }
        servicebus: {
          kind: 'rabbitmq.com/MessageQueue'
          source: rabbitmq.id
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
      gateway: {
        source: gateway.id
        hostname: ESHOP_EXTERNAL_DNS_NAME_OR_IP
        rules: {
          basket: {
            path: {
              value: '/basket-api'
            }
          }
        }
      }
    }
  }

  resource basketGrpc 'HttpRoute' = {
    name: 'basket-grpc'
    properties: {
      port: 9103
    }
  }

  // Based on https://github.com/dotnet-architecture/eShopOnContainers/tree/dev/deploy/k8s/helm/webhooks-api
  resource webhooks 'Container' = {
    name: 'webhooks-api'
    properties: {
      container: {
        image: 'eshop/webhooks.api:linux-dev'
        env: {
          PATH_BASE: '/webhooks-api'
          ASPNETCORE_ENVIRONMENT: 'Development'
          ASPNETCORE_URLS: 'http://0.0.0.0:80'
          OrchestratorType: OCHESTRATOR_TYPE
          AzureServiceBusEnabled: AZURESERVICEBUSENABLED
          ConnectionString: 'Server=${sqlRoute.properties.host};Initial Catalog=WebhookDb;User Id=${adminLogin};Password=${adminPassword};'
          EventBusConnection: 'eshop-rabbitmq'
          identityUrl: identityHttp.properties.url
          IdentityUrlExternal: '${CLUSTERDNS}${identityHttp.properties.gateway.rules.identity.path.value}'
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
          kind: 'Http'
          source: sqlRoute.id
        }
        servicebus: {
          kind: 'rabbitmq.com/MessageQueue'
          source: rabbitmq.id
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
      gateway: {
        source: gateway.id
        hostname: ESHOP_EXTERNAL_DNS_NAME_OR_IP
        rules: {
          webhooks: {
            path: {
              value: '/webhooks-api'
            }
          }
        }
      }
    }
  }

  // Based on https://github.com/dotnet-architecture/eShopOnContainers/tree/dev/deploy/k8s/helm/payment-api
  resource payment 'Container' = {
    name: 'payment-api'
    properties: {
      container: {
        image: 'eshop/payment.api:linux-dev'
        env: {
          ApplicationInsights__InstrumentationKey: APPLICATION_INSIGHTS_KEY
          'Serilog__MinimumLevel__Override__payment-api.IntegrationEvents.EventHandling': 'Verbose'
          'Serilog__MinimumLevel__Override__Microsoft.eShopOnContainers.BuildingBlocks.EventBusRabbitMQ': 'Verbose'
          OrchestratorType: OCHESTRATOR_TYPE
          AzureServiceBusEnabled: AZURESERVICEBUSENABLED
          EventBusConnection: 'eshop-rabbitmq'
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
          kind: 'rabbitmq.com/MessageQueue'
          source: rabbitmq.id
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
  resource orderbgtasks 'Container' = {
    name: 'ordering-backgroundtasks'
    properties: {
      container: {
        image: 'eshop/ordering.backgroundtasks:linux-dev'
        env: {
          ASPNETCORE_ENVIRONMENT: 'Development'
          ASPNETCORE_URLS: 'http://0.0.0.0:80'
          UseCustomizationData: 'False'
          CheckUpdateTime: '30000'
          GracePeriodTime: '1'
          ApplicationInsights__InstrumentationKey: APPLICATION_INSIGHTS_KEY
          UseLoadTest: 'False'
          'Serilog__MinimumLevel__Override__Microsoft.eShopOnContainers.BuildingBlocks.EventBusRabbitMQ': 'Verbose'
          OrchestratorType: OCHESTRATOR_TYPE
          AzureServiceBusEnabled: AZURESERVICEBUSENABLED
          ConnectionString: 'Server=${sqlRoute.properties.host};Initial Catalog=OrderingDb;User Id=${adminLogin};Password=${adminPassword};'
          EventBusConnection: 'eshop-rabbitmq'
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
          kind: 'Http'
          source: sqlRoute.id
        }
        servicebus: {
          kind: 'rabbitmq.com/MessageQueue'
          source: rabbitmq.id
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
  resource webshoppingagg 'Container' = {
    name: 'webshoppingagg'
    properties: {
      container: {
        image: 'eshop/webshoppingagg:${TAG}'
        env: {
          ASPNETCORE_ENVIRONMENT: 'Development'
          PATH_BASE: '/webshoppingagg'
          ASPNETCORE_URLS: 'http://0.0.0.0:80'
          OrchestratorType: OCHESTRATOR_TYPE
          IsClusterEnv: 'True'
          urls__basket: basketHttp.properties.url
          urls__catalog: catalogHttp.properties.url
          urls__orders: orderingHttp.properties.url
          urls__identity: identityHttp.properties.url
          urls__grpcBasket: basketGrpc.properties.url
          urls__grpcCatalog: catalogGrpc.properties.url
          urls__grpcOrdering: orderingGrpc.properties.url
          CatalogUrlHC: '${catalogHttp.properties.url}/hc'
          OrderingUrlHC: '${orderingHttp.properties.url}/hc'
          IdentityUrlHC: '${identityHttp.properties.url}/hc'
          BasketUrlHC: '${basketHttp.properties.url}/hc'
          PaymentUrlHC: '${paymentHttp.properties.url}/hc'
          IdentityUrlExternal: '${CLUSTERDNS}${identityHttp.properties.gateway.rules.identity.path.value}'
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
          kind: 'rabbitmq.com/MessageQueue'
          source: rabbitmq.id
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
      gateway: {
        source: gateway.id
        hostname: ESHOP_EXTERNAL_DNS_NAME_OR_IP
        rules: {
          webshoppingagg: {
            path: {
              value: '/webshoppingagg'
            }
          }
        }
      }
    }
  }

  // Based on https://github.com/dotnet-architecture/eShopOnContainers/tree/dev/deploy/k8s/helm/apigwws
  resource webshoppingapigw 'Container' = {
    name: 'webshoppingapigw'
    properties: {
      container: {
        image: 'radius.azurecr.io/eshop-envoy:0.1.3'
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
      gateway: {
        source: gateway.id
        hostname: ESHOP_EXTERNAL_DNS_NAME_OR_IP
        rules: {
          webshoppingapigw: {
            path: {
              value: '/webshoppingapigw'
            }
          }
        }
      }
    }
  }

  resource webshoppingapigwHttp2 'HttpRoute' = {
    name: 'webshoppingapigw-http-2'
    properties: {
      port: 15202
    }
  }

  // Based on https://github.com/dotnet-architecture/eShopOnContainers/tree/dev/deploy/k8s/helm/ordering-signalrhub
  resource orderingsignalrhub 'Container' = {
    name: 'ordering-signalrhub'
    properties: {
      container: {
        image: 'eshop/ordering.signalrhub:${TAG}'
        env: {
          PATH_BASE: '/payment-api'
          ASPNETCORE_ENVIRONMENT: 'Development'
          ASPNETCORE_URLS: 'http://0.0.0.0:80'
          ApplicationInsights__InstrumentationKey: APPLICATION_INSIGHTS_KEY
          OrchestratorType:  OCHESTRATOR_TYPE
          IsClusterEnv: 'True'
          AzureServiceBusEnabled: AZURESERVICEBUSENABLED
          EventBusConnection: 'eshop-rabbitmq'
          SignalrStoreConnectionString: '${redisKeystore.properties.host}'
          identityUrl: identityHttp.properties.url
          IdentityUrlExternal: '${CLUSTERDNS}${identityHttp.properties.gateway.rules.identity.path.value}'
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
        redis: {
          kind: 'redislabs.com/Redis'
          source: redisKeystore.id
        }
        servicebus: {
          kind: 'rabbitmq.com/MessageQueue'
          source: rabbitmq.id
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
  resource webhooksclient 'Container' = {
    name: 'webhooks-client'
    properties: {
      container: {
        image: 'eshop/webhooks.client:linux-dev'
        env: {
          ASPNETCORE_ENVIRONMENT: 'Production'
          ASPNETCORE_URLS: 'http://0.0.0.0:80'
          PATH_BASE: '/webhooks-web'
          Token: 'WebHooks-Demo-Web'
          CallBackUrl: '${CLUSTERDNS}${webhooksclientHttp.properties.gateway.rules.webhooks.path.value}'
          SelfUrl: webhooksclientHttp.properties.url
          WebhooksUrl: webhooksHttp.properties.url
          IdentityUrl: '${CLUSTERDNS}${identityHttp.properties.gateway.rules.identity.path.value}'

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
      gateway: {
        source: gateway.id
        hostname: ESHOP_EXTERNAL_DNS_NAME_OR_IP
        rules: {
          webhooks: {
            path: {
              value: '/webhooks-web'
            }
          }
        }
      }
    }
  }

  // Sites ----------------------------------------------

  // Based on https://github.com/dotnet-architecture/eShopOnContainers/tree/dev/deploy/k8s/helm/webstatus
  resource webstatus 'Container' = {
    name: 'webstatus'
    properties: {
      container: {
        image: 'eshop/webstatus:${TAG}'
        env: {
          ASPNETCORE_ENVIRONMENT: 'Development'
          ASPNETCORE_URLS: 'http://0.0.0.0:80'
          HealthChecksUI__HealthChecks__0__Name: 'WebMVC HTTP Check'
          HealthChecksUI__HealthChecks__0__Uri: '${webmvcHttp.properties.url}/hc'
          HealthChecksUI__HealthChecks__1__Name: 'WebSPA HTTP Check'
          HealthChecksUI__HealthChecks__1__Uri: '${webspaHttp.properties.url}/hc'
          HealthChecksUI__HealthChecks__2__Name: 'Web Shopping Aggregator GW HTTP Check'
          HealthChecksUI__HealthChecks__2__Uri: '${webshoppingaggHttp.properties.url}/hc'
          HealthChecksUI__HealthChecks__4__Name: 'Ordering HTTP Check'
          HealthChecksUI__HealthChecks__4__Uri: '${orderingHttp.properties.url}/hc'
          HealthChecksUI__HealthChecks__5__Name: 'Basket HTTP Check'
          HealthChecksUI__HealthChecks__5__Uri: '${basketHttp.properties.url}/hc'
          HealthChecksUI__HealthChecks__6__Name: 'Catalog HTTP Check'
          HealthChecksUI__HealthChecks__6__Uri: '${catalogHttp.properties.url}/hc'
          HealthChecksUI__HealthChecks__7__Name: 'Identity HTTP Check'
          HealthChecksUI__HealthChecks__7__Uri: '${identityHttp.properties.url}/hc'
          HealthChecksUI__HealthChecks__8__Name: 'Payments HTTP Check'
          HealthChecksUI__HealthChecks__8__Uri: '${paymentHttp.properties.url}/hc'
          HealthChecksUI__HealthChecks__9__Name: 'Ordering SignalRHub HTTP Check'
          HealthChecksUI__HealthChecks__9__Uri: '${orderingsignalrhubHttp.properties.url}/hc'
          HealthChecksUI__HealthChecks__10__Name: 'Ordering HTTP Background Check'
          HealthChecksUI__HealthChecks__10__Uri: '${orderbgtasksHttp.properties.url}/hc'
          ApplicationInsights__InstrumentationKey: APPLICATION_INSIGHTS_KEY
          OrchestratorType: OCHESTRATOR_TYPE
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
      gateway: {
        source: gateway.id
        hostname: ESHOP_EXTERNAL_DNS_NAME_OR_IP
        rules: {
          webstatus: {
            path: {
              value: '/webstatus'
            }
          }
        }
      }
    }
  }

  // Based on https://github.com/dotnet-architecture/eShopOnContainers/tree/dev/deploy/k8s/helm/webspa
  resource webspa 'Container' = {
    name: 'web-spa'
    properties: {
      container: {
        image: 'eshop/webspa:${TAG}'
        env: {
          PATH_BASE: '/'
          ASPNETCORE_ENVIRONMENT: 'Production'
          ASPNETCORE_URLS: 'http://0.0.0.0:80'
          UseCustomizationData: 'False'
          ApplicationInsights__InstrumentationKey: APPLICATION_INSIGHTS_KEY
          OrchestratorType: OCHESTRATOR_TYPE
          IsClusterEnv: 'True'
          CallBackUrl: '${CLUSTERDNS}/'
          DPConnectionString: '${redisKeystore.properties.host}'
          IdentityUrl: '${CLUSTERDNS}${identityHttp.properties.gateway.rules.identity.path.value}'
          IdentityUrlHC: '${identityHttp.properties.url}/hc'
          PurchaseUrl: '${CLUSTERDNS}${webshoppingapigwHttp.properties.gateway.rules.webshoppingapigw.path.value}'
          SignalrHubUrl: orderingsignalrhubHttp.properties.url
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
        redis: {
          kind: 'redislabs.com/Redis'
          source: redisKeystore.id
        }
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
      gateway: {
        source: gateway.id
        hostname: ESHOP_EXTERNAL_DNS_NAME_OR_IP
        rules: {
          webspa: {
            path: {
              value: '/'
            }
          }
        }
      }
    }
  }

  // Based on https://github.com/dotnet-architecture/eShopOnContainers/tree/dev/deploy/k8s/helm/webmvc
  resource webmvc 'Container' = {
    name: 'webmvc'
    properties: {
      container: {
        image: 'eshop/webmvc:${TAG}'
        env: {
          ASPNETCORE_ENVIRONMENT: 'Development'
          ASPNETCORE_URLS: 'http://0.0.0.0:80'
          PATH_BASE: '/webmvc'
          UseCustomizationData: 'False'
          DPConnectionString: '${redisKeystore.properties.host}'
          ApplicationInsights__InstrumentationKey: APPLICATION_INSIGHTS_KEY
          UseLoadTest: 'False'
          OrchestratorType: OCHESTRATOR_TYPE
          IsClusterEnv: 'True'
          ExternalPurchaseUrl: '${CLUSTERDNS}${webshoppingapigwHttp.properties.gateway.rules.webshoppingapigw.path.value}'
          CallBackUrl: 'http://${CLUSTER_IP}.nip.io/webmvc'
          IdentityUrl: 'http://${CLUSTER_IP}.nip.io/identity-api'
          IdentityUrlHC: '${identityHttp.properties.url}/hc'
          PurchaseUrl: webshoppingapigwHttp.properties.url
          SignalrHubUrl: orderingsignalrhubHttp.properties.url
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
        redis: {
          kind: 'redislabs.com/Redis'
          source: redisKeystore.id
        }
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

  resource webmvcHttp 'HttpRoute' = {
    name: 'webmvc-http'
    properties: {
      port: 5100
      gateway: {
        source: gateway.id
        hostname: ESHOP_EXTERNAL_DNS_NAME_OR_IP
        rules: {
          webmvc: {
            path: {
              value: '/webmvc'
            }
          }
        }
      }
    }
  }

    // Gateway --------------------------------------------

    resource gateway 'Gateway' = {
      name: 'gateway'
      properties: {
        listeners: {
          http: {
            protocol: 'HTTP'
            port: 80
          }
        }
      }
    }

  // Logging --------------------------------------------

  resource seq 'Container' = {
    name: 'seq'
    properties: {
      container: {
        image: 'datalust/seq:latest'
        env: {
          ACCEPT_EULA: 'Y'
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

  resource sqlServer 'Container' = {
    name: 'sql-server'
    properties: {
      container: {
        image: 'mcr.microsoft.com/mssql/server:2019-latest'
        env: {
          ACCEPT_EULA: 'Y'
          MSSQL_PID: 'Developer'
          MSSQL_SA_PASSWORD: adminPassword
        }
        ports: {
          sql: {
            containerPort: 1433
            provides: sqlRoute.id
          }
        }
      }
    }
  }

  resource sqlRoute 'HttpRoute' = {
    name: 'sql-route'
    properties: {
      port: 1433
    }
  }

  resource redisKeystore 'redislabs.com.RedisCache' = {
    name: 'redis-keystore'
    properties: {
      managed: true
    }
  }

  resource redisBasket 'redislabs.com.RedisCache' = {
    name: 'redis-basket'
    properties: {
      managed: true
    }
  }

  resource mongo 'mongo.com.MongoDatabase' = {
    name: 'mongo'
    properties: {
      managed: true
    }
  }

  resource rabbitmq 'rabbitmq.com.MessageQueue' = {
    name: 'rabbitmq'
    properties: {
      managed: true
      queue: 'eshop_event_bus'
    }
  }
  
}
