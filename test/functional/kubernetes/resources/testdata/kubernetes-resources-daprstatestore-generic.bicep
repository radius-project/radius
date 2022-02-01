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
        image: 'radius.azurecr.io/magpie:latest'
        readinessProbe:{
          kind:'httpGet'
          containerPort:3000
          path: '/healthz'
        }
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


