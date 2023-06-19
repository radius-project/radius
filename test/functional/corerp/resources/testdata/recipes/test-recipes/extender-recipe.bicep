import kubernetes as kubernetes {
  kubeConfig: ''
  namespace: context.runtime.kubernetes.namespace
}

param context object
param containerImage string

resource extender 'apps/Deployment@v1' = {
  metadata: {
    name: 'extender-${uniqueString(context.resource.id)}'
  }
  spec: {
    selector: {
      matchLabels: {
        app: 'extender'
        resource: context.resource.name
      }
    }
    template: {
      metadata: {
        labels: {
          app: 'extender'
          resource: context.resource.name
        }
      }
      spec: {
        containers: [
          {
            name: 'extender'
            image: containerImage
            ports: [
              {
                containerPort: 6379
              }
            ]
          }
        ]
      }
    }
  }
}

resource svc 'core/Service@v1' = {
  metadata: {
    name: 'extender-${uniqueString(context.resource.id)}'
  }
  spec: {
    type: 'ClusterIP'
    selector: {
      app: 'extender'
      resource: context.resource.name
    }
    ports: [
      {
        port: 6379
      }
    ]
  }
}

output result object = {
  // This workaround is needed because the deployment engine omits Kubernetes resources from its output.
  //
  // Once this gap is addressed, users won't need to do this.
  resources: [
    '/planes/kubernetes/local/namespaces/${svc.metadata.namespace}/providers/core/Service/${svc.metadata.name}'
    '/planes/kubernetes/local/namespaces/${extender.metadata.namespace}/providers/apps/Deployment/${extender.metadata.name}'
  ]
  values: {
    host: '${svc.metadata.name}.${svc.metadata.namespace}.svc.cluster.local'
    port: 6379
  }
}

