import kubernetes as kubernetes {
  kubeConfig: ''
  namespace: context.runtime.kubernetes.namespace
}

param context object

var username = 'pguser'
var password = 'p@ssword'

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
  secrets: {
    uri: 'postgresql://${username}:${password}@${service.metadata.name}.${service.metadata.namespace}.svc.cluster.local:5432/postgres'
    #disable-next-line outputs-should-not-contain-secrets
    password: password
  }
}
