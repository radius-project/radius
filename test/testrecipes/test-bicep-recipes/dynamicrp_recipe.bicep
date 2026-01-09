extension kubernetes with {
  kubeConfig: ''
  namespace: context.runtime.kubernetes.namespace
} as kubernetes

param context object

@description('Specifies the port the container listens on.')
param port int

resource usertypealpha 'apps/Deployment@v1' = {
  metadata: {
    name: 'usertypealpha-${uniqueString(context.resource.id)}'
  }
  spec: {
    selector: {
      matchLabels: {
        app: 'usertypealpha'
        resource: context.resource.name
      }
    }
    template: {
      metadata: {
        labels: {
          app: 'usertypealpha'
          resource: context.resource.name
        }
      }
      spec: {
        containers: [
          {
            name: 'usertypealpha'
            image: 'alpine:latest'
            ports: [
              {
                containerPort: port
              }
            ]
            command: ['/bin/sh']
            args: ['-c', 'while true; do sleep 30; done']
            env: [
              {
                name: 'CONN_INJECTED'
                value: contains(context.resource, 'connections') && contains(context.resource.connections, 'externalresource') ? context.resource.connections.externalresource.properties.configMap : ''
              }
            ]
          }
        ]
      }
    }
  }
}

output result object = {
  // This workaround is needed because the deployment engine omits Kubernetes resources from its output.
  //
  // Once this gap is addressed, users won't need to do this.
  resources: [
    '/planes/kubernetes/local/namespaces/${usertypealpha.metadata.namespace}/providers/apps/Deployment/${usertypealpha.metadata.name}'
  ]
  values: {
    port: port
  }
}
