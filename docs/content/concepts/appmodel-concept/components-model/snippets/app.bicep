resource app 'radius.dev/Applications@v1alpha1' = {
  name: 'shopping-app'

  //CONTAINER
  resource store 'Components' = {
    name: 'storefront'
    kind: 'radius.dev/Container@v1alpha1'
    properties: {
      run: {
        container: {
          image: 'radius.azurecr.io/storefront'
        }
      }
    }
  }
  //CONTAINER

  //STATESTORE
  resource inventory 'Components' = {
    name: 'inventorystore'
    kind: 'dapr.io/StateStore@v1alpha1'
    properties: {
      config: {
        kind: 'state.azure.tablestorage'
        managed: true
      }
    }
  }
//STATESTORE

}
