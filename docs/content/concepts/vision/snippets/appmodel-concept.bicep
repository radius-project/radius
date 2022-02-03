// Define app 
resource app 'radius.dev/Application@v1alpha3' = {
  name: 'webapp'

  // Define container resource to run app code
  resource todoapplication 'Container' = {
    name: 'todoapp'
    properties: {
      container: {
        image: 'radius.azurecr.io/webapptutorial-todoapp'
        ports: {
          web: {
            containerPort: 3000
          }
        }
      }
      // Connect container to database 
      connections: {
        itemstore: {
          kind: 'mongo.com/MongoDB'
          source: db.id
        }
      }
    }
  }
 
  // Define database
  resource db 'mongo.com.MongoDatabase' = {
    name: 'db'
    properties: {
      managed: true
    }
  }
}

