resource app 'radius.dev/Application@v1alpha3' = {
  name: 'shopping-app'

  resource store 'Container' = {
    name: 'storefront'
    properties: {
      container: {
        image: 'radius.azurecr.io/storefront'
        ports: {
          web: {
            containerPort: 80
            provides: storefrontHttp.id
          }
        }
      }
      connections: {
        inventory: {
          kind: 'dapr.io/StateStore'
          source: inventory.id
        }
      }
      traits: [
        {
          kind: 'dapr.io/Sidecar@v1alpha1'
          appPort: 3000
          appId: 'storefront'
          provides: storefrontDapr.id
        }
      ]
    }
  }

  resource storefrontDapr 'dapr.io.InvokeHttpRoute' = {
    name: 'storefront-dapr'
    properties: {
      appId: 'storefront'
    }
  }

  resource storefrontHttp 'HttpRoute' = {
    name: 'storefront-http'
    properties: {
      gateway: {
        hostname: 'example.com'
      }
    }
  }

  resource cart 'Container' = {
    name: 'cart-api'
    properties: {
      container: {
        image: 'radiusteam/cart-api'
        env: {
          STOREFRONT_ID: storefrontDapr.properties.appId
        }
      }
      connections: {
        store: {
          kind: 'dapr.io/InvokeHttp'
          source: storefrontDapr.id
        }
      }
      traits: [
        {
          kind: 'dapr.io/Sidecar@v1alpha1'
          appId: 'cart-api'
        }
      ]
    }
  }

  resource inventory 'dapr.io.StateStore' = {
    name: 'inventorystore'
    properties: {
      kind: 'state.azure.tablestorage'
      managed: true
    }
  }
}
