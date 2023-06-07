param context object
import kubernetes as kubernetes {
  kubeConfig: ''
  namespace: context.runtime.kubernetes.namespace
}
@description('Specifies the SQL username.')
param username string

@description('Specifies the SQL password.')
@secure()
param password string
resource sql 'apps/Deployment@v1' = {
  metadata: {
    name: 'sql-${uniqueString(context.resource.id)}'
  }
  spec: {
    replicas: 1
    selector: {
      matchLabels: {
        app: 'sql-app'
        resource: context.resource.name
      }
    }
    template: {
      metadata: {
        labels: {
          app: 'sql-app'
          resource: context.resource.name
        }
      }
      spec: {
        containers: [
          {
            name: 'sql'
            image: 'mcr.microsoft.com/mssql/server:2022-latest'
            env: [
              {
                name: 'MSSQL_SA_PASSWORD'
                value: password
              }
              {
                name: 'MSSQL_SA_USERNAME'
                value: username
              }
              {
                name: 'ACCEPT_EULA'
                value: 'Y'
              }
            ]
            resources: {
              requests: {
                cpu: '600m'
                memory: '1024Mi'
              }
              limits: {
                cpu: '900m'
                memory: '1024Mi'
              }
            }
            ports: [
              {
                containerPort: 1433
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
    name: 'sql-${uniqueString(context.resource.id)}'
  }
  spec: {
    type: 'ClusterIP'
    ports: [
      {
        port: 1433
      }
    ]
    selector: {
      app: 'sql-app'
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
    database: 'master'
  }
}
