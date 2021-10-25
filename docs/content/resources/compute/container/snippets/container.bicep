resource app 'radius.dev/Application@v1alpha3' = {
  name: 'myapp'

  //CONTAINER
  resource frontend 'ContainerComponent' = {
    name: 'frontend'
    properties: {
      container: {
        image: 'registry/container:tag'
        env:{
          DEPLOYMENT_ENV: 'prod'
          DB_CONNECTION: db.connectionString()
        }
        ports: {
          http: {
            containerPort: 80
            protocol: 'TCP'
            provides: http.id
          }
        }
        volumes: {
          tempdir: {
            kind: 'ephemeral'
            mountPath: '/tmpfs'
            managedStore: 'memory'
          }
        }
      }
      connections: {
        inventory: {
          kind: 'mongo.com/MongoDB'
          source: db.id
        }
      }
    }
  }
  //CONTAINER

  resource http 'HttpRoute' = {
    name: 'http'
  }

  resource db 'mongodb.com.MongoDBComponent' = {
    name: 'database'
    properties: {
      managed: true
    }
  }

}

