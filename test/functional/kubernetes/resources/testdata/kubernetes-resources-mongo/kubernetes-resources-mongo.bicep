import kubernetes from kubernetes

resource mongoService 'kubernetes.core/Service@v1' existing = {
  metadata: {
    name: 'mongo-svc'
    namespace: 'default'
  }
}

resource mongoSecret 'kubernetes.core/Secret@v1' existing = {
  metadata: {
    name: 'mongo-pw'
    namespace: 'default'
  }
}

resource app 'radius.dev/Application@v1alpha3' = {
  name: 'kubernetes-resources-mongo'

  resource webapp 'Container' = {
    name: 'todoapp'
    properties: {
      container: {
        image: 'radius.azurecr.io/magpiego:latest'
      }
      connections: {
        mongodb: {
          kind: 'mongo.com/MongoDB'
          source: mongo.id
        }
      }
    }
  }

  resource mongo 'mongo.com.MongoDatabase' = {
    name: 'mongo'
    properties: {
      secrets: {
        connectionString: 'mongodb://admin:${base64ToString(mongoSecret.data['mongo-password'])}@${mongoService.metadata.name}.${mongoService.metadata.namespace}.svc.cluster.local:${mongoService.spec.ports[0].port}'
      }
    }
  }
}
