param magpieimage string = 'radiusdev.azurecr.io/magpiego:latest'

resource app 'radius.dev/Application@v1alpha3' = {
  name: 'kubernetes-resources-daprstatestore-generic'

  resource myapp 'Container' = {
    name: 'myapp'
    properties: {
      connections: {
        daprstatestore: {
          kind: 'dapr.io/StateStore'
          source: statestore.id
        }
      }
      container: {
        image:magpieimage
      }
    }
  }
  
  resource statestore 'dapr.io.StateStore@v1alpha3' = {
    name: 'statestore'
    properties: {
      kind: 'generic'
      type: 'state.zookeeper'
      metadata: {
        servers: 'zookeeper.default.svc.cluster.local:2181'
      }
      version: 'v1'
    }
  }
}
