param database_id string
param todo_build object

resource app 'radius.dev/Application@v1alpha3' = {
  name: 'todo'

  resource route 'HttpRoute' = {
    name: 'todo-route'
    properties: {
      gateway: {
        hostname: '*'
      }
    }
  }

  resource web 'Service' = {
    name: 'todo-website'
    properties: {
      connections: {
        itemstore: {
          kind: 'mongo.com/MongoDB'
          source: db.id
        }
      }
      run: todo_build
      ports: {
        web: {
          dynamic: true
          provides: route.id
        }
      }
    }
  }

  resource db 'mongodb.com.MongoDBComponent' = {
    name: 'db'
    properties: {
      resource: database_id
    }
  }
}
