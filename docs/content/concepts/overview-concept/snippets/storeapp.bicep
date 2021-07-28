resource app 'radius.dev/Applications@v1alpha1' = {
  name: 'shopping-app'  
  
  resource store 'Components' = {
    name: 'storefront'
    kind: 'radius.dev/Container@v1alpha1'
    properties: {
      run: {
        container: {
          image: 'radius.azurecr.io/storefront'
        }
      }
      bindings: {
        web: {
          kind: 'http'
          targetPort: 80
        }
        invoke: {
          kind: 'dapr.io/Invoke'
        }
      }
      uses: [
        {
          binding: inventory.properties.bindings.default
        }
      ]
      traits: [
        {
           kind: 'dapr.io/App@v1alpha1'
           appId: 'storefront'
           appPort: 80
        }
      ]
    }
  }

  resource cart 'Components' = {
    name: 'cart-api'
    kind: 'radius.dev/Container@v1alpha1'
    properties: {
      run: {
        container: {
            image: 'radiusteam/cart-api'
        }
      }
      uses: [
        {
          binding: store.properties.bindings.invoke
        }
      ]
      traits: [
        {
          kind: 'dapr.io/App@v1alpha1'
          appId: 'cart-api'
        }
      ]
    }
  }

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

}
