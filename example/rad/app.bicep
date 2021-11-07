param database_id string
param todo_build object

resource app 'radius.dev/Application@v1alpha3' = {
  name: 'todo'

  resource web 'Website' = {
    name: 'todo-website'
    properties: {
      connections: {
        db: {
          kind: 'mongo.com/MongoDB'
          source: db.id
        }
      }
      executable: todo_build
      env: {
        DB_CONNECTION: db.connectionString()
      }
      ports: {
        web: {
          dynamic: true
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
