resource app 'radius.dev/Application@v1alpha3' = {
  name: 'shopping-app'

  //CONTAINER
  resource store 'ContainerComponent' = {
    name: 'storefront'
    properties: {
      container: {
        image: 'radius.azurecr.io/storefront'
        env: {
          PATH_BASE: '/identity-api'
        }
        ports: {
          http: {
            containerPort: 80
            provides: identityHttp.id
          }
        }
      }
      connections: {
        sql: {
          kind: 'microsoft.com/SQL'
          source: sqlIdentity.id
        }
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

  
