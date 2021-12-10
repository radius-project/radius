resource app 'radius.dev/Application@v1alpha3' = {
  name: 'myapp'

  //SAMPLE
  resource myshare 'Volume' = {
    name: 'myshare'
    properties: {
      kind: 'azure.com.fileshare'
      managed: true
    }
  }

  resource frontend 'ContainerComponent' = {
    name: 'frontend'
    properties: {
      container: {
        image: 'registry/container:tag'
        volumes: {
          myPersistentVolume: {
            kind: 'persistent'
            mountPath: '/tmpfs2'
            source: myshare.id
            rbac: 'read'
          }
        }
      }
    }
  }
  //SAMPLE
}

