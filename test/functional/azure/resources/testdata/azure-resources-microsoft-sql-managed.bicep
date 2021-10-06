// NOTE: This file is here for manual testing purposes.
// we intentionally omit automated tests for some of the Azure resource
// types because it would massively bloat our runs.

resource app 'radius.dev/Application@v1alpha3' = {
  name: 'azure-resources-microsoft-sql-managed'

  resource webapp 'ContainerComponent' = {
    name: 'todoapp'
    properties: {
      connections: {
        sql: {
          kind: 'microsoft.com/SQL'
          source: db.id
        }
      }
      container: {
        image: 'radius.azurecr.io/magpie:latest'
      }
    }
  }

  resource db 'microsoft.com.SQLComponent' = {
    name: 'db'
    properties: {
      managed: true
    }
  }
}
