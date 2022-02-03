resource app 'radius.dev/Application@v1alpha3' = {
  name: 'shopping-app'

  //CONTAINER
  resource store 'Container' = {
    name: 'storefront'
    properties: {
      container: {
        image: 'radius.azurecr.io/storefront'
        env: {
          ENVIRONMENT: 'production'
          INV_STATESTORE: inventory.name
        }
        ports: {
          http: {
            containerPort: 80
          }
        }
      }
      connections: {
        inventory: {
          kind: 'dapr.io/StateStore'
          source: inventory.id
        }
      }
    }
  }
  //CONTAINER

  //STATESTORE
  resource inventory 'dapr.io.StateStore' = {
    name: 'inventorystore'
    properties: {
      kind: 'state.azure.tablestorage'
      managed: true
    }
  }
  //STATESTORE
}
  
