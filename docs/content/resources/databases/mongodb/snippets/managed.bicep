resource app 'radius.dev/Application@v1alpha3' = {
  name: 'cosmos-container'
  
  //SAMPLE
  resource db 'mongodb.com.MongoDBComponent' = {
    name: 'db'
    properties: {
      managed: true
    }
  }
  //SAMPLE

  resource webapp 'ContainerComponent' = {
    name: 'todoapp'
    properties: {
      //HIDE
        container: {
          image: 'rynowak/node-todo:latest'
          env:{
            DBCONNECTION: db.id
          }
        }
      //HIDE
      connections: {
        mongo: {
          kind: 'mongo.com/MongoDB'
          source: db.id
        }
      }
    }
  }

}
