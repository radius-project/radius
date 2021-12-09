resource app 'radius.dev/Application@v1alpha3' = {
  name: 'myapp'

  //CONTAINER
  resource frontend 'ContainerComponent' = {
    name: 'frontend'
    properties: {
      container: {
        image: 'registry/container:tag'
        volumes: {
          tempdir: {
            kind: 'ephemeral'
            mountPath: '/tmpfs'
            managedStore: 'memory'
          }
        }
      }
    }

  }
  //CONTAINER

}

