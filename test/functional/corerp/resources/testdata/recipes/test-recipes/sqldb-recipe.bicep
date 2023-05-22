param context object
import kubernetes as kubernetes {
  kubeConfig: ''
  namespace: context.runtime.kubernetes.namespace
}
@description('Specifies the SQL username.')
param username string = 'admin'

@description('Specifies the SQL password.')
@secure()
param password string = 'password'
resource sql 'apps/Deployment@v1' = {
  metadata: {
    name: 'sql-recipe-resource'
  }
  spec: {
    replicas: 1
    selector: {
      matchLabels: {
        app: context.resource.name
      }
    }
    template: {
      metadata: {
        labels: {
          app: context.resource.name
        }
      }
      spec: {
        containers: [
          {
            name: 'mysql'
            image: 'mysql'
            env: [
              {
                name: 'MSSQL_SA_PASSWORD'
                value: username
              }
              {
                name: 'MSSQL_SA_USERNAME'
                value: password
              }
              {
                name: 'MYSQL_ROOT_PASSWORD'
                value: password
              }
            ]
            resources: {
              requests: {
                cpu: '200m'
                memory: '1024Mi'
              }
              limits: {
                cpu: '450m'
                memory: '1024Mi'
              }
            }
            ports: [
              {
                containerPort: 1433
                name: 'sql'
              }
            ]
          }
        ]
      }
    }
  }
}


@description('Configure back-end service')
resource svc 'core/Service@v1' = {
  metadata: {
    name: 'sql-recipe-svc'
    labels: {
      name: 'sql-recipe-svc'
    }
  }
  spec: {
    type: 'ClusterIP'
    ports: [
      {
        port: 1433
      }
    ]
    selector: {
      app: 'sql'
      resource: context.resource.name
    }
  }
}

output result object = {
  // This workaround is needed because the deployment engine omits Kubernetes resources from its output.
  //
  // Once this gap is addressed, users won't need to do this.
  resources: [
    '/planes/kubernetes/local/namespaces/${svc.metadata.namespace}/providers/core/Service/${svc.metadata.name}'
    '/planes/kubernetes/local/namespaces/${sql.metadata.namespace}/providers/apps/Deployment/${sql.metadata.name}'
  ]
  values: {
    server: '${svc.metadata.name}.${svc.metadata.namespace}.svc.cluster.local'
    database: 'mysql'
  }
}
