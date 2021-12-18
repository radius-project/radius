resource app 'radius.dev/Application@v1alpha3' = {
  name: 'myapp'

  //CONTAINER
  resource frontend 'Container' = {
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
          persistentVolume:{
            kind: 'persistent'
            mountPath:'/tmpfs2'
            source: myshare.id
            rbac: 'read'
          }
        }
        readinessProbe:{
          kind:'httpGet'
          containerPort:8080
          path: '/healthz'
          initialDelaySeconds:3
          failureThreshold:4
          periodSeconds:20
        }
        livenessProbe:{
          kind:'exec'
          command:'ls /tmp'
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
  
  resource myshare 'Volume' = {
    name: 'myshare'
    properties:{
      kind: 'azure.com.fileshare'
      managed:true
    }
  }

  resource http 'HttpRoute' = {
    name: 'http'
  }

  resource db 'mongo.com.MongoDatabase' = {
    name: 'database'
    properties: {
      managed: true
    }
  }

}

