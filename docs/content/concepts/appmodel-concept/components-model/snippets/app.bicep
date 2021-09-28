resource app 'radius.dev/Application@v1alpha3' = {
  name: 'shopping-app'

  //CONTAINER
  resource store 'ContainerComponent' = {
    name: 'storefront'
    properties: {
      container: {
        image: 'radius.azurecr.io/storefront'
      }
    }
  }
  //CONTAINER

  //STATESTORE
  resource inventory 'dapr.io.StateStoreComponent' = {
    name: 'inventorystore'
    properties: {
      kind: 'state.azure.tablestorage'
      managed: true
    }
  }
  //STATESTORE
}
