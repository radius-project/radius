resource app 'radius.dev/Application@v1alpha3' = {
  name: 'myapp'

  resource frontend 'ContainerComponent' = {
    name: 'frontend'
    properties: {
      container: {
        image: 'registry/container:tag'
        env:{
          DEPLOYMENT_ENV: 'prod'
        }
        // VOLUME
        volumes: {
          myPersistentVolume:{
            kind: 'persistent'
            mountPath:'/tmpfs2'
            source: myshare.id
            rbac: 'read'
          }
        }
        // VOLUME
      }
    }
    resource myshare 'Volume' = {
      name: 'myshare'
      properties:{
        kind: 'azure.com.fileshare'
        managed:true
      }
    }
  }
}

