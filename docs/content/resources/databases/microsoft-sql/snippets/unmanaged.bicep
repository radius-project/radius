//PARAMETERS
param sqldb string
@secure()
param username string
@secure()
param password string
//PARAMETERS

resource app 'radius.dev/Application@v1alpha3' = {
  name: 'cosmos-container'
  
  //DATABASE
  resource db 'microsoft.com.SQLComponent' = {
    name: 'db'
    properties: {
      resource: sqldb
    }
  }
  //DATABASE

  //CONTAINER
  resource webapp 'ContainerComponent' = {
    name: 'todoapp'
    properties: {
      connections: {
        tododb: {
          kind: 'microsoft.com/SQL'
          source: db.id
        }
      }
      container: {
        image: 'rynowak/node-todo:latest'
        env: {
          DBCONNECTION: 'Data Source=tcp:${db.properties.server},1433;Initial Catalog=${db.properties.database};User Id=${username}@${db.properties.server};Password=${password};Encrypt=true'
        }
      }
    }
  }
  //CONTAINER

}
