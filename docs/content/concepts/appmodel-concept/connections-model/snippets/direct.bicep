resource app 'radius.dev/Application@v1alpha3' = {
  name: 'shopping-app'

  //SAMPLE
  resource store 'ContainerComponent' = {
    name: 'storefront'
    properties: {
      //CONTAINER
      container: {
        image: 'radius.azurecr.io/storefront'
      }
      //CONTAINER
      connections: {
        inventory: {
          kind: 'dapr.io/StateStore'
          source: inventory.id
        }
      }
    }
  }

  resource inventory 'dapr.io.StateStoreComponent' = {
    name: 'inventorystore'
    //PROPERTIES
    properties: {
      kind: 'state.azure.tablestorage'
      managed: true
    }
    //PROPERTIES
  }
  //SAMPLE

}
