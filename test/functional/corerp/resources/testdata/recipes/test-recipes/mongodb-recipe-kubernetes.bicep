import kubernetes as kubernetes {
  kubeConfig: ''
  namespace: context.runtime.kubernetes.namespace
}

param context object

@description('Admin username for the Mongo database. Default is "admin"')
param username string = 'admin'

@description('Admin password for the Mongo database')
@secure()
param password string = newGuid()

resource mongo 'apps/Deployment@v1' = {
  metadata: {
    name: 'mongo-recipe-resource'
  }
  spec: {
    selector: {
      matchLabels: {
        app: 'mongo'
        resource: context.resource.name
      }
    }
    template: {
      metadata: {
        labels: {
          app: 'mongo'
          resource: context.resource.name
          'radius.dev/application': 'corerp-resources-mongodb-recipe'
          'radius.dev/resource': 'mongo-recipe-resource'
        }
      }
      spec: {
        containers: [
          {
            name: 'mongo'
            image: 'mongo:4.2'
            ports: [
              {
                containerPort: 27017
              }
            ]
            env: [
              {
                name: 'MONGO_INITDB_ROOT_USERNAME'
                value: username
              }
              {
                name: 'MONGO_INITDB_ROOT_PASSWORD'
                value: password
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
    name: 'mongo-recipe-svc'
    labels: {
      name: 'mongo-recipe-svc'
    }
  }
  spec: {
    type: 'ClusterIP'
    selector: {
      app: 'mongo'
      resource: context.resource.name
    }
    ports: [
      {
        port: 27017
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
    '/planes/kubernetes/local/namespaces/${mongo.metadata.namespace}/providers/apps/Deployment/${mongo.metadata.name}'
  ]
  values: {
    host: '${svc.metadata.name}.${svc.metadata.namespace}.svc.cluster.local'
    port: 27017
    
  }
  secrets: {
    connectionString: 'mongodb://${username}:${password}@${svc.metadata.name}.${svc.metadata.namespace}.svc.cluster.local:27017'
    username: username
    password: password
  }
}
