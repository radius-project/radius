import kubernetes as kubernetes {
  kubeConfig: ''
  namespace: context.runtime.kubernetes.namespace
}

param context object

var username = 'pguser'
var password = 'p@ssword'

var size = context.resource.properties.size

var resourcesBySize = {
  Small: {
    cpu: '0.5'
    memory: '512Mi'
  }
  Medium: {
    cpu: '1'
    memory: '1Gi'
  }
  Large: {
    cpu: '2'
    memory: '2Gi'
  }
}

var resources = resourcesBySize[size]

resource deployment 'apps/Deployment@v1' = {
  metadata: {
    name: 'postgres'
  }
  spec: {
    selector: {
      matchLabels: {
        app: 'postgres'
      }
    }
    template: {
      metadata: {
        labels: {
          app: 'postgres'
        }
      }
      spec: {
        containers: [
          {
            image: 'ghcr.io/radius-project/mirror/postgres:latest'
            name: 'postgres'
            env: [
              {
                name: 'POSTGRES_USER'
                value: password
              }
              {
                name: 'POSTGRES_PASSWORD'
                value: password
              }
            ]
            ports: [
              {
                containerPort: 5432
              }
            ]
            // Guaranteed QoS
            resources: {
              limits: {
                cpu: resources.cpu
                memory: resources.memory
              }
              requests: {
                cpu: resources.cpu
                memory: resources.memory
              }
            }
          }
        ]
      }
    }
  }
}

resource service 'core/Service@v1' = {
  metadata: {
    name: 'postgres'
  }
  spec: {
    selector: {
      app: 'postgres'
    }
    ports: [
      {
        port: 5432
      }
    ]
  }
}

output result object = {
  values: {
    host: '${service.metadata.name}.${service.metadata.namespace}.svc.cluster.local'
    port: 5432
    username: username
    database: 'postgres'
  }
  resources: [
    '/planes/kubernetes/local/namespaces/${service.metadata.namespace}/providers/core/Service/${service.metadata.name}'
    '/planes/kubernetes/local/namespaces/${deployment.metadata.namespace}/providers/apps/Deployment/${deployment.metadata.name}'
  ]
  secrets: {
    uri: 'postgresql://${username}:${password}@${service.metadata.name}.${service.metadata.namespace}.svc.cluster.local:5432/postgres'
    #disable-next-line outputs-should-not-contain-secrets
    password: password
  }
}
