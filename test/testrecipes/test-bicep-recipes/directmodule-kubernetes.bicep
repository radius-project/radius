// This is a "direct" Bicep module: it has NO `context` input parameter and NO
// structured `result` output. Radius resolves the `name`/`namespace`/`containerPort`
// parameters from {{context.*}} expressions in the recipe definition, deploys the
// module via the existing ARM deployment engine, and maps the plain outputs below onto
// the resource's properties via the recipe's `outputs` field.
extension kubernetes with {
  kubeConfig: ''
  namespace: namespace
} as kubernetes

@description('The name to use for the Kubernetes resources. Radius resolves this from a {{context.resource.name}} expression.')
param name string

@description('The namespace to deploy into. Radius resolves this from a {{context.runtime.kubernetes.namespace}} expression.')
param namespace string

@description('The container port to expose.')
param containerPort int = 6379

resource redis 'apps/Deployment@v1' = {
  metadata: {
    name: name
  }
  spec: {
    selector: {
      matchLabels: {
        app: 'redis'
        resource: name
      }
    }
    template: {
      metadata: {
        labels: {
          app: 'redis'
          resource: name
        }
      }
      spec: {
        containers: [
          {
            name: 'redis'
            image: 'ghcr.io/radius-project/mirror/redis:6.2'
            ports: [
              {
                containerPort: containerPort
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
    name: name
  }
  spec: {
    type: 'ClusterIP'
    selector: {
      app: 'redis'
      resource: name
    }
    ports: [
      {
        port: containerPort
      }
    ]
  }
}

@description('The in-cluster DNS name of the Service. Mapped onto a resource property via the recipe `outputs` field.')
output host string = '${name}.${namespace}.svc.cluster.local'

@description('The port exposed by the Service.')
output port string = string(containerPort)
