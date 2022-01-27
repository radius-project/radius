resource server 'Microsoft.Sql/servers@2021-05-01-preview' existing = {
  name: 'server'

  resource sqldb 'databases' existing = {
    name: 'database'
  }
}

resource app 'radius.dev/Application@v1alpha3' = {
  name: 'cosmos-container'
  
  //DATABASE
  resource db 'microsoft.com.SQLDatabase' = {
    name: 'db'
    properties: {
      resource: server::sqldb.id
    }
  }
  //DATABASE

  //CONTAINER
  resource webapp 'Container' = {
    name: 'todoapp'
    properties: {
      container: {
        image: 'myregistry/myimage'
        env: {
          DBCONNECTION: 'Data Source=tcp:${db.properties.server},1433;Initial Catalog=${db.properties.database};User Id=${username}@${db.properties.server};Password=${password};Encrypt=true'
        }
      }
      connections: {
        tododb: {
          kind: 'microsoft.com/SQL'
          source: db.id
        }
      }
    }
  }
  //CONTAINER

}
