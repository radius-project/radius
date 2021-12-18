resource app 'radius.dev/Application@v1alpha3' = {
  name: 'shopping-app'

  //CONTAINER
  resource store 'Container' = {
    name: 'storefront'
    properties: {
      container: {
        image: 'radius.azurecr.io/storefront'
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
