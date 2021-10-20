param app object
param servicebus object
param identityHttp object
param orderingHttp object
param catalogHttp object
param basketHttp object

param ESHOP_EXTERNAL_DNS_NAME_OR_IP string

// Based on https://github.com/dotnet-architecture/eShopOnContainers/tree/dev/deploy/k8s/helm/webshoppingagg
resource webshoppingagg 'radius.dev/Application/ContainerComponent@v1alpha3' = {
  name: '${app.name}/webshoppingagg'
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

resource webshoppingaggHttp 'radius.dev/Application/HttpRoute@v1alpha3' = {
  name: '${app.name}/webshoppingagg-http'
  properties: {
    port: 5121
  }
}

output webshoppingagg object = webshoppingagg
output webshoppingaggHttp object = webshoppingaggHttp
