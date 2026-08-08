// -----------------------------------------------------------------------------
// Sample Bicep recipe for the private-registry demo (Scenario 1).
//
// Publish this to your *private* OCI registry with `rad bicep publish`, then
// point the RecipePack's recipe `source` at the published artifact. See
// ../README.md for the publishing commands.
//
// The recipe provisions a small Redis Deployment + Service on Kubernetes and
// returns its connection values, so the consuming Applications.Core/extenders
// resource gets a working result back.
// -----------------------------------------------------------------------------

extension kubernetes with {
  kubeConfig: ''
  namespace: context.runtime.kubernetes.namespace
} as kubernetes

@description('Information about the resource calling the Recipe. Provided by Radius.')
param context object

@description('Container image to run for the Redis cache.')
param image string = 'redis:7'

resource redis 'apps/Deployment@v1' = {
  metadata: {
    name: 'redis-${uniqueString(context.resource.id)}'
  }
  spec: {
    selector: {
      matchLabels: {
        app: 'redis'
        resource: context.resource.name
      }
    }
    template: {
      metadata: {
        labels: {
          app: 'redis'
          resource: context.resource.name
        }
      }
      spec: {
        containers: [
          {
            name: 'redis'
            image: image
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
    name: 'redis-${uniqueString(context.resource.id)}'
  }
  spec: {
    type: 'ClusterIP'
    selector: {
      app: 'redis'
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
  // The deployment engine omits Kubernetes resources from its output, so we
  // surface them explicitly here.
  resources: [
    '/planes/kubernetes/local/namespaces/${svc.metadata.namespace}/providers/core/Service/${svc.metadata.name}'
    '/planes/kubernetes/local/namespaces/${redis.metadata.namespace}/providers/apps/Deployment/${redis.metadata.name}'
  ]
  values: {
    host: '${svc.metadata.name}.${svc.metadata.namespace}.svc.cluster.local'
    port: 6379
  }
}
